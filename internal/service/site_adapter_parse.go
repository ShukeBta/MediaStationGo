package service

import (
	"regexp"
	"strconv"
	"strings"
)

func mteamCodeOK(code any) bool {
	codeStr := mteamCodeString(code)
	return codeStr == "0" || codeStr == "200"
}

func mteamCodeString(code any) string {
	switch v := code.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.Itoa(int(v))
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

// parseSizeString 将带单位的字符串转换为字节数。
func parseSizeString(value string, unit string) int64 {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	switch strings.TrimSuffix(strings.ToLower(unit), "i") {
	case "kb":
		return int64(v * 1024)
	case "mb":
		return int64(v * 1024 * 1024)
	case "gb":
		return int64(v * 1024 * 1024 * 1024)
	case "tb":
		return int64(v * 1024 * 1024 * 1024 * 1024)
	default:
		return int64(v)
	}
}

// stripHTML 移除 HTML 标签。
func stripHTML(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(s, "")
}

func firstImageURLFromHTML(baseURL, html string) string {
	re := regexp.MustCompile(`(?i)<img[^>]+src=["' ]?([^"' >]+)`)
	match := re.FindStringSubmatch(html)
	if len(match) < 2 {
		return ""
	}
	return absolutizeURL(baseURL, strings.TrimSpace(match[1]))
}

func collectSiteCategoriesFromJSON(payload any, siteType, parentID string) []SiteCategory {
	out := []SiteCategory{}
	var walk func(any, string)
	walk = func(value any, parent string) {
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				walk(item, parent)
			}
		case map[string]any:
			id := firstJSONText(v, "id", "ID", "category_id", "categoryId", "value")
			name := firstJSONText(v, "name", "Name", "title", "label", "text")
			group := firstJSONText(v, "group", "type", "parent", "parent_name")
			if id != "" || name != "" {
				if name == "" {
					name = id
				}
				out = append(out, SiteCategory{ID: id, Name: name, Group: group, ParentID: parent, SiteType: siteType, Adult: looksAdultPTResource(group + " " + name)})
			}
			nextParent := id
			if nextParent == "" {
				nextParent = parent
			}
			for _, key := range []string{"children", "items", "data", "categories", "subcategories"} {
				if child, ok := v[key]; ok {
					walk(child, nextParent)
				}
			}
		}
	}
	walk(payload, parentID)
	return out
}

func firstJSONText(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := obj[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case float64:
			return strconv.Itoa(int(v))
		case int:
			return strconv.Itoa(v)
		}
	}
	return ""
}

func dedupeSiteCategories(items []SiteCategory) []SiteCategory {
	seen := map[string]struct{}{}
	out := make([]SiteCategory, 0, len(items))
	for _, item := range items {
		key := strings.ToLower(strings.Join([]string{item.SiteType, item.ParentID, item.ID, item.Name}, "\x00"))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}
