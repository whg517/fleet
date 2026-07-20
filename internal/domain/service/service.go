package service

import (
	"context"
)

// Service defines the service catalog operations.
type Service interface {
	Create(ctx context.Context, req CreateServiceReq) (*ServiceEntry, error)
	List(ctx context.Context, filter ServiceFilter) (*ServiceListResult, error)
	Get(ctx context.Context, id string) (*ServiceEntry, error)
	Update(ctx context.Context, id string, req UpdateServiceReq) (*ServiceEntry, error)
	Delete(ctx context.Context, id string) error
}
