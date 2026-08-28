package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const maxExternalIDPresenceBatch = 100

type externalIDPresenceRequest struct {
	TMDbIDs   []int    `json:"tmdb_ids"`
	DoubanIDs []string `json:"douban_ids"`
}

type externalIDPresenceResponse struct {
	TMDbIDs   []int    `json:"tmdb_ids"`
	DoubanIDs []string `json:"douban_ids"`
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
		doubanIDs, err := normalizeDoubanPresenceIDs(request.DoubanIDs)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if len(tmdbIDs)+len(doubanIDs) > maxExternalIDPresenceBatch {
			c.JSON(http.StatusBadRequest, gin.H{"error": "外部 ID 数量不能超过 100"})
			return
		}

		tmdbPresent, doubanPresent, err := svc.Media.FindVisibleExternalIDPresence(
			c.Request.Context(), tmdbIDs, doubanIDs, mediaVisibilityForRequest(c, svc),
		)
		if err != nil {
			writeInternalOrCanceled(c, err)
			return
		}
		response := externalIDPresenceResponse{
			TMDbIDs:   make([]int, 0, len(tmdbPresent)),
			DoubanIDs: make([]string, 0, len(doubanPresent)),
		}
		for _, id := range tmdbIDs {
			if tmdbPresent[id] {
				response.TMDbIDs = append(response.TMDbIDs, id)
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
