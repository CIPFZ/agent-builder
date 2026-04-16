package tools

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

type MCPOAuthFlowOptions struct {
	HTTPClient            *http.Client
	RedirectListenAddress string
	RedirectPath          string
	Now                   func() time.Time
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

type MCPOAuthFileStore struct {
	mu      sync.Mutex
	path    string
	entries map[string]MCPOAuthEntry
	loaded  bool
}

func NewMCPOAuthFileStore(path string) *MCPOAuthFileStore {
	return &MCPOAuthFileStore{path: strings.TrimSpace(path)}
}

func NewDefaultMCPOAuthStore() MCPOAuthStore {
	configDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configDir) == "" {
		return NewMCPOAuthMemoryStore()
	}
	return NewMCPOAuthFileStore(filepath.Join(configDir, "myclaw", "mcp-oauth.json"))
}

func NewDefaultMCPOAuthAuthenticator(store MCPOAuthStore) MCPAuthenticator {
	return NewDefaultMCPOAuthAuthenticatorWithOptions(store, MCPOAuthFlowOptions{})
}

func NewDefaultMCPOAuthAuthenticatorWithOptions(store MCPOAuthStore, opts MCPOAuthFlowOptions) MCPAuthenticator {
	flow := mcpOAuthFlow{store: store, opts: opts}
	if flow.opts.HTTPClient == nil {
		flow.opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if strings.TrimSpace(flow.opts.RedirectListenAddress) == "" {
		flow.opts.RedirectListenAddress = "127.0.0.1:0"
	}
	if strings.TrimSpace(flow.opts.RedirectPath) == "" {
		flow.opts.RedirectPath = "/callback"
	}
	if flow.opts.Now == nil {
		flow.opts.Now = time.Now
	}
	return flow.authenticate
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

func (s *MCPOAuthFileStore) Entry(serverName string, connection MCPConnection) (MCPOAuthEntry, bool) {
	if s == nil {
		return MCPOAuthEntry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return MCPOAuthEntry{}, false
	}
	entry, ok := s.entries[mcpOAuthServerKey(serverName, connection)]
	return entry, ok
}

func (s *MCPOAuthFileStore) SaveEntry(serverName string, connection MCPConnection, entry MCPOAuthEntry) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
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
	return s.saveLocked()
}

func (s *MCPOAuthFileStore) DeleteEntry(serverName string, connection MCPConnection) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	delete(s.entries, mcpOAuthServerKey(serverName, connection))
	return s.saveLocked()
}

func (s *MCPOAuthFileStore) loadLocked() error {
	if s.loaded {
		return nil
	}
	s.loaded = true
	s.entries = make(map[string]MCPOAuthEntry)
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, &s.entries)
}

func (s *MCPOAuthFileStore) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
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

type mcpOAuthFlow struct {
	store MCPOAuthStore
	opts  MCPOAuthFlowOptions
}

type mcpOAuthProtectedResourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
	AuthorizationServer  string   `json:"authorization_server"`
}

type mcpOAuthAuthorizationServerMetadata struct {
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RegistrationEndpoint  string   `json:"registration_endpoint"`
	ScopesSupported       []string `json:"scopes_supported"`
}

type mcpOAuthRegistrationResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type mcpOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

