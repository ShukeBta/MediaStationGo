package service

import (
	"encoding/xml"
	"strconv"
	"strings"
)

// LocalMetadata contains metadata read from Kodi/Jellyfin sidecar NFO files.
type LocalMetadata struct {
	Title        string
	OriginalName string
	EpisodeTitle string
	AdultCode    string
	Year         int
	ReleaseDate  string
	Overview     string
	Rating       float32
	PosterURL    string
	BackdropURL  string
	TMDbID       int
	BangumiID    int
	DoubanID     string
	TheTVDBID    string
	SeasonNum    int
	EpisodeNum   int
	Genres       string
	Countries    string
	Languages    string
	NSFW         bool
	HasNFO       bool
	HasArtwork   bool
	PathHint     bool
}

type nfoUniqueID struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type nfoFanart struct {
	Value  string   `xml:",chardata"`
	Thumbs []string `xml:"thumb"`
}

type nfoThumb struct {
	Aspect string `xml:"aspect,attr"`
	Value  string `xml:",chardata"`
}

type nfoArt struct {
	Poster     string `xml:"poster"`
	Thumb      string `xml:"thumb"`
	Fanart     string `xml:"fanart"`
	Backdrop   string `xml:"backdrop"`
	Background string `xml:"background"`
	Banner     string `xml:"banner"`
	Landscape  string `xml:"landscape"`
}

type nfoDocument struct {
	XMLName       xml.Name      `xml:""`
	Title         string        `xml:"title"`
	ShowTitle     string        `xml:"showtitle"`
	OriginalTitle string        `xml:"originaltitle"`
	SortTitle     string        `xml:"sorttitle"`
	Num           string        `xml:"num"`
	Year          nfoInt        `xml:"year"`
	Premiered     string        `xml:"premiered"`
	ReleaseDate   string        `xml:"releasedate"`
	Release       string        `xml:"release"`
	Aired         string        `xml:"aired"`
	Plot          string        `xml:"plot"`
	Outline       string        `xml:"outline"`
	OriginalPlot  string        `xml:"originalplot"`
	Rating        nfoFloat      `xml:"rating"`
	Poster        string        `xml:"poster"`
	Thumbs        []nfoThumb    `xml:"thumb"`
	Fanart        nfoFanart     `xml:"fanart"`
	Art           nfoArt        `xml:"art"`
	TMDbID        nfoInt        `xml:"tmdbid"`
	UniqueIDs     []nfoUniqueID `xml:"uniqueid"`
	Season        nfoInt        `xml:"season"`
	Episode       nfoInt        `xml:"episode"`
	Genres        []string      `xml:"genre"`
	Tags          []string      `xml:"tag"`
	Countries     []string      `xml:"country"`
	Languages     []string      `xml:"language"`
	Studio        string        `xml:"studio"`
	Maker         string        `xml:"maker"`
	Publisher     string        `xml:"publisher"`
	Label         string        `xml:"label"`
	Directors     []string      `xml:"director"`
	Actors        []nfoActor    `xml:"actor"`
}

type nfoInt int

func (n *nfoInt) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var raw string
	if err := d.DecodeElement(&raw, &start); err != nil {
		return err
	}
	raw = cleanXMLText(raw)
	if raw == "" || strings.EqualFold(raw, "none") || strings.EqualFold(raw, "null") || strings.EqualFold(raw, "nan") {
		*n = 0
		return nil
	}
	if v, err := strconv.Atoi(raw); err == nil {
		*n = nfoInt(v)
		return nil
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		*n = nfoInt(int(f))
	}
	return nil
}

type nfoFloat float32

func (n *nfoFloat) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var raw string
	if err := d.DecodeElement(&raw, &start); err != nil {
		return err
	}
	raw = cleanXMLText(raw)
	if raw == "" || strings.EqualFold(raw, "none") || strings.EqualFold(raw, "null") || strings.EqualFold(raw, "nan") {
		*n = 0
		return nil
	}
	if v, err := strconv.ParseFloat(raw, 32); err == nil {
		*n = nfoFloat(v)
	}
	return nil
}

type nfoActor struct {
	Name string `xml:"name"`
	Role string `xml:"role"`
}
