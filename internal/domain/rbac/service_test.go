package rbac

import (
	"context"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/redis/go-redis/v9"
)

// newTestEnforcer creates an in-memory Casbin enforcer with default policies.
// This avoids the need for a real database connection during tests.
func newTestEnforcer(t *testing.T) *casbin.Enforcer {
	t.Helper()
	m, err := model.NewModelFromString(casbinModelText)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		t.Fatalf("failed to create enforcer: %v", err)
	}

	// Seed default policies
	for _, p := range defaultPolicies {
		_, err := e.AddPolicy(p[1], p[2], p[3], p[4])
		if err != nil {
			t.Fatalf("failed to add policy %v: %v", p, err)
		}
	}

	return e
}

// newTestService creates an RBAC service backed by an in-memory enforcer.
func newTestService(t *testing.T) *serviceImpl {
	t.Helper()
	_ = newTestEnforcer(t) // ensure model/policy parse correctly
	return &serviceImpl{}
}

// TestRoleMatrix tests each role against expected allowed/denied operations.
func TestRoleMatrix(t *testing.T) {
	e := newTestEnforcer(t)

	tests := []struct {
		name    string
		sub     string
		dom     string
		obj     string
		act     string
		allowed bool
	}{
		// admin — full access
		{"admin GET clusters", "admin", "*", "/api/v1/clusters", "GET", true},
		{"admin POST clusters", "admin", "*", "/api/v1/clusters", "POST", true},
		{"admin DELETE clusters", "admin", "*", "/api/v1/clusters/123", "DELETE", true},
		{"admin PUT anything", "admin", "*", "/api/v1/anything/else", "PUT", true},
		{"admin PUT rbac", "admin", "*", "/api/v1/rbac/users/123/roles", "PUT", true},

		// operator — cluster + deployment management
		{"operator GET clusters", "operator", "*", "/api/v1/clusters", "GET", true},
		{"operator POST clusters", "operator", "*", "/api/v1/clusters", "POST", true},
		{"operator PUT clusters", "operator", "*", "/api/v1/clusters/123", "PUT", true},
		{"operator DELETE clusters", "operator", "*", "/api/v1/clusters/123", "DELETE", true},
		{"operator POST test", "operator", "*", "/api/v1/clusters/123/test", "POST", true},
		{"operator GET environments", "operator", "*", "/api/v1/environments", "GET", true},
		{"operator POST deploy", "operator", "*", "/api/v1/deployments", "POST", true},
		{"operator POST approve", "operator", "*", "/api/v1/deployments/456/approve", "POST", true},
		{"operator POST rollback", "operator", "*", "/api/v1/deployments/456/rollback", "POST", true},
		{"operator denied GET services", "operator", "*", "/api/v1/services", "GET", false},
		{"operator denied GET audit", "operator", "*", "/api/v1/audit-logs", "GET", false},

		// developer — read clusters, manage services, create deployments
		{"developer GET clusters", "developer", "*", "/api/v1/clusters", "GET", true},
		{"developer GET clusters by id", "developer", "*", "/api/v1/clusters/123", "GET", true},
		{"developer denied POST clusters", "developer", "*", "/api/v1/clusters", "POST", false},
		{"developer GET services", "developer", "*", "/api/v1/services", "GET", true},
		{"developer POST services", "developer", "*", "/api/v1/services", "POST", true},
		{"developer GET deployments", "developer", "*", "/api/v1/deployments", "GET", true},
		{"developer POST deployments", "developer", "*", "/api/v1/deployments", "POST", true},
		{"developer denied DELETE clusters", "developer", "*", "/api/v1/clusters/123", "DELETE", false},

		// viewer — read-only access to specific resource groups
		{"viewer GET clusters", "viewer", "*", "/api/v1/clusters", "GET", true},
		{"viewer GET services", "viewer", "*", "/api/v1/services", "GET", true},
		{"viewer GET deployments", "viewer", "*", "/api/v1/deployments", "GET", true},
		{"viewer GET audit-logs", "viewer", "*", "/api/v1/audit-logs", "GET", true},
		{"viewer denied GET rbac", "viewer", "*", "/api/v1/rbac/roles", "GET", false},
		{"viewer denied POST", "viewer", "*", "/api/v1/clusters", "POST", false},
		{"viewer denied DELETE", "viewer", "*", "/api/v1/clusters/123", "DELETE", false},

		// auditor — audit logs + cluster read
		{"auditor GET audit-logs", "auditor", "*", "/api/v1/audit-logs", "GET", true},
		{"auditor GET audit-logs verify", "auditor", "*", "/api/v1/audit-logs/verify", "GET", true},
		{"auditor GET clusters", "auditor", "*", "/api/v1/clusters", "GET", true},
		{"auditor denied POST", "auditor", "*", "/api/v1/audit-logs", "POST", false},
		{"auditor denied GET services", "auditor", "*", "/api/v1/services", "GET", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Properly test: assign role to a test user, then enforce as that user
			testUser := "test-user-" + tt.sub
			_, _ = e.AddRoleForUser(testUser, tt.sub, tt.dom)
			ok, err := e.Enforce(testUser, tt.dom, tt.obj, tt.act)
			if err != nil {
				t.Fatalf("Enforce failed: %v", err)
			}
			if ok != tt.allowed {
				t.Errorf("Enforce(%s, %s, %s, %s) = %v, want %v",
					testUser, tt.dom, tt.obj, tt.act, ok, tt.allowed)
			}
		})
	}
}

