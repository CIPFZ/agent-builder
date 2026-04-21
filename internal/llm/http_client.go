package llm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func newHTTPClient(timeout time.Duration, globalProxy, providerProxy ProxySettings) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	proxyFunc, err := resolveProxyFunc(globalProxy, providerProxy)
	if err != nil {
		return nil, err
	}
	transport.Proxy = proxyFunc
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}

func mustNewHTTPClient(timeout time.Duration, globalProxy, providerProxy ProxySettings) *http.Client {
	client, err := newHTTPClient(timeout, globalProxy, providerProxy)
	if err != nil {
		panic(err)
	}
	return client
}

func resolveProxyFunc(globalProxy, providerProxy ProxySettings) (func(*http.Request) (*url.URL, error), error) {
	if envProxyConfigured() {
		return envProxyFunc, nil
	}
	selected, ok := selectConfiguredProxy(globalProxy, providerProxy)
	if !ok {
		return nil, nil
	}
	proxyURL, err := url.Parse(selected.URL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy url %q: %w", selected.URL, err)
	}
	if proxyURL.Scheme == "" {
		return nil, fmt.Errorf("parse proxy url %q: missing scheme", selected.URL)
	}
	switch strings.ToLower(proxyURL.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", proxyURL.Scheme)
	}
	return func(req *http.Request) (*url.URL, error) {
		if req == nil || req.URL == nil {
			return proxyURL, nil
		}
		if shouldBypassProxy(req.URL, selected.NoProxy) {
			return nil, nil
		}
		return proxyURL, nil
	}, nil
}

var (
	envProxyFunc       = http.ProxyFromEnvironment
	envProxyConfigured = hasEnvironmentProxy
)

func selectConfiguredProxy(globalProxy, providerProxy ProxySettings) (ProxySettings, bool) {
	if proxyConfigured(providerProxy) {
		if !providerProxy.Enabled || strings.TrimSpace(providerProxy.URL) == "" {
			return ProxySettings{}, false
		}
		return providerProxy, true
	}
	if proxyConfigured(globalProxy) {
		if !globalProxy.Enabled || strings.TrimSpace(globalProxy.URL) == "" {
			return ProxySettings{}, false
		}
		return globalProxy, true
	}
	return ProxySettings{}, false
}

func proxyConfigured(cfg ProxySettings) bool {
	return cfg.Explicit || strings.TrimSpace(cfg.URL) != "" || len(cfg.NoProxy) > 0
}

func hasEnvironmentProxy() bool {
	for _, rawURL := range []string{
		"https://proxy-check.invalid",
		"http://proxy-check.invalid",
	} {
		target, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		proxyURL, err := http.ProxyFromEnvironment(&http.Request{URL: target})
		if err == nil && proxyURL != nil {
			return true
		}
	}
	return false
}

func shouldBypassProxy(target *url.URL, noProxy []string) bool {
	host := strings.TrimSpace(target.Hostname())
	if host == "" {
		return false
	}
	for _, pattern := range noProxy {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.EqualFold(host, pattern) {
			return true
		}
		if strings.HasPrefix(pattern, ".") && strings.HasSuffix(strings.ToLower(host), strings.ToLower(pattern)) {
			return true
		}
		if ip := net.ParseIP(host); ip != nil && strings.EqualFold(host, pattern) {
			return true
		}
	}
	return false
}

func doStreamingRequest(ctx context.Context, client *http.Client, method, url string, body []byte, headers map[string]string, maxRetries int) (*http.Response, error) {
	attempts := maxRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		for key, value := range headers {
			if strings.TrimSpace(key) == "" || value == "" {
				continue
			}
			req.Header.Set(key, value)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt == attempts-1 || waitForRetry(ctx, attempt) != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode < 400 {
			return resp, nil
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastErr = fmt.Errorf("llm request failed: %s: %s", resp.Status, strings.TrimSpace(string(data)))
		if !retryableStatus(resp.StatusCode) || attempt == attempts-1 || waitForRetry(ctx, attempt) != nil {
			return nil, lastErr
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("llm request failed")
}

func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

func waitForRetry(ctx context.Context, attempt int) error {
	backoff := time.Duration(attempt+1) * 100 * time.Millisecond
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func copyHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return make(map[string]string)
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
	}
	return out
}
