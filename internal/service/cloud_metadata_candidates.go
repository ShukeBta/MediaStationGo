package service

import (
	"path/filepath"
	"strconv"
	"strings"
)

func cloudShowNFOCandidates(displayDir string) []string {
	names := []string{"tvshow.nfo", "series.nfo", "show.nfo", "movie.nfo"}
	base := strings.TrimSpace(pathBaseSlash(displayDir))
	if base != "" {
		names = append(names, base+".nfo")
	}
	return names
}

func cloudDirectoryJSONCandidates(displayDir string) []string {
	names := []string{"movie.json", "metadata.json", "tvshow.json", "series.json", "show.json"}
	base := strings.TrimSpace(pathBaseSlash(displayDir))
	if base != "" {
		names = append(names, base+".json", base+"-metadata.json", base+".metadata.json", base+"-mediainfo.json", base+".mediainfo.json")
	}
	return names
}

func cloudFileJSONCandidates(fileName, base string) []string {
	if base == "" {
		base = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(fileName, filepath.Ext(fileName))))
	}
	cleanBases := cloudCleanArtworkBases(fileName)
	bases := uniqueCloudArtworkNames(append([]string{base}, cleanBases...)...)
	out := make([]string, 0, len(bases)*5+2)
	for _, value := range bases {
		out = append(out, value+".json", value+"-metadata.json", value+".metadata.json", value+"-mediainfo.json", value+".mediainfo.json")
	}
	return append(out, "movie.json", "metadata.json")
}

func cloudJSONRefByName(sidecars cloudSidecarSet, name string) string {
	name = normalizeCloudArtworkName(name)
	if name == "" || isHTTPURL(name) {
		return ""
	}
	if ref := sidecars.jsonByName[strings.ToLower(name)]; ref != "" {
		return ref
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return sidecars.jsonByBase[strings.ToLower(base)]
}

func cloudFileArtworkBases(displayPath, fileName, base string) []string {
	return uniqueCloudArtworkNames(append(
		[]string{base},
		append(cloudCleanArtworkBases(fileName), cloudDirectoryArtworkBases(pathDirSlash(displayPath))...)...,
	)...)
}

func cloudDirectoryArtworkBases(displayDir string) []string {
	base := pathBaseSlash(displayDir)
	return uniqueCloudArtworkNames(append([]string{base}, cloudCleanArtworkBases(base)...)...)
}

func cloudCleanArtworkBases(value string) []string {
	title, year := CleanQuery(value)
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	out := []string{title}
	if year > 0 {
		yearText := strconv.Itoa(year)
		out = append(out,
			title+" ("+yearText+")",
			title+"."+yearText,
			title+" "+yearText,
		)
	}
	return out
}

func cloudPosterNameCandidates(bases []string, fallback ...string) []string {
	out := make([]string, 0, len(bases)*7+len(fallback))
	for _, base := range bases {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		out = append(out, base, base+"-poster", base+".poster", base+"-cover", base+".cover", base+"-thumb", base+".thumb")
	}
	return append(out, fallback...)
}

func cloudBackdropNameCandidates(bases []string, fallback ...string) []string {
	out := make([]string, 0, len(bases)*6+len(fallback))
	for _, base := range bases {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		out = append(out, base+"-fanart", base+".fanart", base+"-backdrop", base+".backdrop", base+"-background", base+".background")
	}
	return append(out, fallback...)
}

func uniqueCloudArtworkNames(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
