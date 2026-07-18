package rbac

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ErrUserNotBlacklisted is returned when removing a user from the blacklist who is not on it.
var ErrUserNotBlacklisted = errors.New("user is not blacklisted")

// Default policies for each role.
// Domain "*" means the policy applies to all environments.
// For resource paths, we include both the base path (e.g. /api/v1/clusters)
// and the wildcard (e.g. /api/v1/clusters/*) to cover sub-resources.
var defaultPolicies = [][]string{
	// admin — full access
	{"p", "admin", "*", "*", "*"},

	// operator — cluster + deployment management
	{"p", "operator", "*", "/api/v1/clusters", "GET"},
	{"p", "operator", "*", "/api/v1/clusters/*", "GET"},
	{"p", "operator", "*", "/api/v1/clusters", "POST"},
	{"p", "operator", "*", "/api/v1/clusters/*", "PUT"},
	{"p", "operator", "*", "/api/v1/clusters/*", "DELETE"},
	{"p", "operator", "*", "/api/v1/clusters/*/test", "POST"},
	{"p", "operator", "*", "/api/v1/environments", "GET"},
	{"p", "operator", "*", "/api/v1/environments/*", "GET"},
	{"p", "operator", "*", "/api/v1/deployments", "POST"},
	{"p", "operator", "*", "/api/v1/deployments/*", "POST"},
	{"p", "operator", "*", "/api/v1/deployments/*/approve", "POST"},
	{"p", "operator", "*", "/api/v1/deployments/*/rollback", "POST"},

	// developer — read clusters, manage services, create deployments
	{"p", "developer", "*", "/api/v1/clusters", "GET"},
	{"p", "developer", "*", "/api/v1/clusters/*", "GET"},
	{"p", "developer", "*", "/api/v1/services", "GET"},
	{"p", "developer", "*", "/api/v1/services/*", "GET"},
	{"p", "developer", "*", "/api/v1/services", "POST"},
	{"p", "developer", "*", "/api/v1/services/*", "POST"},
	{"p", "developer", "*", "/api/v1/deployments", "GET"},
	{"p", "developer", "*", "/api/v1/deployments/*", "GET"},
	{"p", "developer", "*", "/api/v1/deployments", "POST"},
	{"p", "developer", "*", "/api/v1/deployments/*", "POST"},

	// viewer — read-only across all v1 endpoints
	{"p", "viewer", "*", "/api/v1/*", "GET"},

	// auditor — audit logs + cluster read
	{"p", "auditor", "*", "/api/v1/audit-logs", "GET"},
	{"p", "auditor", "*", "/api/v1/audit-logs/*", "GET"},
	{"p", "auditor", "*", "/api/v1/clusters", "GET"},
	{"p", "auditor", "*", "/api/v1/clusters/*", "GET"},
}

// RoleNames lists all defined roles in the system.
var RoleNames = []string{"admin", "operator", "developer", "viewer", "auditor"}

// RoleDescriptions maps role names to human-readable descriptions.
var RoleDescriptions = map[string]string{
	"admin":     "Full system access including user management",
	"operator":  "Cluster and deployment management",
	"developer": "Service management and deployment creation",
	"viewer":    "Read-only access to all resources",
	"auditor":   "Audit log access and cluster read",
}

// Service encapsulates the Casbin enforcer for RBAC operations.
type Service interface {
	// Enforce checks if subject (role) can perform act on obj in dom.
	Enforce(sub, dom, obj, act string) (bool, error)
	// AddRoleForUser assigns a role to a user in a domain.
	AddRoleForUser(userID, role, domain string) (bool, error)
	// DeleteRoleForUser removes a role from a user in a domain.
	DeleteRoleForUser(userID, role, domain string) (bool, error)
	// GetRolesForUser returns all roles assigned to a user.
	GetRolesForUser(userID string) ([]string, error)
	// GetUserPermissions returns the permission matrix for a user.
	GetUserPermissions(userID string) ([][]string, error)
	// ReloadPolicy reloads policies from the database.
	ReloadPolicy() error
	// IsBlacklisted checks if a user is in the blacklist.
	IsBlacklisted(ctx context.Context, userID string) (bool, error)
	// AddToBlacklist adds a user to the blacklist.
	AddToBlacklist(ctx context.Context, userID string) error
	// RemoveFromBlacklist removes a user from the blacklist.
	RemoveFromBlacklist(ctx context.Context, userID string) error
}

// serviceImpl is the concrete Casbin-backed RBAC service.
type serviceImpl struct {
	enforcer *casbin.SyncedEnforcer
	redis    *redis.Client
	mu       sync.Mutex
	logger   *zap.Logger
}

