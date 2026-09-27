package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type discoverPreferenceInput struct {
	SelectedSections *[]string `json:"selected_sections"`
	AdultFD2PPVSort  *string   `json:"adult_fd2ppv_sort"`
}

const defaultDiscoverFD2PPVSort = "release"

func getDiscoverPreferenceHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		row, err := svc.Repo.DiscoverPreference.FindByUserID(c.Request.Context(), currentUserID(c))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if row == nil {
			c.JSON(http.StatusOK, gin.H{
				"configured":        false,
				"selected_sections": []string{},
				"adult_fd2ppv_sort": defaultDiscoverFD2PPVSort,
			})
			return
		}
		selected, err := normalizeDiscoverPreferenceSections(
			row.SelectedSections,
			enabledDiscoverSections(c.Request.Context(), svc),
			discoverAdultAllowed(c, svc),
			false,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		sortKey, err := normalizeDiscoverPreferenceFD2PPVSort(row.AdultFD2PPVSort)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"configured":        true,
			"selected_sections": selected,
			"adult_fd2ppv_sort": sortKey,
		})
	}
}

func updateDiscoverPreferenceHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input discoverPreferenceInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if input.SelectedSections == nil && input.AdultFD2PPVSort == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "selected_sections or adult_fd2ppv_sort is required"})
			return
		}
		existing, err := svc.Repo.DiscoverPreference.FindByUserID(c.Request.Context(), currentUserID(c))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if existing == nil && input.SelectedSections == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "selected_sections is required for a new preference"})
			return
		}
		selected := []string{}
		sortKey := defaultDiscoverFD2PPVSort
		if existing != nil {
			selected = existing.SelectedSections
			sortKey = existing.AdultFD2PPVSort
		}
		if input.SelectedSections != nil {
			selected, err = normalizeDiscoverPreferenceSections(
				*input.SelectedSections,
				enabledDiscoverSections(c.Request.Context(), svc),
				discoverAdultAllowed(c, svc),
				true,
			)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		if input.AdultFD2PPVSort != nil {
			sortKey = *input.AdultFD2PPVSort
		}
		sortKey, err = normalizeDiscoverPreferenceFD2PPVSort(sortKey)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		row := &model.UserDiscoverPreference{
			UserID:           currentUserID(c),
			SelectedSections: selected,
			AdultFD2PPVSort:  sortKey,
		}
		if err := svc.Repo.DiscoverPreference.Upsert(c.Request.Context(), row); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"configured":        true,
			"selected_sections": selected,
			"adult_fd2ppv_sort": sortKey,
		})
	}
}

func normalizeDiscoverPreferenceFD2PPVSort(raw string) (string, error) {
	sortKey, ok := service.NormalizeFD2PPVDiscoverSort(raw)
	if !ok {
		return "", fmt.Errorf("unsupported FC2 sort: %s", strings.TrimSpace(raw))
	}
	return sortKey, nil
}

func normalizeDiscoverPreferenceSections(
	raw []string,
	sections []discoverSectionDef,
	adultAllowed bool,
	rejectUnknown bool,
) ([]string, error) {
	if len(raw) > len(discoverSectionCatalog)+8 {
		return nil, fmt.Errorf("too many discover sections")
	}
	allowed := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		if section.Group == "adult" && !adultAllowed {
			continue
		}
		allowed[section.Key] = struct{}{}
	}
	selected := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		key := strings.TrimSpace(value)
		if key == "adult_javdb_performers" {
			key = "adult_javdb_performers_monthly"
		}
		if key == "" {
			if rejectUnknown {
				return nil, fmt.Errorf("discover section key is empty")
			}
			continue
		}
		if _, ok := allowed[key]; !ok {
			if rejectUnknown {
				return nil, fmt.Errorf("unsupported discover section: %s", key)
			}
			continue
		}
		if _, exists := selected[key]; exists {
			continue
		}
		selected[key] = struct{}{}
		out = append(out, key)
	}
	return out, nil
}
