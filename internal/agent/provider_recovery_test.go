package agent

import (
	"errors"
	"net"
	"net/http"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestClassifyProviderErrorKinds(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		kind    string
		retry   bool
		compact bool
		action  string
	}{
		{name: "context", err: errors.New("prompt is too long for maximum context"), kind: ProviderErrorContextLengthExceeded, compact: true, action: "compact_context"},
		{name: "rate", err: &fantasy.ProviderError{StatusCode: http.StatusTooManyRequests, Message: "rate limit"}, kind: ProviderErrorRateLimited, retry: true},
		{name: "overloaded", err: &fantasy.ProviderError{StatusCode: http.StatusServiceUnavailable, Message: "overloaded"}, kind: ProviderErrorOverloaded, retry: true},
		{name: "network", err: timeoutErr{}, kind: ProviderErrorNetworkTransient, retry: true},
		{name: "auth", err: &fantasy.ProviderError{StatusCode: http.StatusUnauthorized, Message: "bad key"}, kind: ProviderErrorAuthExpired, action: "refresh_auth"},
		{name: "model", err: &fantasy.ProviderError{StatusCode: http.StatusNotFound, Message: "model not found"}, kind: ProviderErrorModelNotFound, action: "select_model"},
		{name: "capability", err: errors.New("model does not support image input"), kind: ProviderErrorModelCapabilityUnsupported, action: "adjust_input_or_model"},
		{name: "permission", err: errors.New("permission required for tool"), kind: ProviderErrorPermissionRequired, action: "grant_permission"},
		{name: "policy", err: errors.New("policy denied execution"), kind: ProviderErrorPolicyDenied, action: "change_policy"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ClassifyProviderError(test.err)
			require.Equal(t, test.kind, got.Kind)
			require.Equal(t, test.retry, got.Retryable)
			require.Equal(t, test.compact, got.CompactEligible)
			if test.action != "" {
				require.Equal(t, test.action, got.UserAction)
			}
		})
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "dial timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

var _ net.Error = timeoutErr{}
