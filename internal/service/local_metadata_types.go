package service

import (
	"encoding/xml"
	"strconv"
	"strings"
)

// LocalTechnicalMetadata contains stream details read from a sidecar NFO.
// They provide immediate metadata when probing is unavailable; ffprobe remains
// the authority once it has inspected the actual media stream.
type LocalTechnicalMetadata struct {
	DurationSec        int
	Width              int
	Height             int
	VideoCodec         string
	AudioCodec         string
	VideoBitRate       int64
	FrameRate          float64
	VideoProfile       string
	VideoRange         string
	VideoBitDepth      int
	AudioBitRate       int64
	AudioChannels      int
	AudioChannelLayout string
	AudioSampleRate    int
}

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
	Actors       string
	Countries    string
	Languages    string
	NSFW         bool
	HasNFO       bool
	HasArtwork   bool
	PathHint     bool
	Technical    LocalTechnicalMetadata
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
	Ratings       nfoRatings    `xml:"ratings"`
	Runtime       string        `xml:"runtime"`
	Poster        string        `xml:"poster"`
	Thumbs        []nfoThumb    `xml:"thumb"`
	Fanart        nfoFanart     `xml:"fanart"`
	Art           nfoArt        `xml:"art"`
	FileInfo      nfoFileInfo   `xml:"fileinfo"`
	StreamDetails nfoStreamInfo `xml:"streamdetails"`
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

// nfoRatings 承载 Kodi v18+ / tinyMediaManager 5.x 的嵌套评分块：
//
//	<ratings>
//	  <rating default="true" max="10" name="themoviedb">
//	    <value>6.6</value>
//	    <votes>3320</votes>
//	  </rating>
//	</ratings>
//
// 新版刮削器只写这种嵌套格式，不再写旧版独立 <rating>。若缺失，评分会
// 被误读为 0，因此 nfoDocument 需要同时解析两处并在取值时回退。
type nfoRatings struct {
	Items []nfoRating `xml:"rating"`
}

type nfoRating struct {
	Default string   `xml:"default,attr"`
	Max     string   `xml:"max,attr"`
	Name    string   `xml:"name,attr"`
	Value   nfoFloat `xml:"value"`
	Votes   nfoInt   `xml:"votes"`
}

type nfoFileInfo struct {
	StreamDetails nfoStreamInfo `xml:"streamdetails"`
}

type nfoStreamInfo struct {
	Videos []nfoVideoStream `xml:"video"`
	Audios []nfoAudioStream `xml:"audio"`
}

type nfoVideoStream struct {
	Codec             string   `xml:"codec"`
	Width             nfoInt   `xml:"width"`
	Height            nfoInt   `xml:"height"`
	DurationInSeconds nfoInt   `xml:"durationinseconds"`
	BitRate           string   `xml:"bitrate"`
	FrameRate         nfoFloat `xml:"framerate"`
	FPS               nfoFloat `xml:"fps"`
	Profile           string   `xml:"profile"`
	HDRType           string   `xml:"hdrtype"`
	ColorRange        string   `xml:"colorrange"`
	BitDepth          nfoInt   `xml:"bitdepth"`
}

type nfoAudioStream struct {
	Codec         string `xml:"codec"`
	BitRate       string `xml:"bitrate"`
	Channels      string `xml:"channels"`
	ChannelLayout string `xml:"channellayout"`
	SampleRate    nfoInt `xml:"samplerate"`
	SamplingRate  nfoInt `xml:"samplingrate"`
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

type nfoFloat float64

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
	if v, err := strconv.ParseFloat(raw, 64); err == nil {
		*n = nfoFloat(v)
	}
	return nil
}

type nfoActor struct {
	Name string `xml:"name"`
	Role string `xml:"role"`
}
