package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func parseRecognitionWordRules(raw string) []recognitionWordRule {
	var out []recognitionWordRule
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		out = append(out, parseRecognitionWordRule(line))
	}
	return out
}

func parseRecognitionWordRule(line string) recognitionWordRule {
	rule := recognitionWordRule{raw: line}
	for _, part := range strings.Split(line, "&&") {
		part = strings.TrimSpace(part)
		switch {
		case strings.Contains(part, "=>"):
			pieces := strings.SplitN(part, "=>", 2)
			rule.replaceFrom = strings.TrimSpace(pieces[0])
			rule.replaceTo = normalizeRecognitionReplacement(strings.TrimSpace(pieces[1]))
		case strings.Contains(part, "<>") && strings.Contains(part, ">>"):
			beforeAfter := strings.SplitN(part, ">>", 2)
			bounds := strings.SplitN(beforeAfter[0], "<>", 2)
			rule.offsetLeft = strings.TrimSpace(bounds[0])
			rule.offsetRight = strings.TrimSpace(bounds[1])
			rule.offsetExpr = strings.TrimSpace(beforeAfter[1])
		default:
			rule.block = part
		}
	}
	return rule
}

func normalizeRecognitionReplacement(value string) string {
	re := regexp.MustCompile(`\\([0-9]+)`)
	return re.ReplaceAllString(value, "$$$1")
}

func applyRecognitionWordRules(raw string, rules []recognitionWordRule) string {
	out := strings.TrimSpace(raw)
	for _, rule := range rules {
		if rule.block != "" {
			out = applyRecognitionBlock(out, rule.block)
		}
		if rule.replaceFrom != "" {
			out = applyRecognitionReplace(out, rule.replaceFrom, rule.replaceTo)
		}
		if rule.offsetLeft != "" || rule.offsetRight != "" {
			out = applyRecognitionOffset(out, rule.offsetLeft, rule.offsetRight, rule.offsetExpr)
		}
	}
	return strings.Join(strings.Fields(out), " ")
}

func applyRecognitionBlock(raw, block string) string {
	if re, err := regexp.Compile(block); err == nil {
		return re.ReplaceAllString(raw, " ")
	}
	return strings.ReplaceAll(raw, block, " ")
}

func applyRecognitionReplace(raw, from, to string) string {
	if re, err := regexp.Compile(from); err == nil {
		return re.ReplaceAllString(raw, to)
	}
	return strings.ReplaceAll(raw, from, to)
}

func applyRecognitionOffset(raw, left, right, expr string) string {
	if strings.TrimSpace(expr) == "" {
		return raw
	}
	leftPattern := firstNonEmpty(left, `^`)
	rightPattern := firstNonEmpty(right, `$`)
	re, err := regexp.Compile(`(?i)(` + leftPattern + `)(\d{1,5})(` + rightPattern + `)`)
	if err != nil {
		return raw
	}
	return re.ReplaceAllStringFunc(raw, func(match string) string {
		return applyRecognitionOffsetMatch(re, match, expr)
	})
}

func applyRecognitionOffsetMatch(re *regexp.Regexp, match, expr string) string {
	parts := re.FindStringSubmatch(match)
	if len(parts) < 4 {
		return match
	}
	ep, err := strconv.Atoi(parts[2])
	if err != nil {
		return match
	}
	next, ok := evalRecognitionEpisodeExpr(expr, ep)
	if !ok || next < 0 {
		return match
	}
	format := "%d"
	if width := len(parts[2]); width > 1 && width <= 2 {
		format = "%0" + strconv.Itoa(width) + "d"
	}
	return parts[1] + fmt.Sprintf(format, next) + parts[3]
}

func evalRecognitionEpisodeExpr(expr string, ep int) (int, bool) {
	value := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(expr), " ", ""))
	if value == "" || value == "EP" {
		return ep, true
	}
	if n, err := strconv.Atoi(value); err == nil {
		return n, true
	}
	if out, ok := evalRecognitionEpisodeAddSub(value, ep); ok {
		return out, true
	}
	return evalRecognitionEpisodeMul(value, ep)
}

func evalRecognitionEpisodeAddSub(value string, ep int) (int, bool) {
	for _, op := range []string{"+", "-"} {
		if !strings.HasPrefix(value, "EP"+op) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(value, "EP"+op))
		if err != nil {
			return 0, false
		}
		if op == "+" {
			return ep + n, true
		}
		return ep - n, true
	}
	return 0, false
}

func evalRecognitionEpisodeMul(value string, ep int) (int, bool) {
	if strings.HasPrefix(value, "EP*") {
		n, err := strconv.Atoi(strings.TrimPrefix(value, "EP*"))
		return ep * n, err == nil
	}
	if strings.HasSuffix(value, "*EP") {
		n, err := strconv.Atoi(strings.TrimSuffix(value, "*EP"))
		return ep * n, err == nil
	}
	return 0, false
}
