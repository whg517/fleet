package config

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent"
	entdeployment "github.com/whg517/fleet/internal/store/ent/deployment"
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
	updateErr error

	updateCalls []updateCall
}

type updateCall struct {
	name   string
	values map[string]any
}

func (m *mockArgoCD) UpdateApplication(_ context.Context, name string, values map[string]any) error {
	m.updateCalls = append(m.updateCalls, updateCall{name: name, values: values})
	return m.updateErr
}

// valuesEqual compares a JSON-deserialized value with an expected Go value.
// JSON round-trip may convert int to float64 or int64.
func valuesEqual(got, want any) bool {
	switch v := got.(type) {
	case float64:
		return v == float64(toFloat(want))
	case int64:
		return v == int64(toFloat(want))
	case int:
		return v == int(toFloat(want))
	default:
		return reflect.DeepEqual(got, want)
	}
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

// newTestService creates a test config service with SQLite in-memory DB.
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

// seedPrerequisites creates a Service, Environment, Cluster, Template, TemplateVersion,
// and a Deployment so that config operations have valid references.
func seedPrerequisites(t *testing.T, client *ent.Client) (svcID, envID, deploymentID string) {
	t.Helper()
	ctx := context.Background()

	cl, err := client.Cluster.Create().
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
		SetClusterID(cl.ID).
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
		SetRepo("https://charts.example.com").
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

	dep, err := client.Deployment.Create().
		SetID("dep-1").
		SetServiceID(svc.ID).
		SetEnvironmentID(env.ID).
		SetClusterID(cl.ID).
		SetTemplateVersionID(tv.ID).
		SetVersion("v1.0.0").
		SetStatus(entdeployment.StatusHealthy).
		SetArgocdAppName("fleet-svc-1-env-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed deployment: %v", err)
	}

	return svc.ID, env.ID, dep.ID
}

// --- Tests ---

func TestUpdateValues_Success(t *testing.T) {
	svc, client, argocd := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client)

	newValues := map[string]any{
		"replicas": 3,
		"image": map[string]any{
			"tag": "v2.0.0",
		},
	}

	snap, err := svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
		Values:       newValues,
		ChangedBy:    "user-1",
		ChangeReason: "scale up replicas",
	})
	if err != nil {
		t.Fatalf("UpdateValues: %v", err)
	}

	if snap.ID == "" {
		t.Error("expected non-empty ID")
	}
	if snap.ServiceID != svcID {
		t.Errorf("ServiceID: got %q, want %q", snap.ServiceID, svcID)
	}
	if !valuesEqual(snap.Values["replicas"], 3) {
		t.Errorf("Values replicas: got %v (%T), want 3", snap.Values["replicas"], snap.Values["replicas"])
	}
	if snap.ChangedBy != "user-1" {
		t.Errorf("ChangedBy: got %q, want %q", snap.ChangedBy, "user-1")
	}

	// Verify Argo CD was called
	if len(argocd.updateCalls) != 1 {
		t.Fatalf("expected 1 UpdateApplication call, got %d", len(argocd.updateCalls))
	}
	if argocd.updateCalls[0].name != "fleet-svc-1-env-1" {
		t.Errorf("ArgoCD app name: got %q, want %q", argocd.updateCalls[0].name, "fleet-svc-1-env-1")
	}
}

func TestUpdateValues_ServiceNotFound(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	_, envID, _ := seedPrerequisites(t, client)

	_, err := svc.UpdateValues(ctx, "nonexistent-service", envID, UpdateValuesReq{
		Values: map[string]any{"key": "val"},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestUpdateValues_ArchivedService(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client)

	// Archive the service
	_, err := client.Service.UpdateOneID(svcID).SetStatus("archived").Save(ctx)
	if err != nil {
		t.Fatalf("archive service: %v", err)
	}

	_, err = svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
		Values: map[string]any{"key": "val"},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for archived service, got %v", err)
	}
}

func TestUpdateValues_ArgoCDClientError(t *testing.T) {
	svc, client, argocd := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client)
	argocd.updateErr = fmt.Errorf("connection refused")

	_, err := svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
		Values: map[string]any{"key": "val"},
	})
	if !errors.Is(err, ErrArgoCDUnavailable) {
		t.Errorf("expected ErrArgoCDUnavailable, got %v", err)
	}
}

func TestUpdateValues_PreviousValuesRecorded(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client)

	// First update
	firstValues := map[string]any{"replicas": 1}
	_, err := svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
		Values: firstValues,
	})
	if err != nil {
		t.Fatalf("first UpdateValues: %v", err)
	}

	// Second update — should record previous values
	secondValues := map[string]any{"replicas": 5}
	snap2, err := svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
		Values: secondValues,
	})
	if err != nil {
		t.Fatalf("second UpdateValues: %v", err)
	}

	if snap2.PreviousValues == nil {
		t.Fatal("expected non-nil PreviousValues on second update")
	}
	if !valuesEqual(snap2.PreviousValues["replicas"], 1) {
		t.Errorf("PreviousValues replicas: got %v (%T), want 1", snap2.PreviousValues["replicas"], snap2.PreviousValues["replicas"])
	}
}

