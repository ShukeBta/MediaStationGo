package service

import (
	"fmt"
	"regexp"
	"strings"
)

// srtToVTT performs the minimal SRT -> WebVTT transformation: prepend
// "WEBVTT\n\n" and replace ',' with '.' in the timecode separators.
func srtToVTT(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	out := strings.Builder{}
	out.WriteString("WEBVTT\n\n")
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "-->") {
			line = strings.ReplaceAll(line, ",", ".")
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

// assToVTT extracts the dialogue lines from an ASS/SSA subtitle. Styling
// is dropped so the browser <track> element can still display text.
func assToVTT(body string) string {
	out := strings.Builder{}
	out.WriteString("WEBVTT\n\n")
	for i, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Dialogue:") {
			continue
		}
		parts := strings.SplitN(line, ",", 10)
		if len(parts) < 10 {
			continue
		}
		fmt.Fprintf(&out, "%d\n%s --> %s\n%s\n\n",
			i,
			normaliseTimecode(parts[1]),
			normaliseTimecode(parts[2]),
			stripASSTags(parts[9]),
		)
	}
	return out.String()
}

func normaliseTimecode(t string) string {
	t = strings.TrimSpace(t)
	parts := strings.Split(t, ":")
	if len(parts) != 3 {
		return t
	}
	hh := parts[0]
	if len(hh) == 1 {
		hh = "0" + hh
	}
	return hh + ":" + parts[1] + ":" + strings.ReplaceAll(parts[2], ".", ".")
}

var assTag = regexp.MustCompile(`\{[^}]*\}`)

func stripASSTags(s string) string {
	return assTag.ReplaceAllString(s, "")
}
