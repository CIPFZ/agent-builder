package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

const auditPreviewLimit = 600
const runtimePartPreviewLimit = 4000

func preview(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func newRequestID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(data[:]))
}

func newRuntimeEventID() string {
	return "evt_" + newRequestID()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func isPathInside(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if root == target {
		return true
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func redactRuntimePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	redacted, _ := redactRuntimeValue(payload).(map[string]any)
	return redacted
}

func redactRuntimeValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if isSensitiveKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactRuntimeValue(item)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if isSensitiveKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactRuntimeString(key, item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactRuntimeValue(item)
		}
		return out
	case string:
		return redactRuntimeString("", v)
	default:
		return redactRuntimeStruct(v)
	}
}

func redactRuntimeStruct(value any) any {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return value
	}
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		data := make(map[string]any)
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			field := rt.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				name = field.Name
			}
			if isSensitiveKey(name) {
				data[name] = "[REDACTED]"
				continue
			}
			data[name] = redactRuntimeValue(rv.Field(i).Interface())
		}
		return data
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = redactRuntimeValue(rv.Index(i).Interface())
		}
		return out
	default:
		return value
	}
}

func redactRuntimeString(key, value string) string {
	if value == "" {
		return value
	}
	if key != "" && isSensitiveKey(key) {
		return "[REDACTED]"
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{`"api_key"`, `"apikey"`, `"authorization"`, `"password"`, `"secret"`, `"token"`, `"credential"`, `"access_key"`, `"private_key"`, "proxy-authorization"} {
		if strings.Contains(lower, marker) {
			return "[REDACTED]"
		}
	}
	if strings.Contains(lower, "bearer ") || strings.Contains(lower, "api_key=") || strings.Contains(lower, "apikey=") || strings.Contains(lower, "token=") || strings.Contains(lower, "password=") || strings.Contains(lower, "secret=") || strings.Contains(lower, "proxy-authorization") {
		return "[REDACTED]"
	}
	if parsed, err := url.Parse(value); err == nil && parsed.User != nil {
		parsed.User = url.UserPassword("[REDACTED]", "[REDACTED]")
		return parsed.String()
	}
	return value
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	for _, marker := range []string{"api_key", "apikey", "authorization", "password", "passwd", "secret", "token", "credential", "access_key", "private_key", "proxy"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
