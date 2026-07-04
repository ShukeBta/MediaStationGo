package service

import (
	"path"
	"strings"
)

func parseSTRMTreeText(raw string) []string {
	out, _ := parseSTRMTreeTextWithIgnored(raw)
	return out
}

func parseSTRMTreeTextWithIgnored(raw string) ([]string, []string) {
	var out []string
	var ignored []string
	stack := make([]string, 0, 8)
	plainIndents := make([]int, 0, 8)
	rootOffset := 0
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if item, level, ok := parseSTRMTreeLine(line); ok {
			plainIndents = plainIndents[:0]
			level += rootOffset
			stack = stack[:min(level, len(stack))]
			if strmTreeItemIsVideoSource(item) {
				out = append(out, strmTreeJoinedSource(stack, item))
				continue
			}
			if strmTreeItemIsIgnoredFileSource(item) {
				ignored = append(ignored, strmTreeJoinedSource(stack, item))
				continue
			}
			stack = append(stack, item)
			continue
		}
		if item, level, ok := parseSTRMTreeContinuationLine(line, len(plainIndents) > 0); ok {
			plainIndents = plainIndents[:0]
			level += rootOffset
			stack = stack[:min(level, len(stack))]
			if strmTreeItemIsVideoSource(item) {
				out = append(out, path.Join(append(append([]string{}, stack...), item)...))
			} else if strmTreeItemIsIgnoredFileSource(item) {
				ignored = append(ignored, path.Join(append(append([]string{}, stack...), item)...))
			}
			continue
		}
		item := cleanSTRMTreeItemName(line)
		if indent := strmTreePlainIndent(line); indent > 0 && !strings.ContainsAny(item, `/\`) {
			for len(plainIndents) > 0 && indent <= plainIndents[len(plainIndents)-1] {
				plainIndents = plainIndents[:len(plainIndents)-1]
			}
			level := rootOffset + len(plainIndents)
			stack = stack[:min(level, len(stack))]
			if strmTreeItemIsVideoSource(item) {
				out = append(out, strmTreeJoinedSource(stack, item))
				continue
			}
			if strmTreeItemIsIgnoredFileSource(item) {
				ignored = append(ignored, strmTreeJoinedSource(stack, item))
				continue
			}
			stack = append(stack, item)
			plainIndents = append(plainIndents, indent)
			continue
		}
		if strmTreeItemIsVideoSource(item) || strings.ContainsAny(item, `/\`) {
			plainIndents = plainIndents[:0]
			if strmTreeItemIsVideoSource(item) {
				out = append(out, item)
			} else if strmTreeItemIsIgnoredFileSource(item) {
				ignored = append(ignored, item)
			}
			continue
		}
		if strmTreeItemIsIgnoredFileSource(item) {
			ignored = append(ignored, item)
			continue
		}
		stack = []string{item}
		plainIndents = plainIndents[:0]
		rootOffset = 1
	}
	return out, ignored
}

func parseSTRMTreeLine(line string) (string, int, bool) {
	if idx := strings.Index(line, "──"); idx >= 0 {
		prefix := line[:idx]
		level := strmTreeIndentLevelWithWidth(prefix, 4)
		item := cleanSTRMTreeItemName(strings.Trim(strings.TrimSpace(line[idx+len("──"):]), "─- "))
		return item, level, item != ""
	}
	if idx := strings.Index(line, "─"); idx >= 0 {
		prefix := line[:idx]
		level := strmTreeIndentLevelWithWidth(prefix, 3)
		item := cleanSTRMTreeItemName(strings.Trim(strings.TrimSpace(line[idx+len("─"):]), "─- "))
		return item, level, item != ""
	}
	for _, marker := range []string{"|--", "+--", "`--"} {
		if idx := strings.Index(line, marker); idx >= 0 {
			item := cleanSTRMTreeItemName(line[idx+len(marker):])
			return item, strmTreeIndentLevel(line[:idx]), item != ""
		}
	}
	return "", 0, false
}

func parseSTRMTreeContinuationLine(line string, plainTreeActive bool) (string, int, bool) {
	prefixLen := 0
	hasGuide := false
	for _, r := range line {
		switch r {
		case ' ', '\t', '│', '|':
			if r == '│' || r == '|' {
				hasGuide = true
			}
			prefixLen += len(string(r))
		default:
			if plainTreeActive && !hasGuide {
				return "", 0, false
			}
			item := cleanSTRMTreeItemName(line[prefixLen:])
			if item == "" || (!strmTreeItemIsVideoSource(item) && !strmTreeItemIsIgnoredFileSource(item)) {
				return "", 0, false
			}
			return item, strmTreeIndentLevel(line[:prefixLen]), true
		}
	}
	return "", 0, false
}

func strmTreeIndentLevel(prefix string) int {
	return strmTreeIndentLevelWithWidth(prefix, 4)
}

func strmTreeIndentLevelWithWidth(prefix string, width int) int {
	if prefix == "" {
		return 0
	}
	if width <= 0 {
		width = 4
	}
	verticals := strings.Count(prefix, "│") + strings.Count(prefix, "|")
	runeLen := len([]rune(strings.ReplaceAll(strings.ReplaceAll(prefix, "│", " "), "|", " ")))
	byWidth := 0
	if runeLen > 0 {
		byWidth = (runeLen - 1) / width
	}
	if verticals > byWidth {
		return verticals
	}
	return byWidth
}

func strmTreePlainIndent(line string) int {
	indent := 0
	for _, r := range line {
		switch r {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent
		}
	}
	return indent
}

func strmTreeItemIsVideoSource(item string) bool {
	if strmTreeSourceIsVideo(item) {
		return true
	}
	source := normalizeSTRMTreeSourceWithProvider(item, "openlist")
	return source.Path != "" && strmTreeSourceIsVideo(source.Path)
}

func strmTreeItemIsIgnoredFileSource(item string) bool {
	_, ok := strmTreeIgnoredFileLikeSource(item)
	return ok
}

func strmTreeJoinedSource(stack []string, item string) string {
	if strmTreeItemIsAbsoluteSource(item) {
		return item
	}
	return path.Join(append(append([]string{}, stack...), item)...)
}

func strmTreeItemIsAbsoluteSource(item string) bool {
	value := strings.ToLower(strings.TrimSpace(item))
	return strings.Contains(value, "://") || strings.HasPrefix(value, "/api/")
}
