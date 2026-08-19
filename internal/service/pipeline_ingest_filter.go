package service

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

const pipelineAdultExtraMinBytes int64 = 200 * 1024 * 1024

var (
	pipelinePureEpisodePattern          = regexp.MustCompile(`^\d{1,4}$`)
	pipelineEpisodeOnlyPattern          = regexp.MustCompile(`(?i)^e(?:p)?[-_. ]?\d{1,4}$`)
	pipelineSeasonEpisodePattern        = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s\d{1,2}e\d{1,4}(?:[^a-z0-9]|$)`)
	pipelineNamedEpisodePattern         = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])ep\s*[-_.]?\s*\d{1,4}(?:[^a-z0-9]|$)`)
	pipelineIngestChineseEpisodePattern = regexp.MustCompile(`第\s*\d{1,4}\s*[集话話]`)
)

func pipelineIngestCloudCandidateFilter(req PipelineIngestRequest) cloudCandidateFilter {
	if req.FilterSmallVideoMaxBytes <= 0 && !req.FilterAdultExtras {
		return nil
	}
	return func(candidates []cloudCandidate) ([]cloudCandidate, []cloudIgnoredCandidate) {
		accepted := make([]cloudCandidate, 0, len(candidates))
		ignored := make([]cloudIgnoredCandidate, 0)
		for _, candidate := range candidates {
			reason := pipelineIngestCandidateIgnoreReason(candidate, req)
			if reason == "" {
				accepted = append(accepted, candidate)
				continue
			}
			ignored = append(ignored, cloudIgnoredCandidate{candidate: candidate, reason: reason})
		}
		if req.FilterAdultExtras && normalizePipelineCategory(req.Category) == "adult" {
			accepted, ignored = pipelineFilterAdultExtraCandidates(accepted, ignored)
		}
		sort.Slice(ignored, func(i, j int) bool {
			return ignored[i].candidate.path < ignored[j].candidate.path
		})
		return accepted, ignored
	}
}

func pipelineIngestCandidateIgnoreReason(candidate cloudCandidate, req PipelineIngestRequest) string {
	stem := strings.TrimSuffix(strings.TrimSpace(candidate.name), path.Ext(candidate.name))
	normalized := pipelineNormalizeJunkToken(stem)
	if (req.Category == "movie" || req.Category == "adult") && pipelinePathHasKnownExtraDirectory(candidate.path) {
		return "known_extra_directory"
	}
	if pipelineKnownJunkToken(normalized) {
		return "known_junk_name"
	}
	if req.FilterSmallVideoMaxBytes > 0 && candidate.size > 0 && candidate.size < req.FilterSmallVideoMaxBytes && !pipelineVideoLooksEpisodic(stem) {
		return "small_non_episode_video"
	}
	return ""
}

func pipelineFilterAdultExtraCandidates(accepted []cloudCandidate, ignored []cloudIgnoredCandidate) ([]cloudCandidate, []cloudIgnoredCandidate) {
	maxByParent := make(map[string]int64)
	countByParent := make(map[string]int)
	for _, candidate := range accepted {
		parent := path.Dir(candidate.path)
		countByParent[parent]++
		if candidate.size > maxByParent[parent] {
			maxByParent[parent] = candidate.size
		}
	}
	kept := make([]cloudCandidate, 0, len(accepted))
	for _, candidate := range accepted {
		parent := path.Dir(candidate.path)
		maxSize := maxByParent[parent]
		threshold := pipelineAdultExtraMinBytes
		if maxSize/5 > threshold {
			threshold = maxSize / 5
		}
		if countByParent[parent] > 1 && candidate.size > 0 && candidate.size < maxSize && candidate.size < threshold {
			ignored = append(ignored, cloudIgnoredCandidate{candidate: candidate, reason: "adult_extra_video"})
			continue
		}
		kept = append(kept, candidate)
	}
	return kept, ignored
}

func pipelineIngestIgnoredMediaResults(items []cloudIgnoredCandidate) []PipelineIngestIgnoredMedia {
	results := make([]PipelineIngestIgnoredMedia, 0, len(items))
	for _, item := range items {
		openListPath := pipelineCloudPathToOpenListPath(item.candidate.path)
		if item.reason == "known_extra_directory" {
			if extraPath := pipelineKnownExtraDirectoryPath(openListPath); extraPath != "" {
				openListPath = extraPath
			}
		}
		name := path.Base(openListPath)
		if openListPath == "" || name == "" || name == "." || name == "/" {
			continue
		}
		results = append(results, PipelineIngestIgnoredMedia{
			OpenListPath: openListPath,
			HidePath:     pipelineNormalizeOpenListPath(path.Dir(openListPath)),
			HidePattern:  "^" + regexp.QuoteMeta(name) + "$",
			Reason:       item.reason,
			SizeBytes:    item.candidate.size,
		})
	}
	return results
}

func pipelineKnownExtraDirectoryPath(openListPath string) string {
	parts := strings.Split(strings.Trim(openListPath, "/"), "/")
	for index, part := range parts[:max(0, len(parts)-1)] {
		if pipelineKnownExtraDirectoryToken(pipelineNormalizeJunkToken(part)) {
			return "/" + path.Join(parts[:index+1]...)
		}
	}
	return ""
}

func pipelineVideoLooksEpisodic(stem string) bool {
	bare := strings.Trim(strings.TrimSpace(stem), "[](){}【】（）")
	return pipelinePureEpisodePattern.MatchString(bare) ||
		pipelineEpisodeOnlyPattern.MatchString(bare) ||
		pipelineSeasonEpisodePattern.MatchString(stem) ||
		pipelineNamedEpisodePattern.MatchString(stem) ||
		pipelineIngestChineseEpisodePattern.MatchString(stem)
}

func pipelineNormalizeJunkToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "_", "", "-", "", ".", "")
	return replacer.Replace(value)
}

func pipelineKnownJunkToken(value string) bool {
	switch value {
	case "ad", "ads", "advert", "advertisement", "sample", "trailer", "preview", "promo", "readme",
		"宣传", "宣傳", "预告", "預告", "样片", "樣片", "广告", "廣告":
		return true
	default:
		return false
	}
}

func pipelinePathHasKnownExtraDirectory(value string) bool {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	for _, part := range parts[:max(0, len(parts)-1)] {
		if pipelineKnownExtraDirectoryToken(pipelineNormalizeJunkToken(part)) {
			return true
		}
	}
	return false
}

func pipelineKnownExtraDirectoryToken(value string) bool {
	switch value {
	case "bonus", "extra", "extras", "sample", "samples", "trailer", "trailers", "tokuten", "特典", "映像特典", "花絮", "预告", "預告":
		return true
	default:
		return false
	}
}
