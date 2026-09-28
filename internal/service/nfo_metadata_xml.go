package service

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// Only edited top-level fields are replaced. Unrelated elements, nested
// technical information, actor roles, attributes and comments remain intact.
func nfoEditFields(before, after *model.Media, root string, fresh bool, updates map[string]any) map[string][]xml.Token {
	fields := map[string][]xml.Token{}
	put := func(column, tag string, old, next any) {
		if !fresh {
			if _, ok := updates[column]; !ok || reflect.DeepEqual(old, next) {
				return
			}
		}
		values := []string{nfoScalar(next)}
		if tag == "genre" || tag == "country" || tag == "language" || tag == "actor" {
			values = splitNFOList(nfoScalar(next))
		}
		fields[tag] = nil
		for _, value := range values {
			if value == "" {
				continue
			}
			start := xml.StartElement{Name: xml.Name{Local: tag}}
			fields[tag] = append(fields[tag], start)
			if tag == "actor" {
				fields[tag] = append(fields[tag], xml.StartElement{Name: xml.Name{Local: "name"}})
			}
			fields[tag] = append(fields[tag], xml.CharData(value))
			if tag == "actor" {
				fields[tag] = append(fields[tag], xml.EndElement{Name: xml.Name{Local: "name"}})
			}
			fields[tag] = append(fields[tag], start.End())
		}
	}
	titleTag := "title"
	if root == "episodedetails" {
		titleTag = "showtitle"
		put("episode_title", "title", before.EpisodeTitle, after.EpisodeTitle)
		put("season_num", "season", before.SeasonNum, after.SeasonNum)
		put("episode_num", "episode", before.EpisodeNum, after.EpisodeNum)
	}
	put("title", titleTag, before.Title, after.Title)
	put("original_name", "originaltitle", before.OriginalName, after.OriginalName)
	put("overview", "plot", before.Overview, after.Overview)
	put("year", "year", before.Year, after.Year)
	dateTag := "premiered"
	if root == "episodedetails" {
		dateTag = "aired"
	}
	put("release_date", dateTag, before.ReleaseDate, after.ReleaseDate)
	put("rating", "rating", before.Rating, after.Rating)
	// Explicit zero must not fall back to a stale nested tMM rating on rescan.
	if _, ok := fields["rating"]; ok && after.Rating == 0 {
		fields["ratings"] = nil
	}
	put("genres", "genre", before.Genres, after.Genres)
	put("countries", "country", before.Countries, after.Countries)
	put("languages", "language", before.Languages, after.Languages)
	put("actors", "actor", before.Actors, after.Actors)
	put("poster_url", "thumb", before.PosterURL, after.PosterURL)
	put("backdrop_url", "fanart", before.BackdropURL, after.BackdropURL)
	put("nsfw", "nsfw", before.NSFW, after.NSFW)
	// Episodic external IDs in Media identify the show, while NFO IDs identify
	// the episode. Never write show IDs into an episode's uniqueid elements.
	if root != "episodedetails" {
		for _, id := range []struct {
			column, kind string
			old, next    any
		}{
			{"tm_db_id", "tmdb", before.TMDbID, after.TMDbID},
			{"bangumi_id", "bangumi", before.BangumiID, after.BangumiID},
			{"douban_id", "douban", before.DoubanID, after.DoubanID},
			{"thetvdb_id", "tvdb", before.TheTVDBID, after.TheTVDBID},
		} {
			if _, ok := updates[id.column]; !fresh && (!ok || reflect.DeepEqual(id.old, id.next)) {
				continue
			}
			key := "uniqueid:" + id.kind
			fields[key] = nil
			value := nfoScalar(id.next)
			if value != "" && value != "0" {
				start := xml.StartElement{Name: xml.Name{Local: "uniqueid"}, Attr: []xml.Attr{{Name: xml.Name{Local: "type"}, Value: id.kind}}}
				fields[key] = []xml.Token{start, xml.CharData(value), start.End()}
			}
		}
		put("tm_db_id", "tmdbid", before.TMDbID, after.TMDbID)
	}
	return fields
}

func rewriteEditedNFO(input []byte, root string, fields map[string][]xml.Token) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(input))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	depth, roots := 0, 0
	seen := map[string]bool{}
	emit := func(key string) error {
		if seen[key] {
			return nil
		}
		seen[key] = true
		for _, token := range fields[key] {
			if err := encoder.EncodeToken(token); err != nil {
				return err
			}
		}
		return nil
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch node := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || node.Name.Local != root {
					return nil, errors.New("NFO root does not match this media type")
				}
			}
			if depth == 1 && node.Name.Space == "" {
				key := node.Name.Local
				if key == "uniqueid" {
					for _, a := range node.Attr {
						if a.Name.Local == "type" {
							key += ":" + strings.ToLower(a.Value)
						}
					}
				}
				if _, replace := fields[key]; replace {
					if err := emit(key); err != nil {
						return nil, err
					}
					if err := decoder.Skip(); err != nil {
						return nil, err
					}
					continue
				}
			}
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				keys := make([]string, 0, len(fields))
				for key := range fields {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					if err := emit(key); err != nil {
						return nil, err
					}
				}
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(node)) != "" {
				return nil, errors.New("invalid text outside NFO root")
			}
		}
		if err := encoder.EncodeToken(token); err != nil {
			return nil, err
		}
	}
	if roots != 1 || depth != 0 {
		return nil, errors.New("NFO must contain one complete XML document")
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
