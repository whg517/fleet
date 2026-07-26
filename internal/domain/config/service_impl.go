package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"entgo.io/ent/dialect/sql"
	"github.com/whg517/fleet/internal/store/ent"
	"github.com/whg517/fleet/internal/store/ent/configsnapshot"
	entservice "github.com/whg517/fleet/internal/store/ent/service"
)

// EntStore adapts *ent.Client to the ConfigStore interface.
type EntStore struct {
	client *ent.Client
}

// NewEntStore creates a new EntStore for ConfigSnapshot.
func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) NewConfigSnapshotCreate() *ent.ConfigSnapshotCreate {
	return s.client.ConfigSnapshot.Create()
}

func (s *EntStore) SaveSnapshot(ctx context.Context, c *ent.ConfigSnapshotCreate) (*ent.ConfigSnapshot, error) {
	return c.Save(ctx)
}

func (s *EntStore) ListSnapshots(ctx context.Context, limit, offset int, orgID, serviceID, environmentID string) ([]*ent.ConfigSnapshot, int, error) {
	q := s.client.ConfigSnapshot.Query()
	if orgID != "" {
		q = q.Where(configsnapshot.OrgIDEQ(orgID))
	}
	if serviceID != "" {
		q = q.Where(configsnapshot.ServiceIDEQ(serviceID))
	}
	if environmentID != "" {
		q = q.Where(configsnapshot.EnvironmentIDEQ(environmentID))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	snapshots, err := q.Order(configsnapshot.ByCreatedAt(sql.OrderDesc())).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return snapshots, total, nil
}

func (s *EntStore) GetSnapshot(ctx context.Context, id string) (*ent.ConfigSnapshot, error) {
	return s.client.ConfigSnapshot.Get(ctx, id)
}

// LookupEntStore adapts *ent.Client to the LookupStore interface.
type LookupEntStore struct {
	client *ent.Client
}

// NewLookupEntStore creates a new LookupEntStore.
func NewLookupEntStore(client *ent.Client) *LookupEntStore {
	return &LookupEntStore{client: client}
}

func (s *LookupEntStore) GetServiceByID(ctx context.Context, id string) (*ent.Service, error) {
	return s.client.Service.Get(ctx, id)
}

func (s *LookupEntStore) GetEnvironmentByID(ctx context.Context, id string) (*ent.Environment, error) {
	return s.client.Environment.Get(ctx, id)
}

func (s *LookupEntStore) GetLatestDeployment(ctx context.Context, serviceID, environmentID string) (*ent.Deployment, error) {
	deps, err := s.client.Deployment.Query().
		Where(
			func(sel *sql.Selector) {
				sel.Where(sql.And(
					sql.EQ(sel.C("service_id"), serviceID),
					sql.EQ(sel.C("environment_id"), environmentID),
				))
			},
		).
		Order(func(sel *sql.Selector) {
			sel.OrderBy(sql.Desc(sel.C("created_at")))
		}).
		Limit(1).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(deps) == 0 {
		return nil, &ent.NotFoundError{}
	}
	return deps[0], nil
}

// --- Service Implementation ---

const (
	maxValuesKeys = 100
)

// ServiceImpl implements the Service interface.
type ServiceImpl struct {
	store  ConfigStore
	lookup LookupStore
	argocd ArgoCDClient
	logger *zap.Logger
}

// NewService creates a new config service.
func NewService(store ConfigStore, lookup LookupStore, argocd ArgoCDClient, logger *zap.Logger) *ServiceImpl {
	return &ServiceImpl{
		store:  store,
		lookup: lookup,
		argocd: argocd,
		logger: logger,
	}
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

// UpdateValues modifies Helm values for a service+environment.
func (s *ServiceImpl) UpdateValues(ctx context.Context, serviceID, environmentID string, req UpdateValuesReq) (*ConfigSnapshot, error) {
	// Validate inputs
	if strings.TrimSpace(serviceID) == "" {
		return nil, fmt.Errorf("%w: service_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(environmentID) == "" {
		return nil, fmt.Errorf("%w: environment_id is required", ErrInvalidInput)
	}
	if req.Values == nil {
		return nil, fmt.Errorf("%w: values is required", ErrInvalidInput)
	}
	if len(req.Values) > maxValuesKeys {
		return nil, fmt.Errorf("%w: values must have at most %d keys", ErrInvalidInput, maxValuesKeys)
	}

	// Validate service exists and is active
	svc, err := s.lookup.GetServiceByID(ctx, serviceID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: service not found", ErrInvalidInput)
		}
		return nil, fmt.Errorf("lookup service: %w", err)
	}
	if svc.Status == entservice.StatusArchived {
		return nil, fmt.Errorf("%w: service is archived", ErrInvalidInput)
	}

	// Validate environment exists
	_, err = s.lookup.GetEnvironmentByID(ctx, environmentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: environment not found", ErrInvalidInput)
		}
		return nil, fmt.Errorf("lookup environment: %w", err)
	}

	// Get the latest deployment to find the argocd_app_name
	dep, err := s.lookup.GetLatestDeployment(ctx, serviceID, environmentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNoActiveDeployment
		}
		return nil, fmt.Errorf("lookup latest deployment: %w", err)
	}
	if dep.ArgocdAppName == "" {
		return nil, fmt.Errorf("%w: deployment has no Argo CD application", ErrNoActiveDeployment)
	}

	// Get previous values (from the most recent ConfigSnapshot, or empty)
	var previousValues map[string]any
	existing, _, err := s.store.ListSnapshots(ctx, 1, 0, req.OrgID, serviceID, environmentID)
	if err != nil {
		s.logger.Warn("failed to get previous config snapshot", zap.Error(err))
	}
	if len(existing) > 0 {
		previousValues = existing[0].Values
	}

	// Update Argo CD Application with new values
	err = s.argocd.UpdateApplication(ctx, dep.ArgocdAppName, req.Values)
	if err != nil {
		s.logger.Error("failed to update Argo CD application",
			zap.String("name", dep.ArgocdAppName),
			zap.Error(err),
		)
		return nil, fmt.Errorf("%w: %v", ErrArgoCDUnavailable, err)
	}

	// Save ConfigSnapshot
	snapshotID := uuid.NewString()
	builder := s.store.NewConfigSnapshotCreate().
		SetID(snapshotID).
		SetServiceID(serviceID).
		SetEnvironmentID(environmentID).
		SetValues(req.Values)

	if req.OrgID != "" {
		builder.SetOrgID(req.OrgID)
	}
	if previousValues != nil {
		builder.SetPreviousValues(previousValues)
	}
	if req.ChangedBy != "" {
		builder.SetChangedBy(req.ChangedBy)
	}
	if req.ChangeReason != "" {
		builder.SetChangeReason(req.ChangeReason)
	}

	saved, err := s.store.SaveSnapshot(ctx, builder)
	if err != nil {
		return nil, fmt.Errorf("save config snapshot: %w", err)
	}

	s.logger.Info("config values updated",
		zap.String("id", saved.ID),
		zap.String("service_id", serviceID),
		zap.String("environment_id", environmentID),
		zap.String("argocd_app", dep.ArgocdAppName),
	)

	return toDomainSnapshot(saved), nil
}

