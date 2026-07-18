package audit

import (
	"strings"
)

// sensitiveKeys are keys whose values should be masked in audit logs.
var sensitiveKeys = []string{
	"password",
	"secret",
	"token",
	"key",
	"kubeconfig",
	"credentials",
}

// isSensitiveKey returns true if the key contains any sensitive substring
// (case-insensitive).
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, sk := range sensitiveKeys {
		if strings.Contains(lower, sk) {
			return true
		}
	}
	return false
}

// Sanitize recursively walks the given map, replacing values of sensitive
// keys with "***". It handles nested maps and slices. The input map is
// not mutated; a deep-sanitized copy is returned.
func Sanitize(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		out[k] = sanitizeValue(k, v)
	}
	return out
}

// sanitizeValue sanitizes a value based on its key and type.
func sanitizeValue(key string, val any) any {
	if isSensitiveKey(key) {
		return "***"
	}
	switch v := val.(type) {
	case map[string]any:
		return Sanitize(v)
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = sanitizeValue(key, item)
		}
		return result
	default:
		return val
	}
}
