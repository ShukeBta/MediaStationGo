package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func parseEmbyItemsParams(c *gin.Context) service.ItemsParams {
	limitRaw := firstQueryValue(c, "Limit", "limit")
	limit, _ := strconv.Atoi(embyFirstNonEmptyString(limitRaw, "50"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "StartIndex", "startIndex", "startindex"), "0"))
	uid := c.Param("userId")
	if uid == "" {
		uid = firstQueryValue(c, "UserId", "userId", "userid")
	}
	if uid == "" {
		uid = embyUserID(c)
	}
	splitOpt := func(s string) []string {
		if s == "" {
			return nil
		}
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return service.ItemsParams{
		UserID:           uid,
		ParentID:         firstQueryValue(c, "ParentId", "parentId", "parentid"),
		IDs:              splitOpt(firstQueryValue(c, "Ids", "ids")),
		SearchTerm:       firstQueryValue(c, "SearchTerm", "searchTerm", "searchterm"),
		NameStartsWith:   firstQueryValue(c, "NameStartsWith", "nameStartsWith", "namestartswith"),
		IncludeItemTypes: splitOpt(firstQueryValue(c, "IncludeItemTypes", "includeItemTypes", "includeitemtypes")),
		Filters:          splitOpt(firstQueryValue(c, "Filters", "filters")),
		Recursive:        strings.EqualFold(firstQueryValue(c, "Recursive", "recursive"), "true"),
		SortBy:           firstQueryValue(c, "SortBy", "sortBy", "sortby"),
		SortOrder:        firstQueryValue(c, "SortOrder", "sortOrder", "sortorder"),
		Limit:            limit,
		StartIndex:       offset,
	}
}

func embyFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func embyRequestFieldsContains(c *gin.Context, field string) bool {
	for _, part := range strings.Split(firstQueryValue(c, "Fields", "fields"), ",") {
		if strings.EqualFold(strings.TrimSpace(part), field) {
			return true
		}
	}
	return false
}

func embyStripMediaSourcesFromItemsEnvelope(out any) {
	switch typed := out.(type) {
	case map[string]any:
		embyStripMediaSourcesFromItemsValue(typed["Items"])
	case gin.H:
		embyStripMediaSourcesFromItemsValue(typed["Items"])
	}
}

func embyStripMediaSourcesFromItemsValue(value any) {
	switch typed := value.(type) {
	case []map[string]any:
		for _, item := range typed {
			delete(item, "MediaSources")
		}
	case []any:
		for _, item := range typed {
			if itemMap, ok := item.(map[string]any); ok {
				delete(itemMap, "MediaSources")
			}
		}
	}
}

func embyItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		params := parseEmbyItemsParams(c)
		out, err := svc.Emby.Items(c.Request.Context(), params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if params.Limit > 100 && !embyRequestFieldsContains(c, "MediaSources") {
			embyStripMediaSourcesFromItemsEnvelope(out)
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyItemByIDHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		uid := c.Param("userId")
		if uid == "" {
			uid = embyUserID(c)
		}
		out, err := svc.Emby.Item(c.Request.Context(), id, uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if out == nil {
			embyError(c, http.StatusNotFound, "item not found")
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyUserItemByIDHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch strings.ToLower(c.Param("id")) {
		case "latest":
			embyLatestItemsHandler(svc)(c)
		case "resume":
			embyResumeItemsHandler(svc)(c)
		default:
			embyItemByIDHandler(svc)(c)
		}
	}
}

func embyLatestItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = firstQueryValue(c, "UserId", "userId", "userid")
		}
		if uid == "" {
			uid = embyUserID(c)
		}
		limit, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "Limit", "limit"), "20"))
		out, err := svc.Emby.LatestItems(c.Request.Context(), uid, firstQueryValue(c, "ParentId", "parentId", "parentid"), limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyResumeItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = firstQueryValue(c, "UserId", "userId", "userid")
		}
		if uid == "" {
			uid = embyUserID(c)
		}
		limit, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "Limit", "limit"), "20"))
		out, err := svc.Emby.ResumeItems(c.Request.Context(), uid, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyItemsCountsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc != nil && svc.Emby != nil {
			uid := firstQueryValue(c, "UserId", "userId")
			if uid == "" {
				uid = c.Param("userId")
			}
			if uid == "" {
				uid = embyUserID(c)
			}
			out, err := svc.Emby.ItemCounts(c.Request.Context(), uid)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, out)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"MovieCount":   0,
			"SeriesCount":  0,
			"EpisodeCount": 0,
			"ItemCount":    0,
		})
	}
}

func embyDisplayPreferencesHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"Id":                 c.Param("id"),
			"ViewType":           "Poster",
			"SortBy":             "SortName",
			"SortOrder":          "Ascending",
			"IndexBy":            "SortName",
			"RememberIndexing":   false,
			"PrimaryImageHeight": 250,
			"PrimaryImageWidth":  250,
			"ScrollDirection":    "Vertical",
			"ShowSidebar":        true,
			"CustomPrefs": gin.H{
				"homeexploresection": "1",
				"homesection0":       "smalllibrarytiles",
				"homesection1":       "resume",
				"homesection2":       "none",
				"homesection3":       "nextup",
				"homesection4":       "none",
				"homesection5":       "none",
				"homesection6":       "none",
				"latestItems":        "false",
				"landing-livetv":     "false",
			},
		})
	}
}

func embySaveDisplayPreferencesHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	}
}

func embyShowSeasonsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		params := service.ItemsParams{
			UserID:   firstQueryValue(c, "UserId", "userId"),
			ParentID: c.Param("id"),
			Limit:    500,
		}
		out, err := svc.Emby.Items(c.Request.Context(), params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyShowEpisodesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		parentID := firstQueryValue(c, "SeasonId", "seasonId")
		if parentID == "" {
			parentID = c.Param("id")
		}
		params := service.ItemsParams{
			UserID:           embyFirstNonEmptyString(firstQueryValue(c, "UserId", "userId"), embyUserID(c)),
			ParentID:         parentID,
			IncludeItemTypes: []string{"Episode"},
			Recursive:        true,
			Limit:            500,
		}
		out, err := svc.Emby.Items(c.Request.Context(), params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}
