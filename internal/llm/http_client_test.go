package llm

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestNewHTTPClientUsesProviderProxyOverrideOverGlobalProxy(t *testing.T) {
	previousConfigured := envProxyConfigured
	previousFunc := envProxyFunc
	envProxyConfigured = func() bool { return false }
	envProxyFunc = func(req *http.Request) (*url.URL, error) { return nil, nil }
	t.Cleanup(func() {
		envProxyConfigured = previousConfigured
		envProxyFunc = previousFunc
	})

	client, err := newHTTPClient(time.Second, ProxySettings{
		Enabled: true,
		URL:     "http://global-proxy.example:8080",
	}, ProxySettings{
		Enabled: true,
		URL:     "socks5://provider-proxy.example:1080",
	})
	if err != nil {
		t.Fatalf("newHTTPClient: %v", err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %#v, want *http.Transport", client.Transport)
	}
	reqURL, _ := url.Parse("https://api.example.test/v1/messages")
	proxyURL, err := transport.Proxy(&http.Request{URL: reqURL})
	if err != nil {
		t.Fatalf("resolve proxy: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "socks5://provider-proxy.example:1080" {
		t.Fatalf("proxy url = %#v, want provider socks5 proxy", proxyURL)
	}
}

func TestNewHTTPClientUsesEnvironmentProxyOverConfiguredProxy(t *testing.T) {
	previousConfigured := envProxyConfigured
	previousFunc := envProxyFunc
	envProxyConfigured = func() bool { return true }
	envProxyFunc = func(req *http.Request) (*url.URL, error) {
		return url.Parse("http://env-proxy.example:8443")
	}
	t.Cleanup(func() {
		envProxyConfigured = previousConfigured
		envProxyFunc = previousFunc
	})

	client, err := newHTTPClient(time.Second, ProxySettings{
		Enabled: true,
		URL:     "http://global-proxy.example:8080",
	}, ProxySettings{})
	if err != nil {
		t.Fatalf("newHTTPClient: %v", err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %#v, want *http.Transport", client.Transport)
	}
	reqURL, _ := url.Parse("https://api.example.test/v1/messages")
	proxyURL, err := transport.Proxy(&http.Request{URL: reqURL})
	if err != nil {
		t.Fatalf("resolve proxy: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://env-proxy.example:8443" {
		t.Fatalf("proxy url = %#v, want env proxy", proxyURL)
	}
}

func TestNewHTTPClientBypassesConfiguredProxyForNoProxyHosts(t *testing.T) {
	previousConfigured := envProxyConfigured
	previousFunc := envProxyFunc
	envProxyConfigured = func() bool { return false }
	envProxyFunc = func(req *http.Request) (*url.URL, error) { return nil, nil }
	t.Cleanup(func() {
		envProxyConfigured = previousConfigured
		envProxyFunc = previousFunc
	})

	client, err := newHTTPClient(time.Second, ProxySettings{
		Enabled: true,
		URL:     "http://global-proxy.example:8080",
		NoProxy: []string{"api.example.test", ".internal.example"},
	}, ProxySettings{})
	if err != nil {
		t.Fatalf("newHTTPClient: %v", err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %#v, want *http.Transport", client.Transport)
	}
	reqURL, _ := url.Parse("https://api.example.test/v1/messages")
	proxyURL, err := transport.Proxy(&http.Request{URL: reqURL})
	if err != nil {
		t.Fatalf("resolve proxy: %v", err)
	}
	if proxyURL != nil {
		t.Fatalf("proxy url = %#v, want direct connection for no_proxy host", proxyURL)
	}
}
