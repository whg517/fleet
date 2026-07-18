package audit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/store/ent"
	"github.com/whg517/fleet/internal/store/ent/auditlog"
)

// Record is the input data for creating an audit log entry.
type Record struct {
	UserID       string
	Action       string
	ResourceType string
	ResourceID   string
	Detail       map[string]any
	IP           string
}

// AuditFilter holds query parameters for listing audit logs.
type AuditFilter struct {
	UserID       string
	ResourceType string
	Action       string
	StartTime    *time.Time
	EndTime      *time.Time
	Page         int
	PageSize     int
}

// Result is a paginated list of audit logs.
type Result struct {
	Logs       []*ent.AuditLog
	Total      int
	Page       int
	PageSize   int
}

// Service defines the audit log business operations.
// Implementation is INSERT-only: no Update or Delete methods are exposed.
type Service interface {
	Record(ctx context.Context, record Record) error
	List(ctx context.Context, filter AuditFilter) (*Result, error)
	VerifyChain(ctx context.Context) (bool, []VerificationGap, error)
}

// EntClient is the subset of the Ent client used by the audit service.
type EntClient interface {
	Query() *ent.AuditLogQuery
	Create() *ent.AuditLogCreate
}

// service implements Service using Ent ORM.
type service struct {
	client  *ent.Client
	logger  *zap.Logger
}

// NewService creates a new audit Service backed by the given Ent client.
func NewService(client *ent.Client, logger *zap.Logger) Service {
	return &service{
		client:  client,
		logger:  logger,
	}
}

// Record creates a new audit log entry with proper hash chain linkage.
func (s *service) Record(ctx context.Context, record Record) error {
	// Sanitize sensitive fields in detail
	detail := Sanitize(record.Detail)

	// Generate UUID for the new entry
	id := uuid.New().String()
	now := time.Now().UTC()

	// Get the last entry's prev_hash to link the chain
	lastEntry, err := s.getLastEntry(ctx)
	if err != nil {
		return fmt.Errorf("audit: get last entry: %w", err)
	}

	var prevHash string
	if lastEntry == nil {
		// First entry in the chain
		prevHash = GenesisHash()
	} else {
		// Compute the hash of the previous record
		lastChainRecord := ToChainRecord(
			lastEntry.ID, lastEntry.UserID, lastEntry.Action,
			lastEntry.ResourceType, lastEntry.ResourceID, lastEntry.IP,
			lastEntry.PrevHash, lastEntry.Detail, lastEntry.CreatedAt,
		)
		prevHash = ComputeHash(lastChainRecord)
	}

	// Create the audit log entry (INSERT only)
	_, err = s.client.AuditLog.Create().
		SetID(id).
		SetNillableUserID(&record.UserID).
		SetAction(record.Action).
		SetResourceType(record.ResourceType).
		SetNillableResourceID(&record.ResourceID).
		SetDetail(detail).
		SetNillableIP(&record.IP).
		SetPrevHash(prevHash).
		SetCreatedAt(now).
		Save(ctx)
	if err != nil {
		s.logger.Error("audit: failed to write audit log",
			zap.Error(err),
			zap.String("action", record.Action),
			zap.String("resource_type", record.ResourceType),
		)
		return fmt.Errorf("audit: write log: %w", err)
	}

	return nil
}

// getLastEntry returns the most recent audit log entry by created_at desc.
func (s *service) getLastEntry(ctx context.Context) (*ent.AuditLog, error) {
	entry, err := s.client.AuditLog.Query().
		Order(auditlog.ByCreatedAt(sql.OrderDesc())).
		Limit(1).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entry, nil
}

// List returns a paginated, filtered list of audit logs.
func (s *service) List(ctx context.Context, filter AuditFilter) (*Result, error) {
	// Normalize pagination
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	offset := (filter.Page - 1) * filter.PageSize

	q := s.client.AuditLog.Query()

	// Apply filters
	if filter.UserID != "" {
		q = q.Where(auditlog.UserIDEQ(filter.UserID))
	}
	if filter.ResourceType != "" {
		q = q.Where(auditlog.ResourceTypeEQ(filter.ResourceType))
	}
	if filter.Action != "" {
		q = q.Where(auditlog.ActionEQ(filter.Action))
	}
	if filter.StartTime != nil {
		q = q.Where(auditlog.CreatedAtGTE(*filter.StartTime))
	}
	if filter.EndTime != nil {
		q = q.Where(auditlog.CreatedAtLTE(*filter.EndTime))
	}

	// Count total
	total, err := q.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit: count logs: %w", err)
	}

	// Fetch page (newest first)
	logs, err := q.
		Order(auditlog.ByCreatedAt(sql.OrderDesc())).
		Offset(offset).
		Limit(filter.PageSize).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit: list logs: %w", err)
	}

	return &Result{
		Logs:     logs,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

// VerifyChain loads all audit logs in order and verifies the hash chain.
func (s *service) VerifyChain(ctx context.Context) (bool, []VerificationGap, error) {
	logs, err := s.client.AuditLog.Query().
		Order(auditlog.ByCreatedAt()).
		All(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("audit: load logs for verification: %w", err)
	}

	records := make([]ChainRecord, len(logs))
	for i, l := range logs {
		records[i] = ToChainRecord(
			l.ID, l.UserID, l.Action, l.ResourceType, l.ResourceID,
			l.IP, l.PrevHash, l.Detail, l.CreatedAt,
		)
	}

	valid, gaps := VerifyChain(records)
	return valid, gaps, nil
}

// ExtractResourceTypeAndID parses the Echo path to extract resource type and ID.
// Example: "/api/v1/clusters/123" → ("clusters", "123")
// Example: "/api/v1/audit-logs" → ("audit-logs", "")
// Example: "/api/v1" → ("", "")
func ExtractResourceTypeAndID(path string) (string, string) {
	// Strip /api/v1/ or /api/ prefix
	path = strings.TrimPrefix(path, "/api/v1/")
	path = strings.TrimPrefix(path, "/api/v1")
	path = strings.TrimPrefix(path, "/api/")
	path = strings.TrimPrefix(path, "/api")
	path = strings.Trim(path, "/")

	if path == "" {
		return "", ""
	}

	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", ""
	}
	resourceType := parts[0]
	resourceID := ""
	if len(parts) >= 2 {
		resourceID = parts[1]
	}
	return resourceType, resourceID
}
