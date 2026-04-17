package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func TestMCPOAuthProviderOmitsRefreshTokenWhenStepUpScopeIsMissing(t *testing.T) {
	store := tools.NewMCPOAuthMemoryStore()
	connection := tools.MCPConnection{Name: "filesystem", BaseURL: "https://mcp.example"}
	provider := tools.NewMCPOAuthProvider(store, "filesystem", connection)

	if err := provider.SaveTokens(tools.MCPOAuthTokens{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		Scope:        "files:read",
		TokenType:    "Bearer",
	}); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	provider.MarkStepUpPending("files:read files:write")

	tokens, ok := provider.Tokens(time.Now())
	if !ok {
		t.Fatal("tokens returned false, want current access token without refresh token")
	}
	if tokens.AccessToken != "access-token" {
		t.Fatalf("access token = %q, want current token", tokens.AccessToken)
	}
	if tokens.RefreshToken != "" {
		t.Fatalf("refresh token = %q, want omitted during step-up", tokens.RefreshToken)
	}
	if tokens.Scope != "files:read" {
		t.Fatalf("scope = %q, want original token scope preserved", tokens.Scope)
	}
}

func TestMCPOAuthProviderKeepsRefreshTokenWhenStepUpScopeAlreadyCovered(t *testing.T) {
	store := tools.NewMCPOAuthMemoryStore()
	provider := tools.NewMCPOAuthProvider(store, "filesystem", tools.MCPConnection{BaseURL: "https://mcp.example"})

	if err := provider.SaveTokens(tools.MCPOAuthTokens{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		Scope:        "files:read files:write",
		TokenType:    "Bearer",
	}); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	provider.MarkStepUpPending("files:write")

	tokens, ok := provider.Tokens(time.Now())
	if !ok {
		t.Fatal("tokens returned false, want current tokens")
	}
	if tokens.RefreshToken != "refresh-token" {
		t.Fatalf("refresh token = %q, want retained when current token covers step-up scope", tokens.RefreshToken)
	}
}

func TestMCPOAuthProviderDoesNotOmitRefreshTokenForCachedStepUpScopeAlone(t *testing.T) {
	store := tools.NewMCPOAuthMemoryStore()
	provider := tools.NewMCPOAuthProvider(store, "filesystem", tools.MCPConnection{BaseURL: "https://mcp.example"})
	if err := provider.SaveTokens(tools.MCPOAuthTokens{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		Scope:        "files:read",
		TokenType:    "Bearer",
	}); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	if err := provider.SaveStepUpScope("files:admin"); err != nil {
		t.Fatalf("save step-up scope: %v", err)
	}

	tokens, ok := provider.Tokens(time.Now())
	if !ok {
		t.Fatal("tokens returned false, want current tokens")
	}
	if tokens.RefreshToken != "refresh-token" {
		t.Fatalf("refresh token = %q, want cached step-up scope to seed auth but not suppress refresh", tokens.RefreshToken)
	}
}

func TestMCPOAuthProviderPersistsStepUpScopeFromAuthorizationURLWithoutRedirection(t *testing.T) {
	store := tools.NewMCPOAuthMemoryStore()
	provider := tools.NewMCPOAuthProvider(store, "filesystem", tools.MCPConnection{BaseURL: "https://mcp.example"})
	if err := provider.SaveTokens(tools.MCPOAuthTokens{
		AccessToken: "access-token",
		ExpiresAt:   time.Now().Add(time.Hour),
		Scope:       "files:read",
	}); err != nil {
		t.Fatalf("save tokens: %v", err)
	}
	authURL, err := url.Parse("https://auth.example/authorize?scope=files%3Aread+files%3Awrite")
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}

	provider.RedirectToAuthorization(*authURL, false)

	entry, ok := store.Entry("filesystem", tools.MCPConnection{BaseURL: "https://mcp.example"})
	if !ok {
		t.Fatal("store entry missing")
	}
	if entry.StepUpScope != "files:read files:write" {
		t.Fatalf("step-up scope = %q, want persisted authorization URL scope", entry.StepUpScope)
	}
}

