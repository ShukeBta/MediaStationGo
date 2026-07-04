package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// transmissionStateStr 将 Transmission 状态码转为可读字符串。
func transmissionStateStr(status int) string {
	switch status {
	case 0:
		return "stopped"
	case 1:
		return "check_pending"
	case 2:
		return "checking"
	case 3:
		return "download_pending"
	case 4:
		return "downloading"
	case 5:
		return "seed_pending"
	case 6:
		return "seeding"
	default:
		return "unknown"
	}
}

// toInt64 安全地将 interface{} 转为 int64。
func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case int64:
		return val
	case json.Number:
		n, _ := val.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	default:
		return 0
	}
}

// toFloat64 安全地将 interface{} 转为 float64。
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		n, _ := val.Float64()
		return n
	case string:
		n, _ := strconv.ParseFloat(val, 64)
		return n
	default:
		return 0
	}
}

// strVal 安全地提取字符串。
func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// toJSONLabels 将 Transmission labels 转为逗号分隔字符串。
func toJSONLabels(v interface{}) string {
	if v == nil {
		return ""
	}
	arr, ok := v.([]interface{})
	if !ok {
		return ""
	}
	labels := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			labels = append(labels, s)
		}
	}
	return strings.Join(labels, ",")
}
