package service

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func configureMediaSeriesKeys(repo *repository.MediaRepository) {
	repo.SetSeriesKeyFunc(MediaSeriesKey)
	repo.SetSeriesBindingFunc(mediaSeriesBinding)
}

func mediaSeriesBinding(media model.Media) repository.SeriesBinding {
	identity := confirmedSeriesIdentity(media)
	if !mediaLooksEpisodicForGrouping(media) || media.LibraryID == "" ||
		(media.PartGroupKey != "" && !strings.HasPrefix(media.PartGroupKey, autoMediaPartPrefix)) {
		return repository.SeriesBinding{}
	}
	path := strings.TrimSpace(strings.ReplaceAll(media.Path, "\\", "/"))
	end := strings.LastIndexByte(path, '/')
	if end < 0 || !seriesPathPartLooksLikeFile(path[end+1:]) {
		return repository.SeriesBinding{}
	}
	directory := path[:end]
	for directory != "" {
		base := pathBaseSlash(directory)
		if !seriesPathPartIsEpisodeContainer(base) && !singleFileWrapperDirectory(base, path[end+1:]) {
			break
		}
		parent := strings.LastIndexByte(directory, '/')
		if parent < 0 {
			return repository.SeriesBinding{}
		}
		directory = directory[:parent]
	}
	title := normalizeSeriesPathTitle(pathBaseSlash(directory))
	if directory == "" || title == "" || unsafeAutomaticEpisodeQuery(title) ||
		seriesPathPartIsGenericContainer(pathBaseSlash(directory)) || seriesTitleIsGenericContainer(title, media) {
		return repository.SeriesBinding{}
	}
	scope := sha256.Sum256([]byte(seriesFingerprint(media.LibraryID, directory)))
	return repository.SeriesBinding{Scope: fmt.Sprintf("%x", scope), Directory: directory, Key: identity}
}

func confirmedSeriesIdentity(media model.Media) string {
	if !mediaLooksEpisodicForGrouping(media) {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(media.ScrapeStatus), "matched") {
		switch {
		case media.TMDbID > 0:
			return fmt.Sprintf("tmdb:%d", media.TMDbID)
		case media.BangumiID > 0:
			return fmt.Sprintf("bgm:%d", media.BangumiID)
		case strings.TrimSpace(media.DoubanID) != "":
			return "douban:" + strings.TrimSpace(media.DoubanID)
		case strings.TrimSpace(media.TheTVDBID) != "":
			return "thetvdb:" + strings.TrimSpace(media.TheTVDBID)
		}
	}
	if id := strings.TrimSpace(media.SeriesID); id != "" {
		return "series:" + id
	}
	return ""
}

// boundSeriesIdentity is shared by the web and Emby projections. A copied
// binding cannot survive a physical library/directory move or a new identity.
func boundSeriesIdentity(media model.Media) string {
	if key := confirmedSeriesIdentity(media); key != "" {
		return key
	}
	if media.SeriesBindingScope == "" || media.SeriesBindingKey == "" {
		return ""
	}
	binding := mediaSeriesBinding(media)
	if binding.Scope != media.SeriesBindingScope || !repository.SeriesBindingCompatible(media, media.SeriesBindingKey) {
		return ""
	}
	return media.SeriesBindingKey
}