func TestMCPOAuthProviderAuthStartContextUsesCachedStepUpAndResourceMetadata(t *testing.T) {
	store := tools.NewMCPOAuthMemoryStore()
	connection := tools.MCPConnection{
		Name:                    "filesystem",
		BaseURL:                 "https://mcp.example",
		AuthScope:               "files:admin",
		AuthResourceMetadataURL: "https://auth.example/resource",
	}
	provider := tools.NewMCPOAuthProvider(store, "filesystem", connection)
	if err := provider.SaveDiscoveryState(tools.MCPOAuthDiscoveryState{
		AuthorizationServerURL: "https://auth.example",
		ResourceMetadataURL:    "https://cached.example/resource",
	}); err != nil {
		t.Fatalf("save discovery: %v", err)
	}
	if err := provider.SaveStepUpScope("files:cached"); err != nil {
		t.Fatalf("save step-up scope: %v", err)
	}

	start := provider.AuthStartContext()

	if start.Scope != "files:admin" {
		t.Fatalf("scope = %q, want live challenge scope to win over cached scope", start.Scope)
	}
	if start.ResourceMetadataURL != "https://auth.example/resource" {
		t.Fatalf("resource metadata = %q, want live challenge resource metadata to win", start.ResourceMetadataURL)
	}
	cachedProvider := tools.NewMCPOAuthProvider(store, "filesystem", tools.MCPConnection{BaseURL: "https://mcp.example"})
	cached := cachedProvider.AuthStartContext()
	if cached.Scope != "files:cached" || cached.ResourceMetadataURL != "https://cached.example/resource" {
		t.Fatalf("cached context = %#v, want cached step-up and resource metadata", cached)
	}
}

func TestMCPOAuthProviderSaveTokensClearsStepUpScope(t *testing.T) {
	store := tools.NewMCPOAuthMemoryStore()
	provider := tools.NewMCPOAuthProvider(store, "filesystem", tools.MCPConnection{BaseURL: "https://mcp.example"})
	if err := provider.SaveStepUpScope("files:admin"); err != nil {
		t.Fatalf("save step-up scope: %v", err)
	}

	if err := provider.SaveTokens(tools.MCPOAuthTokens{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		Scope:        "files:admin",
	}); err != nil {
		t.Fatalf("save tokens: %v", err)
	}

	entry, ok := store.Entry("filesystem", tools.MCPConnection{BaseURL: "https://mcp.example"})
	if !ok {
		t.Fatal("store entry missing")
	}
	if entry.StepUpScope != "" {
		t.Fatalf("step-up scope = %q, want cleared after saving new tokens", entry.StepUpScope)
	}
}

func TestMCPOAuthFileStorePersistsEntriesAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-oauth.json")
	connection := tools.MCPConnection{Name: "filesystem", BaseURL: "https://mcp.example"}
	store := tools.NewMCPOAuthFileStore(path)
	if err := store.SaveEntry("filesystem", connection, tools.MCPOAuthEntry{
		ServerName:   "filesystem",
		ServerURL:    "https://mcp.example",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Tokens: tools.MCPOAuthTokens{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			ExpiresAt:    time.Unix(12345, 0).UTC(),
			Scope:        "files:read",
			TokenType:    "Bearer",
		},
		DiscoveryState: tools.MCPOAuthDiscoveryState{
			AuthorizationServerURL: "https://auth.example",
			ResourceMetadataURL:    "https://auth.example/resource",
		},
		StepUpScope: "files:write",
	}); err != nil {
		t.Fatalf("save file entry: %v", err)
	}

	reopened := tools.NewMCPOAuthFileStore(path)
	entry, ok := reopened.Entry("filesystem", connection)
	if !ok {
		t.Fatal("reopened store missing OAuth entry")
	}
	if entry.ClientID != "client-id" ||
		entry.ClientSecret != "client-secret" ||
		entry.Tokens.AccessToken != "access-token" ||
		entry.Tokens.RefreshToken != "refresh-token" ||
		entry.Tokens.ExpiresAt.Unix() != 12345 ||
		entry.DiscoveryState.AuthorizationServerURL != "https://auth.example" ||
		entry.StepUpScope != "files:write" {
		t.Fatalf("reopened entry = %#v, want persisted OAuth entry", entry)
	}
}

