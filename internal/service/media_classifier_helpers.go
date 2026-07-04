package service

import "strings"

func normalizeTokens(values ...string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '/' || r == '|' || r == ';'
		}) {
			part = strings.ToUpper(strings.TrimSpace(part))
			if part != "" {
				out[part] = struct{}{}
			}
		}
	}
	return out
}

func hasAny(values map[string]struct{}, needles ...string) bool {
	for _, needle := range needles {
		if _, ok := values[strings.ToUpper(needle)]; ok {
			return true
		}
	}
	return false
}

func containsAnyText(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func containsHan(text string) bool {
	for _, r := range text {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

func containsJapaneseKana(text string) bool {
	for _, r := range text {
		if (r >= '\u3040' && r <= '\u30ff') || (r >= '\u31f0' && r <= '\u31ff') {
			return true
		}
	}
	return false
}

func containsKoreanHangul(text string) bool {
	for _, r := range text {
		if (r >= '\uac00' && r <= '\ud7af') || (r >= '\u1100' && r <= '\u11ff') || (r >= '\u3130' && r <= '\u318f') {
			return true
		}
	}
	return false
}

func containsLatin(text string) bool {
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func isDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func categoryName(categories map[string]string, key, fallback string) string {
	if categories != nil {
		if name := strings.TrimSpace(categories[key]); name != "" {
			return name
		}
	}
	return fallback
}
