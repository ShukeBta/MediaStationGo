package service

import (
	"path/filepath"
	"strings"
)

func AdultCodeFromMediaPath(path string) string {
	if code := normalizeAdultCode(filepath.Base(path)); code != "" {
		return code
	}
	return normalizeAdultCode(path)
}

func normalizeAdultCode(input string) string {
	input = strings.ToUpper(strings.TrimSpace(input))
	if input == "" {
		return ""
	}
	input = strings.ReplaceAll(input, "_", "-")
	if m := adultFC2Pattern.FindStringSubmatch(input); len(m) > 1 {
		return "FC2-PPV-" + m[1]
	}
	if m := adultHEYZOPattern.FindStringSubmatch(input); len(m) > 1 {
		return "HEYZO-" + m[1]
	}
	if m := adultUncensoredPattern.FindStringSubmatch(input); len(m) > 2 {
		return m[1] + "-" + m[2]
	}
	for _, m := range adultStandardPattern.FindAllStringSubmatch(input, -1) {
		if len(m) < 3 {
			continue
		}
		prefix := strings.TrimSpace(m[1])
		if _, excluded := adultExcludedPrefixes[prefix]; excluded {
			continue
		}
		return prefix + "-" + m[2]
	}
	return ""
}

// CleanAdultTitle 去除标题中冗余的前缀番号、方括号、分隔符，提取纯标题内容
func CleanAdultTitle(code, rawTitle string) string {
	rawTitle = strings.TrimSpace(rawTitle)
	code = normalizeAdultCode(code)
	if rawTitle == "" {
		return ""
	}
	clean := rawTitle
	for {
		trimmed := strings.TrimSpace(clean)
		if trimmed == "" {
			return ""
		}
		if code != "" {
			upper := strings.ToUpper(trimmed)
			upperCode := strings.ToUpper(code)
			if strings.HasPrefix(upper, upperCode) {
				clean = strings.TrimSpace(trimmed[len(upperCode):])
				clean = strings.TrimLeft(clean, "-_ :：]】")
				continue
			}
		}
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "【") {
			idx := strings.IndexAny(trimmed, "]】")
			if idx > 0 {
				bracketContent := normalizeAdultCode(trimmed[1:idx])
				if bracketContent != "" && (code == "" || bracketContent == code) {
					clean = strings.TrimSpace(trimmed[idx+len("]"):])
					clean = strings.TrimLeft(clean, "-_ :：")
					continue
				}
			}
		}
		break
	}
	return strings.TrimSpace(clean)
}

// FormatAdultTitle 格式化番号标题：把番号放在最前面，如 "IPZZ-090-具体标题" 或 "IPZZ-090"
func FormatAdultTitle(code, rawTitle string) string {
	code = normalizeAdultCode(code)
	if code == "" {
		code = normalizeAdultCode(rawTitle)
	}
	clean := CleanAdultTitle(code, rawTitle)
	if code == "" {
		return clean
	}
	if clean == "" || strings.EqualFold(clean, code) {
		return code
	}
	return code + "-" + clean
}
