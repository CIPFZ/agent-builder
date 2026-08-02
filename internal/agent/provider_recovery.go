package agent

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"charm.land/fantasy"
)

const (
	ProviderErrorContextLengthExceeded      = "context_length_exceeded"
	ProviderErrorRateLimited                = "rate_limited"
	ProviderErrorOverloaded                 = "overloaded"
	ProviderErrorNetworkTransient           = "network_transient"
	ProviderErrorAuthExpired                = "auth_expired"
	ProviderErrorModelNotFound              = "model_not_found"
	ProviderErrorModelCapabilityUnsupported = "model_capability_unsupported"
	ProviderErrorPermissionRequired         = "permission_required"
	ProviderErrorPolicyDenied               = "policy_denied"
	ProviderErrorFallbackAvailable          = "provider_fallback_available"
	ProviderErrorUnknown                    = "unknown"
)

type ProviderErrorClassification struct {
	Kind             string
	Retryable        bool
	CompactEligible  bool
	RefreshAuth      bool
	FallbackEligible bool
	UserAction       string
	RetryAfter       time.Duration
	Details          map[string]any
}

func ClassifyProviderError(err error) ProviderErrorClassification {
	classification := ProviderErrorClassification{Kind: ProviderErrorUnknown, Details: map[string]any{}}
	if err == nil {
		return classification
	}
	message := strings.ToLower(err.Error())
	var providerErr *fantasy.ProviderError
	if errors.As(err, &providerErr) {
		classification.Details["status_code"] = providerErr.StatusCode
		classification.Details["title"] = providerErr.Title
		classification.Details["message"] = providerErr.Message
		message = strings.ToLower(providerErr.Title + " " + providerErr.Message + " " + err.Error())
		structuredCode := strings.ToLower(strings.TrimSpace(providerErr.Title))
		switch structuredCode {
		case "context_length_exceeded", "context_window_exceeded", "prompt_too_long", "input_too_long":
			classification.Kind = ProviderErrorContextLengthExceeded
			classification.CompactEligible = true
			classification.UserAction = "compact_context"
			return classification
		}
		switch providerErr.StatusCode {
		case http.StatusRequestEntityTooLarge:
			classification.Kind = ProviderErrorContextLengthExceeded
			classification.CompactEligible = true
			classification.UserAction = "compact_context"
			return classification
		case http.StatusUnauthorized, http.StatusForbidden:
			classification.Kind = ProviderErrorAuthExpired
			classification.RefreshAuth = true
			classification.UserAction = "refresh_auth"
			return classification
		case http.StatusTooManyRequests:
			classification.Kind = ProviderErrorRateLimited
			classification.Retryable = true
			classification.RetryAfter = 2 * time.Second
			return classification
		case http.StatusNotFound:
			classification.Kind = ProviderErrorModelNotFound
			classification.UserAction = "select_model"
			return classification
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			classification.Kind = ProviderErrorOverloaded
			classification.Retryable = true
			return classification
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		classification.Kind = ProviderErrorNetworkTransient
		classification.Retryable = true
		return classification
	}
	switch {
	case strings.Contains(message, "context length") || strings.Contains(message, "context_length") || strings.Contains(message, "maximum context") || strings.Contains(message, "prompt is too long") || strings.Contains(message, "prompt too long") || strings.Contains(message, "prompt_too_long") || strings.Contains(message, "too many tokens"):
		classification.Kind = ProviderErrorContextLengthExceeded
		classification.CompactEligible = true
		classification.UserAction = "compact_context"
	case strings.Contains(message, "rate limit") || strings.Contains(message, "too many requests"):
		classification.Kind = ProviderErrorRateLimited
		classification.Retryable = true
		classification.RetryAfter = 2 * time.Second
	case strings.Contains(message, "overloaded") || strings.Contains(message, "temporarily unavailable"):
		classification.Kind = ProviderErrorOverloaded
		classification.Retryable = true
	case strings.Contains(message, "connection reset") || strings.Contains(message, "connection refused") || strings.Contains(message, "timeout"):
		classification.Kind = ProviderErrorNetworkTransient
		classification.Retryable = true
	case strings.Contains(message, "api key") || strings.Contains(message, "unauthorized") || strings.Contains(message, "authentication"):
		classification.Kind = ProviderErrorAuthExpired
		classification.RefreshAuth = true
		classification.UserAction = "refresh_auth"
	case strings.Contains(message, "model not found") || strings.Contains(message, "unknown model"):
		classification.Kind = ProviderErrorModelNotFound
		classification.UserAction = "select_model"
	case strings.Contains(message, "does not support") || strings.Contains(message, "unsupported") || strings.Contains(message, "image"):
		classification.Kind = ProviderErrorModelCapabilityUnsupported
		classification.UserAction = "adjust_input_or_model"
	case strings.Contains(message, "permission required"):
		classification.Kind = ProviderErrorPermissionRequired
		classification.UserAction = "grant_permission"
	case strings.Contains(message, "policy denied"):
		classification.Kind = ProviderErrorPolicyDenied
		classification.UserAction = "change_policy"
	}
	return classification
}
