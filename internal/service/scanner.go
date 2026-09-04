// Package service — filesystem scanner.
//
// ScannerService walks the configured library roots looking for video
// files, then upserts a model.Media row per file. Each upsert also runs
// ffprobe (when available) and queues a metadata lookup for newly added
// rows.
//
// When a filename exposes season + episode numbers we store them on the
// Media row for every library type, so variety shows and other episodic
// collections get the same grouping experience as TV/anime.
package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// videoExtensions lists the file extensions treated as media. Matches the
// legacy Python defaults.
var videoExtensions = map[string]struct{}{
	".mkv":  {},
	".mp4":  {},
	".m4v":  {},
	".avi":  {},
	".mov":  {},
	".webm": {},
	".flv":  {},
	".wmv":  {},
	".ts":   {},
	".m2ts": {},
	".mts":  {},
	".vob":  {},
	".rmvb": {},
	".rm":   {},
	".3gp":  {},
	".mpg":  {},
	".mpeg": {},
	".iso":  {}, // disc image (Blu-ray/DVD ISO) — allow ingest so ISO 原盘 entries get a cloud direct link
	".strm": {},
}

// ScannerService walks libraries on disk and upserts model.Media rows.
type ScannerService struct {
	cfg              *config.Config
	log              *zap.Logger
	repo             *repository.Container
	hub              *Hub
	probe            *FFprobeService
	scraper          *ScraperService
	organizer        *OrganizerService
	storage          *StorageConfigService
	cache            *RuntimeCacheService
	notify           *NotifyChannelService
	subtitle         *SubtitleService
	generatedArtwork *GeneratedArtworkService

	imageProxy *ImageProxy

	cloudScanMu             sync.Mutex
	cloudScans              map[string]*cloudScanEntry
	cloudSlots              chan struct{}
	cloudImagePrefetchOnce  sync.Once
	cloudImagePrefetchQueue chan cloudImagePrefetchTask
	cloudImagePrefetchMu    sync.Mutex
	cloudImagePrefetching   map[string]struct{}
	localMediaProbeOnce     sync.Once
	localMediaProbeQueue    chan localMediaProbeTask
	localMediaProbeMu       sync.Mutex
	localMediaProbing       map[string]struct{}
	localScanMu             sync.Mutex
	localScans              map[string]struct{}
}

// NewScannerService is the constructor.
func NewScannerService(
	cfg *config.Config,
	log *zap.Logger,
	repo *repository.Container,
	hub *Hub,
	probe *FFprobeService,
	scraper *ScraperService,
) *ScannerService {
	return &ScannerService{
		cfg: cfg, log: log, repo: repo, hub: hub,
		probe:                   probe,
		scraper:                 scraper,
		cloudScans:              make(map[string]*cloudScanEntry),
		cloudSlots:              make(chan struct{}, 1),
		cloudImagePrefetchQueue: make(chan cloudImagePrefetchTask, 256),
		cloudImagePrefetching:   make(map[string]struct{}),
		localMediaProbeQueue:    make(chan localMediaProbeTask, 1024),
		localMediaProbing:       make(map[string]struct{}),
		localScans:              make(map[string]struct{}),
	}
}

// SetStorageConfig wires cloud-disk storage access into the scanner. It is set
// after service construction because StorageConfigService depends on Crypto,
// while the scanner is needed earlier by watcher/download services.
func (s *ScannerService) SetStorageConfig(storage *StorageConfigService) {
	s.storage = storage
}

func (s *ScannerService) SetOrganizer(organizer *OrganizerService) {
	if s != nil {
		s.organizer = organizer
	}
}

func (s *ScannerService) SetRuntimeCache(cache *RuntimeCacheService) {
	if s != nil {
		s.cache = cache
	}
}

func (s *ScannerService) SetNotifyChannels(notify *NotifyChannelService) {
	if s != nil {
		s.notify = notify
	}
}

func (s *ScannerService) SetGeneratedArtworkService(generated *GeneratedArtworkService) {
	if s != nil {
		s.generatedArtwork = generated
	}
}

func (s *ScannerService) SetSubtitleService(subtitle *SubtitleService) {
	if s != nil {
		s.subtitle = subtitle
	}
}

// SetImageProxy lets cloud scans warm sidecar poster/backdrop files into the
// local image cache. This keeps library opening fast without forcing the UI or
// Emby clients to resolve/download every cloud poster on demand.
func (s *ScannerService) SetImageProxy(imageProxy *ImageProxy) {
	s.imageProxy = imageProxy
	if imageProxy != nil {
		s.cloudImagePrefetchOnce.Do(func() {
			go s.cloudImagePrefetchWorker()
		})
	}
}

