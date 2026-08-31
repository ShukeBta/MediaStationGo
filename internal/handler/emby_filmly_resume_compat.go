package handler

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const embyFilmlyResumeETagSuffix = "-filmly-resume-episode-v1"

func embyRequestIsFilmly(c *gin.Context) bool {
	return c != nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(embyClientInfoFromRequest(c).Client)), "filmly")
}

func embyAttachFilmlyUserDataIdentity(userData map[string]any, itemID string) {
	userData["Key"] = itemID
	userData["ItemId"] = itemID
}

func embyAttachFilmlyResumeItemIdentity(item map[string]any) error {
	itemID, ok := item["Id"].(string)
	if !ok || strings.TrimSpace(itemID) == "" {
		return fmt.Errorf("apply Filmly Resume compatibility: item Id has type %T or is empty", item["Id"])
	}
	userData, ok := item["UserData"].(map[string]any)
	if !ok {
		return fmt.Errorf("apply Filmly Resume compatibility: item %s UserData has type %T", itemID, item["UserData"])
	}
	embyAttachFilmlyUserDataIdentity(userData, itemID)
	return nil
}

func embyItemsParamsRequestResumeRows(params service.ItemsParams) bool {
	for _, filter := range params.Filters {
		if strings.EqualFold(strings.TrimSpace(filter), "IsResumable") {
			return true
		}
	}
	return false
}

// embyApplyFilmlyResumeCompatibility gives each Filmly Resume UserData payload
// an explicit item identity and keeps episode presentation at episode scope.
// Other Emby clients retain the standard DTO.
func embyApplyFilmlyResumeCompatibility(c *gin.Context, out map[string]any) error {
	if c == nil {
		return fmt.Errorf("apply Filmly Resume compatibility: request context is nil")
	}
	if !embyRequestIsFilmly(c) {
		return nil
	}
	items, ok := out["Items"].([]map[string]any)
	if !ok {
		return fmt.Errorf("apply Filmly Resume compatibility: Items has type %T", out["Items"])
	}
	for _, item := range items {
		if err := embyAttachFilmlyResumeItemIdentity(item); err != nil {
			return err
		}
		itemType, ok := item["Type"].(string)
		if !ok {
			return fmt.Errorf("apply Filmly Resume compatibility: item %v Type has type %T", item["Id"], item["Type"])
		}
		if !strings.EqualFold(strings.TrimSpace(itemType), "Episode") {
			continue
		}

		seriesName, ok := item["SeriesName"].(string)
		if !ok {
			return fmt.Errorf("apply Filmly Resume compatibility: episode %v SeriesName has type %T", item["Id"], item["SeriesName"])
		}
		name, ok := item["Name"].(string)
		if !ok {
			return fmt.Errorf("apply Filmly Resume compatibility: episode %v Name has type %T", item["Id"], item["Name"])
		}
		if strings.TrimSpace(seriesName) != "" {
			indexNumber, ok := item["IndexNumber"].(int)
			if !ok {
				return fmt.Errorf("apply Filmly Resume compatibility: episode %v IndexNumber has type %T", item["Id"], item["IndexNumber"])
			}
			item["Name"] = embyFilmlyResumeEpisodeTitle(seriesName, indexNumber, name)
		}
		etag, ok := item["Etag"].(string)
		if !ok || strings.TrimSpace(etag) == "" {
			return fmt.Errorf("apply Filmly Resume compatibility: episode %v Etag has type %T or is empty", item["Id"], item["Etag"])
		}
		if !strings.HasSuffix(etag, embyFilmlyResumeETagSuffix) {
			item["Etag"] = etag + embyFilmlyResumeETagSuffix
		}

		for _, key := range []string{
			"ParentId",
			"ParentIndexNumber",
			"SeasonId",
			"SeasonName",
			"SeriesId",
			"SeriesName",
			"BackdropImageItemId",
			"ParentBackdropItemId",
			"ParentBackdropImageTags",
		} {
			delete(item, key)
		}
	}
	return nil
}

func embyFilmlyResumeEpisodeTitle(seriesName string, indexNumber int, name string) string {
	parts := make([]string, 0, 3)
	if seriesName = strings.TrimSpace(seriesName); seriesName != "" {
		parts = append(parts, seriesName)
	}
	if indexNumber > 0 {
		episodeLabel := fmt.Sprintf("第%d集", indexNumber)
		parts = append(parts, episodeLabel)
		if strings.TrimSpace(name) == episodeLabel {
			name = ""
		}
	}
	if name = strings.TrimSpace(name); name != "" {
		parts = append(parts, name)
	}
	return strings.Join(parts, " ")
}
