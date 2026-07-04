package service

import (
	"fmt"
	"strings"
)

func firstStringFromMap(values map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := values[key].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case map[string]any:
			if s := firstStringFromMap(v, "normal", "large", "small", "url"); s != "" {
				return s
			}
		}
	}
	return ""
}

func float32FromMap(values map[string]any, keys ...string) float32 {
	for _, key := range keys {
		switch v := values[key].(type) {
		case float64:
			return float32(v)
		case string:
			var out float32
			if _, err := fmt.Sscanf(strings.TrimSpace(v), "%f", &out); err == nil {
				return out
			}
		case map[string]any:
			if out := float32FromMap(v, "value", "score"); out > 0 {
				return out
			}
		}
	}
	return 0
}

func doubanEpisodeCountFromValue(value any) int {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	case string:
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err == nil && n > 0 {
			return n
		}
	case []any:
		if len(v) > 0 {
			return len(v)
		}
	}
	return 0
}
