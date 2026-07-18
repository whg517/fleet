package middleware

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/domain/audit"
)

// auditedMethods are the HTTP methods that trigger audit logging.
var auditedMethods = map[string]string{
	http.MethodPost:   "create",
	http.MethodPut:    "update",
	http.MethodPatch:  "update",
	http.MethodDelete: "delete",
}

// maxBodyBytes limits how much of the request body we read for audit detail.
const maxBodyBytes = 64 * 1024 // 64 KB

// AuditMiddleware intercepts all write operations (POST/PUT/PATCH/DELETE)
// and records an audit log entry asynchronously after the handler completes.
func AuditMiddleware(auditSvc audit.Service, logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Read and buffer the request body before the handler consumes it
			detail := readBodyForAudit(c)

			// Execute the handler first
			err := next(c)

			// Determine action from HTTP method
			action, shouldAudit := auditedMethods[c.Request().Method]
			if !shouldAudit {
				return err
			}

			// Extract resource info from the actual request URL path
			// (c.Path() returns the route template, e.g. /api/v1/clusters/:id)
			resourceType, resourceID := audit.ExtractResourceTypeAndID(c.Request().URL.Path)

			// Skip auditing audit-log endpoints themselves to avoid recursive noise
			if resourceType == "audit-logs" {
				return err
			}

			// Use a detached context so the audit record is not cancelled
			// when the HTTP response completes and the request context is freed.
			auditCtx := context.WithoutCancel(c.Request().Context())

			// Asynchronously record audit log — does not block the response
			go func() {
				recordErr := auditSvc.Record(auditCtx, audit.Record{
					UserID:       getUserID(c),
					Action:       action,
					ResourceType: resourceType,
					ResourceID:   resourceID,
					Detail:       detail,
					IP:           c.RealIP(),
				})
				if recordErr != nil {
					logger.Error("audit middleware: failed to record",
						zap.Error(recordErr),
						zap.String("method", c.Request().Method),
						zap.String("path", c.Request().URL.Path),
					)
				}
			}()

			return err
		}
	}
}

// getUserID extracts the user ID from the Echo context.
// This will be populated by the auth middleware (Issue #12).
// For now, it returns an empty string.
func getUserID(c echo.Context) string {
	if val := c.Get("user_id"); val != nil {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// readBodyForAudit reads the request body up to maxBodyBytes and returns it
// as a map. If the body is not valid JSON or is empty, it returns nil.
// The original body is restored so the handler can read it normally.
func readBodyForAudit(c echo.Context) map[string]any {
	req := c.Request()
	if req.Body == nil {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, maxBodyBytes))
	if err != nil {
		return nil
	}

	// Restore the body for downstream handlers
	req.Body = io.NopCloser(strings.NewReader(string(body)))

	if len(body) == 0 {
		return nil
	}

	var detail map[string]any
	if err := json.Unmarshal(body, &detail); err != nil {
		// Not JSON — store the raw body as a detail field
		detail = map[string]any{"raw_body": string(body)}
	}

	return detail
}
