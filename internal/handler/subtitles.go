// Package handler — subtitle endpoints.
package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type invalidateCloudSubtitleCacheRequest struct {
	MediaID  string `json:"media_id"`
	Provider string `json:"provider"`
}

type backfillCloudSubtitleCacheRequest struct {
	LibraryID string `json:"library_id"`
}

type subtitleSearchRequest struct {
	Limit int `json:"limit"`
}

type subtitleSeasonSearchRequest struct {
	Season   int                                 `json:"season"`
	Title    string                              `json:"title"`
	Limit    int                                 `json:"limit"`
	Episodes []subtitleSeasonApplyEpisodeRequest `json:"episodes"`
}

type subtitleSeasonApplyEpisodeRequest struct {
	MediaID     string `json:"media_id"`
	CandidateID string `json:"candidate_id"`
}

type subtitleSeasonApplyRequest struct {
	SearchSessionID string                              `json:"search_session_id"`
	Season          int                                 `json:"season"`
	Episodes        []subtitleSeasonApplyEpisodeRequest `json:"episodes"`
}

type subtitleSeasonRetryRequest struct {
	MediaIDs []string `json:"media_ids"`
}

type subtitleCandidateRequest struct {
	SearchSessionID string `json:"search_session_id"`
	CandidateID     string `json:"candidate_id"`
}

type subtitleASRRequest struct {
	SourceLanguage      string `json:"source_language"`
	ASRModel            string `json:"asr_model"`
	TranslationProvider string `json:"translation_provider"`
	TranslationModel    string `json:"translation_model"`
}

type subtitleTranslationRequest struct {
	Provider         string   `json:"provider"`
	Model            string   `json:"model"`
	Text             string   `json:"text"`
	Context          []string `json:"context"`
	Glossary         string   `json:"glossary"`
	RetryInstruction string   `json:"retry_instruction"`
}

func listSubtitlesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		media, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil || media == nil || !mediaVisibleForRequest(c, svc, media) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		tracks, err := svc.Subtitle.Discover(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if tracks == nil {
			tracks = []service.SubtitleTrack{}
		}
		c.JSON(http.StatusOK, gin.H{"tracks": tracks})
	}
}

func serveSubtitleHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		media, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil || media == nil || !mediaVisibleForRequest(c, svc, media) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		path := c.Query("path")
		if path == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing path"})
			return
		}
		c.Header("Content-Type", "text/vtt; charset=utf-8")
		c.Header("Cache-Control", "public, max-age=3600")
		if err := svc.Subtitle.Serve(c.Request.Context(), c.Param("id"), path, c.Writer); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
	}
}

func invalidateCloudSubtitleCacheHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Subtitle == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle service unavailable"})
			return
		}
		var in invalidateCloudSubtitleCacheRequest
		if c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&in); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		mediaID := strings.TrimSpace(in.MediaID)
		provider := strings.TrimSpace(in.Provider)
		invalidated := svc.Subtitle.InvalidateCloudDiscovery(mediaID, provider)
		c.JSON(http.StatusOK, gin.H{
			"invalidated": invalidated,
			"media_id":    mediaID,
			"provider":    provider,
		})
	}
}

func refreshCloudSubtitlesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Subtitle == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle service unavailable"})
			return
		}
		result, err := svc.Subtitle.RefreshCloudSubtitles(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "result": result})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func backfillCloudSubtitlesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Subtitle == nil || svc.Tasks == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cloud subtitle backfill service unavailable"})
			return
		}
		var in backfillCloudSubtitleCacheRequest
		if c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&in); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		in.LibraryID = strings.TrimSpace(in.LibraryID)
		task, started := svc.Tasks.StartUnique(service.TaskKindSubtitle, "回填云盘字幕", service.TaskUpdate{
			Stage:      "queued",
			SourcePath: in.LibraryID,
			Message:    "云盘字幕回填任务已排队",
		})
		if !started {
			c.JSON(http.StatusAccepted, gin.H{"queued": false, "already_running": true})
			return
		}

		go func() {
			result, err := svc.Subtitle.BackfillCloudSubtitles(svc.Context(), in.LibraryID, func(progress service.CloudSubtitleBackfillResult) {
				task.Update(service.TaskUpdate{
					Stage:   "materializing",
					Message: fmt.Sprintf("正在回填云盘字幕：%d/%d", progress.Processed, progress.Total),
					Metrics: cloudSubtitleBackfillTaskMetrics(progress),
					Details: progress.Errors,
				})
			})
			stage := "completed"
			message := fmt.Sprintf("云盘字幕回填完成：缓存 %d，空结果 %d", result.Cached, result.Empty)
			if err != nil {
				stage = "failed"
				message = fmt.Sprintf("云盘字幕回填失败：缓存 %d，空结果 %d，失败 %d", result.Cached, result.Empty, result.Failed)
			}
			finishHTTPTask(task, err, stage, message, cloudSubtitleBackfillTaskMetrics(result), result.Errors)
		}()

		c.JSON(http.StatusAccepted, gin.H{"queued": true, "task_id": task.ID(), "library_id": in.LibraryID})
	}
}

func cloudSubtitleBackfillTaskMetrics(result service.CloudSubtitleBackfillResult) map[string]int64 {
	return map[string]int64{
		"total":     int64(result.Total),
		"processed": int64(result.Processed),
		"cached":    int64(result.Cached),
		"empty":     int64(result.Empty),
		"skipped":   int64(result.Skipped),
		"failed":    int64(result.Failed),
	}
}

func deleteSubtitleHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := strings.TrimSpace(c.Query("path"))
		if path == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing subtitle path"})
			return
		}
		if svc == nil || svc.Subtitle == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle service unavailable"})
			return
		}
		if err := svc.Subtitle.Delete(c.Request.Context(), c.Param("id"), path); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"deleted": true})
	}
}

func searchSubtitleCandidatesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in subtitleSearchRequest
		if c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&in); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		if svc == nil || svc.Subtitle == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle service unavailable"})
			return
		}
		result, err := svc.Subtitle.SearchCandidates(c.Request.Context(), middleware.GetUserID(c), c.Param("id"), in.Limit)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func searchSeasonSubtitleCandidatesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in subtitleSeasonSearchRequest
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if svc == nil || svc.Subtitle == nil || svc.Media == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle service unavailable"})
			return
		}
		if in.Season < 1 || in.Season > 99 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "season must be between 1 and 99"})
			return
		}
		anchor, targets, validationErr := seasonSubtitleEpisodesForRequest(c, svc, in.Season, in.Episodes, false)
		if validationErr != nil {
			c.JSON(validationErr.status, gin.H{"error": validationErr.message})
			return
		}
		result, err := svc.Subtitle.SearchSeasonCandidates(
			c.Request.Context(), middleware.GetUserID(c), anchor.ID, in.Season, in.Title, in.Limit, targets,
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func applySeasonSubtitleCandidatesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in subtitleSeasonApplyRequest
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if svc == nil || svc.Subtitle == nil || svc.Media == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle service unavailable"})
			return
		}
		if in.Season < 1 || in.Season > 99 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "season must be between 1 and 99"})
			return
		}
		anchor, targets, validationErr := seasonSubtitleEpisodesForRequest(c, svc, in.Season, in.Episodes, true)
		if validationErr != nil {
			c.JSON(validationErr.status, gin.H{"error": validationErr.message})
			return
		}
		task, err := svc.Subtitle.StartSeasonSubtitles(
			c.Request.Context(), middleware.GetUserID(c), anchor.ID, in.Season,
			in.SearchSessionID, targets,
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

type seasonSubtitleValidationError struct {
	status  int
	message string
}

func seasonSubtitleEpisodesForRequest(
	c *gin.Context,
	svc *service.Container,
	season int,
	requested []subtitleSeasonApplyEpisodeRequest,
	requireCandidate bool,
) (*model.Media, []service.SubtitleSeasonEpisode, *seasonSubtitleValidationError) {
	if len(requested) == 0 || len(requested) > 500 {
		return nil, nil, &seasonSubtitleValidationError{http.StatusBadRequest, "episode count must be between 1 and 500"}
	}
	anchor, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
	if err != nil || anchor == nil || !mediaVisibleForRequest(c, svc, anchor) {
		return nil, nil, &seasonSubtitleValidationError{http.StatusNotFound, "not found"}
	}
	anchorKey := service.MediaSeriesKey(*anchor)
	if anchorKey == "" || anchor.SeasonNum != season || anchor.EpisodeNum < 1 {
		return nil, nil, &seasonSubtitleValidationError{http.StatusBadRequest, "season subtitle request requires an episode in this season"}
	}

	ids := make([]string, 0, len(requested))
	seenIDs := make(map[string]struct{}, len(requested))
	for _, item := range requested {
		mediaID := strings.TrimSpace(item.MediaID)
		if mediaID == "" {
			return nil, nil, &seasonSubtitleValidationError{http.StatusBadRequest, "episode media_id is required"}
		}
		if requireCandidate && strings.TrimSpace(item.CandidateID) == "" {
			return nil, nil, &seasonSubtitleValidationError{http.StatusBadRequest, "episode candidate_id is required"}
		}
		if _, exists := seenIDs[mediaID]; exists {
			return nil, nil, &seasonSubtitleValidationError{http.StatusBadRequest, "duplicate episode media_id"}
		}
		seenIDs[mediaID] = struct{}{}
		ids = append(ids, mediaID)
	}

	items, err := svc.Media.GetMediaByIDs(c.Request.Context(), ids)
	if err != nil || len(items) != len(ids) {
		return nil, nil, &seasonSubtitleValidationError{http.StatusBadRequest, "one or more selected episodes do not exist"}
	}
	byID := make(map[string]*model.Media, len(items))
	for index := range items {
		byID[items[index].ID] = &items[index]
	}
	targets := make([]service.SubtitleSeasonEpisode, 0, len(ids))
	for _, item := range requested {
		mediaID := strings.TrimSpace(item.MediaID)
		episode := byID[mediaID]
		if episode == nil || !mediaVisibleForRequest(c, svc, episode) {
			return nil, nil, &seasonSubtitleValidationError{http.StatusNotFound, "selected episode is not visible"}
		}
		if episode.LibraryID != anchor.LibraryID || service.MediaSeriesKey(*episode) != anchorKey || episode.SeasonNum != season || episode.EpisodeNum < 1 {
			return nil, nil, &seasonSubtitleValidationError{http.StatusBadRequest, "selected episode does not belong to this season"}
		}
		targets = append(targets, service.SubtitleSeasonEpisode{
			MediaID:     episode.ID,
			EpisodeKey:  fmt.Sprintf("S%02dE%02d", season, episode.EpisodeNum),
			CandidateID: strings.TrimSpace(item.CandidateID),
		})
	}
	return anchor, targets, nil
}

func getSeasonSubtitleTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Subtitle == nil || svc.Media == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle service unavailable"})
			return
		}
		anchor, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil || anchor == nil || !mediaVisibleForRequest(c, svc, anchor) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		task, err := svc.Subtitle.GetSeasonSubtitleTask(
			c.Request.Context(), middleware.GetUserID(c), anchor.ID, c.Param("task_id"),
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusOK, task)
	}
}

func retrySeasonSubtitleTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in subtitleSeasonRetryRequest
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if svc == nil || svc.Subtitle == nil || svc.Media == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle service unavailable"})
			return
		}
		anchor, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil || anchor == nil || !mediaVisibleForRequest(c, svc, anchor) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		task, err := svc.Subtitle.RetrySeasonSubtitles(
			c.Request.Context(), middleware.GetUserID(c), anchor.ID, c.Param("task_id"), in.MediaIDs,
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func previewSubtitleCandidateHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Subtitle == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle service unavailable"})
			return
		}
		in, ok := bindSubtitleCandidateRequest(c)
		if !ok {
			return
		}
		result, err := svc.Subtitle.PreviewCandidate(
			c.Request.Context(), middleware.GetUserID(c), c.Param("id"), in.SearchSessionID, in.CandidateID,
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func applySubtitleCandidateHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Subtitle == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle service unavailable"})
			return
		}
		in, ok := bindSubtitleCandidateRequest(c)
		if !ok {
			return
		}
		result, err := svc.Subtitle.ApplyCandidate(
			c.Request.Context(), middleware.GetUserID(c), c.Param("id"), in.SearchSessionID, in.CandidateID,
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		tracks, err := svc.Subtitle.Discover(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "subtitle saved but cannot be discovered: " + err.Error()})
			return
		}
		visible := false
		for _, track := range tracks {
			if subtitleTrackMatchesSavedFilename(track, result.Filename) {
				visible = true
				break
			}
		}
		if !visible {
			c.JSON(http.StatusBadGateway, gin.H{"error": "subtitle saved by pipeline but is not visible in MediaStationGo subtitle cache"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result, "tracks": tracks})
	}
}

func subtitleTrackMatchesSavedFilename(track service.SubtitleTrack, filename string) bool {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return false
	}
	if strings.TrimSpace(track.Name) == filename {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(track.Path))
	return err == nil && path.Base(parsed.Path) == filename
}

func createSubtitleASRHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		media, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil || media == nil || !mediaVisibleForRequest(c, svc, media) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		var in subtitleASRRequest
		if c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&in); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		task, err := svc.Subtitle.CreateASRTask(
			c.Request.Context(), middleware.GetUserID(c), media.ID, in.SourceLanguage,
			in.ASRModel, in.TranslationProvider, in.TranslationModel,
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func listSubtitleASRProfilesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		profiles, err := svc.Subtitle.ListASRProfiles(c.Request.Context())
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusOK, service.SubtitleASRProfileList{Items: profiles})
	}
}

func listSubtitleASRModelsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		models, err := svc.Subtitle.ListASRModels(c.Request.Context())
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": models})
	}
}

func retrySubtitleASRTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in subtitleASRRequest
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		task, err := svc.Subtitle.RetryASRTask(
			c.Request.Context(), middleware.GetUserID(c), c.Param("task_id"),
			in.ASRModel, in.TranslationProvider, in.TranslationModel,
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func updateSubtitleASRTaskModelHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in subtitleASRRequest
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		task, err := svc.Subtitle.UpdateQueuedASRTaskModel(
			c.Request.Context(), middleware.GetUserID(c), c.Param("task_id"),
			in.ASRModel, in.TranslationProvider, in.TranslationModel,
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusOK, task)
	}
}

func cancelSubtitleASRTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		task, err := svc.Subtitle.CancelQueuedASRTask(
			c.Request.Context(), middleware.GetUserID(c), c.Param("task_id"),
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusOK, task)
	}
}

func retranslateSubtitleASRTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in subtitleASRRequest
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		task, err := svc.Subtitle.RetranslateASRTask(
			c.Request.Context(), middleware.GetUserID(c), c.Param("task_id"),
			in.TranslationProvider, in.TranslationModel,
		)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func deleteSubtitleASRTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Subtitle.DeleteASRTask(
			c.Request.Context(), middleware.GetUserID(c), c.Param("task_id"),
		); err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"deleted": true})
	}
}

func getSubtitleASRHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		task, err := svc.Subtitle.GetASRTask(c.Request.Context(), middleware.GetUserID(c), c.Param("task_id"))
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		if task.MediaID != c.Param("id") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, task)
	}
}

func listSubtitleASRTasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		tasks, err := svc.Subtitle.ListASRTasks(c.Request.Context(), 50)
		if err != nil {
			writeSubtitlePipelineError(c, err)
			return
		}
		mediaIDs := make([]string, 0, len(tasks))
		for i := range tasks {
			mediaIDs = append(mediaIDs, tasks[i].MediaID)
		}
		mediaRows, mediaErr := svc.Media.GetMediaByIDs(c.Request.Context(), mediaIDs)
		if mediaErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": mediaErr.Error()})
			return
		}
		mediaByID := make(map[string]model.Media, len(mediaRows))
		for i := range mediaRows {
			mediaByID[mediaRows[i].ID] = mediaRows[i]
		}
		for i := range tasks {
			media, ok := mediaByID[tasks[i].MediaID]
			if !ok {
				continue
			}
			tasks[i].MediaAvailable = true
			tasks[i].MediaTitle = strings.TrimSpace(media.DisplayTitle)
			if tasks[i].MediaTitle == "" {
				tasks[i].MediaTitle = strings.TrimSpace(media.Title)
			}
			if tasks[i].MediaTitle == "" {
				tasks[i].MediaTitle = strings.TrimSpace(media.OriginalName)
			}
			normalizedPath := strings.Trim(strings.ReplaceAll(strings.TrimSpace(media.Path), "\\", "/"), "/")
			if slash := strings.LastIndex(normalizedPath, "/"); slash >= 0 {
				normalizedPath = normalizedPath[slash+1:]
			}
			tasks[i].MediaFilename = normalizedPath
		}
		c.JSON(http.StatusOK, service.SubtitleASRTaskList{Items: tasks})
	}
}

func pipelineASRAudioHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Stream == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "media stream service unavailable"})
			return
		}
		stream, wait, info, err := svc.Stream.StartASRAudioExtraction(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		defer stream.Close()
		c.Header("Content-Type", "audio/mpeg")
		c.Header("Content-Disposition", `attachment; filename="asr-audio.mp3"`)
		c.Header("Cache-Control", "no-store")
		if info.DurationSeconds > 0 {
			c.Header("X-Media-Duration-Seconds", strconv.Itoa(info.DurationSeconds))
		}
		if info.BitrateBPS > 0 {
			c.Header("X-ASR-Audio-Bitrate", strconv.Itoa(info.BitrateBPS))
		}
		copied, copyErr := io.Copy(c.Writer, stream)
		waitErr := wait()
		if copyErr == nil && waitErr == nil && copied > 0 {
			return
		}
		if copied == 0 && !c.Writer.Written() {
			c.Writer.Header().Del("Content-Type")
			c.Writer.Header().Del("Content-Disposition")
			if copyErr != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": "audio extraction stream failed: " + copyErr.Error()})
				return
			}
			if waitErr != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": waitErr.Error()})
				return
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": "audio extraction returned no data"})
			return
		}
		if svc.Log != nil {
			svc.Log.Error("ASR audio extraction stream failed", zap.Int64("bytes", copied), zap.Error(errors.Join(copyErr, waitErr)))
		}
	}
}

func pipelineTranslateSubtitlesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in subtitleTranslationRequest
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := svc.Subtitle.TranslateText(
			c.Request.Context(), in.Provider, in.Model, service.SubtitleTranslationInput{
				Text: in.Text, Context: in.Context, Glossary: in.Glossary,
				RetryInstruction: in.RetryInstruction,
			},
		)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func bindSubtitleCandidateRequest(c *gin.Context) (subtitleCandidateRequest, bool) {
	var in subtitleCandidateRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return subtitleCandidateRequest{}, false
	}
	in.SearchSessionID = strings.TrimSpace(in.SearchSessionID)
	in.CandidateID = strings.TrimSpace(in.CandidateID)
	if in.SearchSessionID == "" || in.CandidateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "search_session_id and candidate_id are required"})
		return subtitleCandidateRequest{}, false
	}
	return in, true
}

func writeSubtitlePipelineError(c *gin.Context, err error) {
	var pipelineStatus interface{ HTTPStatus() int }
	if errors.As(err, &pipelineStatus) {
		status := pipelineStatus.HTTPStatus()
		if (status >= 400 && status < 500) || status == http.StatusServiceUnavailable {
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
}
