package service

import (
	"errors"
	"time"
)

var (
	// ErrServiceNotFound is returned when a service does not exist.
	ErrServiceNotFound = errors.New("service not found")
	// ErrServiceAlreadyExists is returned when a service name already exists within the org.
	ErrServiceAlreadyExists = errors.New("service already exists")
	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")
)

// ServiceEntry represents a microservice registered in the service catalog.
type ServiceEntry struct {
	ID            string            `json:"id"`
	OrgID         string            `json:"org_id,omitempty"`
	Name          string            `json:"name"`
	Team          string            `json:"team,omitempty"`
	Description   string            `json:"description,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Status        string            `json:"status"`
	HarborProject string            `json:"harbor_project,omitempty"`
	GitRepo       string            `json:"git_repo,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// CreateServiceReq is the request payload for registering a new service.
type CreateServiceReq struct {
	OrgID         string            `json:"org_id,omitempty"`
	Name          string            `json:"name"`
	Team          string            `json:"team,omitempty"`
	Description   string            `json:"description,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	HarborProject string            `json:"harbor_project,omitempty"`
	GitRepo       string            `json:"git_repo,omitempty"`
}

// UpdateServiceReq is the request payload for updating a service.
// All fields are optional (pointer) — only provided fields are updated.
type UpdateServiceReq struct {
	Name          *string            `json:"name,omitempty"`
	Team          *string            `json:"team,omitempty"`
	Description   *string            `json:"description,omitempty"`
	Labels        *map[string]string `json:"labels,omitempty"`
	HarborProject *string            `json:"harbor_project,omitempty"`
	GitRepo       *string            `json:"git_repo,omitempty"`
}

// ServiceFilter is used for filtering and paginating the service catalog.
type ServiceFilter struct {
	OrgID    string
	Name     string // fuzzy match (LIKE %name%)
	Team     string
	Status   string
	Labels   map[string]string
	Page     int
	PageSize int
}

// ServiceListResult holds a paginated list of services.
type ServiceListResult struct {
	Services []*ServiceEntry
	Total    int
	Page     int
	PageSize int
}
