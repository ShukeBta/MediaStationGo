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

func positiveIntFromMap(values map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := values[key].(type) {
		case float64:
			if value > 0 {
				return int(value)
			}
		case string:
			var out int
			if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &out); err == nil && out > 0 {
				return out
			}
		}
	}
	return 0
}

func stringsFromMap(values map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		var out []string
		switch v := value.(type) {
		case string:
			out = strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '，' || r == '/' })
		case []any:
			for _, item := range v {
				switch typed := item.(type) {
				case string:
					out = append(out, typed)
				case map[string]any:
					out = append(out, firstStringFromMap(typed, "name", "title"))
				}
			}
		}
		clean := out[:0]
		for _, item := range out {
			if item = strings.TrimSpace(item); item != "" {
				clean = append(clean, item)
			}
		}
		if len(clean) > 0 {
			return clean
		}
	}
	return nil
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
