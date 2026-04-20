package llm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func newHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &http.Client{Timeout: timeout}
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