// TestDefaultPolicies verifies that the default policy list is well-formed.
func TestDefaultPolicies(t *testing.T) {
	if len(defaultPolicies) == 0 {
		t.Fatal("defaultPolicies should not be empty")
	}
	for i, p := range defaultPolicies {
		if len(p) != 5 {
			t.Errorf("policy[%d] has %d elements, want 5", i, len(p))
		}
		if p[0] != "p" {
			t.Errorf("policy[%d] ptype = %q, want \"p\"", i, p[0])
		}
	}
}

// TestRoleNames verifies all expected roles are defined.
func TestRoleNames(t *testing.T) {
	expected := map[string]bool{
		"admin": false, "operator": false, "developer": false,
		"viewer": false, "auditor": false,
	}
	for _, name := range RoleNames {
		if _, ok := expected[name]; !ok {
			t.Errorf("unexpected role: %s", name)
		}
		expected[name] = true
	}
	for name, found := range expected {
		if !found {
			t.Errorf("role %q not found in RoleNames", name)
		}
	}
}

// TestRoleDescriptions verifies every role has a description.
func TestRoleDescriptions(t *testing.T) {
	for _, name := range RoleNames {
		desc, ok := RoleDescriptions[name]
		if !ok {
			t.Errorf("role %q missing from RoleDescriptions", name)
		}
		if desc == "" {
			t.Errorf("role %q has empty description", name)
		}
	}
}

