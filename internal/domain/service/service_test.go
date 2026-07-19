package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent/enttest"
	"github.com/whg517/fleet/internal/store/ent"

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

func newTestService(t *testing.T) (*ServiceImpl, *ent.Client) {
	t.Helper()

	client := enttest.Open(t, "sqlite3", fmt.Sprintf("file:%s?mode=memory&_fk=1&_pragma=foreign_keys(1)", t.Name()))
	store := NewEntStore(client)
	svc := NewService(store, zap.NewNop())

	t.Cleanup(func() { _ = client.Close() })

	return svc, client
}

func TestCreateService_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	s, err := svc.Create(ctx, CreateServiceReq{
		Name:        "payment-api",
		Team:        "payments",
		Description: "Payment processing service",
		Labels:      map[string]string{"lang": "go", "tier": "critical"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if s.ID == "" {
		t.Error("expected non-empty ID")
	}
	if s.Name != "payment-api" {
		t.Errorf("Name: got %q, want %q", s.Name, "payment-api")
	}
	if s.Team != "payments" {
		t.Errorf("Team: got %q, want %q", s.Team, "payments")
	}
	if s.Status != "active" {
		t.Errorf("Status: got %q, want %q", s.Status, "active")
	}
	if s.Labels["lang"] != "go" {
		t.Errorf("Labels[lang]: got %q", s.Labels["lang"])
	}
}

func TestCreateService_ValidationErrors(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateServiceReq{Name: ""})
	if err == nil {
		t.Fatal("expected validation error for empty name, got nil")
	}
}

func TestCreateService_DuplicateName(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	// Create an organization first to satisfy FK constraint.
	org, err := client.Organization.Create().
		SetID("org-1").
		SetName("test-org").
		SetSlug("test-org").
		Save(ctx)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	_, err = svc.Create(ctx, CreateServiceReq{OrgID: org.ID, Name: "dup-service"})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err = svc.Create(ctx, CreateServiceReq{OrgID: org.ID, Name: "dup-service"})
	if err != ErrServiceAlreadyExists {
		t.Errorf("Create duplicate: got %v, want ErrServiceAlreadyExists", err)
	}
}

func TestGetService_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.Get(ctx, "nonexistent")
	if err != ErrServiceNotFound {
		t.Errorf("Get: got %v, want ErrServiceNotFound", err)
	}
}

func TestGetService_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	created, _ := svc.Create(ctx, CreateServiceReq{Name: "test-svc"})

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, created.ID)
	}
	if got.Name != "test-svc" {
		t.Errorf("Name mismatch: got %q, want %q", got.Name, "test-svc")
	}
}

func TestListServices_Pagination(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := svc.Create(ctx, CreateServiceReq{
			Name: fmt.Sprintf("svc-%d", i),
		})
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// Page 1, size 2
	result, err := svc.List(ctx, ServiceFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Services) != 2 {
		t.Errorf("Page 1: got %d services, want 2", len(result.Services))
	}
	if result.Total != 5 {
		t.Errorf("Total: got %d, want 5", result.Total)
	}

	// Page 3, size 2
	result, err = svc.List(ctx, ServiceFilter{Page: 3, PageSize: 2})
	if err != nil {
		t.Fatalf("List page 3: %v", err)
	}
	if len(result.Services) != 1 {
		t.Errorf("Page 3: got %d services, want 1", len(result.Services))
	}
}

func TestListServices_NameSearch(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateServiceReq{Name: "payment-api"})
	_, _ = svc.Create(ctx, CreateServiceReq{Name: "payment-worker"})
	_, _ = svc.Create(ctx, CreateServiceReq{Name: "notification-api"})

	result, err := svc.List(ctx, ServiceFilter{Page: 1, PageSize: 10, Name: "payment"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Services) != 2 {
		t.Fatalf("got %d services, want 2", len(result.Services))
	}
	for _, s := range result.Services {
		if !contains(s.Name, "payment") {
			t.Errorf("Name %q does not contain 'payment'", s.Name)
		}
	}
}

