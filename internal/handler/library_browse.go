package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func libraryBrowseHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
		if err != nil || page < 1 || page > 10000000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
			return
		}
		visibility := mediaVisibilityForRequest(c, svc)
		lib, err := svc.Repo.Library.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeInternalOrCanceled(c, err)
			return
		}
		if lib == nil || !service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, *lib, visibility) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		yearFrom, yearTo := 0, 0
		if rawYear := strings.TrimSpace(c.Query("year")); rawYear != "" {
			yearFrom, yearTo, err = parseLibraryBrowseYear(rawYear)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year"})
				return
			}
		}
		options := service.LibraryBrowseOptions{
			Page: page, Query: strings.TrimSpace(c.Query("q")), Sort: strings.TrimSpace(c.Query("sort")),
			Category: strings.TrimSpace(c.Query("category")), Genre: strings.TrimSpace(c.Query("genre")), YearFrom: yearFrom, YearTo: yearTo,
			Language: strings.TrimSpace(c.Query("language")), Actor: strings.TrimSpace(c.Query("actor")),
			AdultType: strings.ToUpper(strings.TrimSpace(c.Query("adult_type"))), SeriesKey: c.Query("series"),
			FocusMediaID: c.Query("focus_media"), IncludeFacets: c.Query("facets") == "1",
		}
		if (options.Actor != "" && lib.Type != "adult") || (options.AdultType != "" && (lib.Type != "adult" || (options.AdultType != "AV" && options.AdultType != "FC2"))) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported library filter"})
			return
		}
		result, err := svc.Media.BrowseLibrary(c.Request.Context(), lib.ID, options, visibility)
		if errors.Is(err, service.ErrInvalidLibraryBrowseFilter) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			writeInternalOrCanceled(c, err)
			return
		}
		result.Items = mediaItemsForResponse(c, result.Items)
		result.SeriesCards = seriesCardsForResponse(c, result.SeriesCards)
		if result.Items == nil {
			result.Items = []service.MediaItem{}
		}
		if result.SeriesCards == nil {
			result.SeriesCards = []service.SeriesCard{}
		}
		if result.SelectedSeries != nil {
			cards := seriesCardsForResponse(c, []service.SeriesCard{*result.SelectedSeries})
			result.SelectedSeries = &cards[0]
		}
		c.JSON(http.StatusOK, result)
	}
}

func parseLibraryBrowseYear(value string) (int, int, error) {
	if strings.HasPrefix(value, "before-") {
		cutoff, err := strconv.Atoi(strings.TrimPrefix(value, "before-"))
		if err != nil || cutoff < 1801 || cutoff > 3001 {
			return 0, 0, errors.New("invalid year range")
		}
		return 1, cutoff - 1, nil
	}
	parts := strings.Split(value, "-")
	if len(parts) > 2 {
		return 0, 0, errors.New("invalid year range")
	}
	from, err := strconv.Atoi(parts[0])
	if err != nil || from < 1800 || from > 3000 {
		return 0, 0, errors.New("invalid year range")
	}
	to := from
	if len(parts) == 2 {
		to, err = strconv.Atoi(parts[1])
		if err != nil || to < from || to > 3000 {
			return 0, 0, errors.New("invalid year range")
		}
	}
	return from, to, nil
}