// ScanResult summarises a scan run.
type ScanResult struct {
	LibraryID     string   `json:"library_id"`
	Visited       int      `json:"visited"`
	Added         int      `json:"added"`
	Updated       int      `json:"updated"`
	Skipped       int      `json:"skipped"`
	Probed        int      `json:"probed"`
	LocalMetadata int      `json:"local_metadata"`
	Removed       int64    `json:"removed"`
	ErrorCount    int      `json:"error_count,omitempty"`
	Errors        []string `json:"errors,omitempty"`
}

var ErrCloudScanAlreadyRunning = errors.New("cloud scan already running")
var ErrLocalScanAlreadyRunning = errors.New("local scan already running")

const maxScanErrorDetails = 20

func addScanError(res *ScanResult, path string, err error) {
	if res == nil || err == nil {
		return
	}
	res.ErrorCount++
	if len(res.Errors) >= maxScanErrorDetails {
		return
	}
	path = strings.TrimSpace(path)
	msg := strings.TrimSpace(err.Error())
	if path != "" {
		msg = path + ": " + msg
	}
	res.Errors = append(res.Errors, msg)
}

// CloudScanStatus is the operator-facing state for long-running cloud scans.
type CloudScanStatus struct {
	LibraryID      string    `json:"library_id"`
	Provider       string    `json:"provider"`
	Stage          string    `json:"stage"`
	State          string    `json:"state"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
	Dirs           int       `json:"dirs"`
	Discovered     int       `json:"discovered"`
	Visited        int       `json:"visited"`
	Added          int       `json:"added"`
	Updated        int       `json:"updated"`
	Skipped        int       `json:"skipped"`
	Removed        int64     `json:"removed"`
	ErrorCount     int       `json:"error_count,omitempty"`
	Errors         []string  `json:"errors,omitempty"`
	Error          string    `json:"error,omitempty"`
	ResumeHint     string    `json:"resume_hint,omitempty"`
	Estimate       string    `json:"estimate_message,omitempty"`
	FilesPerSecond float64   `json:"files_per_second,omitempty"`
}

type cloudScanEntry struct {
	status CloudScanStatus
	cancel context.CancelFunc
}

type localMediaProbeTask struct {
	path string
}

type existingCloudMedia struct {
	LibraryID          string
	Title              string
	OriginalName       string
	EpisodeTitle       string
	SizeBytes          int64
	DurationSec        int
	Width              int
	Height             int
	VideoCodec         string
	AudioCodec         string
	Container          string
	BitRate            int64
	VideoBitRate       int64
	FrameRate          float64
	VideoProfile       string
	VideoRange         string
	VideoBitDepth      int
	AudioBitRate       int64
	AudioChannels      int
	AudioChannelLayout string
	AudioSampleRate    int
	MediaProbeVersion  int
	PosterURL          string
	BackdropURL        string
	STRMURL            string
	Overview           string
	Year               int
	ReleaseDate        string
	Rating             float32
	TMDbID             int
	BangumiID          int
	DoubanID           string
	TheTVDBID          string
	SeasonNum          int
	EpisodeNum         int
	Genres             string
	Actors             string
	Countries          string
	Languages          string
	NSFW               bool
	ScrapeStatus       string
}

type existingLocalMedia struct {
	LibraryRootID      string
	RelativePath       string
	Title              string
	OriginalName       string
	EpisodeTitle       string
	SizeBytes          int64
	DurationSec        int
	Width              int
	Height             int
	VideoCodec         string
	AudioCodec         string
	Container          string
	BitRate            int64
	VideoBitRate       int64
	FrameRate          float64
	VideoProfile       string
	VideoRange         string
	VideoBitDepth      int
	AudioBitRate       int64
	AudioChannels      int
	AudioChannelLayout string
	AudioSampleRate    int
	MediaProbeVersion  int
	STRMURL            string
	FileID             string
	PosterURL          string
	BackdropURL        string
	Overview           string
	Year               int
	ReleaseDate        string
	Rating             float32
	TMDbID             int
	BangumiID          int
	DoubanID           string
	TheTVDBID          string
	SeasonNum          int
	EpisodeNum         int
	Genres             string
	Actors             string
	Countries          string
	Languages          string
	NSFW               bool
	ScrapeStatus       string
}