func TestListServices_TeamFilter(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateServiceReq{Name: "a", Team: "infra"})
	_, _ = svc.Create(ctx, CreateServiceReq{Name: "b", Team: "app"})

	result, err := svc.List(ctx, ServiceFilter{Page: 1, PageSize: 10, Team: "infra"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Services) != 1 {
		t.Fatalf("got %d services, want 1", len(result.Services))
	}
	if result.Services[0].Team != "infra" {
		t.Errorf("Team: got %q, want %q", result.Services[0].Team, "infra")
	}
}

func TestListServices_LabelFilter(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, _ = svc.Create(ctx, CreateServiceReq{
		Name:   "a",
		Labels: map[string]string{"tier": "critical"},
	})
	_, _ = svc.Create(ctx, CreateServiceReq{
		Name:   "b",
		Labels: map[string]string{"tier": "normal"},
	})

	result, err := svc.List(ctx, ServiceFilter{
		Page:     1,
		PageSize: 10,
		Labels:   map[string]string{"tier": "critical"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Services) != 1 {
		t.Fatalf("got %d services, want 1", len(result.Services))
	}
	if result.Services[0].Name != "a" {
		t.Errorf("Name: got %q, want %q", result.Services[0].Name, "a")
	}
}

func TestListServices_LabelFilterWithPagination(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, _ = svc.Create(ctx, CreateServiceReq{
			Name:   fmt.Sprintf("infra-%d", i),
			Labels: map[string]string{"team": "infra"},
		})
	}
	for i := 0; i < 2; i++ {
		_, _ = svc.Create(ctx, CreateServiceReq{
			Name:   fmt.Sprintf("app-%d", i),
			Labels: map[string]string{"team": "app"},
		})
	}

	result, err := svc.List(ctx, ServiceFilter{
		Page:     1,
		PageSize: 2,
		Labels:   map[string]string{"team": "infra"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Services) != 2 {
		t.Errorf("Page 1: got %d, want 2", len(result.Services))
	}
	if result.Total != 3 {
		t.Errorf("Total: got %d, want 3", result.Total)
	}
}

func TestUpdateService_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	s, _ := svc.Create(ctx, CreateServiceReq{Name: "orig"})

	newName := "updated"
	newTeam := "platform"
	updated, err := svc.Update(ctx, s.ID, UpdateServiceReq{Name: &newName, Team: &newTeam})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "updated" {
		t.Errorf("Name: got %q, want %q", updated.Name, "updated")
	}
	if updated.Team != "platform" {
		t.Errorf("Team: got %q, want %q", updated.Team, "platform")
	}
}

func TestUpdateService_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	name := "x"
	_, err := svc.Update(ctx, "nonexistent", UpdateServiceReq{Name: &name})
	if err != ErrServiceNotFound {
		t.Errorf("Update: got %v, want ErrServiceNotFound", err)
	}
}

func TestDeleteService_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	s, _ := svc.Create(ctx, CreateServiceReq{Name: "to-archive"})

	if err := svc.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify the service still exists but is archived
	got, err := svc.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get after archive: %v", err)
	}
	if got.Status != "archived" {
		t.Errorf("Status: got %q, want %q", got.Status, "archived")
	}
}

func TestDeleteService_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	if err := svc.Delete(ctx, "nonexistent"); err != ErrServiceNotFound {
		t.Errorf("Delete: got %v, want ErrServiceNotFound", err)
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

func TestMatchLabels(t *testing.T) {
	tests := []struct {
		name      string
		svc       map[string]string
		filter    map[string]string
		wantMatch bool
	}{
		{"no filter", map[string]string{"a": "b"}, nil, true},
		{"exact match", map[string]string{"a": "b"}, map[string]string{"a": "b"}, true},
		{"no match", map[string]string{"a": "b"}, map[string]string{"a": "c"}, false},
		{"missing key", map[string]string{"a": "b"}, map[string]string{"c": "d"}, false},
		{"multi match", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "2"}, true},
		{"multi partial", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "3"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchLabels(tt.svc, tt.filter); got != tt.wantMatch {
				t.Errorf("matchLabels: got %v, want %v", got, tt.wantMatch)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
