package system

import "context"

type Service interface {
	List(ctx context.Context, orgID, category string) ([]*SystemSetting, error)
	Get(ctx context.Context, orgID, key string) (*SystemSetting, error)
	Set(ctx context.Context, orgID, key string, req SetSettingReq) (*SystemSetting, error)
	Delete(ctx context.Context, orgID, key string) error
	HealthCheck(ctx context.Context) (*HealthCheckResult, error)
}
