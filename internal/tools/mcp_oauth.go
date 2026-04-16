package tools

import (
	"net/url"
	"strings"
	"sync"
	"time"
)

type MCPOAuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scope        string
	TokenType    string
}

type MCPOAuthDiscoveryState struct {
	AuthorizationServerURL string
	ResourceMetadataURL    string
}

type MCPOAuthEntry struct {
	ServerName     string
	ServerURL      string
	ClientID       string
	ClientSecret   string
	Tokens         MCPOAuthTokens
	DiscoveryState MCPOAuthDiscoveryState
	StepUpScope    string
}

type MCPOAuthStore interface {
	Entry(serverName string, connection MCPConnection) (MCPOAuthEntry, bool)
	SaveEntry(serverName string, connection MCPConnection, entry MCPOAuthEntry) error
	DeleteEntry(serverName string, connection MCPConnection) error
}

type MCPOAuthMemoryStore struct {
	mu      sync.Mutex
	entries map[string]MCPOAuthEntry
}

func NewMCPOAuthMemoryStore() *MCPOAuthMemoryStore {
	return &MCPOAuthMemoryStore{entries: make(map[string]MCPOAuthEntry)}
}

func (s *MCPOAuthMemoryStore) Entry(serverName string, connection MCPConnection) (MCPOAuthEntry, bool) {
	if s == nil {
		return MCPOAuthEntry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[mcpOAuthServerKey(serverName, connection)]
	return entry, ok
}

func (s *MCPOAuthMemoryStore) SaveEntry(serverName string, connection MCPConnection, entry MCPOAuthEntry) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]MCPOAuthEntry)
	}
	if entry.ServerName == "" {
		entry.ServerName = serverName
	}
	if entry.ServerURL == "" {
		entry.ServerURL = mcpOAuthConnectionURL(connection)
	}
	s.entries[mcpOAuthServerKey(serverName, connection)] = entry
	return nil
}

func (s *MCPOAuthMemoryStore) DeleteEntry(serverName string, connection MCPConnection) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, mcpOAuthServerKey(serverName, connection))
	return nil
}

type MCPOAuthProvider struct {
	store              MCPOAuthStore
	serverName         string
	connection         MCPConnection
	pendingStepUpScope string
	capturedScopes     string
}

func NewMCPOAuthProvider(store MCPOAuthStore, serverName string, connection MCPConnection) *MCPOAuthProvider {
	if strings.TrimSpace(connection.Name) == "" {
		connection.Name = serverName
	}
	return &MCPOAuthProvider{
		store:      store,
		serverName: strings.TrimSpace(serverName),
		connection: connection,
	}
}

func (p *MCPOAuthProvider) Tokens(now time.Time) (MCPOAuthTokens, bool) {
	if p == nil || p.store == nil {
		return MCPOAuthTokens{}, false
	}
	entry, ok := p.store.Entry(p.serverName, p.connection)
	if !ok || strings.TrimSpace(entry.Tokens.AccessToken) == "" {
		return MCPOAuthTokens{}, false
	}
	tokens := entry.Tokens
	if !tokens.ExpiresAt.IsZero() && !tokens.ExpiresAt.After(now) && strings.TrimSpace(tokens.RefreshToken) == "" {
		return MCPOAuthTokens{}, false
	}
	if p.needsStepUp(tokens.Scope) {
		tokens.RefreshToken = ""
	}
	if strings.TrimSpace(tokens.TokenType) == "" {
		tokens.TokenType = "Bearer"
	}
	return tokens, true
}

func (p *MCPOAuthProvider) SaveTokens(tokens MCPOAuthTokens) error {
	if p == nil || p.store == nil {
		return nil
	}
	entry, _ := p.store.Entry(p.serverName, p.connection)
	entry.ServerName = p.serverName
	entry.ServerURL = mcpOAuthConnectionURL(p.connection)
	if strings.TrimSpace(tokens.TokenType) == "" {
		tokens.TokenType = "Bearer"
	}
	entry.Tokens = tokens
	entry.StepUpScope = ""
	p.pendingStepUpScope = ""
	return p.store.SaveEntry(p.serverName, p.connection, entry)
}

