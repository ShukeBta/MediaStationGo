package service

import "strings"

// PersonMetadata carries provider person artwork alongside a metadata match.
// It is separate from model.Person because provider results are not persisted
// until a match has been accepted.
type PersonMetadata struct {
	Name       string `json:"name"`
	ImageURL   string `json:"image_url,omitempty"`
	ProfileURL string `json:"profile_url,omitempty"`
	Source     string `json:"source,omitempty"`
	SourceID   string `json:"source_id,omitempty"`
}

func personMetadataNames(people []PersonMetadata) []string {
	names := make([]string, 0, len(people))
	for _, person := range people {
		if name := strings.TrimSpace(person.Name); name != "" {
			names = append(names, name)
		}
	}
	return deduplicate(names)
}

func deduplicatePersonMetadata(people []PersonMetadata) []PersonMetadata {
	out := make([]PersonMetadata, 0, len(people))
	indexes := map[string]int{}
	for _, person := range people {
		person.Name = strings.TrimSpace(person.Name)
		if person.Name == "" {
			continue
		}
		key := strings.ToLower(person.Name)
		if index, ok := indexes[key]; ok {
			if out[index].ImageURL == "" {
				out[index].ImageURL = strings.TrimSpace(person.ImageURL)
			}
			if out[index].ProfileURL == "" {
				out[index].ProfileURL = strings.TrimSpace(person.ProfileURL)
			}
			if out[index].Source == "" {
				out[index].Source = strings.TrimSpace(person.Source)
			}
			if out[index].SourceID == "" {
				out[index].SourceID = strings.TrimSpace(person.SourceID)
			}
			continue
		}
		indexes[key] = len(out)
		person.ImageURL = strings.TrimSpace(person.ImageURL)
		person.ProfileURL = strings.TrimSpace(person.ProfileURL)
		person.Source = strings.TrimSpace(person.Source)
		person.SourceID = strings.TrimSpace(person.SourceID)
		out = append(out, person)
	}
	return out
}
