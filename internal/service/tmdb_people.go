package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type tmdbCreditCrew struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	ProfilePath  string `json:"profile_path"`
	Department   string `json:"department"`
	Job          string `json:"job"`
}

func localizedPersonDepartment(department string) string {
	translations := map[string]string{"Acting": "演员", "Directing": "导演", "Writing": "编剧", "Production": "制作", "Editing": "剪辑", "Camera": "摄影", "Sound": "声音", "Art": "美术", "Visual Effects": "视觉特效", "Costume & Make-Up": "服装与化妆", "Crew": "剧组"}
	if translated := translations[department]; translated != "" {
		return translated
	}
	return department
}

func localizedCrewRole(typ, role string) string {
	if typ == "Actor" || typ == "GuestStar" {
		return role
	}
	translations := map[string]string{"director": "导演", "writer": "编剧", "screenplay": "编剧", "story": "故事创作", "teleplay": "电视剧编剧", "creator": "主创", "producer": "制片人", "executive producer": "执行制片人", "original music composer": "原创音乐", "director of photography": "摄影指导", "editor": "剪辑"}
	if translated := translations[strings.ToLower(strings.TrimSpace(role))]; translated != "" {
		return translated
	}
	return role
}

func tmdbCrewPeople(crew []tmdbCreditCrew, imageCDN string) []PersonMetadata {
	people := make([]PersonMetadata, 0, len(crew))
	seen := map[string]bool{}
	for _, person := range crew {
		typ := ""
		switch {
		case person.Job == "Director":
			typ = "Director"
		case person.Department == "Writing":
			typ = "Writer"
		default:
			continue
		}
		key := fmt.Sprintf("%d/%s/%s", person.ID, person.Name, person.Job)
		if seen[key] || strings.TrimSpace(person.Name) == "" {
			continue
		}
		seen[key] = true
		rows := topTMDbPeople([]tmdbCreditCast{{ID: person.ID, Name: person.Name, OriginalName: person.OriginalName, ProfilePath: person.ProfilePath}}, imageCDN)
		if len(rows) == 0 {
			continue
		}
		row := rows[0]
		row.Type, row.OriginalRole = typ, person.Job
		row.Role = localizedCrewRole(typ, person.Job)
		people = append(people, row)
	}
	return people
}

func applyTMDbCrew(match *Match, crew []tmdbCreditCrew, imageCDN string) {
	for _, person := range tmdbCrewPeople(crew, imageCDN) {
		match.People = append(match.People, person)
		if person.Type == "Director" {
			match.Directors = append(match.Directors, person.Name)
		}
		if person.Type == "Writer" {
			match.Writers = append(match.Writers, person.Name)
		}
	}
	match.Directors, match.Writers = deduplicate(match.Directors), deduplicate(match.Writers)
}

// GetPersonDetails fetches biography and personal data with an English fallback
// for an unavailable Chinese biography; the original provider name is retained.
func (t *TMDbProvider) GetPersonDetails(ctx context.Context, sourceID string) (*model.Person, error) {
	if t == nil || t.resolveAPIKey(ctx) == "" {
		return nil, fmt.Errorf("TMDb 未配置")
	}
	var id int
	if _, err := fmt.Sscanf(sourceID, "%d", &id); err != nil || id <= 0 || fmt.Sprint(id) != sourceID {
		return nil, fmt.Errorf("无效的 TMDb 人物 ID")
	}
	q := url.Values{"api_key": {t.resolveAPIKey(ctx)}, "language": {"zh-CN"}, "append_to_response": {"translations"}}
	var result struct {
		Name               string   `json:"name"`
		Biography          string   `json:"biography"`
		Birthday           string   `json:"birthday"`
		Deathday           string   `json:"deathday"`
		PlaceOfBirth       string   `json:"place_of_birth"`
		KnownForDepartment string   `json:"known_for_department"`
		ProfilePath        string   `json:"profile_path"`
		AlsoKnownAs        []string `json:"also_known_as"`
		Translations       struct {
			Translations []struct {
				Language string `json:"iso_639_1"`
				Data     struct {
					Biography string `json:"biography"`
				} `json:"data"`
			} `json:"translations"`
		} `json:"translations"`
	}
	if err := t.getJSON(ctx, t.resolveBaseURL(ctx)+"/person/"+sourceID+"?"+q.Encode(), &result); err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.Biography) == "" {
		for _, translated := range result.Translations.Translations {
			if translated.Language == "en" {
				result.Biography = translated.Data.Biography
				break
			}
		}
	}
	now := time.Now()
	person := &model.Person{OriginalName: result.Name, Overview: result.Biography, Birthday: result.Birthday, Deathday: result.Deathday, Birthplace: result.PlaceOfBirth, Department: result.KnownForDepartment, Aliases: strings.Join(result.AlsoKnownAs, ","), DetailsRefreshedAt: &now}
	if result.ProfilePath != "" {
		person.ImageURL = strings.TrimRight(t.imgCDN, "/") + "/w500/" + strings.TrimLeft(result.ProfilePath, "/")
	}
	return person, nil
}
