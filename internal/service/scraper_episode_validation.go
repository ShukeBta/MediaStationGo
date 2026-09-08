package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// validateEpisodeMatch completes identity checks before any metadata is written.
// The returned copy prevents a failed lookup from changing the caller's row.
func (s *ScraperService) validateEpisodeMatch(ctx context.Context, media *model.Media, lib *model.Library, match *Match, options ScrapeOptions) (*model.Media, error) {
	if media == nil || match == nil {
		return nil, fmt.Errorf("media and match are required")
	}
	mediaType := s.determineMediaTypeForMedia(lib, media, match)
	if options.manualEpisodeIdentity != nil && (mediaType != "tv" || match.TMDbID <= 0) {
		return nil, fmt.Errorf("手动指定季集号需要选择 TMDB 剧集条目")
	}
	if options.automaticSelection && mediaType == "movie" && ParseEpisodeEvidence(strings.ReplaceAll(media.Path, "\\", "/")).EpisodeExplicit {
		return nil, fmt.Errorf("路径包含剧集集号，但候选是电影，请手动确认条目")
	}
	if match.TMDbID <= 0 || mediaType != "tv" {
		return media, nil
	}
	if options.automaticSelection && !episodePathTitleTrusted(media.Path, match) {
		return nil, fmt.Errorf("剧名校验未通过：路径与 TMDB %d（%s）不一致，请手动选择条目", match.TMDbID, match.Title)
	}
	copy := *media
	if options.manualEpisodeIdentity != nil && !options.automaticSelection {
		copy.SeasonNum = options.manualEpisodeIdentity.Season
		copy.EpisodeNum = options.manualEpisodeIdentity.Episode
		copy.EpisodeEndNum = options.manualEpisodeEnd
		copy.EpisodePartNum = options.manualEpisodePart
	} else if err := episodeIdentityFromPath(&copy); err != nil {
		return nil, err
	}
	if copy.EpisodeNum <= 0 {
		return nil, fmt.Errorf("无法确认季集号，请指定季集后重新刮削")
	}
	key := [2]int{match.TMDbID, copy.SeasonNum}
	if err := options.episodeFailures[key]; err != nil {
		return nil, err
	}
	episodes, ok := options.episodeValidation[key]
	if !ok {
		if s.tmdb == nil || !s.tmdb.Enabled() {
			return nil, fmt.Errorf("TMDB 不可用，季集校验未完成")
		}
		callCtx, cancel := context.WithTimeout(ctx, tmdbDetailsTimeout)
		var err error
		episodes, err = s.tmdb.GetTVSeasonEpisodeDetails(callCtx, key[0], key[1])
		cancel()
		if err != nil {
			err = fmt.Errorf("TMDB 季集校验请求失败，未修改数据: %w", err)
			if options.episodeFailures != nil {
				options.episodeFailures[key] = err
			}
			return nil, err
		}
		if episodes == nil {
			return nil, fmt.Errorf("TMDB 未返回季详情，校验未完成")
		}
		if options.episodeValidation != nil {
			options.episodeValidation[key] = episodes
		}
	}
	end := copy.EpisodeNum
	if copy.EpisodeEndNum > end {
		end = copy.EpisodeEndNum
	}
	for number := copy.EpisodeNum; number <= end; number++ {
		if episode := episodes[number]; episode == nil || strings.TrimSpace(episode.Name) == "" {
			return nil, fmt.Errorf("TMDB %d 缺少 S%02dE%02d 的有效详情，请确认季集映射", key[0], key[1], number)
		}
	}
	return &copy, nil
}

func episodePathTitleTrusted(path string, match *Match) bool {
	if match == nil {
		return false
	}
	trusted := func(value string) bool {
		key := metadataTrustKey(value)
		if key == "" {
			return false
		}
		for _, name := range append([]string{match.Title, match.OriginalName}, match.Aliases...) {
			if key == metadataTrustKey(name) {
				return true
			}
		}
		return false
	}
	_, hints := pathHintMetadata(path, true)
	if hints.TMDbID > 0 {
		return hints.TMDbID == match.TMDbID
	}
	// A filename with explicit episode markers is also path evidence when a
	// file sits directly in the library root or a synthetic wrapper directory.
	name := filepath.Base(strings.ReplaceAll(path, "\\", "/"))
	if evidence := ParseEpisodeEvidence(name); evidence.EpisodeExplicit {
		filenameTitle := normalizeSeriesPathTitle(strings.TrimSuffix(name, filepath.Ext(name)))
		key := metadataTrustKey(filenameTitle)
		if key != "" && trusted(filenameTitle) {
			return true
		}
	}
	title := seriesTitleFromMediaPath(path)
	key := metadataTrustKey(title)
	if key == "" {
		return false
	}
	// SearchKeyword is the request, not evidence that TMDB returned this show.
	return trusted(title)
}

func episodeIdentityFromPath(media *model.Media) error {
	path := strings.ReplaceAll(media.Path, "\\", "/")
	evidence := ParseEpisodeEvidence(path)
	if evidence.Issue != "" {
		return fmt.Errorf("季集映射需要确认（%s），请明确指定季集号", evidence.Issue)
	}
	for _, part := range strings.Split(path, "/") {
		if part == "Extras" || part == "extras" || part == "Bonus" || part == "bonus" {
			return fmt.Errorf("花絮目录不能自动归入正片或特别篇，请明确指定映射")
		}
	}
	if !evidence.SeasonExplicit || !evidence.EpisodeExplicit {
		return fmt.Errorf("路径缺少明确季集号，不能沿用旧值或默认第一季，请指定季集号")
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if patSEnE.MatchString(name) || patNxE.MatchString(name) {
		if parentSeason, ok := seasonFromParents(path); ok && parentSeason != evidence.Season {
			return fmt.Errorf("路径季号冲突：文件名第 %d 季，目录第 %d 季，请确认季集映射", evidence.Season, parentSeason)
		}
	}
	if evidence.SeasonExplicit {
		media.SeasonNum = evidence.Season
	}
	if evidence.EpisodeExplicit {
		media.EpisodeNum = evidence.Episode
	}
	media.EpisodeEndNum = 0
	media.EpisodePartNum = 0
	return nil
}
