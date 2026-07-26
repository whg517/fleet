package deployment

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent"
	"github.com/whg517/fleet/internal/store/ent/enttest"
	enttemplateversion "github.com/whg517/fleet/internal/store/ent/templateversion"

	modernsqlite "modernc.org/sqlite"
)

func init() {
	// Register modernc.org/sqlite under the "sqlite3" name that ent expects.
	sql.Register("sqlite3", &sqliteFKDriver{inner: &modernsqlite.Driver{}})
}

// sqliteFKDriver wraps modernc.org/sqlite to enable foreign keys by default.
type sqliteFKDriver struct {
	inner driver.Driver
}

func (d *sqliteFKDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	if execCtx, ok := conn.(driver.ExecerContext); ok {
		_, _ = execCtx.ExecContext(context.Background(), "PRAGMA foreign_keys = ON", nil)
	}
	return conn, nil
}

// mockArgoCD is a mock implementation of the ArgoCDClient interface for testing.
type mockArgoCD struct {
	createErr  error
	getStatus  *ArgoCDAppStatus
	getErr     error
	syncErr    error
	rollbackErr error
	deleteErr  error

	createCalls  []ArgoCDAppReq
	syncCalls    []string
	rollbackCalls []rollbackCall
	deleteCalls  []string
}

type rollbackCall struct {
	name     string
	revision string
}

func (m *mockArgoCD) CreateApplication(_ context.Context, req ArgoCDAppReq) error {
	m.createCalls = append(m.createCalls, req)
	return m.createErr
}

func (m *mockArgoCD) GetApplication(_ context.Context, name string) (*ArgoCDAppStatus, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.getStatus != nil {
		return m.getStatus, nil
	}
	return &ArgoCDAppStatus{SyncStatus: "Synced", HealthStatus: "Healthy"}, nil
}

func (m *mockArgoCD) SyncApplication(_ context.Context, name string) error {
	m.syncCalls = append(m.syncCalls, name)
	return m.syncErr
}

func (m *mockArgoCD) RollbackApplication(_ context.Context, name, revision string) error {
	m.rollbackCalls = append(m.rollbackCalls, rollbackCall{name: name, revision: revision})
	return m.rollbackErr
}

func (m *mockArgoCD) DeleteApplication(_ context.Context, name string) error {
	m.deleteCalls = append(m.deleteCalls, name)
	return m.deleteErr
}

// newTestService creates a test deployment service with SQLite in-memory DB.
func newTestService(t *testing.T) (*ServiceImpl, *ent.Client, *mockArgoCD) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", fmt.Sprintf("file:%s?mode=memory&_fk=1&_pragma=foreign_keys(1)", t.Name()))
	store := NewEntStore(client)
	lookup := NewLookupEntStore(client)
	argocd := &mockArgoCD{}
	svc := NewService(store, lookup, argocd, zap.NewNop())

	t.Cleanup(func() { _ = client.Close() })

	return svc, client, argocd
}

// seedPrerequisites creates a Service, Environment, Cluster, and TemplateVersion
// in the test DB for deployment creation to succeed.
func seedPrerequisites(t *testing.T, client *ent.Client) (svcID, envID, tvID string) {
	t.Helper()
	ctx := context.Background()

	cluster, err := client.Cluster.Create().
		SetID("cluster-1").
		SetName("test-cluster").
		SetAPIServer("https://10.0.0.1:6443").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed cluster: %v", err)
	}

	svc, err := client.Service.Create().
		SetID("svc-1").
		SetName("payment-api").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed service: %v", err)
	}

	env, err := client.Environment.Create().
		SetID("env-1").
		SetName("dev").
		SetClusterID(cluster.ID).
		SetNamespacePattern("default").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed environment: %v", err)
	}

	tmpl, err := client.Template.Create().
		SetID("tmpl-1").
		SetName("deploy-template").
		SetType("deploy_k8s").
		SetSource("platform").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed template: %v", err)
	}

	tv, err := client.TemplateVersion.Create().
		SetID("tv-1").
		SetTemplateID(tmpl.ID).
		SetVersion("1.0.0").
		SetStatus(enttemplateversion.StatusActive).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed template version: %v", err)
	}

	return svc.ID, env.ID, tv.ID
}

// --- Tests ---

