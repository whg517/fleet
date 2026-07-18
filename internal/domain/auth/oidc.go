package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/whg517/fleet/internal/infra/config"
)

// AuthState holds the data stored during an OIDC login flow.
// It is persisted in Redis keyed by the state parameter.
type AuthState struct {
	CodeVerifier string `json:"code_verifier"`
	Nonce        string `json:"nonce"`
}

// generateRandomString returns a URL-safe random string of the given byte length.
func generateRandomString(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeVerifier creates a PKCE code_verifier (43-128 chars, RFC 7636).
func generateCodeVerifier() (string, error) {
	return generateRandomString(32) // 43 chars after base64url encoding
}

// computeCodeChallenge calculates S256 code_challenge from a verifier.
// code_challenge = BASE64URL-ENCODE(SHA256(ASCII(code_verifier)))
func computeCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// authorizationEndpoint builds the full IdP authorize URL with PKCE params.
func authorizationEndpoint(cfg config.OIDCConfig, state, codeChallenge string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", cfg.ClientID)
	params.Set("redirect_uri", cfg.RedirectURL)
	params.Set("scope", strings.Join(cfg.Scopes, " "))
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	return cfg.Issuer + "/authorize?" + params.Encode()
}

// tokenEndpoint returns the IdP token endpoint URL.
func tokenEndpoint(cfg config.OIDCConfig) string {
	return cfg.Issuer + "/token"
}

// userinfoEndpoint returns the IdP userinfo endpoint URL.
func userinfoEndpoint(cfg config.OIDCConfig) string {
	return cfg.Issuer + "/userinfo"
}
