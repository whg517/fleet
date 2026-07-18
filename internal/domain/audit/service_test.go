package audit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"
	"github.com/whg517/fleet/internal/store/ent"
	"github.com/whg517/fleet/internal/store/ent/enttest"
	"github.com/whg517/fleet/internal/store/ent/auditlog"
)

func newTestClient(t *testing.T) *ent.Client {
	t.Helper()
	// Use a unique in-memory DB per test to avoid shared cache contention between parallel tests
	name := strings.ReplaceAll(t.Name(), "/", "_")
	name = strings.ReplaceAll(name, " ", "_")
	dsn := "file:" + name + "?mode=memory&cache=private&_fk=1"
	c := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func newTestService(t *testing.T) (Service, *ent.Client) {
	c := newTestClient(t)
	svc := NewService(c, zap.NewNop())
	return svc, c
}

func TestService_Record_CreatesEntry(t *testing.T) {
	t.Parallel()
	svc, c := newTestService(t)
	ctx := context.Background()

	err := svc.Record(ctx, Record{
		UserID:       "user-1",
		Action:       "create",
		ResourceType: "clusters",
		ResourceID:   "res-1",
		Detail:       map[string]any{"name": "my-cluster"},
		IP:           "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	count, err := c.AuditLog.Query().Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 audit log, got %d", count)
	}
}

func TestService_Record_GenesisHash(t *testing.T) {
	t.Parallel()
	svc, c := newTestService(t)
	ctx := context.Background()

	err := svc.Record(ctx, Record{
		Action:       "create",
		ResourceType: "clusters",
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	entry, err := c.AuditLog.Query().First(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if entry.PrevHash != GenesisHash() {
		t.Errorf("first entry should have genesis hash, got %s", entry.PrevHash)
	}
}

func TestService_Record_ChainLinkage(t *testing.T) {
	t.Parallel()
	svc, c := newTestService(t)
	ctx := context.Background()

	// Record 3 entries
	for i := 0; i < 3; i++ {
		err := svc.Record(ctx, Record{
			UserID:       "user-1",
			Action:       "create",
			ResourceType: "clusters",
			ResourceID:   uuid.New().String(),
			Detail:       map[string]any{"index": i},
			IP:           "127.0.0.1",
		})
		if err != nil {
			t.Fatalf("Record %d failed: %v", i, err)
		}
	}

	logs, err := c.AuditLog.Query().
		Order(auditlog.ByCreatedAt()).
		All(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}

	// First entry: genesis hash
	if logs[0].PrevHash != GenesisHash() {
		t.Errorf("entry 0 should have genesis hash")
	}

	// Second entry: hash of first
	r0 := ToChainRecord(
		logs[0].ID, logs[0].UserID, logs[0].Action,
		logs[0].ResourceType, logs[0].ResourceID, logs[0].IP,
		logs[0].PrevHash, logs[0].Detail, logs[0].CreatedAt,
	)
	expectedHash1 := ComputeHash(r0)
	if logs[1].PrevHash != expectedHash1 {
		t.Errorf("entry 1 prev_hash mismatch: expected %s, got %s",
			expectedHash1, logs[1].PrevHash)
	}

	// Third entry: hash of second
	r1 := ToChainRecord(
		logs[1].ID, logs[1].UserID, logs[1].Action,
		logs[1].ResourceType, logs[1].ResourceID, logs[1].IP,
		logs[1].PrevHash, logs[1].Detail, logs[1].CreatedAt,
	)
	expectedHash2 := ComputeHash(r1)
	if logs[2].PrevHash != expectedHash2 {
		t.Errorf("entry 2 prev_hash mismatch: expected %s, got %s",
			expectedHash2, logs[2].PrevHash)
	}
}

func TestService_Record_SanitizesDetail(t *testing.T) {
	t.Parallel()
	svc, c := newTestService(t)
	ctx := context.Background()

	err := svc.Record(ctx, Record{
		Action:       "create",
		ResourceType: "users",
		Detail: map[string]any{
			"username": "alice",
			"password": "supersecret",
			"nested": map[string]any{
				"api_key": "key-123",
			},
		},
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	entry, err := c.AuditLog.Query().First(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if entry.Detail["username"] != "alice" {
		t.Errorf("username should be preserved")
	}
	if entry.Detail["password"] != "***" {
		t.Errorf("password should be masked, got %v", entry.Detail["password"])
	}
	nested, ok := entry.Detail["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested should be a map")
	}
	if nested["api_key"] != "***" {
		t.Errorf("nested api_key should be masked, got %v", nested["api_key"])
	}
}

func TestService_Record_EmptyDetail(t *testing.T) {
	t.Parallel()
	svc, c := newTestService(t)
	ctx := context.Background()

	err := svc.Record(ctx, Record{
		Action:       "delete",
		ResourceType: "clusters",
		ResourceID:   "res-1",
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	entry, err := c.AuditLog.Query().First(ctx)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(entry.Detail) > 0 {
		t.Errorf("empty detail should be nil/empty, got %v", entry.Detail)
	}
}

func TestService_List_Pagination(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Create 5 entries
	for i := 0; i < 5; i++ {
		_ = svc.Record(ctx, Record{
			Action:       "create",
			ResourceType: "clusters",
			ResourceID:   uuid.New().String(),
		})
	}

	result, err := svc.List(ctx, AuditFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(result.Logs) != 2 {
		t.Errorf("expected 2 logs on page 1, got %d", len(result.Logs))
	}
	if result.Total != 5 {
		t.Errorf("expected total 5, got %d", result.Total)
	}

	result2, err := svc.List(ctx, AuditFilter{Page: 3, PageSize: 2})
	if err != nil {
		t.Fatalf("List page 3 failed: %v", err)
	}
	if len(result2.Logs) != 1 {
		t.Errorf("expected 1 log on page 3, got %d", len(result2.Logs))
	}
}

func TestService_List_Defaults(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_ = svc.Record(ctx, Record{Action: "create", ResourceType: "clusters"})

	result, err := svc.List(ctx, AuditFilter{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if result.Page != 1 {
		t.Errorf("default page should be 1, got %d", result.Page)
	}
	if result.PageSize != 20 {
		t.Errorf("default page_size should be 20, got %d", result.PageSize)
	}
}

func TestService_List_Filters(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_ = svc.Record(ctx, Record{
		UserID: "user-a", Action: "create", ResourceType: "clusters", ResourceID: "r1",
	})
	_ = svc.Record(ctx, Record{
		UserID: "user-b", Action: "delete", ResourceType: "clusters", ResourceID: "r2",
	})
	_ = svc.Record(ctx, Record{
		UserID: "user-a", Action: "update", ResourceType: "users", ResourceID: "r3",
	})

	// Filter by user
	result, err := svc.List(ctx, AuditFilter{UserID: "user-a"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("user-a should have 2 logs, got %d", result.Total)
	}

	// Filter by action
	result, err = svc.List(ctx, AuditFilter{Action: "delete"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("delete action should have 1 log, got %d", result.Total)
	}

	// Filter by resource type
	result, err = svc.List(ctx, AuditFilter{ResourceType: "clusters"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("clusters type should have 2 logs, got %d", result.Total)
	}
}

func TestService_List_TimeRange(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	start := time.Now().UTC().Add(-1 * time.Hour)
	_ = svc.Record(ctx, Record{Action: "create", ResourceType: "clusters"})

	mid := time.Now().UTC()
	_ = svc.Record(ctx, Record{Action: "delete", ResourceType: "clusters"})

	end := time.Now().UTC().Add(1 * time.Hour)

	// All
	result, err := svc.List(ctx, AuditFilter{StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("time range should match 2 logs, got %d", result.Total)
	}

	// Only first
	result, err = svc.List(ctx, AuditFilter{StartTime: &start, EndTime: &mid})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("narrowed time range should match 1 log, got %d", result.Total)
	}
}

func TestService_VerifyChain_Valid(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		err := svc.Record(ctx, Record{
			Action:       "create",
			ResourceType: "clusters",
			ResourceID:   uuid.New().String(),
		})
		if err != nil {
			t.Fatalf("Record %d failed: %v", i, err)
		}
	}

	valid, gaps, err := svc.VerifyChain(ctx)
	if err != nil {
		t.Fatalf("VerifyChain failed: %v", err)
	}
	if !valid {
		t.Errorf("chain should be valid, gaps: %v", gaps)
	}
}

func TestService_VerifyChain_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	valid, gaps, err := svc.VerifyChain(ctx)
	if err != nil {
		t.Fatalf("VerifyChain failed: %v", err)
	}
	if !valid {
		t.Errorf("empty chain should be valid")
	}
	if len(gaps) != 0 {
		t.Errorf("empty chain should have no gaps")
	}
}

func TestExtractResourceTypeAndID(t *testing.T) {
	tests := []struct {
		path         string
		wantType     string
		wantID       string
	}{
		{"/api/v1/clusters/123", "clusters", "123"},
		{"/api/v1/clusters", "clusters", ""},
		{"/api/v1/audit-logs", "audit-logs", ""},
		{"/api/v1/users/abc/nested/def", "users", "abc"},
		{"/api/v1/", "", ""},
		{"/", "", ""},
		{"/api/v1", "", ""},
	}
	for _, tt := range tests {
		rt, rid := ExtractResourceTypeAndID(tt.path)
		if rt != tt.wantType || rid != tt.wantID {
			t.Errorf("ExtractResourceTypeAndID(%q) = (%q, %q), want (%q, %q)",
				tt.path, rt, rid, tt.wantType, tt.wantID)
		}
	}
}
