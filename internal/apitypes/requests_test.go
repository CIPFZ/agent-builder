package apitypes_test

import (
	"encoding/json"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/oauth"
	"github.com/stretchr/testify/require"
)

func TestConfigProviderKeyRequestStringRoundTrip(t *testing.T) {
	t.Parallel()

	apiKey, err := json.Marshal("sk-test-123")
	require.NoError(t, err)

	src := apitypes.ConfigProviderKeyRequest{
		Scope:      config.ScopeGlobal,
		ProviderID: "openai",
		Kind:       apitypes.APIKeyKindString,
		APIKey:     apiKey,
	}
	b, err := json.Marshal(src)
	require.NoError(t, err)

	var got apitypes.ConfigProviderKeyRequest
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, apitypes.APIKeyKindString, got.Kind)

	decoded, err := got.DecodeAPIKey()
	require.NoError(t, err)
	s, ok := decoded.(string)
	require.True(t, ok, "expected string, got %T", decoded)
	require.Equal(t, "sk-test-123", s)
}

func TestConfigProviderKeyRequestOAuthRoundTrip(t *testing.T) {
	t.Parallel()

	tok := &oauth.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresIn:    60,
		ExpiresAt:    1234567890,
	}
	apiKey, err := json.Marshal(tok)
	require.NoError(t, err)

	src := apitypes.ConfigProviderKeyRequest{
		Scope:      config.ScopeGlobal,
		ProviderID: "hyper",
		Kind:       apitypes.APIKeyKindOAuth,
		APIKey:     apiKey,
	}
	b, err := json.Marshal(src)
	require.NoError(t, err)

	var got apitypes.ConfigProviderKeyRequest
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, apitypes.APIKeyKindOAuth, got.Kind)

	decoded, err := got.DecodeAPIKey()
	require.NoError(t, err)
	gotTok, ok := decoded.(*oauth.Token)
	require.True(t, ok, "expected *oauth.Token, got %T", decoded)
	require.Equal(t, tok, gotTok)
}

func TestConfigProviderKeyRequestUnknownKind(t *testing.T) {
	t.Parallel()

	req := apitypes.ConfigProviderKeyRequest{
		Kind:   apitypes.APIKeyKind("bogus"),
		APIKey: json.RawMessage(`"x"`),
	}
	_, err := req.DecodeAPIKey()
	require.Error(t, err)
	require.Contains(t, err.Error(), "bogus")
}

func TestConfigProviderKeyRequestMalformedPayload(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		kind apitypes.APIKeyKind
		raw  string
	}{
		{"string kind with object payload", apitypes.APIKeyKindString, `{"foo":"bar"}`},
		{"oauth kind with string payload", apitypes.APIKeyKindOAuth, `"not-a-token"`},
		{"oauth kind with invalid json", apitypes.APIKeyKindOAuth, `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := apitypes.ConfigProviderKeyRequest{
				Kind:   tc.kind,
				APIKey: json.RawMessage(tc.raw),
			}
			_, err := req.DecodeAPIKey()
			require.Error(t, err)
		})
	}
}
