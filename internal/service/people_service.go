package service

import (
	"context"
	"errors"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

type peopleVisibilityKey struct{}

// PeopleContext carries the Web playback-profile restrictions to the shared
// people queries. It does not write to the Emby user-visibility cache.
func (e *EmbyService) PeopleContext(ctx context.Context, visibility MediaVisibility) context.Context {
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, e.repo, visibility)
	if !visibility.IncludeNSFW {
		visibility.HiddenLibraryIDs = e.hiddenLibraryIDs(ctx, visibility)
	}
	return context.WithValue(ctx, peopleVisibilityKey{}, visibility)
}

func (e *EmbyService) MediaPeople(ctx context.Context, mediaID, userID string) ([]model.EmbyPerson, error) {
	var media model.Media
	err := e.applyUserMediaVisibility(ctx, e.repo.DB.WithContext(ctx).Model(&model.Media{}), userID).Where("media.id = ?", mediaID).First(&media).Error
	if err != nil {
		return nil, err
	}
	return e.embyPeopleForMedia(ctx, &media), nil
}

func (e *EmbyService) PersonWorks(ctx context.Context, id, userID string, offset, limit int) (map[string]any, error) {
	name, ok := embyPersonName(id)
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := e.applyUserMediaVisibility(ctx, e.repo.DB.WithContext(ctx).Model(&model.Media{}), userID)
	q = applyEmbyPersonFilter(q, []string{embyPersonID(name)})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []model.Media
	if err := q.Order("title, id").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	// The public person page needs only these display fields, never source paths.
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{"id": row.ID, "title": row.Title, "year": row.Year, "poster_url": row.PosterURL, "episode_title": row.EpisodeTitle})
	}
	return map[string]any{"items": items, "total": total}, nil
}

func (c *Container) storedPerson(ctx context.Context, id string) (*model.Person, error) {
	name, ok := embyPersonName(id)
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	var person model.Person
	err := c.Repo.DB.WithContext(ctx).Where("name_key = ?", normalizePersonNameKey(name)).First(&person).Error
	return &person, err
}

func (c *Container) RefreshPerson(ctx context.Context, id string) error {
	person, err := c.storedPerson(ctx, id)
	if err != nil {
		return err
	}
	if person.Source != "tmdb" || person.SourceID == "" {
		return errors.New("该人物没有 TMDb 资料标识")
	}
	details, err := c.TMDb.GetPersonDetails(ctx, person.SourceID)
	if err != nil {
		return err
	}
	updates := map[string]any{"overview": details.Overview, "birthday": details.Birthday, "deathday": details.Deathday, "birthplace": details.Birthplace, "department": details.Department, "aliases": details.Aliases, "details_refreshed_at": details.DetailsRefreshedAt}
	updates["original_overview"] = details.Overview
	updates["aliases"] = strings.Join(deduplicate(append(splitCSV(person.Aliases), splitCSV(details.Aliases)...)), ",")
	if strings.TrimSpace(person.OriginalName) == "" {
		updates["original_name"] = details.OriginalName
	}
	if details.ImageURL != "" {
		updates["image_url"] = details.ImageURL
	}
	if err := c.Repo.DB.WithContext(ctx).Model(person).Updates(updates).Error; err != nil {
		return err
	}
	personMetadataVersion.Add(1)
	return nil
}

// RefreshMediaPeople also upgrades previously scraped media that only have the
// legacy actors CSV, without redoing title/artwork/episode selection.
func (c *Container) RefreshMediaPeople(ctx context.Context, mediaID string) error {
	media, err := c.Repo.Media.FindByID(ctx, mediaID)
	if err != nil {
		return err
	}
	if media == nil {
		return gorm.ErrRecordNotFound
	}
	if media.TMDbID <= 0 {
		return errors.New("作品没有 TMDb 标识，请先刮削元数据")
	}
	kind := "movie"
	if media.SeasonNum > 0 || media.EpisodeNum > 0 {
		kind = "tv"
	}
	if library, err := c.Repo.Library.FindByID(ctx, media.LibraryID); err == nil && library != nil && (library.Type == "tv" || library.Type == "anime") {
		kind = "tv"
	}
	details, err := c.TMDb.GetDetails(ctx, media.TMDbID, kind)
	if err != nil {
		return err
	}
	if details == nil {
		return errors.New("TMDb 没有返回演职员资料")
	}
	result := c.Repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ? AND tm_db_id = ?", media.ID, media.TMDbID).Update("actors", strings.Join(details.Actors, ","))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("作品元数据已变化，请重试")
	}
	if err := c.Scraper.persistMediaPeople(ctx, media.ID, &Match{People: details.People, Actors: details.Actors}); err != nil {
		return err
	}
	c.Scraper.invalidateMediaCache(ctx)
	return nil
}
