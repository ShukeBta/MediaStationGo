package service

import (
	"context"
	"strings"
)

type DiscoverCatalogSearchResult struct {
	Items  []ExternalMediaResult
	Errors map[string]error
}

type discoverCatalogSearchTask struct {
	key string
	run func() ([]ExternalMediaResult, error)
}

type discoverCatalogSearchTaskResult struct {
	key   string
	items []ExternalMediaResult
	err   error
}

// SearchDiscoverCatalog searches the configured catalog providers in
// parallel. Callers decide which providers the current user may access by
// passing nil for disabled providers.
func SearchDiscoverCatalog(
	ctx context.Context,
	query string,
	tmdb *TMDbProvider,
	douban *DoubanProvider,
	bangumi *BangumiProvider,
	adult *AdultProvider,
) DiscoverCatalogSearchResult {
	if ctx == nil {
		ctx = context.Background()
	}
	query = strings.TrimSpace(query)
	result := DiscoverCatalogSearchResult{Items: []ExternalMediaResult{}, Errors: map[string]error{}}
	if query == "" {
		return result
	}

	fd2NumberQuery := adultFD2SearchNumber(query) != ""
	adultCodeQuery := normalizeAdultCode(query) != "" || fd2NumberQuery
	fd2OnlyQuery := fd2NumberQuery
	tasks := make([]discoverCatalogSearchTask, 0, 8)
	if tmdb != nil && !adultCodeQuery {
		tasks = append(tasks,
			discoverCatalogSearchTask{key: "tmdb_movie", run: func() ([]ExternalMediaResult, error) {
				matches, err := tmdb.SearchMovieCandidates(ctx, query, 0)
				return externalMediaResultsFromMatches("tmdb", "movie", matches, 12), err
			}},
			discoverCatalogSearchTask{key: "tmdb_tv", run: func() ([]ExternalMediaResult, error) {
				matches, err := tmdb.SearchTVCandidates(ctx, query, 0)
				return externalMediaResultsFromMatches("tmdb", "tv", matches, 12), err
			}},
		)
	}
	if douban != nil && !adultCodeQuery {
		tasks = append(tasks, discoverCatalogSearchTask{key: "douban", run: func() ([]ExternalMediaResult, error) {
			match, err := douban.SearchMatch(ctx, query)
			if err != nil || match == nil {
				return nil, err
			}
			mediaType := normalizeDoubanType(match.MediaType, "")
			return []ExternalMediaResult{externalMediaResultFromMatch("douban", mediaType, match)}, nil
		}})
	}
	if bangumi != nil && !adultCodeQuery {
		tasks = append(tasks, discoverCatalogSearchTask{key: "bangumi", run: func() ([]ExternalMediaResult, error) {
			match, err := bangumi.Search(ctx, query)
			if err != nil || match == nil {
				return nil, err
			}
			return []ExternalMediaResult{externalMediaResultFromMatch("bangumi", "anime", match)}, nil
		}})
	}
	if adult != nil {
		if !adultCodeQuery {
			tasks = append(tasks, discoverCatalogSearchTask{key: "javdb_performer", run: func() ([]ExternalMediaResult, error) {
				return adult.SearchPerformers(ctx, query)
			}})
		}
		if !fd2OnlyQuery {
			tasks = append(tasks, discoverCatalogSearchTask{key: "javdb_adult", run: func() ([]ExternalMediaResult, error) {
				return adult.SearchMovies(ctx, query)
			}})
		}
		if adult.FD2PPVEnabled() && (!adultCodeQuery || fd2OnlyQuery) {
			if !adultCodeQuery {
				tasks = append(tasks, discoverCatalogSearchTask{key: "fd2ppv_performer", run: func() ([]ExternalMediaResult, error) {
					return adult.SearchFD2PPVPerformers(ctx, query)
				}})
			}
			tasks = append(tasks, discoverCatalogSearchTask{key: "fd2ppv_adult", run: func() ([]ExternalMediaResult, error) {
				return adult.SearchFD2PPVMovies(ctx, query)
			}})
		}
	}

	return runDiscoverCatalogSearchTasks(ctx, tasks)
}

func runDiscoverCatalogSearchTasks(
	ctx context.Context,
	tasks []discoverCatalogSearchTask,
) DiscoverCatalogSearchResult {
	result := DiscoverCatalogSearchResult{Items: []ExternalMediaResult{}, Errors: map[string]error{}}
	if len(tasks) == 0 {
		return result
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		for _, task := range tasks {
			result.Errors[task.key] = err
		}
		return result
	}

	results := make(chan discoverCatalogSearchTaskResult, len(tasks))
	for _, task := range tasks {
		task := task
		go func() {
			items, err := task.run()
			results <- discoverCatalogSearchTaskResult{key: task.key, items: items, err: err}
		}()
	}

	byKey := make(map[string]discoverCatalogSearchTaskResult, len(tasks))
	for len(byKey) < len(tasks) {
		select {
		case taskResult := <-results:
			byKey[taskResult.key] = taskResult
		case <-ctx.Done():
			// Preserve every result that reached the buffered channel before the
			// deadline, then mark only the still-pending sources as timed out.
			for {
				select {
				case taskResult := <-results:
					byKey[taskResult.key] = taskResult
				default:
					for _, task := range tasks {
						if _, ok := byKey[task.key]; !ok {
							byKey[task.key] = discoverCatalogSearchTaskResult{key: task.key, err: ctx.Err()}
						}
					}
					return buildDiscoverCatalogSearchResult(tasks, byKey)
				}
			}
		}
	}
	return buildDiscoverCatalogSearchResult(tasks, byKey)
}

func buildDiscoverCatalogSearchResult(
	tasks []discoverCatalogSearchTask,
	byKey map[string]discoverCatalogSearchTaskResult,
) DiscoverCatalogSearchResult {
	result := DiscoverCatalogSearchResult{Items: []ExternalMediaResult{}, Errors: map[string]error{}}
	for _, task := range tasks {
		taskResult := byKey[task.key]
		if taskResult.err != nil {
			result.Errors[task.key] = taskResult.err
			continue
		}
		result.Items = append(result.Items, taskResult.items...)
	}
	result.Items = dedupeExternalMedia(result.Items)
	return result
}

func externalMediaResultsFromMatches(source, mediaType string, matches []*Match, limit int) []ExternalMediaResult {
	if limit <= 0 || limit > len(matches) {
		limit = len(matches)
	}
	items := make([]ExternalMediaResult, 0, limit)
	for _, match := range matches[:limit] {
		if match == nil || strings.TrimSpace(match.Title) == "" {
			continue
		}
		items = append(items, externalMediaResultFromMatch(source, mediaType, match))
	}
	return items
}