// NewService creates a new RBAC service backed by Casbin + GORM (PostgreSQL).
// It connects to the same PostgreSQL database used by Ent.
// On first run, it seeds the default role policies.
func NewService(dsn string, redisClient *redis.Client, logger *zap.Logger) (Service, error) {
	// Open GORM connection to the same PostgreSQL
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("rbac: open gorm: %w", err)
	}

	// Create Casbin adapter backed by PostgreSQL
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		return nil, fmt.Errorf("rbac: create gorm adapter: %w", err)
	}

	// Load model from embedded string
	m, err := model.NewModelFromString(casbinModelText)
	if err != nil {
		return nil, fmt.Errorf("rbac: load model: %w", err)
	}

	// Create synced enforcer (thread-safe)
	enforcer, err := casbin.NewSyncedEnforcer(m, adapter)
	if err != nil {
		return nil, fmt.Errorf("rbac: create enforcer: %w", err)
	}

	// Load existing policies from database
	if err := enforcer.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("rbac: load policy: %w", err)
	}

	svc := &serviceImpl{
		enforcer: enforcer,
		redis:    redisClient,
		logger:   logger,
	}

	// Seed default policies if none exist
	if err := svc.seedDefaultPolicies(); err != nil {
		return nil, fmt.Errorf("rbac: seed policies: %w", err)
	}

	logger.Info("rbac service initialized",
		zap.Int("policies", func() int { p, _ := enforcer.GetPolicy(); return len(p) }()),
	)

	return svc, nil
}

// Enforce checks if subject can perform act on obj in dom.
func (s *serviceImpl) Enforce(sub, dom, obj, act string) (bool, error) {
	return s.enforcer.Enforce(sub, dom, obj, act)
}

// AddRoleForUser assigns a role to a user in a domain.
func (s *serviceImpl) AddRoleForUser(userID, role, domain string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enforcer.AddRoleForUser(userID, role, domain)
}

// DeleteRoleForUser removes a role from a user in a domain.
func (s *serviceImpl) DeleteRoleForUser(userID, role, domain string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enforcer.DeleteRoleForUser(userID, role, domain)
}

// GetRolesForUser returns all roles assigned to a user across all domains.
func (s *serviceImpl) GetRolesForUser(userID string) ([]string, error) {
	// With the 3-arg g definition (user, role, domain), we need to pass domain.
	// Use "*" as the default domain.
	return s.enforcer.GetRolesForUser(userID, "*")
}

// GetUserPermissions returns the permission matrix for a user.
// It combines permissions from all roles the user has.
func (s *serviceImpl) GetUserPermissions(userID string) ([][]string, error) {
	roles, err := s.enforcer.GetRolesForUser(userID)
	if err != nil {
		return nil, err
	}

	var perms [][]string
	for _, role := range roles {
		rolePerms, err := s.enforcer.GetFilteredPolicy(0, role)
		if err != nil {
			continue
		}
		perms = append(perms, rolePerms...)
	}
	return perms, nil
}

// ReloadPolicy reloads all policies from the database.
func (s *serviceImpl) ReloadPolicy() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enforcer.LoadPolicy()
}

// --- Redis blacklist for instant permission revocation ---

// blacklistKey returns the Redis key for a user's blacklist entry.
func blacklistKey(userID string) string {
	return "rbac:blacklist:user:" + userID
}

// IsBlacklisted checks if a user is on the blacklist (instant permission revocation).
func (s *serviceImpl) IsBlacklisted(ctx context.Context, userID string) (bool, error) {
	if s.redis == nil {
		return false, nil
	}
	return s.redis.SIsMember(ctx, blacklistKey(userID), "disabled").Result()
}

// AddToBlacklist adds a user to the blacklist, instantly revoking all permissions.
func (s *serviceImpl) AddToBlacklist(ctx context.Context, userID string) error {
	if s.redis == nil {
		return fmt.Errorf("rbac: redis not available")
	}
	return s.redis.SAdd(ctx, blacklistKey(userID), "disabled").Err()
}

// RemoveFromBlacklist removes a user from the blacklist, restoring permissions.
func (s *serviceImpl) RemoveFromBlacklist(ctx context.Context, userID string) error {
	if s.redis == nil {
		return fmt.Errorf("rbac: redis not available")
	}
	n, err := s.redis.SRem(ctx, blacklistKey(userID), "disabled").Result()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotBlacklisted
	}
	return nil
}

// seedDefaultPolicies inserts default role policies if the policy table is empty.
func (s *serviceImpl) seedDefaultPolicies() error {
	policies, err := s.enforcer.GetPolicy()
	if err != nil {
		return fmt.Errorf("get policy: %w", err)
	}
	if len(policies) > 0 {
		return nil // Already seeded
	}

	s.logger.Info("seeding default RBAC policies")
	for _, p := range defaultPolicies {
		// p = [ptype, sub, dom, obj, act]
		_, err := s.enforcer.AddPolicy(p[1], p[2], p[3], p[4])
		if err != nil {
			return fmt.Errorf("add policy %v: %w", p, err)
		}
	}

	return nil
}

// casbinModelText is the embedded Casbin model definition.
const casbinModelText = `
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _
g2 = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.dom) && keyMatch2(r.obj, p.obj) && keyMatch2(r.act, p.act) && r.dom == p.dom
`
