package repository

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const mediaSearchAliasVersion = 1

var (
	mediaSearchPinyinArgs = func() pinyin.Args {
		args := pinyin.NewArgs()
		args.Style = pinyin.Normal
		args.Fallback = mediaSearchPinyinFallback
		return args
	}()
	mediaSearchInitialArgs = func() pinyin.Args {
		args := pinyin.NewArgs()
		args.Style = pinyin.FirstLetter
		args.Fallback = mediaSearchPinyinFallback
		return args
	}()
)

func mediaSearchPinyinFallback(r rune, _ pinyin.Args) []string {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return []string{strings.ToLower(string(r))}
	}
	return nil
}

func prepareMediaSearchAliases(media *model.Media) {
	if media == nil {
		return
	}
	media.SearchPinyin, media.SearchInitials = mediaSearchAliases(*media)
	media.SearchAliasVersion = mediaSearchAliasVersion
}

func mediaSearchAliases(media model.Media) (string, string) {
	values := []string{media.Title, media.OriginalName}
	values = append(values, splitMediaSearchAliasCSV(media.Actors)...)
	values = append(values, splitMediaSearchAliasCSV(media.Genres)...)
	return joinMediaSearchAliases(values, mediaSearchPinyinArgs), joinMediaSearchAliases(values, mediaSearchInitialArgs)
}

func splitMediaSearchAliasCSV(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
}

func joinMediaSearchAliases(values []string, args pinyin.Args) string {
	out := make([]string, 0, len(values)*2)
	seen := make(map[string]struct{}, len(values)*2)
	for _, value := range values {
		parts := pinyin.LazyPinyin(strings.TrimSpace(value), args)
		if len(parts) == 0 {
			continue
		}
		compact := normalizeMediaSearchAlias(strings.Join(parts, ""))
		spaced := normalizeMediaSearchAlias(strings.Join(parts, " "))
		for _, alias := range []string{compact, spaced} {
			if alias == "" {
				continue
			}
			if _, ok := seen[alias]; ok {
				continue
			}
			seen[alias] = struct{}{}
			out = append(out, alias)
		}
	}
	return strings.Join(out, " ")
}

func normalizeMediaSearchAlias(value string) string {
	var builder strings.Builder
	spacePending := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if spacePending && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteRune(r)
			spacePending = false
			continue
		}
		spacePending = true
	}
	return builder.String()
}

func (r *MediaRepository) RefreshSearchAliases(ctx context.Context, mediaID string) error {
	if r == nil || r.db == nil {
		return errors.New("media repository is not initialized")
	}
	if strings.TrimSpace(mediaID) == "" {
		return errors.New("media id is required")
	}
	var media model.Media
	if err := r.db.WithContext(ctx).Where("id = ?", strings.TrimSpace(mediaID)).First(&media).Error; err != nil {
		return err
	}
	prepareMediaSearchAliases(&media)
	if err := r.db.WithContext(ctx).Model(&model.Media{}).Where("id = ?", media.ID).Updates(map[string]any{
		"search_pinyin":        media.SearchPinyin,
		"search_initials":      media.SearchInitials,
		"search_alias_version": media.SearchAliasVersion,
	}).Error; err != nil {
		return err
	}
	r.indexMediaBestEffort(ctx, media)
	return nil
}

func (r *MediaRepository) BackfillSearchAliases(ctx context.Context, batchLimit int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("media repository is not initialized")
	}
	if batchLimit <= 0 {
		batchLimit = 100
	}
	var rows []model.Media
	if err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL AND search_alias_version < ?", mediaSearchAliasVersion).
		Order("id ASC").
		Limit(batchLimit).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	for i := range rows {
		prepareMediaSearchAliases(&rows[i])
		if err := r.db.WithContext(ctx).Model(&model.Media{}).Where("id = ?", rows[i].ID).Updates(map[string]any{
			"search_pinyin":        rows[i].SearchPinyin,
			"search_initials":      rows[i].SearchInitials,
			"search_alias_version": rows[i].SearchAliasVersion,
		}).Error; err != nil {
			return int64(i), err
		}
	}
	return int64(len(rows)), nil
}