func TestUpdateValues_NoActiveDeployment(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	// Create service and environment but NO deployment
	cl, err := client.Cluster.Create().
		SetID("cluster-2").
		SetName("cluster-2").
		SetAPIServer("https://10.0.0.2:6443").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed cluster: %v", err)
	}

	s := client.Service.Create().
		SetID("svc-2").
		SetName("svc-2").
		SaveX(ctx)

	e := client.Environment.Create().
		SetID("env-2").
		SetName("test").
		SetClusterID(cl.ID).
		SetNamespacePattern("default").
		SaveX(ctx)

	_ = s
	_ = e

	_, err = svc.UpdateValues(ctx, "svc-2", "env-2", UpdateValuesReq{
		Values: map[string]any{"key": "val"},
	})
	if !errors.Is(err, ErrNoActiveDeployment) {
		t.Errorf("expected ErrNoActiveDeployment, got %v", err)
	}
}

func TestGetValues_Success(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client)

	// Create a config snapshot first
	values := map[string]any{"replicas": 3, "image": "nginx:1.21"}
	_, err := svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
		Values: values,
	})
	if err != nil {
		t.Fatalf("UpdateValues: %v", err)
	}

	got, err := svc.GetValues(ctx, svcID, envID)
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}

	if !valuesEqual(got["replicas"], 3) {
		t.Errorf("replicas: got %v (%T), want 3", got["replicas"], got["replicas"])
	}
	if got["image"] != "nginx:1.21" {
		t.Errorf("image: got %v, want nginx:1.21", got["image"])
	}
}

func TestGetValues_NoHistory(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client)

	got, err := svc.GetValues(ctx, svcID, envID)
	if err != nil {
		t.Fatalf("GetValues: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestListHistory_Pagination(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client)

	// Create 5 config snapshots
	for i := 0; i < 5; i++ {
		_, err := svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
			Values: map[string]any{"version": i},
		})
		if err != nil {
			t.Fatalf("UpdateValues %d: %v", i, err)
		}
	}

	// Page 1, size 2
	result, err := svc.ListHistory(ctx, svcID, envID, 1, 2)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(result.Snapshots) != 2 {
		t.Errorf("Page 1: got %d snapshots, want 2", len(result.Snapshots))
	}
	if result.Total != 5 {
		t.Errorf("Total: got %d, want 5", result.Total)
	}

	// Page 3, size 2
	result, err = svc.ListHistory(ctx, svcID, envID, 3, 2)
	if err != nil {
		t.Fatalf("ListHistory page 3: %v", err)
	}
	if len(result.Snapshots) != 1 {
		t.Errorf("Page 3: got %d snapshots, want 1", len(result.Snapshots))
	}
}

func TestDiff_Success(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client)

	// Create first snapshot
	snap1, err := svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
		Values: map[string]any{
			"replicas": 1,
			"image":    "nginx:1.20",
			"removed":  "old-value",
		},
	})
	if err != nil {
		t.Fatalf("UpdateValues 1: %v", err)
	}

	// Create second snapshot
	snap2, err := svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
		Values: map[string]any{
			"replicas": 3,
			"image":    "nginx:1.20",
			"added":    "new-value",
		},
	})
	if err != nil {
		t.Fatalf("UpdateValues 2: %v", err)
	}

	// Diff from snap1 to snap2
	diff, err := svc.Diff(ctx, svcID, envID, snap1.ID, snap2.ID)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if diff.FromSnapshotID != snap1.ID {
		t.Errorf("FromSnapshotID: got %q, want %q", diff.FromSnapshotID, snap1.ID)
	}
	if diff.ToSnapshotID != snap2.ID {
		t.Errorf("ToSnapshotID: got %q, want %q", diff.ToSnapshotID, snap2.ID)
	}

	// Verify diff entries
	// Expected: replicas changed, "removed" removed, "added" added
	entryMap := make(map[string]ConfigDiffEntry)
	for _, e := range diff.Entries {
		entryMap[e.Path] = e
	}

	if entry, ok := entryMap["replicas"]; !ok || entry.Type != "changed" {
		t.Errorf("expected 'replicas' changed, got %v", entry)
	}
	if entry, ok := entryMap["removed"]; !ok || entry.Type != "removed" {
		t.Errorf("expected 'removed' removed, got %v", entry)
	}
	if entry, ok := entryMap["added"]; !ok || entry.Type != "added" {
		t.Errorf("expected 'added' added, got %v", entry)
	}

	// "image" should not be in the diff since it didn't change
	if _, ok := entryMap["image"]; ok {
		t.Error("unexpected diff entry for 'image' which didn't change")
	}
}

func TestDiff_ToLatest(t *testing.T) {
	svc, client, _ := newTestService(t)
	ctx := context.Background()

	svcID, envID, _ := seedPrerequisites(t, client)

	// Create two snapshots
	snap1, err := svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
		Values: map[string]any{"key": "v1"},
	})
	if err != nil {
		t.Fatalf("UpdateValues 1: %v", err)
	}

	_, err = svc.UpdateValues(ctx, svcID, envID, UpdateValuesReq{
		Values: map[string]any{"key": "v2"},
	})
	if err != nil {
		t.Fatalf("UpdateValues 2: %v", err)
	}

	// Diff from snap1 to latest (toVer empty)
	diff, err := svc.Diff(ctx, svcID, envID, snap1.ID, "")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if len(diff.Entries) == 0 {
		t.Error("expected diff entries, got none")
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
