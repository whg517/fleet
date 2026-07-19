package system

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrSettingNotFound      = errors.New("setting not found")
	ErrSettingAlreadyExists = errors.New("setting already exists")
	ErrInvalidInput         = errors.New("invalid input")
)

type Category string

const (
	CategoryArgocd       Category = "argocd"
	CategoryHarbor       Category = "harbor"
	CategoryGit          Category = "git"
	CategoryNotification Category = "notification"
	CategoryGeneral      Category = "general"
)

type SystemSetting struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id,omitempty"`
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Encrypted   bool      `json:"encrypted"`
	Category    Category  `json:"category"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SetSettingReq struct {
	Value       string   `json:"value"`
	Category    Category `json:"category,omitempty"`
	Description string   `json:"description,omitempty"`
}

type HealthStatus struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type HealthCheckResult struct {
	Status string         `json:"status"`
	Checks []HealthStatus `json:"checks"`
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, keyword := range []string{"token", "password", "secret", "credential"} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}