// GetValues returns the current effective Helm values.
func (s *ServiceImpl) GetValues(ctx context.Context, serviceID, environmentID string) (map[string]any, error) {
	if strings.TrimSpace(serviceID) == "" {
		return nil, fmt.Errorf("%w: service_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(environmentID) == "" {
		return nil, fmt.Errorf("%w: environment_id is required", ErrInvalidInput)
	}

	snapshots, _, err := s.store.ListSnapshots(ctx, 1, 0, "", serviceID, environmentID)
	if err != nil {
		return nil, fmt.Errorf("query latest snapshot: %w", err)
	}
	if len(snapshots) == 0 {
		return map[string]any{}, nil
	}
	return snapshots[0].Values, nil
}

// ListHistory returns a paginated list of config changes.
func (s *ServiceImpl) ListHistory(ctx context.Context, serviceID, environmentID string, page, pageSize int) (*ConfigHistoryResult, error) {
	if strings.TrimSpace(serviceID) == "" {
		return nil, fmt.Errorf("%w: service_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(environmentID) == "" {
		return nil, fmt.Errorf("%w: environment_id is required", ErrInvalidInput)
	}

	page, pageSize = normalizePage(page, pageSize)
	offset := (page - 1) * pageSize

	snapshots, total, err := s.store.ListSnapshots(ctx, pageSize, offset, "", serviceID, environmentID)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}

	result := make([]*ConfigSnapshot, 0, len(snapshots))
	for _, snap := range snapshots {
		result = append(result, toDomainSnapshot(snap))
	}

	return &ConfigHistoryResult{
		Snapshots: result,
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
	}, nil
}

// Diff compares two config snapshots by ID.
func (s *ServiceImpl) Diff(ctx context.Context, serviceID, environmentID string, fromVer, toVer string) (*ConfigDiff, error) {
	if strings.TrimSpace(serviceID) == "" {
		return nil, fmt.Errorf("%w: service_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(environmentID) == "" {
		return nil, fmt.Errorf("%w: environment_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(fromVer) == "" {
		return nil, fmt.Errorf("%w: from_version is required", ErrInvalidInput)
	}

	// Get "from" snapshot
	fromSnap, err := s.store.GetSnapshot(ctx, fromVer)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: from snapshot not found", ErrConfigNotFound)
		}
		return nil, fmt.Errorf("get from snapshot: %w", err)
	}

	// Determine "to" snapshot: if toVer is empty, use the latest
	var toSnap *ent.ConfigSnapshot
	if strings.TrimSpace(toVer) == "" {
		snapshots, _, err := s.store.ListSnapshots(ctx, 1, 0, "", serviceID, environmentID)
		if err != nil {
			return nil, fmt.Errorf("query latest snapshot: %w", err)
		}
		if len(snapshots) == 0 {
			return nil, fmt.Errorf("%w: no snapshots found for comparison", ErrConfigNotFound)
		}
		toSnap = snapshots[0]
	} else {
		toSnap, err = s.store.GetSnapshot(ctx, toVer)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, fmt.Errorf("%w: to snapshot not found", ErrConfigNotFound)
			}
			return nil, fmt.Errorf("get to snapshot: %w", err)
		}
	}

	entries := diffMaps(fromSnap.Values, toSnap.Values, "")

	return &ConfigDiff{
		FromSnapshotID: fromSnap.ID,
		ToSnapshotID:   toSnap.ID,
		Entries:        entries,
	}, nil
}

