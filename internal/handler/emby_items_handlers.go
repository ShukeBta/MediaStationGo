package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func parseEmbyItemsParams(c *gin.Context) service.ItemsParams {
	limit, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "Limit", "limit"), "50"))
	offset, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "StartIndex", "startIndex", "startindex"), "0"))
	uid := embyUserID(c)
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
	fieldsRaw, fieldsSpecified := embyQueryValueWithPresence(c, "Fields", "fields")
	fields := splitEmbyItemsFields(fieldsRaw)
	omitMediaSources := fieldsSpecified && !embyFieldContains(fields, "MediaSources")
	return service.ItemsParams{
		UserID:           uid,
		ParentID:         firstQueryValue(c, "ParentId", "parentId", "parentid"),
		IDs:              splitOpt(firstQueryValue(c, "Ids", "ids")),
		PersonIDs:        splitOpt(firstQueryValue(c, "PersonIds", "personIds", "personids")),
		GenreIDs:         splitOpt(firstQueryValue(c, "GenreIds", "genreIds", "genreids")),
		Genres:           splitOpt(firstQueryValue(c, "Genres", "genres", "Genre", "genre")),
		SearchTerm:       firstQueryValue(c, "SearchTerm", "searchTerm", "searchterm"),
		NameStartsWith:   firstQueryValue(c, "NameStartsWith", "nameStartsWith", "namestartswith"),
		IncludeItemTypes: splitOpt(firstQueryValue(c, "IncludeItemTypes", "includeItemTypes", "includeitemtypes")),
		Filters:          splitOpt(firstQueryValue(c, "Filters", "filters")),
		Recursive:        strings.EqualFold(firstQueryValue(c, "Recursive", "recursive"), "true"),
		SortBy:           firstQueryValue(c, "SortBy", "sortBy", "sortby"),
		SortOrder:        firstQueryValue(c, "SortOrder", "sortOrder", "sortorder"),
		Limit:            limit,
		StartIndex:       offset,
		OmitMediaSources: omitMediaSources,
	}
}

func embyQueryValueWithPresence(c *gin.Context, keys ...string) (string, bool) {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return "", false
	}
	query := c.Request.URL.Query()
	for _, key := range keys {
		values, ok := query[key]
		if !ok {
			continue
		}
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				return value, true
			}
		}
		return "", true
	}
	return "", false
}

func splitEmbyItemsFields(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func embyFieldContains(fields []string, want string) bool {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field), want) {
			return true
		}
	}
	return false
}

func embyFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func splitEmbyQueryCSV(raw string) []string {
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func embyItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		params := parseEmbyItemsParams(c)
		out, err := svc.Emby.Items(c.Request.Context(), params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if embyItemsParamsRequestResumeRows(params) {
			if err := embyApplyFilmlyResumeCompatibility(c, out); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyPersonsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := svc.Emby.Persons(c.Request.Context(), parseEmbyItemsParams(c))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

func embyGenresHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := svc.Emby.Genres(c.Request.Context(), parseEmbyItemsParams(c))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

func embyItemByIDHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		uid := embyUserID(c)
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

func embyAdditionalPartsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		out, err := svc.Emby.AdditionalParts(c.Request.Context(), c.Param("id"), uid)
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
		uid := embyUserID(c)
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
		uid := embyUserID(c)
		if strings.TrimSpace(c.Param("userId")) == "" {
			out, err := svc.Emby.ResumeItems(c.Request.Context(), uid)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if err := embyApplyFilmlyResumeCompatibility(c, out); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			embyAttachRequestTokenToMediaSources(c, out)
			c.JSON(http.StatusOK, out)
			return
		}
		startIndex, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "StartIndex", "startIndex", "startindex"), "0"))
		limit, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "Limit", "limit"), "10"))
		out, err := svc.Emby.ResumeItemsPage(c.Request.Context(), uid, startIndex, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := embyApplyFilmlyResumeCompatibility(c, out); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyNextUpHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		if uid == "" {
			uid = firstQueryValue(c, "UserId", "userId", "userid")
		}
		startIndex, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "StartIndex", "startIndex", "startindex"), "0"))
		// An omitted Limit falls through as 0 so the service applies the
		// NextUp default page size instead of the generic Items default.
		limit, _ := strconv.Atoi(firstQueryValue(c, "Limit", "limit"))
		seriesIDs := splitEmbyQueryCSV(firstQueryValue(c, "SeriesId", "seriesId", "seriesid"))
		out, err := svc.Emby.NextUpItems(c.Request.Context(), service.NextUpParams{
			UserID:     uid,
			SeriesIDs:  seriesIDs,
			SeasonID:   firstQueryValue(c, "SeasonId", "seasonId", "seasonid"),
			StartIndex: startIndex,
			Limit:      limit,
		})
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
			uid := embyUserID(c)
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
		params := parseEmbyItemsParams(c)
		params.UserID = embyUserID(c)
		params.ParentID = c.Param("id")
		// Preserve the historical all-seasons response for clients that omit
		// Limit, while honoring explicit standard pagination parameters.
		if rawLimit, _ := embyQueryValueWithPresence(c, "Limit", "limit"); strings.TrimSpace(rawLimit) == "" {
			params.Limit = 500
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
		params := parseEmbyItemsParams(c)
		params.UserID = embyUserID(c)
		params.ParentID = parentID
		params.IncludeItemTypes = []string{"Episode"}
		params.Recursive = true
		// Emby's Limit is optional on the Episodes endpoint. Clients such as
		// VidHub may omit it and do not always issue a follow-up page, so an
		// omitted value must not inherit the generic 50-item /Items default.
		// Explicit Limits remain paged and are honored by EmbyService.Items.
		if rawLimit, _ := embyQueryValueWithPresence(c, "Limit", "limit"); strings.TrimSpace(rawLimit) == "" {
			params.Limit = service.MaxEmbyItemsPageSize
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
