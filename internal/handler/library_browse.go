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
		options := service.LibraryBrowseOptions{Page: page, Category: strings.TrimSpace(c.Query("category")), Actor: strings.TrimSpace(c.Query("actor")), AdultType: strings.ToUpper(strings.TrimSpace(c.Query("adult_type"))), SeriesKey: c.Query("series"), FocusMediaID: c.Query("focus_media"), IncludeFacets: c.Query("facets") == "1"}
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