func (f mcpOAuthFlow) authenticate(ctx context.Context, serverName string, connection MCPConnection) (MCPAuthStartResult, error) {
	if !isHTTPMCPConnection(connection) {
		return MCPAuthStartResult{
			Status:  "unsupported",
			Message: fmt.Sprintf("Server %q uses a transport that does not support OAuth from this tool.", serverName),
		}, nil
	}
	if f.store == nil {
		f.store = NewMCPOAuthMemoryStore()
	}
	provider := NewMCPOAuthProvider(f.store, serverName, connection)
	startContext := provider.AuthStartContext()
	metadata, discoveryState, err := f.discoverAuthorizationServer(ctx, connection, startContext)
	if err != nil {
		return MCPAuthStartResult{}, err
	}
	if metadata.AuthorizationEndpoint == "" {
		metadata.AuthorizationEndpoint = strings.TrimSpace(connection.AuthURL)
	}
	if metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" {
		return MCPAuthStartResult{}, fmt.Errorf("MCP OAuth metadata for %q is missing authorization or token endpoint", serverName)
	}
	if err := provider.SaveDiscoveryState(discoveryState); err != nil {
		return MCPAuthStartResult{}, err
	}

	entry, _ := f.store.Entry(serverName, connection)
	clientID := strings.TrimSpace(entry.ClientID)
	clientSecret := strings.TrimSpace(entry.ClientSecret)
	redirectURI, waitForCallback, closeCallback, err := f.startCallbackServer(ctx)
	if err != nil {
		return MCPAuthStartResult{}, err
	}
	if clientID == "" {
		registered, err := f.registerClient(ctx, metadata.RegistrationEndpoint, serverName, redirectURI)
		if err != nil {
			closeCallback()
			return MCPAuthStartResult{}, err
		}
		clientID = registered.ClientID
		clientSecret = registered.ClientSecret
		entry.ServerName = serverName
		entry.ServerURL = mcpOAuthConnectionURL(connection)
		entry.ClientID = clientID
		entry.ClientSecret = clientSecret
		entry.DiscoveryState = discoveryState
		if err := f.store.SaveEntry(serverName, connection, entry); err != nil {
			closeCallback()
			return MCPAuthStartResult{}, err
		}
	}

	state, err := randomURLSafeString(32)
	if err != nil {
		closeCallback()
		return MCPAuthStartResult{}, err
	}
	verifier, err := randomURLSafeString(64)
	if err != nil {
		closeCallback()
		return MCPAuthStartResult{}, err
	}
	challenge := pkceS256Challenge(verifier)
	scope := strings.TrimSpace(startContext.Scope)
	if scope == "" {
		scope = strings.Join(metadata.ScopesSupported, " ")
	}
	authURL, err := buildMCPAuthorizationURL(metadata.AuthorizationEndpoint, clientID, redirectURI, state, challenge, scope)
	if err != nil {
		closeCallback()
		return MCPAuthStartResult{}, err
	}
	provider.RedirectToAuthorization(*authURL, true)
	completion := make(chan MCPAuthCompletionResult, 1)
	go func() {
		defer close(completion)
		defer closeCallback()
		code, callbackState, err := waitForCallback()
		if err != nil {
			completion <- MCPAuthCompletionResult{Status: "error", Error: err}
			return
		}
		if callbackState != state {
			completion <- MCPAuthCompletionResult{Status: "error", Error: fmt.Errorf("OAuth state mismatch")}
			return
		}
		tokens, err := f.exchangeCode(ctx, metadata.TokenEndpoint, clientID, clientSecret, code, redirectURI, verifier)
		if err != nil {
			completion <- MCPAuthCompletionResult{Status: "error", Error: err}
			return
		}
		expiresAt := time.Time{}
		if tokens.ExpiresIn > 0 {
			expiresAt = f.opts.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
		}
		tokenType := strings.TrimSpace(tokens.TokenType)
		if tokenType == "" {
			tokenType = "Bearer"
		}
		if err := provider.SaveTokens(MCPOAuthTokens{
			AccessToken:  strings.TrimSpace(tokens.AccessToken),
			RefreshToken: strings.TrimSpace(tokens.RefreshToken),
			ExpiresAt:    expiresAt,
			Scope:        strings.TrimSpace(tokens.Scope),
			TokenType:    tokenType,
		}); err != nil {
			completion <- MCPAuthCompletionResult{Status: "error", Error: err}
			return
		}
		completion <- MCPAuthCompletionResult{Status: "complete", Message: "Authentication completed"}
	}()
	return MCPAuthStartResult{
		Status:              "auth_url",
		AuthURL:             authURL.String(),
		Message:             fmt.Sprintf("Ask the user to open this URL in their browser to authorize the %s MCP server:\n\n%s\n\nOnce they complete the flow, the server's tools will become available automatically.", serverName, authURL.String()),
		Scope:               scope,
		ResourceMetadataURL: startContext.ResourceMetadataURL,
		Challenge:           startContext.Challenge,
		Completion:          completion,
	}, nil
}

