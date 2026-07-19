package system

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/infra/secrets"
	"github.com/whg517/fleet/internal/store/ent"
	entsystem "github.com/whg517/fleet/internal/store/ent/systemsetting"
)

type SystemStore interface {
	NewSettingCreate() *ent.SystemSettingCreate
	SaveSetting(ctx context.Context, c *ent.SystemSettingCreate) (*ent.SystemSetting, error)
	GetSettingByKey(ctx context.Context, orgID, key string) (*ent.SystemSetting, error)
	UpdateSettingOne(id string) *ent.SystemSettingUpdateOne
	SaveSettingUpdate(ctx context.Context, upd *ent.SystemSettingUpdateOne) (*ent.SystemSetting, error)
	DeleteSetting(ctx context.Context, id string) error
	ListSettings(ctx context.Context, orgID, category string) ([]*ent.SystemSetting, error)
}

type HealthChecker interface {
	PingDB(ctx context.Context) error
	PingRedis(ctx context.Context) error
}

type EntStore struct {
	client *ent.Client
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) NewSettingCreate() *ent.SystemSettingCreate {
	return s.client.SystemSetting.Create()
}

func (s *EntStore) SaveSetting(ctx context.Context, c *ent.SystemSettingCreate) (*ent.SystemSetting, error) {
	return c.Save(ctx)
}

func (s *EntStore) GetSettingByKey(ctx context.Context, orgID, key string) (*ent.SystemSetting, error) {
	return s.client.SystemSetting.Query().
		Where(entsystem.KeyEQ(key), entsystem.OrgIDEQ(orgID)).
		Only(ctx)
}

func (s *EntStore) UpdateSettingOne(id string) *ent.SystemSettingUpdateOne {
	return s.client.SystemSetting.UpdateOneID(id)
}

func (s *EntStore) SaveSettingUpdate(ctx context.Context, upd *ent.SystemSettingUpdateOne) (*ent.SystemSetting, error) {
	return upd.Save(ctx)
}

func (s *EntStore) DeleteSetting(ctx context.Context, id string) error {
	return s.client.SystemSetting.DeleteOneID(id).Exec(ctx)
}

func (s *EntStore) ListSettings(ctx context.Context, orgID, category string) ([]*ent.SystemSetting, error) {
	q := s.client.SystemSetting.Query().Where(entsystem.OrgIDEQ(orgID))
	if category != "" {
		q = q.Where(entsystem.CategoryEQ(entsystem.Category(category)))
	}
	return q.Order(entsystem.ByCategory(), entsystem.ByKey()).All(ctx)
}

type ServiceImpl struct {
	store  SystemStore
	health HealthChecker
	dek    []byte
	logger *zap.Logger
}

func NewService(store SystemStore, health HealthChecker, dek []byte, logger *zap.Logger) *ServiceImpl {
	return &ServiceImpl{store: store, health: health, dek: dek, logger: logger}
}

func (s *ServiceImpl) List(ctx context.Context, orgID, category string) ([]*SystemSetting, error) {
	settings, err := s.store.ListSettings(ctx, orgID, category)
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	result := make([]*SystemSetting, 0, len(settings))
	for _, st := range settings {
		result = append(result, toDomainSetting(st, true))
	}
	return result, nil
}

func (s *ServiceImpl) Get(ctx context.Context, orgID, key string) (*SystemSetting, error) {
	st, err := s.store.GetSettingByKey(ctx, orgID, key)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrSettingNotFound
		}
		return nil, fmt.Errorf("get setting: %w", err)
	}
	return toDomainSetting(st, true), nil
}

func (s *ServiceImpl) Set(ctx context.Context, orgID, key string, req SetSettingReq) (*SystemSetting, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("%w: key is required", ErrInvalidInput)
	}
	if req.Value == "" {
		return nil, fmt.Errorf("%w: value is required", ErrInvalidInput)
	}

	sensitive := isSensitiveKey(key)
	storedValue := req.Value
	if sensitive {
		encrypted, err := secrets.Encrypt(s.dek, []byte(req.Value))
		if err != nil {
			s.logger.Error("failed to encrypt setting value", zap.Error(err), zap.String("key", key))
			return nil, fmt.Errorf("failed to encrypt setting value: %w", err)
		}
		storedValue = string(encrypted)
	}

	category := req.Category
	if category == "" {
		category = CategoryGeneral
	}

	existing, err := s.store.GetSettingByKey(ctx, orgID, key)
	if err != nil && !ent.IsNotFound(err) {
		return nil, fmt.Errorf("query existing setting: %w", err)
	}

	if existing != nil {
		upd := s.store.UpdateSettingOne(existing.ID).
			SetValue(storedValue).
			SetEncrypted(sensitive).
			SetCategory(entsystem.Category(category)).
			SetDescription(req.Description)
		updated, err := s.store.SaveSettingUpdate(ctx, upd)
		if err != nil {
			return nil, fmt.Errorf("update setting: %w", err)
		}
		s.logger.Info("setting updated", zap.String("key", key), zap.Bool("encrypted", sensitive))
		return toDomainSetting(updated, true), nil
	}

	builder := s.store.NewSettingCreate().
		SetID(uuid.NewString()).
		SetOrgID(orgID).
		SetKey(key).
		SetValue(storedValue).
		SetEncrypted(sensitive).
		SetCategory(entsystem.Category(category)).
		SetDescription(req.Description)
	created, err := s.store.SaveSetting(ctx, builder)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, ErrSettingAlreadyExists
		}
		return nil, fmt.Errorf("create setting: %w", err)
	}
	s.logger.Info("setting created", zap.String("key", key), zap.Bool("encrypted", sensitive))
	return toDomainSetting(created, true), nil
}

func (s *ServiceImpl) Delete(ctx context.Context, orgID, key string) error {
	st, err := s.store.GetSettingByKey(ctx, orgID, key)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrSettingNotFound
		}
		return fmt.Errorf("get setting for delete: %w", err)
	}
	if err := s.store.DeleteSetting(ctx, st.ID); err != nil {
		return fmt.Errorf("delete setting: %w", err)
	}
	s.logger.Info("setting deleted", zap.String("key", key))
	return nil
}

func (s *ServiceImpl) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result := &HealthCheckResult{}
	allOK := true
	if s.health != nil {
		if err := s.health.PingDB(checkCtx); err != nil {
			result.Checks = append(result.Checks, HealthStatus{Service: "database", Status: "error", Message: err.Error()})
			allOK = false
		} else {
			result.Checks = append(result.Checks, HealthStatus{Service: "database", Status: "ok"})
		}
		if err := s.health.PingRedis(checkCtx); err != nil {
			result.Checks = append(result.Checks, HealthStatus{Service: "redis", Status: "error", Message: err.Error()})
			allOK = false
		} else {
			result.Checks = append(result.Checks, HealthStatus{Service: "redis", Status: "ok"})
		}
	}
	if allOK {
		result.Status = "ok"
	} else {
		result.Status = "degraded"
	}
	return result, nil
}

func toDomainSetting(st *ent.SystemSetting, maskSensitive bool) *SystemSetting {
	value := st.Value
	if maskSensitive && st.Encrypted {
		value = "***"
	}
	return &SystemSetting{
		ID: st.ID, OrgID: st.OrgID, Key: st.Key, Value: value, Encrypted: st.Encrypted,
		Category: Category(st.Category), Description: st.Description,
		CreatedAt: st.CreatedAt, UpdatedAt: st.UpdatedAt,
	}
}