func TestCreateDeployment_Success(t *testing.T) {
	svc, client, argocd := newTestService(t)
	ctx := context.Background()

	svcID, envID, tvID := seedPrerequisites(t, client)

	d, err := svc.Create(ctx, CreateDeploymentReq{
		ServiceID:         svcID,
		EnvironmentID:     envID,
		TemplateVersionID: tvID,
		Version:           "v1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if d.ID == "" {
		t.Error("expected non-empty ID")
	}
	if d.ServiceID != svcID {
		t.Errorf("ServiceID: got %q, want %q", d.ServiceID, svcID)
	}
	if d.Version != "v1.0.0" {
		t.Errorf("Version: got %q, want %q", d.Version, "v1.0.0")
	}
	if d.Status != StatusDeploying {
		t.Errorf("Status: got %q, want %q", d.Status, StatusDeploying)
	}
	if d.ArgocdAppName == "" {
		t.Error("expected non-empty ArgocdAppName")
	}
	if len(argocd.createCalls) != 1 {
		t.Errorf("expected 1 CreateApplication call, got %d", len(argocd.createCalls))
	}
}

func TestCreateDeployment_ValidationErrors(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	tests := []struct {
		name string
		req  CreateDeploymentReq
	}{
		{"empty service_id", CreateDeploymentReq{EnvironmentID: "env-1", TemplateVersionID: "tv-1", Version: "v1"}},
		{"empty environment_id", CreateDeploymentReq{ServiceID: "svc-1", TemplateVersionID: "tv-1", Version: "v1"}},
		{"empty template_version_id", CreateDeploymentReq{ServiceID: "svc-1", EnvironmentID: "env-1", Version: "v1"}},
		{"empty version", CreateDeploymentReq{ServiceID: "svc-1", EnvironmentID: "env-1", TemplateVersionID: "tv-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tt.req)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestCreateDeployment_ServiceNotFound(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	_, envID, tvID := seedPrerequisites(t, client)

	_, err := svc.Create(ctx, CreateDeploymentReq{
		ServiceID:         "nonexistent-service",
		EnvironmentID:     envID,
		TemplateVersionID: tvID,
		Version:           "v1.0.0",
	})
	if err == nil {
		t.Fatal("expected error for non-existent service, got nil")
	}
}

func TestCreateDeployment_ArgoCDClientError(t *testing.T) {
	svc, client, argocd := newTestService(t)
	ctx := context.Background()

	svcID, envID, tvID := seedPrerequisites(t, client)
	argocd.createErr = fmt.Errorf("connection refused")

	_, err := svc.Create(ctx, CreateDeploymentReq{
		ServiceID:         svcID,
		EnvironmentID:     envID,
		TemplateVersionID: tvID,
		Version:           "v1.0.0",
	})
	if !errors.Is(err, ErrArgoCDUnavailable) {
		t.Errorf("expected ErrArgoCDUnavailable, got %v", err)
	}
}

func TestGetDeployment_Success(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, tvID := seedPrerequisites(t, client)

	created, err := svc.Create(ctx, CreateDeploymentReq{
		ServiceID:         svcID,
		EnvironmentID:     envID,
		TemplateVersionID: tvID,
		Version:           "v1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID: got %q, want %q", got.ID, created.ID)
	}
	if got.Version != "v1.0.0" {
		t.Errorf("Version: got %q, want %q", got.Version, "v1.0.0")
	}
}

func TestGetDeployment_NotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Get(ctx, "nonexistent")
	if err != ErrDeploymentNotFound {
		t.Errorf("Get: got %v, want ErrDeploymentNotFound", err)
	}
}

func TestListDeployments_Pagination(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, tvID := seedPrerequisites(t, client)

	// Create multiple deployments for the same service+env
	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, CreateDeploymentReq{
			ServiceID:         svcID,
			EnvironmentID:     envID,
			TemplateVersionID: tvID,
			Version:           fmt.Sprintf("v1.0.%d", i),
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// Page 1, size 2
	result, err := svc.List(ctx, DeploymentFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Deployments) != 2 {
		t.Errorf("Page 1: got %d, want 2", len(result.Deployments))
	}
	if result.Total != 5 {
		t.Errorf("Total: got %d, want 5", result.Total)
	}

	// Page 3, size 2
	result, err = svc.List(ctx, DeploymentFilter{Page: 3, PageSize: 2})
	if err != nil {
		t.Fatalf("List page 3: %v", err)
	}
	if len(result.Deployments) != 1 {
		t.Errorf("Page 3: got %d, want 1", len(result.Deployments))
	}
}

func TestListDeployments_StatusFilter(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, tvID := seedPrerequisites(t, client)

	// Create deployments
	for i := 0; i < 3; i++ {
		_, err := svc.Create(ctx, CreateDeploymentReq{
			ServiceID:         svcID,
			EnvironmentID:     envID,
			TemplateVersionID: tvID,
			Version:           fmt.Sprintf("v2.%d.0", i),
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// All should be in "deploying" status
	result, err := svc.List(ctx, DeploymentFilter{Page: 1, PageSize: 10, Status: "deploying"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Deployments) != 3 {
		t.Fatalf("got %d deployments, want 3", len(result.Deployments))
	}
	for _, d := range result.Deployments {
		if d.Status != StatusDeploying {
			t.Errorf("Status: got %q, want %q", d.Status, StatusDeploying)
		}
	}

	// Filter for healthy should return 0
	result, err = svc.List(ctx, DeploymentFilter{Page: 1, PageSize: 10, Status: "healthy"})
	if err != nil {
		t.Fatalf("List healthy: %v", err)
	}
	if len(result.Deployments) != 0 {
		t.Errorf("got %d healthy deployments, want 0", len(result.Deployments))
	}
}

func TestRollback_Success(t *testing.T) {
	svc, client, argocd := newTestService(t)
	ctx := context.Background()

	svcID, envID, tvID := seedPrerequisites(t, client)

	// Create first deployment (will be the "current" one)
	d1, err := svc.Create(ctx, CreateDeploymentReq{
		ServiceID:         svcID,
		EnvironmentID:     envID,
		TemplateVersionID: tvID,
		Version:           "v2.0.0",
	})
	if err != nil {
		t.Fatalf("Create d1: %v", err)
	}

	// Manually set d1 to healthy (simulating it completed successfully)
	upd := svc.store.UpdateDeploymentOne(d1.ID).SetStatus("healthy")
	_, err = svc.store.SaveDeploymentUpdate(ctx, upd)
	if err != nil {
		t.Fatalf("set d1 healthy: %v", err)
	}

	// Create second deployment (current, non-healthy)
	d2, err := svc.Create(ctx, CreateDeploymentReq{
		ServiceID:         svcID,
		EnvironmentID:     envID,
		TemplateVersionID: tvID,
		Version:           "v3.0.0",
	})
	if err != nil {
		t.Fatalf("Create d2: %v", err)
	}

	// Rollback d2 to d1's version
	rolled, err := svc.Rollback(ctx, d2.ID)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if rolled.Status != StatusDeploying {
		t.Errorf("Status after rollback: got %q, want %q", rolled.Status, StatusDeploying)
	}

	// Verify Argo CD rollback was called with d1's version
	if len(argocd.rollbackCalls) != 1 {
		t.Fatalf("expected 1 rollback call, got %d", len(argocd.rollbackCalls))
	}
	if argocd.rollbackCalls[0].revision != "v2.0.0" {
		t.Errorf("rollback revision: got %q, want %q", argocd.rollbackCalls[0].revision, "v2.0.0")
	}
}

func TestRollback_NoHealthyVersion(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, tvID := seedPrerequisites(t, client)

	// Create a single deployment (no previous healthy version)
	d, err := svc.Create(ctx, CreateDeploymentReq{
		ServiceID:         svcID,
		EnvironmentID:     envID,
		TemplateVersionID: tvID,
		Version:           "v1.0.0",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.Rollback(ctx, d.ID)
	if err != ErrNoHealthyVersion {
		t.Errorf("Rollback: got %v, want ErrNoHealthyVersion", err)
	}
}

func TestNormalizePage(t *testing.T) {
	tests := []struct {
		page, pageSize, wantPage, wantPageSize int
	}{
		{0, 0, 1, 20},
		{-1, -5, 1, 20},
		{1, 10, 1, 10},
		{5, 200, 5, 100},
		{3, 50, 3, 50},
	}

	for _, tt := range tests {
		page, pageSize := normalizePage(tt.page, tt.pageSize)
		if page != tt.wantPage || pageSize != tt.wantPageSize {
			t.Errorf("normalizePage(%d, %d): got (%d, %d), want (%d, %d)",
				tt.page, tt.pageSize, page, pageSize, tt.wantPage, tt.wantPageSize)
		}
	}
}

func TestMapArgocdStatus(t *testing.T) {
	tests := []struct {
		sync, health, want string
	}{
		{"Synced", "Healthy", string(StatusHealthy)},
		{"OutOfSync", "Healthy", string(StatusHealthy)},
		{"Synced", "Degraded", string(StatusDegraded)},
		{"Failed", "Degraded", string(StatusFailed)},
		{"OutOfSync", "Progressing", string(StatusDeploying)},
		{"Pending", "", string(StatusDeploying)},
	}

	for _, tt := range tests {
		got := mapArgocdStatus(tt.sync, tt.health)
		if got != tt.want {
			t.Errorf("mapArgocdStatus(%q, %q): got %q, want %q", tt.sync, tt.health, got, tt.want)
		}
	}
}
