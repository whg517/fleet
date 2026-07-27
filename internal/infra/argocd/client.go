package argocd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"
)

const defaultHTTPTimeout = 30 * time.Second

// Client implements the Argo CD REST API client.
// It communicates with Argo CD via HTTP, not gRPC.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	logger  *zap.Logger
}

// NewClient creates a new Argo CD REST API client.
// Returns an error if baseURL is empty.
func NewClient(baseURL, token string, logger *zap.Logger) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("argocd baseURL must not be empty")
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		http: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		logger: logger,
	}, nil
}

// argocdAppSpec is the Argo CD Application spec sent via REST API.
type argocdAppSpec struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Source struct {
			RepoURL        string      `json:"repoURL"`
			Chart          string      `json:"chart,omitempty"`
			TargetRevision string      `json:"targetRevision"`
			Helm           *helmConfig `json:"helm,omitempty"`
		} `json:"source"`
		Destination struct {
			Namespace string `json:"namespace"`
			Server    string `json:"server"`
		} `json:"destination"`
		SyncPolicy struct {
			Automated *struct {
				Prune    bool `json:"prune"`
				SelfHeal bool `json:"selfHeal"`
			} `json:"automated,omitempty"`
		} `json:"syncPolicy"`
	} `json:"spec"`
}

type helmConfig struct {
	Values map[string]any `json:"values,omitempty"`
}

// CreateApplication creates a new Argo CD Application via the REST API.
func (c *Client) CreateApplication(ctx context.Context, name, namespace, repoURL, chart, targetRev string, values map[string]any) error {
	spec := argocdAppSpec{}
	spec.APIVersion = "argoproj.io/v1alpha1"
	spec.Kind = "Application"
	spec.Metadata.Name = name
	spec.Metadata.Namespace = "argocd"
	spec.Spec.Source.RepoURL = repoURL
	spec.Spec.Source.Chart = chart
	spec.Spec.Source.TargetRevision = targetRev
	if values != nil {
		spec.Spec.Source.Helm = &helmConfig{Values: values}
	}
	spec.Spec.Destination.Namespace = namespace
	spec.Spec.Destination.Server = "https://kubernetes.default.svc"
	spec.Spec.SyncPolicy.Automated = &struct {
		Prune    bool `json:"prune"`
		SelfHeal bool `json:"selfHeal"`
	}{
		Prune:    true,
		SelfHeal: true,
	}

	body, err := json.Marshal(struct {
		Application argocdAppSpec `json:"application"`
	}{Application: spec})
	if err != nil {
		return fmt.Errorf("marshal application spec: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/applications", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("argocd create application failed: status=%d body=%s", resp.StatusCode, truncateBody(respBody))
	}

	c.logger.Info("argocd application created", zap.String("name", name))
	return nil
}

// AppStatus holds the sync and health status returned by Argo CD.
type AppStatus struct {
	SyncStatus   string
	HealthStatus string
}

// GetApplication returns the sync and health status of an Argo CD Application.
func (c *Client) GetApplication(ctx context.Context, name string) (*AppStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/applications/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("argocd get application failed: status=%d body=%s", resp.StatusCode, truncateBody(respBody))
	}

	var result struct {
		Status struct {
			Sync struct {
				Status string `json:"status"`
			} `json:"sync"`
			Health struct {
				Status string `json:"status"`
			} `json:"health"`
		} `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &AppStatus{
		SyncStatus:   result.Status.Sync.Status,
		HealthStatus: result.Status.Health.Status,
	}, nil
}

// SyncApplication triggers a manual sync of an Argo CD Application.
func (c *Client) SyncApplication(ctx context.Context, name string) error {
	body, err := json.Marshal(map[string]any{
		"revision": "HEAD",
	})
	if err != nil {
		return fmt.Errorf("marshal sync request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/applications/"+url.PathEscape(name)+"/sync", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("argocd sync application failed: status=%d body=%s", resp.StatusCode, truncateBody(respBody))
	}

	c.logger.Info("argocd application synced", zap.String("name", name))
	return nil
}

// RollbackApplication rolls back an Argo CD Application to a specific revision.
// Uses the sync API with an explicit revision instead of the rollback API
// (which expects a numeric deployment history ID, not a revision string).
func (c *Client) RollbackApplication(ctx context.Context, name, revision string) error {
	body, err := json.Marshal(map[string]any{
		"revision": revision,
	})
	if err != nil {
		return fmt.Errorf("marshal rollback request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/applications/"+url.PathEscape(name)+"/sync", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("argocd rollback application failed: status=%d body=%s", resp.StatusCode, truncateBody(respBody))
	}

	c.logger.Info("argocd application rollback initiated", zap.String("name", name), zap.String("revision", revision))
	return nil
}

// DeleteApplication deletes an Argo CD Application.
func (c *Client) DeleteApplication(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/v1/applications/"+url.PathEscape(name), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("argocd delete application failed: status=%d body=%s", resp.StatusCode, truncateBody(respBody))
	}

	c.logger.Info("argocd application deleted", zap.String("name", name))
	return nil
}

// UpdateApplication updates an Argo CD Application's Helm values via the REST API.
// It first GETs the existing application spec to avoid overwriting other fields,
// then modifies only source.helm.values and PUTs the full spec back.
func (c *Client) UpdateApplication(ctx context.Context, name string, values map[string]any) error {
	// 1. GET existing application to preserve the full spec
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/applications/"+url.PathEscape(name), nil)
	if err != nil {
		return fmt.Errorf("create get request: %w", err)
	}
	c.setHeaders(getReq)

	getResp, err := c.http.Do(getReq)
	if err != nil {
		return fmt.Errorf("send get request: %w", err)
	}

	if getResp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(getResp.Body)
		_ = getResp.Body.Close()
		return fmt.Errorf("argocd get application for update failed: status=%d body=%s", getResp.StatusCode, truncateBody(respBody))
	}

	var existing struct {
		Application argocdAppSpec `json:"application"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&existing); err != nil {
		_ = getResp.Body.Close()
		return fmt.Errorf("decode existing application: %w", err)
	}
	_ = getResp.Body.Close()

	// 2. Modify only the helm values in the existing spec
	if values != nil {
		existing.Application.Spec.Source.Helm = &helmConfig{Values: values}
	} else {
		existing.Application.Spec.Source.Helm = nil
	}

	// 3. PUT the full spec back
	body, err := json.Marshal(struct {
		Application argocdAppSpec `json:"application"`
	}{Application: existing.Application})
	if err != nil {
		return fmt.Errorf("marshal application update spec: %w", err)
	}

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/api/v1/applications/"+url.PathEscape(name), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create put request: %w", err)
	}
	c.setHeaders(putReq)

	putResp, err := c.http.Do(putReq)
	if err != nil {
		return fmt.Errorf("send put request: %w", err)
	}
	defer func() { _ = putResp.Body.Close() }()

	if putResp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(putResp.Body)
		return fmt.Errorf("argocd update application failed: status=%d body=%s", putResp.StatusCode, truncateBody(respBody))
	}

	c.logger.Info("argocd application updated", zap.String("name", name))
	return nil
}

// truncateBody truncates an HTTP response body to at most 200 characters for safe inclusion in error messages.
func truncateBody(b []byte) string {
	const maxLen = 200
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen]) + "... (truncated)"
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