func (p *MCPOAuthProvider) SaveDiscoveryState(state MCPOAuthDiscoveryState) error {
	if p == nil || p.store == nil {
		return nil
	}
	entry, _ := p.store.Entry(p.serverName, p.connection)
	entry.ServerName = p.serverName
	entry.ServerURL = mcpOAuthConnectionURL(p.connection)
	entry.DiscoveryState = MCPOAuthDiscoveryState{
		AuthorizationServerURL: strings.TrimSpace(state.AuthorizationServerURL),
		ResourceMetadataURL:    strings.TrimSpace(state.ResourceMetadataURL),
	}
	return p.store.SaveEntry(p.serverName, p.connection, entry)
}

func (p *MCPOAuthProvider) SaveStepUpScope(scope string) error {
	scope = strings.TrimSpace(scope)
	if p == nil || p.store == nil || scope == "" {
		return nil
	}
	entry, _ := p.store.Entry(p.serverName, p.connection)
	entry.ServerName = p.serverName
	entry.ServerURL = mcpOAuthConnectionURL(p.connection)
	entry.StepUpScope = scope
	return p.store.SaveEntry(p.serverName, p.connection, entry)
}

func (p *MCPOAuthProvider) MarkStepUpPending(scope string) {
	if p == nil {
		return
	}
	p.pendingStepUpScope = strings.TrimSpace(scope)
}

func (p *MCPOAuthProvider) RedirectToAuthorization(authorizationURL url.URL, handleRedirection bool) {
	if p == nil {
		return
	}
	scopes := strings.TrimSpace(authorizationURL.Query().Get("scope"))
	if scopes == "" && p.store != nil {
		if entry, ok := p.store.Entry(p.serverName, p.connection); ok {
			scopes = strings.TrimSpace(entry.StepUpScope)
		}
	}
	p.capturedScopes = scopes
	if scopes != "" && !handleRedirection {
		_ = p.SaveStepUpScope(scopes)
	}
}

func (p *MCPOAuthProvider) AuthStartContext() MCPAuthStartContext {
	if p == nil {
		return MCPAuthStartContext{}
	}
	ctx := MCPAuthStartContext{
		Scope:               strings.TrimSpace(p.connection.AuthScope),
		ResourceMetadataURL: strings.TrimSpace(p.connection.AuthResourceMetadataURL),
		Challenge:           cloneStringMap(p.connection.AuthChallenge),
	}
	if ctx.ResourceMetadataURL == "" && len(ctx.Challenge) > 0 {
		ctx.ResourceMetadataURL = strings.TrimSpace(ctx.Challenge["resource_metadata"])
	}
	if p.store != nil {
		if entry, ok := p.store.Entry(p.serverName, p.connection); ok {
			if ctx.Scope == "" {
				ctx.Scope = strings.TrimSpace(entry.StepUpScope)
			}
			if ctx.ResourceMetadataURL == "" {
				ctx.ResourceMetadataURL = strings.TrimSpace(entry.DiscoveryState.ResourceMetadataURL)
			}
		}
	}
	return ctx
}

func (p *MCPOAuthProvider) EnrichConnection(connection MCPConnection) MCPConnection {
	ctx := p.AuthStartContext()
	if connection.AuthScope == "" {
		connection.AuthScope = ctx.Scope
	}
	if connection.AuthResourceMetadataURL == "" {
		connection.AuthResourceMetadataURL = ctx.ResourceMetadataURL
	}
	if len(connection.AuthChallenge) == 0 {
		connection.AuthChallenge = cloneStringMap(ctx.Challenge)
	}
	return connection
}

func (p *MCPOAuthProvider) needsStepUp(tokenScope string) bool {
	scope := strings.TrimSpace(p.pendingStepUpScope)
	if scope == "" {
		return false
	}
	current := strings.Fields(tokenScope)
	for _, required := range strings.Fields(scope) {
		if !stringSliceContains(current, required) {
			return true
		}
	}
	return false
}

type MCPAuthStartContext struct {
	Scope               string
	ResourceMetadataURL string
	Challenge           map[string]string
}

func EnrichMCPConnectionWithOAuthStore(store MCPOAuthStore, serverName string, connection MCPConnection) MCPConnection {
	return NewMCPOAuthProvider(store, serverName, connection).EnrichConnection(connection)
}

func mcpOAuthServerKey(serverName string, connection MCPConnection) string {
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		serverName = strings.TrimSpace(connection.Name)
	}
	return serverName + "|" + mcpOAuthConnectionURL(connection)
}

func mcpOAuthConnectionURL(connection MCPConnection) string {
	if value := strings.TrimSpace(connection.BaseURL); value != "" {
		return value
	}
	return strings.TrimSpace(connection.URL)
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
