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
	// The final segment may be "ss" or "ss.mm"/"ss.mmm"/"ss.frames". WebVTT
	// requires hh:mm:ss.mmm, so pad the millisecond part to 3 digits and cap
	// at 3. Leaving 2-digit fractions (common in ASS: "ss.cc") breaks strict
	// parsers.
	last := strings.SplitN(parts[2], ".", 2)
	sec := last[0]
	if len(sec) == 1 {
		sec = "0" + sec
	}
	frac := ""
	if len(last) == 2 {
		frac = last[1]
		// ASS centiseconds -> WebVTT milliseconds: append a trailing zero so
		// "51" becomes "510", "123" stays "123".
		if len(frac) < 3 {
			frac += strings.Repeat("0", 3-len(frac))
		} else if len(frac) > 3 {
			frac = frac[:3]
		}
	}
	out := hh + ":" + parts[1] + ":" + sec
	if frac != "" {
		out += "." + frac
	}
	return out
}

var assTag = regexp.MustCompile(`\{[^}]*\}`)

func stripASSTags(s string) string {
	return assTag.ReplaceAllString(s, "")
}