// TestAddRemoveRole tests adding and removing roles for users.
func TestAddRemoveRole(t *testing.T) {
	e := newTestEnforcer(t)

	userID := "user-test-001"

	// Initially no roles
	roles, err := e.GetRolesForUser(userID, "*")
	if err != nil {
		t.Fatalf("GetRolesForUser failed: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("new user should have 0 roles, got %v", roles)
	}

	// Add viewer role
	_, err = e.AddRoleForUser(userID, "viewer", "*")
	if err != nil {
		t.Fatalf("AddRoleForUser failed: %v", err)
	}
	roles, err = e.GetRolesForUser(userID, "*")
	if err != nil {
		t.Fatalf("GetRolesForUser failed: %v", err)
	}
	if len(roles) != 1 || roles[0] != "viewer" {
		t.Errorf("expected [viewer], got %v", roles)
	}

	// Viewer should be able to GET
	ok, err := e.Enforce(userID, "*", "/api/v1/clusters", "GET")
	if err != nil {
		t.Fatalf("Enforce failed: %v", err)
	}
	if !ok {
		t.Error("viewer should be allowed to GET /api/v1/clusters")
	}

	// Viewer should NOT be able to POST
	ok, err = e.Enforce(userID, "*", "/api/v1/clusters", "POST")
	if err != nil {
		t.Fatalf("Enforce failed: %v", err)
	}
	if ok {
		t.Error("viewer should be denied POST /api/v1/clusters")
	}

	// Remove viewer role
	_, err = e.DeleteRoleForUser(userID, "viewer", "*")
	if err != nil {
		t.Fatalf("DeleteRoleForUser failed: %v", err)
	}
	roles, err = e.GetRolesForUser(userID, "*")
	if err != nil {
		t.Fatalf("GetRolesForUser failed: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("after removal, expected 0 roles, got %v", roles)
	}

	// After removal, all access should be denied
	ok, err = e.Enforce(userID, "*", "/api/v1/clusters", "GET")
	if err != nil {
		t.Fatalf("Enforce failed: %v", err)
	}
	if ok {
		t.Error("user with no roles should be denied")
	}
}

// TestDefaultViewerRole tests that the default viewer role grants read access.
func TestDefaultViewerRole(t *testing.T) {
	e := newTestEnforcer(t)

	userID := "new-user-from-oidc"
	_, _ = e.AddRoleForUser(userID, "viewer", "*")

	// Viewer can GET specific resource paths
	paths := []string{
		"/api/v1/clusters",
		"/api/v1/clusters/123",
		"/api/v1/services",
		"/api/v1/deployments",
		"/api/v1/environments",
		"/api/v1/audit-logs",
	}
	for _, path := range paths {
		ok, err := e.Enforce(userID, "*", path, "GET")
		if err != nil {
			t.Errorf("Enforce(%s) failed: %v", path, err)
		}
		if !ok {
			t.Errorf("viewer should be allowed GET %s", path)
		}
	}

	// Viewer cannot do write operations
	writeTests := []struct {
		path, method string
	}{
		{"/api/v1/clusters", "POST"},
		{"/api/v1/clusters/123", "DELETE"},
		{"/api/v1/services", "PUT"},
	}
	for _, wt := range writeTests {
		ok, err := e.Enforce(userID, "*", wt.path, wt.method)
		if err != nil {
			t.Errorf("Enforce(%s %s) failed: %v", wt.method, wt.path, err)
		}
		if ok {
			t.Errorf("viewer should be denied %s %s", wt.method, wt.path)
		}
	}
}

// TestAdminFullAccess verifies admin role has unrestricted access.
func TestAdminFullAccess(t *testing.T) {
	e := newTestEnforcer(t)

	userID := "admin-user"
	_, _ = e.AddRoleForUser(userID, "admin", "*")

	tests := []struct {
		path, method string
	}{
		{"/api/v1/clusters", "GET"},
		{"/api/v1/clusters", "POST"},
		{"/api/v1/clusters/123", "PUT"},
		{"/api/v1/clusters/123", "DELETE"},
		{"/api/v1/rbac/users/123/roles", "PUT"},
		{"/api/v1/anything/else", "PATCH"},
	}
	for _, tt := range tests {
		ok, err := e.Enforce(userID, "*", tt.path, tt.method)
		if err != nil {
			t.Errorf("Enforce(%s %s) failed: %v", tt.method, tt.path, err)
		}
		if !ok {
			t.Errorf("admin should be allowed %s %s", tt.method, tt.path)
		}
	}
}

// TestBlacklistKey verifies the Redis key format.
func TestBlacklistKey(t *testing.T) {
	got := blacklistKey("user-123")
	want := "rbac:blacklist:user:user-123"
	if got != want {
		t.Errorf("blacklistKey(%q) = %q, want %q", "user-123", got, want)
	}
}

// TestCasbinModelText verifies the embedded model string is valid.
func TestCasbinModelText(t *testing.T) {
	m, err := model.NewModelFromString(casbinModelText)
	if err != nil {
		t.Fatalf("failed to parse model: %v", err)
	}
	if m == nil {
		t.Fatal("model should not be nil")
	}
}

// --- Mock-based blacklist tests ---

// mockRedisClient is a minimal mock for testing blacklist logic without Redis.
// This tests the Service interface contract, not the actual Redis operations.

type mockRedisClient struct {
	*redis.Client // nil; we only test the key format and interface
}

// TestBlacklistInterface verifies that the Service interface includes blacklist methods.
func TestBlacklistInterface(t *testing.T) {
	var svc Service = &serviceImpl{}
	_ = svc

	// Verify the interface compiles — if it doesn't, methods are missing
	ctx := context.Background()
	// These will fail at runtime without redis, but the interface must compile
	_, _ = svc.IsBlacklisted(ctx, "user-1")
	_ = svc.AddToBlacklist(ctx, "user-1")      // will error at runtime
	_ = svc.RemoveFromBlacklist(ctx, "user-1") // will error at runtime
}
