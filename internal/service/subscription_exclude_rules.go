package service

import (
	"strings"
	"unicode"
)

// defaultExcludeWords 是默认过滤的「垃圾版本」排除清单，对所有订阅生效。
// 拉丁词在 containsAnyExcludeToken 里按词边界匹配以避免子串误伤。
const defaultExcludeWords = "cam,ts,tc,telesync,telecine,hdcam,hdts,枪版,抢先,抢鲜,预告,trailer,sample,hr,h&r,hit and run,hit&run,hit-and-run,禁转,禁止转载,禁下,禁止下载"

// defaultCompatibilityExcludeWords 是面向自动订阅的兼容性默认排除清单。
// 仅在用户未真正自定义排除词时启用，避免默认命中 DoVi/H.265/10bit/杜比音轨等版本。
const defaultCompatibilityExcludeWords = "dovi,dv,dolby vision,dolby,杜比视界,杜比,h265,h.265,h-265,h_265,h 265,hevc,x265,10bit,10-bit,10 bit,hi10p,atmos,truehd,ddp,dd+,eac3"

func containsAnyToken(titleFold, csv string) bool {
	for _, token := range strings.FieldsFunc(strings.ToLower(csv), func(r rune) bool {
		return r == ',' || r == '/' || r == '|' || r == ';' || r == '，'
	}) {
		token = strings.TrimSpace(token)
		if token != "" && strings.Contains(titleFold, token) {
			return true
		}
	}
	return false
}

// containsAnyExcludeToken 用于排除词匹配：纯 ASCII 字母数字的词按词边界匹配（避免 "ts"
// 误伤 "tsukihime"、"cam" 误伤 "camp" 之类的子串误判），含 CJK/符号的词仍按子串匹配。
func containsAnyExcludeToken(titleFold, csv string) bool {
	for _, token := range excludeWordTokens(csv) {
		if matchesExcludeToken(titleFold, token) {
			return true
		}
	}
	return false
}

func excludeWordTokens(csv string) []string {
	parts := make([]string, 0)
	for _, token := range strings.FieldsFunc(strings.ToLower(csv), isExcludeSeparator) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		parts = append(parts, token)
		if shouldExpandDottedExcludeToken(token) {
			parts = append(parts, dottedExcludeTokenParts(token)...)
		}
	}
	return parts
}

func isExcludeSeparator(r rune) bool {
	switch r {
	case ',', '/', '|', ';', '，', '、', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

func shouldExpandDottedExcludeToken(token string) bool {
	return strings.Count(token, ".") >= 2
}

func dottedExcludeTokenParts(token string) []string {
	rawParts := strings.Split(token, ".")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if len(part) < 2 || isDigitsOnly(part) {
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

func isDigitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func matchesExcludeToken(titleFold, token string) bool {
	if token == "" {
		return false
	}
	if isASCIIWordToken(token) {
		return matchesWordBoundary(titleFold, token) || matchesReleasePrefixToken(titleFold, token)
	}
	return strings.Contains(titleFold, token)
}

func isASCIIWordToken(token string) bool {
	for _, r := range token {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return token != ""
}

// matchesWordBoundary 判断 token 是否作为独立词出现在 title 中，词边界为「非字母数字」。
func matchesWordBoundary(titleFold, token string) bool {
	from := 0
	for {
		idx := strings.Index(titleFold[from:], token)
		if idx < 0 {
			return false
		}
		start := from + idx
		end := start + len(token)
		leftOK := start == 0 || !isASCIIAlnumByte(titleFold[start-1])
		rightOK := end >= len(titleFold) || !isASCIIAlnumByte(titleFold[end])
		if leftOK && rightOK {
			return true
		}
		from = start + 1
		if from >= len(titleFold) {
			return false
		}
	}
}

func matchesReleasePrefixToken(titleFold, token string) bool {
	if !isReleasePrefixExcludeToken(token) {
		return false
	}
	from := 0
	for {
		idx := strings.Index(titleFold[from:], token)
		if idx < 0 {
			return false
		}
		start := from + idx
		end := start + len(token)
		leftOK := start == 0 || !isASCIIAlnumByte(titleFold[start-1])
		if leftOK && releasePrefixSuffixOK(token, titleFold[end:]) {
			return true
		}
		from = start + 1
		if from >= len(titleFold) {
			return false
		}
	}
}

func isReleasePrefixExcludeToken(token string) bool {
	switch token {
	case "ddp", "dolby":
		return true
	default:
		return false
	}
}

func releasePrefixSuffixOK(token, suffix string) bool {
	if suffix == "" {
		return false
	}
	switch token {
	case "ddp":
		return isASCIIDigitByte(suffix[0])
	case "dolby":
		return strings.HasPrefix(suffix, "vision") ||
			strings.HasPrefix(suffix, "atmos") ||
			strings.HasPrefix(suffix, "digital")
	default:
		return false
	}
}

func isASCIIAlnumByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || isASCIIDigitByte(b)
}

func isASCIIDigitByte(b byte) bool {
	return b >= '0' && b <= '9'
}