func (f mcpOAuthFlow) discoverAuthorizationServer(ctx context.Context, connection MCPConnection, start MCPAuthStartContext) (mcpOAuthAuthorizationServerMetadata, MCPOAuthDiscoveryState, error) {
	resourceMetadataURL := strings.TrimSpace(start.ResourceMetadataURL)
	authServerURL := ""
	if resourceMetadataURL != "" {
		resource, err := f.fetchProtectedResourceMetadata(ctx, resourceMetadataURL)
		if err != nil {
			return mcpOAuthAuthorizationServerMetadata{}, MCPOAuthDiscoveryState{}, err
		}
		if len(resource.AuthorizationServers) > 0 {
			authServerURL = strings.TrimSpace(resource.AuthorizationServers[0])
		}
		if authServerURL == "" {
			authServerURL = strings.TrimSpace(resource.AuthorizationServer)
		}
	}
	if authServerURL == "" {
		authServerURL = inferAuthorizationServerURL(connection)
	}
	if authServerURL == "" {
		return mcpOAuthAuthorizationServerMetadata{}, MCPOAuthDiscoveryState{}, fmt.Errorf("MCP OAuth authorization server discovery failed")
	}
	metadata, err := f.fetchAuthorizationServerMetadata(ctx, authServerURL)
	if err != nil {
		return mcpOAuthAuthorizationServerMetadata{}, MCPOAuthDiscoveryState{}, err
	}
	return metadata, MCPOAuthDiscoveryState{
		AuthorizationServerURL: strings.TrimRight(authServerURL, "/"),
		ResourceMetadataURL:    resourceMetadataURL,
	}, nil
}

func (f mcpOAuthFlow) fetchProtectedResourceMetadata(ctx context.Context, endpoint string) (mcpOAuthProtectedResourceMetadata, error) {
	var metadata mcpOAuthProtectedResourceMetadata
	if err := f.getJSON(ctx, endpoint, &metadata); err != nil {
		return metadata, fmt.Errorf("fetch MCP protected resource metadata: %w", err)
	}
	return metadata, nil
}

func (f mcpOAuthFlow) fetchAuthorizationServerMetadata(ctx context.Context, authServerURL string) (mcpOAuthAuthorizationServerMetadata, error) {
	endpoint := authorizationServerMetadataEndpoint(authServerURL)
	var metadata mcpOAuthAuthorizationServerMetadata
	if err := f.getJSON(ctx, endpoint, &metadata); err != nil {
		return metadata, fmt.Errorf("fetch MCP authorization server metadata: %w", err)
	}
	return metadata, nil
}

func (f mcpOAuthFlow) registerClient(ctx context.Context, endpoint, serverName, redirectURI string) (mcpOAuthRegistrationResponse, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return mcpOAuthRegistrationResponse{}, fmt.Errorf("MCP OAuth server %q did not advertise dynamic client registration", serverName)
	}
	body := map[string]any{
		"client_name":                "Claude Code (" + serverName + ")",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	var registered mcpOAuthRegistrationResponse
	if err := f.postJSON(ctx, endpoint, body, &registered); err != nil {
		return registered, fmt.Errorf("register MCP OAuth client: %w", err)
	}
	if strings.TrimSpace(registered.ClientID) == "" {
		return registered, fmt.Errorf("MCP OAuth registration response missing client_id")
	}
	return registered, nil
}

