package tools_test

import (
	"net/url"
	"testing"
	"time"

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
