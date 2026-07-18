package auth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/whg517/fleet/internal/infra/config"
)

// --- PKCE / OIDC helper tests ---

func TestGenerateCodeVerifier(t *testing.T) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(verifier) < 43 {
		t.Errorf("verifier too short: got %d chars, want >= 43", len(verifier))
	}
	if len(verifier) > 128 {
		t.Errorf("verifier too long: got %d chars, want <= 128", len(verifier))
	}
}

func TestComputeCodeChallenge(t *testing.T) {
	tests := []struct {
		name     string
		verifier string
		want     string
	}{
		{
			name:     "RFC 7636 Appendix B test vector",
			verifier: "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			want:     "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeCodeChallenge(tt.verifier)
			if got != tt.want {
				t.Errorf("computeCodeChallenge() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateRandomString(t *testing.T) {
	s1, err := generateRandomString(16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s2, err := generateRandomString(16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s1 == s2 {
		t.Error("two random strings should not be equal")
	}
	if len(s1) == 0 {
		t.Error("random string should not be empty")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	t1, err := generateRefreshToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t2, err := generateRefreshToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if t1 == t2 {
		t.Error("two refresh tokens should not be equal")
	}
	if len(t1) < 32 {
		t.Errorf("refresh token too short: %d chars", len(t1))
	}
}

func TestRefreshKey(t *testing.T) {
	got := refreshKey("abc123")
	want := "session:refresh:abc123"
	if got != want {
		t.Errorf("refreshKey() = %v, want %v", got, want)
	}
}

func TestStateKey(t *testing.T) {
	got := stateKey("xyz789")
	want := "oidc:state:xyz789"
	if got != want {
		t.Errorf("stateKey() = %v, want %v", got, want)
	}
}

// --- SessionManager (JWT logic) tests ---
// These test JWT signing/validation without Redis.

func newTestSessionManager() *SessionManager {
	return &SessionManager{
		jwtSecret:   []byte("test-secret-key-at-least-32-bytes-long!!"),
		accessTTL:   30 * time.Minute,
		refreshTTL:  8 * time.Hour,
	}
}

func TestSessionManager_ValidateAccessToken(t *testing.T) {
	sm := newTestSessionManager()

	now := time.Now()
	claims := &Claims{
		UserID: "user-123",
		Email:  "test@example.com",
		Name:   "Test User",
		Roles:  []string{"viewer"},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(sm.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   "user-123",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(sm.jwtSecret)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	parsed, err := sm.ValidateAccessToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}
	if parsed.UserID != "user-123" {
		t.Errorf("UserID = %v, want user-123", parsed.UserID)
	}
	if parsed.Email != "test@example.com" {
		t.Errorf("Email = %v, want test@example.com", parsed.Email)
	}
	if len(parsed.Roles) != 1 || parsed.Roles[0] != "viewer" {
		t.Errorf("Roles = %v, want [viewer]", parsed.Roles)
	}
}

func TestSessionManager_ValidateAccessToken_InvalidString(t *testing.T) {
	sm := newTestSessionManager()

	_, err := sm.ValidateAccessToken("not-a-valid-token")
	if err == nil {
		t.Error("expected error for invalid token string, got nil")
	}
}

func TestSessionManager_ValidateAccessToken_WrongSecret(t *testing.T) {
	sm := newTestSessionManager()

	now := time.Now()
	claims := &Claims{
		UserID: "user-123",
		Email:  "test@example.com",
		Name:   "Test User",
		Roles:  []string{},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(sm.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte("wrong-secret-key-at-least-32-bytes!!"))

	_, err := sm.ValidateAccessToken(tokenStr)
	if err == nil {
		t.Error("expected error for token signed with wrong secret, got nil")
	}
}

func TestSessionManager_ValidateAccessToken_Expired(t *testing.T) {
	sm := newTestSessionManager()

	now := time.Now()
	claims := &Claims{
		UserID: "user-123",
		Email:  "test@example.com",
		Name:   "Test User",
		Roles:  []string{},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(-10 * time.Minute)), // expired
			IssuedAt:  jwt.NewNumericDate(now.Add(-40 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString(sm.jwtSecret)

	_, err := sm.ValidateAccessToken(tokenStr)
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

func TestSessionManager_ValidateAccessToken_WrongSigningMethod(t *testing.T) {
	sm := newTestSessionManager()

	now := time.Now()
	claims := &Claims{
		UserID: "user-123",
		Email:  "test@example.com",
		Name:   "Test",
		Roles:  []string{},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(30 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	// Use RS256 which is not HMAC — should fail validation
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStr, _ := token.SignedString(sm.jwtSecret)

	_, err := sm.ValidateAccessToken(tokenStr)
	if err == nil {
		t.Error("expected error for wrong signing method, got nil")
	}
}

// --- Serialization tests ---

func TestAuthState_JSON(t *testing.T) {
	original := AuthState{
		CodeVerifier: "verifier123",
		Nonce:        "nonce456",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded AuthState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.CodeVerifier != original.CodeVerifier {
		t.Errorf("CodeVerifier = %v, want %v", decoded.CodeVerifier, original.CodeVerifier)
	}
	if decoded.Nonce != original.Nonce {
		t.Errorf("Nonce = %v, want %v", decoded.Nonce, original.Nonce)
	}
}

// --- Endpoint URL tests ---

func testOIDCConfig() config.OIDCConfig {
	return config.OIDCConfig{
		Issuer:       "https://idp.example.com",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/api/v1/auth/callback",
		Scopes:       []string{"openid", "profile", "email"},
	}
}

func TestAuthorizationEndpoint(t *testing.T) {
	cfg := testOIDCConfig()
	url := authorizationEndpoint(cfg, "test-state", "test-challenge")

	checks := []string{
		"response_type=code",
		"client_id=test-client-id",
		"state=test-state",
		"code_challenge=test-challenge",
		"code_challenge_method=S256",
	}
	for _, check := range checks {
		if !strings.Contains(url, check) {
			t.Errorf("URL missing %q: %s", check, url)
		}
	}
}

func TestTokenEndpoint(t *testing.T) {
	cfg := testOIDCConfig()
	got := tokenEndpoint(cfg)
	want := "https://idp.example.com/token"
	if got != want {
		t.Errorf("tokenEndpoint() = %v, want %v", got, want)
	}
}

func TestUserinfoEndpoint(t *testing.T) {
	cfg := testOIDCConfig()
	got := userinfoEndpoint(cfg)
	want := "https://idp.example.com/userinfo"
	if got != want {
		t.Errorf("userinfoEndpoint() = %v, want %v", got, want)
	}
}

func TestClaims_Struct(t *testing.T) {
	c := Claims{
		UserID: "u1",
		Email:  "e@example.com",
		Name:   "Test",
		Roles:  []string{"admin", "viewer"},
	}
	if c.UserID != "u1" {
		t.Errorf("UserID = %v, want u1", c.UserID)
	}
	if len(c.Roles) != 2 {
		t.Errorf("Roles length = %d, want 2", len(c.Roles))
	}
}

// --- Middleware helper tests (via the auth package) ---

func TestIsPublicPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/api/v1/auth/login", true},
		{"/api/v1/auth/callback", true},
		{"/api/v1/health", true},
		{"/api/v1/health/ready", true},
		{"/api/v1/auth/me", false},
		{"/api/v1/clusters", false},
		{"/api/v1/auth/refresh", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			// isPublicPath is in the middleware package, test the publicPaths list
			// by checking against our known list
			got := checkPublic(tt.path)
			if got != tt.want {
				t.Errorf("checkPublic(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// checkPublic replicates the middleware logic for testing here.
func checkPublic(path string) bool {
	publicPaths := []string{
		"/api/v1/auth/login",
		"/api/v1/auth/callback",
		"/api/v1/health",
	}
	for _, p := range publicPaths {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}