func (f mcpOAuthFlow) exchangeCode(ctx context.Context, endpoint, clientID, clientSecret, code, redirectURI, verifier string) (mcpOAuthTokenResponse, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("redirect_uri", redirectURI)
	values.Set("client_id", clientID)
	values.Set("code_verifier", verifier)
	if clientSecret != "" {
		values.Set("client_secret", clientSecret)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return mcpOAuthTokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var tokens mcpOAuthTokenResponse
	if err := f.doJSON(request, &tokens); err != nil {
		return tokens, fmt.Errorf("exchange MCP OAuth code: %w", err)
	}
	if strings.TrimSpace(tokens.AccessToken) == "" {
		return tokens, fmt.Errorf("MCP OAuth token response missing access_token")
	}
	return tokens, nil
}

func (f mcpOAuthFlow) refreshTokens(ctx context.Context, serverName string, connection MCPConnection, entry MCPOAuthEntry) (MCPOAuthTokens, error) {
	refreshToken := strings.TrimSpace(entry.Tokens.RefreshToken)
	if refreshToken == "" {
		return MCPOAuthTokens{}, fmt.Errorf("MCP OAuth entry for %q has no refresh token", serverName)
	}
	authServerURL := strings.TrimSpace(entry.DiscoveryState.AuthorizationServerURL)
	if authServerURL == "" {
		authServerURL = inferAuthorizationServerURL(connection)
	}
	if authServerURL == "" {
		return MCPOAuthTokens{}, fmt.Errorf("MCP OAuth entry for %q has no authorization server metadata", serverName)
	}
	metadata, err := f.fetchAuthorizationServerMetadata(ctx, authServerURL)
	if err != nil {
		return MCPOAuthTokens{}, err
	}
	if strings.TrimSpace(metadata.TokenEndpoint) == "" {
		return MCPOAuthTokens{}, fmt.Errorf("MCP OAuth metadata for %q is missing token endpoint", serverName)
	}
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", refreshToken)
	if strings.TrimSpace(entry.ClientID) != "" {
		values.Set("client_id", strings.TrimSpace(entry.ClientID))
	}
	if strings.TrimSpace(entry.ClientSecret) != "" {
		values.Set("client_secret", strings.TrimSpace(entry.ClientSecret))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.TokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return MCPOAuthTokens{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response mcpOAuthTokenResponse
	if err := f.doJSON(request, &response); err != nil {
		return MCPOAuthTokens{}, fmt.Errorf("refresh MCP OAuth token: %w", err)
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return MCPOAuthTokens{}, fmt.Errorf("MCP OAuth refresh response missing access_token")
	}
	expiresAt := time.Time{}
	if response.ExpiresIn > 0 {
		expiresAt = f.opts.Now().Add(time.Duration(response.ExpiresIn) * time.Second)
	}
	tokenType := strings.TrimSpace(response.TokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}
	tokens := MCPOAuthTokens{
		AccessToken:  strings.TrimSpace(response.AccessToken),
		RefreshToken: strings.TrimSpace(response.RefreshToken),
		ExpiresAt:    expiresAt,
		Scope:        strings.TrimSpace(response.Scope),
		TokenType:    tokenType,
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = refreshToken
	}
	if tokens.Scope == "" {
		tokens.Scope = entry.Tokens.Scope
	}
	if err := NewMCPOAuthProvider(f.store, serverName, connection).SaveTokens(tokens); err != nil {
		return MCPOAuthTokens{}, err
	}
	return tokens, nil
}

func (f mcpOAuthFlow) getJSON(ctx context.Context, endpoint string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	return f.doJSON(request, out)
}

func (f mcpOAuthFlow) postJSON(ctx context.Context, endpoint string, body any, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return f.doJSON(request, out)
}

func (f mcpOAuthFlow) doJSON(request *http.Request, out any) error {
	response, err := f.opts.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s returned %d: %s", request.URL.String(), response.StatusCode, strings.TrimSpace(string(data)))
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (f mcpOAuthFlow) startCallbackServer(ctx context.Context) (string, func() (string, string, error), func(), error) {
	listener, err := net.Listen("tcp", f.opts.RedirectListenAddress)
	if err != nil {
		return "", nil, nil, err
	}
	path := f.opts.RedirectPath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	result := make(chan struct {
		code  string
		state string
		err   error
	}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		errText := r.URL.Query().Get("error")
		if errText != "" {
			result <- struct {
				code  string
				state string
				err   error
			}{err: fmt.Errorf("OAuth callback error: %s", errText)}
			http.Error(w, "Authentication failed", http.StatusBadRequest)
			return
		}
		if code == "" || state == "" {
			result <- struct {
				code  string
				state string
				err   error
			}{err: fmt.Errorf("OAuth callback missing code or state")}
			http.Error(w, "Authentication failed", http.StatusBadRequest)
			return
		}
		result <- struct {
			code  string
			state string
			err   error
		}{code: code, state: state}
		_, _ = w.Write([]byte("Authentication complete. You can return to the agent."))
	})
	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(listener)
	}()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		return "", nil, nil, err
	}
	if host == "" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	redirectURI := "http://" + net.JoinHostPort(host, port) + path
	wait := func() (string, string, error) {
		select {
		case value := <-result:
			return value.code, value.state, value.err
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
	}
	closeFn := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}
	return redirectURI, wait, closeFn, nil
}

func inferAuthorizationServerURL(connection MCPConnection) string {
	if raw := strings.TrimSpace(connection.AuthURL); raw != "" {
		parsed, err := url.Parse(raw)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host
		}
	}
	if raw := connectionURL(connection); raw != "" {
		parsed, err := url.Parse(raw)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host
		}
	}
	return ""
}

func authorizationServerMetadataEndpoint(authServerURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(authServerURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(authServerURL, "/") + "/.well-known/oauth-authorization-server"
	}
	if strings.Contains(parsed.Path, "/.well-known/oauth-authorization-server") {
		return parsed.String()
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/.well-known/oauth-authorization-server"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func buildMCPAuthorizationURL(endpoint, clientID, redirectURI, state, challenge, scope string) (*url.URL, error) {
	authURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := authURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	if strings.TrimSpace(scope) != "" {
		query.Set("scope", strings.TrimSpace(scope))
	}
	authURL.RawQuery = query.Encode()
	return authURL, nil
}

func randomURLSafeString(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func pkceS256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
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