func TestDefaultMCPOAuthAuthenticatorRunsPKCECallbackAndSavesTokens(t *testing.T) {
	authServer := newTestMCPOAuthServer(t)
	store := tools.NewMCPOAuthMemoryStore()
	authenticator := tools.NewDefaultMCPOAuthAuthenticator(store)

	started, err := authenticator(context.Background(), "filesystem", tools.MCPConnection{
		Name:                    "filesystem",
		Type:                    "http",
		BaseURL:                 "https://mcp.example",
		AuthScope:               "files:read files:write",
		AuthResourceMetadataURL: authServer.URL + "/resource",
	})
	if err != nil {
		t.Fatalf("start oauth: %v", err)
	}
	if started.Status != "auth_url" || started.AuthURL == "" {
		t.Fatalf("started = %#v, want auth_url with URL", started)
	}

	if _, err := http.Get(started.AuthURL); err != nil {
		t.Fatalf("follow authorization URL: %v", err)
	}
	select {
	case completed := <-started.Completion:
		if completed.Error != nil || completed.Status != "complete" {
			t.Fatalf("completion = %#v, want complete without error", completed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OAuth callback completion")
	}

	entry, ok := store.Entry("filesystem", tools.MCPConnection{BaseURL: "https://mcp.example"})
	if !ok {
		t.Fatal("OAuth store entry missing")
	}
	if entry.ClientID != "registered-client" {
		t.Fatalf("client id = %q, want DCR client id", entry.ClientID)
	}
	if entry.Tokens.AccessToken != "access-token" ||
		entry.Tokens.RefreshToken != "refresh-token" ||
		entry.Tokens.Scope != "files:read files:write" ||
		entry.Tokens.TokenType != "Bearer" {
		t.Fatalf("tokens = %#v, want exchanged bearer tokens", entry.Tokens)
	}
	if entry.DiscoveryState.AuthorizationServerURL != authServer.URL {
		t.Fatalf("authorization server = %q, want %q", entry.DiscoveryState.AuthorizationServerURL, authServer.URL)
	}
	if authServer.sawCodeChallenge == "" || authServer.sawCodeVerifier == "" {
		t.Fatalf("server did not observe PKCE challenge/verifier: challenge=%q verifier=%q", authServer.sawCodeChallenge, authServer.sawCodeVerifier)
	}
}

func TestMCPAuthToolUsesDefaultOAuthAuthenticatorAndReconnectsAfterCallback(t *testing.T) {
	authServer := newTestMCPOAuthServer(t)
	store := tools.NewMCPOAuthMemoryStore()
	tool := tools.NewMCPAuthTool("filesystem", "", "Authenticate filesystem")
	reconnected := make(chan struct{})

	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session: session.Session{ID: "sess-1"},
		MCPClients: []tools.MCPConnection{{
			Name:                    "filesystem",
			Type:                    "http",
			BaseURL:                 "https://mcp.example",
			AuthScope:               "files:read",
			AuthResourceMetadataURL: authServer.URL + "/resource",
		}},
		MCPOAuthStore: store,
		MCPReconnect: func(_ context.Context, server string) (tools.MCPReconnectResult, error) {
			if server != "filesystem" {
				t.Fatalf("reconnect server = %q, want filesystem", server)
			}
			close(reconnected)
			return tools.MCPReconnectResult{
				Client: tools.MCPConnection{Name: "filesystem", Type: "http", BaseURL: "https://mcp.example"},
				Tools: tools.MCPToolsListResult{Tools: []tools.MCPToolListItem{{
					Name:        "read_file",
					Description: "Read file",
					InputSchema: map[string]any{"type": "object"},
				}}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("invoke auth tool: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Output), &payload); err != nil {
		t.Fatalf("decode auth payload: %v", err)
	}
	if payload["status"] != "auth_url" || payload["authUrl"] == "" {
		t.Fatalf("payload = %#v, want default authenticator auth_url", payload)
	}
	if _, err := http.Get(payload["authUrl"].(string)); err != nil {
		t.Fatalf("follow authorization URL: %v", err)
	}
	select {
	case <-reconnected:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reconnect after OAuth callback")
	}
	entry, ok := store.Entry("filesystem", tools.MCPConnection{BaseURL: "https://mcp.example"})
	if !ok || entry.Tokens.AccessToken != "access-token" {
		t.Fatalf("store entry = %#v ok=%v, want saved access token", entry, ok)
	}
}

func TestDiscoverMCPClientToolsWithOAuthSendsStoredBearerToken(t *testing.T) {
	store := tools.NewMCPOAuthMemoryStore()
	connection := tools.MCPConnection{Name: "filesystem", Type: "http"}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer stored-access" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"missing bearer"}`))
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode rpc: %v", err)
		}
		method, _ := request["method"].(string)
		id := request["id"]
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"capabilities": map[string]any{"tools": map[string]any{}},
				},
			})
		case "notifications/initialized":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": map[string]any{}})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{{
						"name":        "read_file",
						"description": "Read file",
						"inputSchema": map[string]any{"type": "object"},
					}},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer server.Close()
	connection.BaseURL = server.URL
	if err := tools.NewMCPOAuthProvider(store, "filesystem", connection).SaveTokens(tools.MCPOAuthTokens{
		AccessToken: "stored-access",
		ExpiresAt:   time.Now().Add(time.Hour),
		TokenType:   "Bearer",
	}); err != nil {
		t.Fatalf("save tokens: %v", err)
	}

	discovered, err := tools.DiscoverMCPClientToolsWithOAuth(context.Background(), []tools.MCPConnection{connection}, store)
	if err != nil {
		t.Fatalf("discover MCP tools with OAuth: %v", err)
	}
	if len(discovered.Tools["filesystem"].Tools) != 1 {
		t.Fatalf("tools = %#v, want discovered tool using bearer token", discovered.Tools)
	}
}

