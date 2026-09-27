package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const maxExternalIDPresenceBatch = 100

type externalIDPresenceRequest struct {
	TMDbIDs   []int                       `json:"tmdb_ids"`
	TMDbRefs  []externalIDPresenceTMDbRef `json:"tmdb_refs"`
	DoubanIDs []string                    `json:"douban_ids"`
}

type externalIDPresenceResponse struct {
	TMDbIDs   []int                       `json:"tmdb_ids"`
	TMDbRefs  []externalIDPresenceTMDbRef `json:"tmdb_refs"`
	DoubanIDs []string                    `json:"douban_ids"`
}

type externalIDPresenceTMDbRef struct {
	ID        int    `json:"id"`
	MediaType string `json:"media_type"`
}

func externalIDPresenceHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request externalIDPresenceRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求体必须是 JSON 对象"})
			return
		}
		tmdbIDs, err := normalizeTMDbPresenceIDs(request.TMDbIDs)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		tmdbRefs, err := normalizeTMDbPresenceRefs(request.TMDbRefs)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		doubanIDs, err := normalizeDoubanPresenceIDs(request.DoubanIDs)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if len(tmdbIDs)+len(tmdbRefs)+len(doubanIDs) > maxExternalIDPresenceBatch {
			c.JSON(http.StatusBadRequest, gin.H{"error": "外部 ID 数量不能超过 100"})
			return
		}

		tmdbPresent, doubanPresent, err := svc.Media.FindVisibleExternalIDPresence(
			c.Request.Context(), append(legacyTMDbRefs(tmdbIDs), toRepositoryTMDbRefs(tmdbRefs)...), doubanIDs, mediaVisibilityForRequest(c, svc),
		)
		if err != nil {
			writeInternalOrCanceled(c, err)
			return
		}
		response := externalIDPresenceResponse{
			TMDbIDs:   make([]int, 0, len(tmdbIDs)),
			TMDbRefs:  make([]externalIDPresenceTMDbRef, 0, len(tmdbRefs)),
			DoubanIDs: make([]string, 0, len(doubanPresent)),
		}
		for _, id := range tmdbIDs {
			if tmdbPresent[repository.ExternalIDReference{ID: id}] {
				response.TMDbIDs = append(response.TMDbIDs, id)
			}
		}
		for _, ref := range tmdbRefs {
			if tmdbPresent[repository.ExternalIDReference{ID: ref.ID, MediaType: ref.MediaType}] {
				response.TMDbRefs = append(response.TMDbRefs, ref)
			}
		}
		for _, id := range doubanIDs {
			if doubanPresent[id] {
				response.DoubanIDs = append(response.DoubanIDs, id)
			}
		}
		c.JSON(http.StatusOK, response)
	}
}

func normalizeTMDbPresenceRefs(values []externalIDPresenceTMDbRef) ([]externalIDPresenceTMDbRef, error) {
	refs := make([]externalIDPresenceTMDbRef, 0, len(values))
	seen := make(map[repository.ExternalIDReference]struct{}, len(values))
	for _, value := range values {
		if value.ID <= 0 {
			return nil, errors.New("tmdb_refs.id 必须为正整数")
		}
		if value.MediaType != "movie" && value.MediaType != "tv" {
			return nil, errors.New("tmdb_refs.media_type 必须为 movie 或 tv")
		}
		ref := repository.ExternalIDReference{ID: value.ID, MediaType: value.MediaType}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, value)
	}
	return refs, nil
}

func legacyTMDbRefs(values []int) []repository.ExternalIDReference {
	refs := make([]repository.ExternalIDReference, 0, len(values))
	for _, id := range values {
		refs = append(refs, repository.ExternalIDReference{ID: id})
	}
	return refs
}

func toRepositoryTMDbRefs(values []externalIDPresenceTMDbRef) []repository.ExternalIDReference {
	refs := make([]repository.ExternalIDReference, 0, len(values))
	for _, value := range values {
		refs = append(refs, repository.ExternalIDReference{ID: value.ID, MediaType: value.MediaType})
	}
	return refs
}

func normalizeTMDbPresenceIDs(values []int) ([]int, error) {
	ids := make([]int, 0, len(values))
	seen := make(map[int]struct{}, len(values))
	for _, id := range values {
		if id <= 0 {
			return nil, errors.New("tmdb_ids 必须为正整数")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func normalizeDoubanPresenceIDs(values []string) ([]string, error) {
	ids := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" || len(id) > 32 {
			return nil, errors.New("douban_ids 必须是 1 到 32 个字符")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}
