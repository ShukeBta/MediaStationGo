package service

func (e *EmbyService) seriesPayload(group embySeriesGroup) map[string]any {
	e.rememberSeriesGroup(group)
	artworkUpdatedAt := group.ArtworkUpdatedAt
	if artworkUpdatedAt.IsZero() {
		artworkUpdatedAt = group.CreatedAt
	}
	imageTags := map[string]string{}
	backdropTags := []string{}
	if group.PosterURL != "" {
		imageTags["Primary"] = embyImageTag(group.ID, "primary", group.PosterURL, artworkUpdatedAt)
	}
	if group.BackdropURL != "" {
		backdropTags = append(backdropTags, embyImageTag(group.ID, "backdrop", group.BackdropURL, artworkUpdatedAt))
	}
	item := map[string]any{
		"Id":                 group.ID,
		"Name":               group.Name,
		"ServerId":           embyServerID,
		"Type":               "Series",
		"MediaType":          "Video",
		"IsFolder":           true,
		"ParentId":           group.LibraryID,
		"ProductionYear":     group.Year,
		"Overview":           group.Overview,
		"CommunityRating":    group.Rating,
		"RecursiveItemCount": len(group.Episodes),
		"ChildCount":         len(e.seasonsForSeries(group)),
		"DateCreated":        group.CreatedAt,
		"ImageTags":          imageTags,
		"BackdropImageTags":  backdropTags,
		"Genres":             group.Genres,
		"GenreItems":         embyGenreItems(group.Genres),
		"ProviderIds": map[string]string{
			"Tmdb":    intToStr(group.TMDbID),
			"Bangumi": intToStr(group.BangumiID),
		},
		"UserData": emptyUserData(),
	}
	if premiered, ok := embyPremiereDate(group.ReleaseDate); ok {
		item["PremiereDate"] = premiered
	}
	embyAttachImageOwnerIDs(item)
	return item
}

func (e *EmbyService) seasonPayload(season embySeasonGroup) map[string]any {
	e.rememberSeasonGroup(season)
	artworkUpdatedAt := season.Series.ArtworkUpdatedAt
	if artworkUpdatedAt.IsZero() {
		artworkUpdatedAt = season.Series.CreatedAt
	}
	imageTags := map[string]string{}
	backdropTags := []string{}
	if season.Series.PosterURL != "" {
		imageTags["Primary"] = embyImageTag(season.ID, "primary", season.Series.PosterURL, artworkUpdatedAt)
	}
	if season.Series.BackdropURL != "" {
		backdropTags = append(backdropTags, embyImageTag(season.ID, "backdrop", season.Series.BackdropURL, artworkUpdatedAt))
	}
	item := map[string]any{
		"Id":                season.ID,
		"Name":              season.Name,
		"ServerId":          embyServerID,
		"Type":              "Season",
		"MediaType":         "Video",
		"IsFolder":          true,
		"ParentId":          season.SeriesID,
		"SeriesId":          season.SeriesID,
		"SeriesName":        season.Series.Name,
		"IndexNumber":       season.SeasonNum,
		"ChildCount":        len(season.Episodes),
		"ImageTags":         imageTags,
		"BackdropImageTags": backdropTags,
		"Genres":            season.Series.Genres,
		"GenreItems":        embyGenreItems(season.Series.Genres),
		"UserData":          emptyUserData(),
	}
	embyAttachImageOwnerIDs(item)
	return item
}
