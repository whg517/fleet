package system

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent/enttest"
	"github.com/whg517/fleet/internal/store/ent"

	modernsqlite "modernc.org/sqlite"
)

func init() {
	sql.Register("sqlite3", &sqliteFKDriver{inner: &modernsqlite.Driver{}})
}

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

type fakeHealthChecker struct {
	dbErr    error
	redisErr error
}

func (f *fakeHealthChecker) PingDB(ctx context.Context) error  { return f.dbErr }
func (f *fakeHealthChecker) PingRedis(ctx context.Context) error { return f.redisErr }

func newTestService(t *testing.T) (*ServiceImpl, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, "sqlite3",
		fmt.Sprintf("file:%s?mode=memory&_fk=1&_pragma=foreign_keys(1)", t.Name()))
	dek, _ := hex.DecodeString("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	store := NewEntStore(client)
	svc := NewService(store, &fakeHealthChecker{}, dek, zap.NewNop())
	t.Cleanup(func() { _ = client.Close() })
	return svc, client
}

func TestSetSetting_CreateNonSensitive(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	setting, err := svc.Set(ctx, "", "argocd.url", SetSettingReq{
		Value: "https://argocd.example.com", Category: CategoryArgocd, Description: "ArgoCD server URL",
	})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if setting.Key != "argocd.url" {
		t.Errorf("expected key 'argocd.url', got %q", setting.Key)
	}
	if setting.Value != "https://argocd.example.com" {
		t.Errorf("expected value, got %q", setting.Value)
	}
	if setting.Encrypted {
		t.Error("expected non-encrypted")
	}
}

func TestSetSetting_CreateSensitive(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	setting, err := svc.Set(ctx, "", "argocd.token", SetSettingReq{
		Value: "super-secret-token", Category: CategoryArgocd,
	})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !setting.Encrypted {
		t.Error("expected encrypted")
	}
	if setting.Value != "***" {
		t.Errorf("expected masked value, got %q", setting.Value)
	}
}

func TestSetSetting_UpdateExisting(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	_, err := svc.Set(ctx, "", "harbor.url", SetSettingReq{Value: "https://harbor.old.com", Category: CategoryHarbor})
	if err != nil {
		t.Fatalf("initial Set: %v", err)
	}
	updated, err := svc.Set(ctx, "", "harbor.url", SetSettingReq{Value: "https://harbor.new.com", Category: CategoryHarbor})
	if err != nil {
		t.Fatalf("update Set: %v", err)
	}
	if updated.Value != "https://harbor.new.com" {
		t.Errorf("expected updated value, got %q", updated.Value)
	}
}

func TestSetSetting_EmptyKey(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Set(context.Background(), "", "  ", SetSettingReq{Value: "test"})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestSetSetting_EmptyValue(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Set(context.Background(), "", "some.key", SetSettingReq{Value: ""})
	if err == nil {
		t.Fatal("expected error for empty value")
	}
}

func TestGetSetting_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Get(context.Background(), "", "nonexistent.key")
	if err == nil {
		t.Fatal("expected error for non-existent key")
	}
}

func TestGetSetting_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	_, err := svc.Set(ctx, "", "git.repo_url", SetSettingReq{Value: "https://github.com/org/repo", Category: CategoryGit})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := svc.Get(ctx, "", "git.repo_url")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != "https://github.com/org/repo" {
		t.Errorf("expected value, got %q", got.Value)
	}
}

func TestGetSetting_SensitiveMasked(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	_, err := svc.Set(ctx, "", "git.token", SetSettingReq{Value: "secret-git-token", Category: CategoryGit})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := svc.Get(ctx, "", "git.token")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != "***" {
		t.Errorf("expected masked value, got %q", got.Value)
	}
}

func TestListSettings(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	settings := []struct {
		key, value string
		category   Category
	}{
		{"argocd.url", "https://argocd.example.com", CategoryArgocd},
		{"argocd.token", "secret-token", CategoryArgocd},
		{"harbor.url", "https://harbor.example.com", CategoryHarbor},
		{"harbor.password", "p@ssw0rd", CategoryHarbor},
		{"git.repo_url", "https://github.com/org/repo", CategoryGit},
	}
	for _, s := range settings {
		if _, err := svc.Set(ctx, "", s.key, SetSettingReq{Value: s.value, Category: s.category}); err != nil {
			t.Fatalf("Set %s: %v", s.key, err)
		}
	}
	all, err := svc.List(ctx, "", "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != len(settings) {
		t.Errorf("expected %d settings, got %d", len(settings), len(all))
	}
	argocdOnly, err := svc.List(ctx, "", "argocd")
	if err != nil {
		t.Fatalf("List argocd: %v", err)
	}
	if len(argocdOnly) != 2 {
		t.Errorf("expected 2 argocd settings, got %d", len(argocdOnly))
	}
	for _, s := range all {
		if s.Encrypted && s.Value != "***" {
			t.Errorf("expected masked value for key %q, got %q", s.Key, s.Value)
		}
	}
}

func TestDeleteSetting_Success(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	_, err := svc.Set(ctx, "", "general.setting", SetSettingReq{Value: "value", Category: CategoryGeneral})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := svc.Delete(ctx, "", "general.setting"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = svc.Get(ctx, "", "general.setting")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteSetting_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	err := svc.Delete(context.Background(), "", "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHealthCheck_AllOK(t *testing.T) {
	svc, _ := newTestService(t)
	result, err := svc.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("expected 'ok', got %q", result.Status)
	}
	if len(result.Checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(result.Checks))
	}
}

func TestHealthCheck_Degraded(t *testing.T) {
	svc, _ := newTestService(t)
	svc.health = &fakeHealthChecker{dbErr: fmt.Errorf("db unreachable")}
	result, err := svc.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if result.Status != "degraded" {
		t.Errorf("expected 'degraded', got %q", result.Status)
	}
}

func TestIsSensitiveKey(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"argocd.token", true},
		{"harbor.password", true},
		{"git.secret", true},
		{"some.credential", true},
		{"argocd.url", false},
		{"harbor.username", false},
		{"general.timeout", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isSensitiveKey(tt.key); got != tt.expected {
			t.Errorf("isSensitiveKey(%q) = %v, want %v", tt.key, got, tt.expected)
		}
	}
}
