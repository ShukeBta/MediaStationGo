package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type adultPerformerFollowRequest struct {
	Name     string `json:"name" binding:"required"`
	Source   string `json:"source" binding:"required"`
	SourceID string `json:"source_id" binding:"required"`
	ImageURL string `json:"image_url"`
}

func listAdultPerformerFollowsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireAdultDiscoverAccess(c, svc) {
			return
		}
		rows, err := svc.Repo.AdultFollow.ListByUser(c.Request.Context(), currentUserID(c))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": rows})
	}
}

func createAdultPerformerFollowHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireAdultDiscoverAccess(c, svc) {
			return
		}
		var req adultPerformerFollowRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演员信息不完整"})
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" || len([]rune(name)) > 255 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演员名称无效"})
			return
		}
		source := strings.ToLower(strings.TrimSpace(req.Source))
		sourceID := strings.TrimSpace(req.SourceID)
		_, profileURL, ok := svc.Adult.AdultPerformerProfile(c.Request.Context(), source, sourceID)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前演员来源不受支持"})
			return
		}
		imageURL, ok := service.NormalizeAdultPerformerImageURL(req.ImageURL)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "演员图片来源无效"})
			return
		}
		follow := model.AdultPerformerFollow{
			UserID:     currentUserID(c),
			Name:       name,
			NameKey:    strings.ToLower(strings.Join(strings.Fields(name), " ")),
			Source:     source,
			SourceID:   sourceID,
			ImageURL:   imageURL,
			ProfileURL: profileURL,
		}
		if err := svc.Repo.AdultFollow.Upsert(c.Request.Context(), &follow); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		forgetAdultDiscoverUserSections(svc, follow.UserID)
		c.JSON(http.StatusOK, follow)
	}
}

func searchAdultPerformersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireAdultDiscoverAccess(c, svc) {
			return
		}
		query := strings.TrimSpace(c.Query("q"))
		if length := len([]rune(query)); length < 2 || length > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "女优搜索词长度需为 2 到 100 个字符"})
			return
		}
		items, err := svc.Adult.SearchPerformers(c.Request.Context(), query)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error":  "女优搜索暂时无法使用",
				"detail": err.Error(),
			})
			return
		}
		if err := markAdultPerformerFollows(c.Request.Context(), svc, currentUserID(c), items); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if svc.Discover != nil {
			svc.Discover.WarmExternalArtwork(items)
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func deleteAdultPerformerFollowHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireAdultDiscoverAccess(c, svc) {
			return
		}
		userID := currentUserID(c)
		deleted, err := svc.Repo.AdultFollow.DeleteOwned(c.Request.Context(), userID, c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !deleted {
			c.JSON(http.StatusNotFound, gin.H{"error": "关注记录不存在"})
			return
		}
		forgetAdultDiscoverUserSections(svc, userID)
		c.Status(http.StatusNoContent)
	}
}

func adultPerformerWorksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireAdultDiscoverAccess(c, svc) {
			return
		}
		page := queryInt(c, "page")
		if page < 1 {
			page = 1
		}
		items, hasNext, err := svc.Adult.DiscoverPerformerWorksPage(
			c.Request.Context(), c.Param("source"), c.Param("source_id"), page,
		)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "演员作品暂时无法加载"})
			return
		}
		service.EnrichExternalMediaLibraryLinks(
			c.Request.Context(), svc.Repo, items, mediaVisibilityForRequest(c, svc),
		)
		if svc.Discover != nil {
			svc.Discover.WarmExternalArtwork(items)
		}
		response := gin.H{"items": items, "page": page, "has_next": hasNext}
		performerName := strings.TrimSpace(c.Query("name"))
		requestedSource := strings.ToLower(strings.TrimSpace(c.Param("source")))
		if page == 1 && performerName != "" && requestedSource == "javdb" {
			if len([]rune(performerName)) > 255 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "演员名称无效"})
				return
			}
			performers, profileErr := svc.Adult.SearchPerformers(c.Request.Context(), performerName)
			if profileErr != nil {
				response["performer_error"] = "女优头像暂时无法加载"
			} else {
				requestedID := strings.TrimSpace(c.Param("source_id"))
				for _, performer := range performers {
					if strings.EqualFold(performer.Source, requestedSource) && performer.ProviderID == requestedID {
						response["performer"] = performer
						break
					}
				}
			}
		}
		c.JSON(http.StatusOK, response)
	}
}

func adultMovieDetailHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireAdultDiscoverAccess(c, svc) {
			return
		}
		code := strings.TrimSpace(c.Query("code"))
		if length := len([]rune(code)); length < 3 || length > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "作品番号无效"})
			return
		}
		source := strings.ToLower(strings.TrimSpace(c.Param("source")))
		providerID := strings.TrimSpace(c.Param("provider_id"))
		cacheKey := "detail:adult:" + source + ":" + providerID + ":" + strings.ToUpper(code)
		cached, cacheHit := cachedDiscoverSection(svc, cacheKey, 1)
		var item service.ExternalMediaResult
		if cacheHit && len(cached) > 0 {
			item = cached[0]
		} else {
			var err error
			item, err = svc.Adult.DiscoverMovieDetail(c.Request.Context(), source, providerID, code)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{
					"error":  "作品详情暂时无法加载",
					"detail": err.Error(),
				})
				return
			}
			rememberDiscoverSection(svc, cacheKey, 1, []service.ExternalMediaResult{item})
		}
		items := []service.ExternalMediaResult{item}
		service.EnrichExternalMediaLibraryLinks(
			c.Request.Context(), svc.Repo, items, mediaVisibilityForRequest(c, svc),
		)
		if svc.Discover != nil {
			svc.Discover.WarmExternalArtwork(items)
		}
		c.JSON(http.StatusOK, items[0])
	}
}

func requireAdultDiscoverAccess(c *gin.Context, svc *service.Container) bool {
	if discoverAdultAllowed(c, svc) && svc.Adult != nil && svc.Repo != nil && svc.Repo.AdultFollow != nil {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "当前用户无权访问成人专区"})
	return false
}

func forgetAdultDiscoverUserSections(svc *service.Container, userID string) {
	if svc == nil || svc.Discover == nil {
		return
	}
	for _, key := range []string{
		"adult_javdb_performers",
		"adult_followed_performers",
		"adult_followed",
		"adult_javdb_performers_new",
		"adult_javdb_performers_monthly",
		"adult_javdb_performers_fanza",
	} {
		svc.Discover.ForgetSection(discoverSectionCacheKeyForUser(key, userID))
	}
}