func TestDiscoverMCPClientToolsWithOAuthRefreshesExpiredBearerToken(t *testing.T) {
	store := tools.NewMCPOAuthMemoryStore()
	var authServer *httptest.Server
	authServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"token_endpoint": authServer.URL + "/token"})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse refresh form: %v", err)
			}
			if r.Form.Get("grant_type") != "refresh_token" ||
				r.Form.Get("refresh_token") != "old-refresh" ||
				r.Form.Get("client_id") != "registered-client" {
				t.Fatalf("refresh form = %#v, want refresh_token grant", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "new-access",
				"refresh_token": "new-refresh",
				"expires_in":    3600,
				"scope":         "files:read",
				"token_type":    "Bearer",
			})
		default:
			t.Fatalf("unexpected auth path %q", r.URL.Path)
		}
	}))
	defer authServer.Close()
	var mcpServer *httptest.Server
	mcpServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer new-access" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"old token"}`))
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode rpc: %v", err)
		}
		id := request["id"]
		switch request["method"] {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  map[string]any{"capabilities": map[string]any{"tools": map[string]any{}}},
			})
		case "notifications/initialized":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": map[string]any{}})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{"tools": []map[string]any{{
					"name": "read_file", "description": "Read file", "inputSchema": map[string]any{"type": "object"},
				}}},
			})
		default:
			t.Fatalf("unexpected method %q", request["method"])
		}
	}))
	defer mcpServer.Close()
	connection := tools.MCPConnection{Name: "filesystem", Type: "http", BaseURL: mcpServer.URL}
	entry := tools.MCPOAuthEntry{
		ServerName:   "filesystem",
		ServerURL:    mcpServer.URL,
		ClientID:     "registered-client",
		ClientSecret: "registered-secret",
		Tokens: tools.MCPOAuthTokens{
			AccessToken:  "old-access",
			RefreshToken: "old-refresh",
			ExpiresAt:    time.Now().Add(-time.Minute),
			Scope:        "files:read",
			TokenType:    "Bearer",
		},
		DiscoveryState: tools.MCPOAuthDiscoveryState{AuthorizationServerURL: authServer.URL},
	}
	if err := store.SaveEntry("filesystem", connection, entry); err != nil {
		t.Fatalf("save oauth entry: %v", err)
	}

	discovered, err := tools.DiscoverMCPClientToolsWithOAuth(context.Background(), []tools.MCPConnection{connection}, store)
	if err != nil {
		t.Fatalf("discover MCP tools with refresh: %v", err)
	}
	if len(discovered.Tools["filesystem"].Tools) != 1 {
		t.Fatalf("tools = %#v, want discovered tool after refresh", discovered.Tools)
	}
	refreshed, _ := store.Entry("filesystem", connection)
	if refreshed.Tokens.AccessToken != "new-access" || refreshed.Tokens.RefreshToken != "new-refresh" {
		t.Fatalf("refreshed tokens = %#v, want saved refreshed tokens", refreshed.Tokens)
	}
}

