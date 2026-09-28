package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var (
	embyAdultStandardCodeRE   = regexp.MustCompile(`^([A-Z]{2,10})-(\d{2,8})$`)
	embyAdultFC2CodeRE        = regexp.MustCompile(`^FC2-PPV-(\d{5,8})$`)
	embyAdultHEYZOCodeRE      = regexp.MustCompile(`^HEYZO-(\d{3,6})$`)
	embyAdultUncensoredCodeRE = regexp.MustCompile(`^(\d{6})-(\d{3,5})$`)
)

func (e *EmbyService) adultDisplayName(ctx context.Context, m *model.Media, currentName string) string {
	return adultDisplayNameForMedia(m, currentName, e.mediaLooksAdultForEmbyTitle(ctx, m))
}

func adultDisplayNameForMedia(m *model.Media, currentName string, adult bool) string {
	name := strings.TrimSpace(currentName)
	if m == nil || name == "" || !adult {
		return name
	}
	code := firstNonEmpty(
		AdultCodeFromMediaPath(m.Path),
		normalizeAdultCode(m.OriginalName),
		normalizeAdultCode(m.Title),
	)
	if code == "" {
		return name
	}
	if suffix, ok := adultDisplayTitleSuffix(name, code); ok {
		if suffix == "" {
			return code
		}
		return code + " " + suffix
	}
	return code + " " + name
}

func (e *EmbyService) mediaLooksAdultForEmbyTitle(ctx context.Context, m *model.Media) bool {
	if e == nil {
		return false
	}
	if m == nil {
		return false
	}
	if m.NSFW {
		return true
	}
	if e == nil || e.repo == nil || e.repo.Library == nil || strings.TrimSpace(m.LibraryID) == "" {
		return false
	}
	if lib, loaded := embyLibraryFromSnapshot(ctx, m.LibraryID); loaded {
		return lib != nil && LibraryIsAdult(*lib)
	}
	lib, err := e.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil || lib == nil {
		return false
	}
	return LibraryIsAdult(*lib)
}

func adultDisplayTitleSuffix(name, code string) (string, bool) {
	name = strings.TrimSpace(name)
	code = strings.ToUpper(strings.TrimSpace(code))
	if name == "" || code == "" {
		return "", false
	}
	if suffix, ok := adultDisplayTitleSuffixByCodePattern(name, code); ok {
		return cleanAdultDisplaySuffix(suffix), true
	}
	return "", false
}

func adultDisplayTitleSuffixByCodePattern(name, code string) (string, bool) {
	patterns := adultDisplayTitlePrefixPatterns(code)
	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(name); len(match) > 1 {
			return match[1], true
		}
	}
	return "", false
}

func adultDisplayTitlePrefixPatterns(code string) []*regexp.Regexp {
	if match := embyAdultStandardCodeRE.FindStringSubmatch(code); len(match) > 2 {
		prefix := regexp.QuoteMeta(match[1])
		number := regexp.QuoteMeta(strings.TrimLeft(match[2], "0"))
		if number == "" {
			number = "0"
		}
		return []*regexp.Regexp{
			regexp.MustCompile(`(?i)^\s*` + prefix + `[\s._-]*0*` + number + `(.*)$`),
		}
	}
	if match := embyAdultFC2CodeRE.FindStringSubmatch(code); len(match) > 1 {
		number := regexp.QuoteMeta(strings.TrimLeft(match[1], "0"))
		if number == "" {
			number = "0"
		}
		return []*regexp.Regexp{
			regexp.MustCompile(`(?i)^\s*FC2[\s._-]*(?:PPV[\s._-]*)?0*` + number + `(.*)$`),
		}
	}
	if match := embyAdultHEYZOCodeRE.FindStringSubmatch(code); len(match) > 1 {
		number := regexp.QuoteMeta(strings.TrimLeft(match[1], "0"))
		if number == "" {
			number = "0"
		}
		return []*regexp.Regexp{
			regexp.MustCompile(`(?i)^\s*HEYZO[\s._-]*0*` + number + `(.*)$`),
		}
	}
	if match := embyAdultUncensoredCodeRE.FindStringSubmatch(code); len(match) > 2 {
		left := regexp.QuoteMeta(match[1])
		right := regexp.QuoteMeta(strings.TrimLeft(match[2], "0"))
		if right == "" {
			right = "0"
		}
		return []*regexp.Regexp{
			regexp.MustCompile(`(?i)^\s*` + left + `[\s._-]*0*` + right + `(.*)$`),
		}
	}
	return nil
}

func cleanAdultDisplaySuffix(suffix string) string {
	suffix = strings.Trim(strings.TrimSpace(suffix), " ._-")
	if strings.EqualFold(suffix, "ch") {
		return ""
	}
	lower := strings.ToLower(suffix)
	if strings.HasPrefix(lower, "ch") && (len(suffix) == 2 || strings.ContainsRune(" ._-", rune(suffix[2]))) {
		suffix = strings.Trim(strings.TrimSpace(suffix[2:]), " ._-")
	}
	return suffix
}