// --- Helpers ---

func toDomainSnapshot(s *ent.ConfigSnapshot) *ConfigSnapshot {
	return &ConfigSnapshot{
		ID:             s.ID,
		OrgID:          s.OrgID,
		ServiceID:      s.ServiceID,
		EnvironmentID:  s.EnvironmentID,
		Values:         s.Values,
		PreviousValues: s.PreviousValues,
		ChangedBy:      s.ChangedBy,
		ChangeReason:   s.ChangeReason,
		CreatedAt:      s.CreatedAt,
	}
}

// diffMaps recursively compares two map[string]any and returns differences.
func diffMaps(oldVal, newVal map[string]any, prefix string) []ConfigDiffEntry {
	var entries []ConfigDiffEntry

	// Collect all keys from both maps
	keys := make(map[string]struct{})
	for k := range oldVal {
		keys[k] = struct{}{}
	}
	for k := range newVal {
		keys[k] = struct{}{}
	}

	for k := range keys {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}

		if _, inOld := oldVal[k]; !inOld {
			// Added in new
			entries = append(entries, ConfigDiffEntry{
				Path:     path,
				Type:     "added",
				NewValue: newVal[k],
			})
			continue
		}
		if _, inNew := newVal[k]; !inNew {
			// Removed in new
			entries = append(entries, ConfigDiffEntry{
				Path:     path,
				Type:     "removed",
				OldValue: oldVal[k],
			})
			continue
		}
		// Both exist
		if !deepEqual(oldVal[k], newVal[k]) {
			// If both are maps, recurse
			if oldMap, ok1 := oldVal[k].(map[string]any); ok1 {
				if newMap, ok2 := newVal[k].(map[string]any); ok2 {
					entries = append(entries, diffMaps(oldMap, newMap, path)...)
					continue
				}
			}
			entries = append(entries, ConfigDiffEntry{
				Path:     path,
				Type:     "changed",
				OldValue: oldVal[k],
				NewValue: newVal[k],
			})
		}
	}

	return entries
}

func deepEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !deepEqual(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i, v := range av {
			if !deepEqual(v, bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