type testMCPOAuthServer struct {
	*httptest.Server
	sawCodeChallenge string
	sawCodeVerifier  string
}

func newTestMCPOAuthServer(t *testing.T) *testMCPOAuthServer {
	t.Helper()
	state := &testMCPOAuthServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/resource", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authorization_servers": []string{state.URL},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authorization_endpoint": state.URL + "/authorize",
			"token_endpoint":         state.URL + "/token",
			"registration_endpoint":  state.URL + "/register",
			"scopes_supported":       []string{"files:read", "files:write"},
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("registration method = %s, want POST", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode registration: %v", err)
		}
		redirects, _ := body["redirect_uris"].([]any)
		if len(redirects) == 0 {
			t.Fatalf("registration body = %#v, want redirect_uris", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"client_id":     "registered-client",
			"client_secret": "registered-secret",
		})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("client_id") != "registered-client" {
			t.Fatalf("client_id = %q, want registered-client", query.Get("client_id"))
		}
		if query.Get("code_challenge_method") != "S256" {
			t.Fatalf("code_challenge_method = %q, want S256", query.Get("code_challenge_method"))
		}
		state.sawCodeChallenge = query.Get("code_challenge")
		redirectURI := query.Get("redirect_uri")
		if redirectURI == "" || query.Get("state") == "" {
			t.Fatalf("authorize query = %s, want redirect_uri and state", r.URL.RawQuery)
		}
		callback, err := url.Parse(redirectURI)
		if err != nil {
			t.Fatalf("parse redirect_uri: %v", err)
		}
		values := callback.Query()
		values.Set("code", "auth-code")
		values.Set("state", query.Get("state"))
		callback.RawQuery = values.Encode()
		http.Redirect(w, r, callback.String(), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("token method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if r.Form.Get("grant_type") != "authorization_code" ||
			r.Form.Get("code") != "auth-code" ||
			r.Form.Get("client_id") != "registered-client" {
			t.Fatalf("token form = %#v, want authorization_code exchange", r.Form)
		}
		state.sawCodeVerifier = r.Form.Get("code_verifier")
		if state.sawCodeVerifier == "" {
			t.Fatal("token exchange missing code_verifier")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_in":    3600,
			"scope":         "files:read files:write",
			"token_type":    "Bearer",
		})
	})
	state.Server = httptest.NewServer(mux)
	t.Cleanup(state.Close)
	return state
}
