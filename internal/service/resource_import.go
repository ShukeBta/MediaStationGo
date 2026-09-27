package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	ResourceImportStatusQueued               = "queued"
	ResourceImportStatusRunning              = "running"
	ResourceImportStatusCompleted            = "completed"
	ResourceImportStatusCompletedWithWarning = "completed_with_warning"
	ResourceImportStatusFailed               = "failed"
	ResourceImportStatusCanceled             = "canceled"
	resourceSearchLimit                      = 200
	resourceSearchSessionTTL                 = 15 * time.Minute
)

var resourceImportFinalStatuses = []string{
	ResourceImportStatusCompleted,
	ResourceImportStatusCompletedWithWarning,
	ResourceImportStatusFailed,
	ResourceImportStatusCanceled,
}

var ErrResourceImportDeleteNotAllowed = errors.New("only failed resource import tasks can be deleted")

type ResourceImportService struct {
	cfg    config.ResourceImportConfig
	log    *zap.Logger
	repos  *repository.Container
	client resourcePipelineClient
	ctx    context.Context

	subscriptionFailureHandler    func(context.Context, model.ResourceImportJob) error
	subscriptionCompletionHandler func(context.Context, model.ResourceImportJob) error

	globalSem chan struct{}
	mu        sync.Mutex
	userSems  map[string]chan struct{}
	executing map[string]struct{}
	closed    bool
}

type ResourceSearchInput struct {
	Query              string `json:"query"`
	Source             string `json:"source,omitempty"`
	Page               int    `json:"page,omitempty"`
	PageSize           int    `json:"page_size,omitempty"`
	RootID             string `json:"root_id,omitempty"`
	ResultQuery        string `json:"result_query,omitempty"`
	SourceFilter       string `json:"source_filter,omitempty"`
	ResolutionFilter   string `json:"resolution_filter,omitempty"`
	SubtitleFilter     string `json:"subtitle_filter,omitempty"`
	SortBy             string `json:"sort_by,omitempty"`
	SubscriptionFollow bool   `json:"-"`
}

type ResourceSearchCapabilities struct {
	Sources   []string `json:"sources,omitempty"`
	Pansou    bool     `json:"pansou,omitempty"`
	BT4G      bool     `json:"bt4g,omitempty"`
	LLMRerank bool     `json:"llm_rerank,omitempty"`
}

type ResourceSearchCandidate struct {
	Index                int    `json:"index"`
	Title                string `json:"title"`
	SizeBytes            int64  `json:"size_bytes,omitempty"`
	SizeText             string `json:"size_text,omitempty"`
	Source               string `json:"source,omitempty"`
	Seeders              int    `json:"seeders,omitempty"`
	Resolution           string `json:"resolution,omitempty"`
	Subtitle             string `json:"subtitle,omitempty"`
	ResourceType         string `json:"resource_type,omitempty"`
	Summary              string `json:"summary,omitempty"`
	CompatibilityWarning string `json:"compatibility_warning,omitempty"`
	CandidateID          string `json:"-"`
}

type ResourceSearchRoot struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}

type ResourceSearchResponse struct {
	SessionID       string                     `json:"session_id"`
	Query           string                     `json:"query"`
	Page            int                        `json:"page"`
	PageSize        int                        `json:"page_size"`
	Total           int                        `json:"total"`
	UnfilteredTotal int                        `json:"unfiltered_total"`
	TotalPages      int                        `json:"total_pages"`
	Roots           []ResourceSearchRoot       `json:"roots,omitempty"`
	Capabilities    ResourceSearchCapabilities `json:"capabilities,omitempty"`
	Facets          ResourceSearchFacets       `json:"facets"`
	Results         []ResourceSearchCandidate  `json:"results"`
}

type ResourceSearchFacets struct {
	Sources     []string `json:"sources"`
	Resolutions []string `json:"resolutions"`
}

type ResourceSearchError struct {
	StatusCode   int
	Code         string
	Message      string
	Capabilities ResourceSearchCapabilities
}

func (e *ResourceSearchError) Error() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return "resource search failed"
	}
	return e.Message
}

func (e *ResourceSearchError) HTTPStatus() int {
	if e == nil {
		return 0
	}
	return e.StatusCode
}

type ResourceImportCreateInput struct {
	SearchSessionID    string `json:"search_session_id"`
	CandidateIndex     int    `json:"candidate_index"`
	RootID             string `json:"root_id"`
	SubscriptionID     string `json:"subscription_id,omitempty"`
	ForceDuplicate     bool   `json:"force_duplicate,omitempty"`
	UpgradeMediaID     string `json:"upgrade_media_id,omitempty"`
	UpgradeScope       string `json:"upgrade_scope,omitempty"`
	KeepOldVersion     *bool  `json:"keep_old_version,omitempty"`
	SubscriptionFollow bool   `json:"-"`
	ManualReplenish    bool   `json:"-"`
	WorkKey            string `json:"-"`
	Season             int    `json:"-"`
	ExistingEpisodes   []int  `json:"-"`
	ReservedEpisodes   []int  `json:"-"`
	ExpectedEpisodes   []int  `json:"-"`
	TargetOpenListPath string `json:"-"`
	TitleClass         string `json:"-"`
	IsAdmin            bool   `json:"-"`
}

// EpisodeReplenishmentContext is the server-derived target for a manual
// episode replenish. Clients may select a resource, but never supply the
// library, work, season, existing episodes, or final OpenList directory.
type EpisodeReplenishmentContext struct {
	MediaID                string `json:"media_id"`
	LibraryID              string `json:"library_id"`
	RootID                 string `json:"root_id"`
	Title                  string `json:"title"`
	Category               string `json:"category"`
	WorkKey                string `json:"work_key"`
	Season                 int    `json:"season"`
	TargetOpenListPath     string `json:"target_openlist_path"`
	ExistingEpisodes       []int  `json:"existing_episodes"`
	MissingEpisodes        []int  `json:"missing_episodes"`
	KnownEpisodeUpperBound int    `json:"known_episode_upper_bound"`
}

type episodeReplenishmentTarget struct {
	context EpisodeReplenishmentContext
	library model.Library
	root    model.LibraryRoot
}

type ResourceImportDuplicate struct {
	CanForce bool   `json:"can_force"`
	MediaID  string `json:"media_id,omitempty"`
	Title    string `json:"title,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type ResourceImportDuplicateError struct {
	Message   string
	Duplicate ResourceImportDuplicate
}

func (e *ResourceImportDuplicateError) Error() string { return e.Message }

type resourcePipelineStateError struct {
	Status string
	Stage  string
}

func (e *resourcePipelineStateError) Error() string {
	return fmt.Sprintf("media-pipeline import returned an invalid state: status=%q stage=%q", e.Status, e.Stage)
}

type ResourceImportTask struct {
	ID                 string     `json:"id"`
	UserID             string     `json:"user_id,omitempty"`
	CreatorUsername    string     `json:"creator_username,omitempty"`
	SubscriptionID     string     `json:"subscription_id,omitempty"`
	SubscriptionFollow bool       `json:"subscription_follow,omitempty"`
	ManualReplenish    bool       `json:"manual_replenish,omitempty"`
	WorkKey            string     `json:"work_key,omitempty"`
	SeasonNumber       int        `json:"season_number,omitempty"`
	TitleClass         string     `json:"title_class,omitempty"`
	TargetOpenListPath string     `json:"target_openlist_path,omitempty"`
	Outcome            string     `json:"outcome,omitempty"`
	ExistingEpisodes   []int      `json:"existing_episodes,omitempty"`
	MissingEpisodes    []int      `json:"missing_episodes,omitempty"`
	SelectedEpisodes   []int      `json:"selected_episodes,omitempty"`
	MovedEpisodes      []int      `json:"moved_episodes,omitempty"`
	VerifiedEpisodes   []int      `json:"verified_episodes,omitempty"`
	ScanAdded          int        `json:"scan_added,omitempty"`
	LibraryID          string     `json:"library_id"`
	LibraryName        string     `json:"library_name,omitempty"`
	RootID             string     `json:"root_id"`
	RootName           string     `json:"root_name,omitempty"`
	SearchSessionID    string     `json:"search_session_id,omitempty"`
	CandidateIndex     int        `json:"candidate_index"`
	CandidateTitle     string     `json:"candidate_title,omitempty"`
	Source             string     `json:"source,omitempty"`
	Status             string     `json:"status"`
	Stage              string     `json:"stage,omitempty"`
	Progress           int        `json:"progress"`
	Message            string     `json:"message,omitempty"`
	Error              string     `json:"error,omitempty"`
	PipelineJobID      string     `json:"pipeline_job_id,omitempty"`
	MediaID            string     `json:"media_id,omitempty"`
	MediaTitle         string     `json:"media_title,omitempty"`
	UpgradeMediaID     string     `json:"upgrade_media_id,omitempty"`
	UpgradeScope       string     `json:"upgrade_scope,omitempty"`
	KeepOldVersion     bool       `json:"keep_old_version"`
	CancelRequested    bool       `json:"cancel_requested"`
	Attempt            int        `json:"attempt"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
}

type ResourceImportListFilter struct {
	LibraryID string
	UserID    string
	Status    string
	Page      int
	PageSize  int
}

type ResourceImportListResult struct {
	Items    []ResourceImportTask `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

type storedResourceSearch struct {
	PipelineSessionID string                     `json:"pipeline_session_id"`
	Capabilities      ResourceSearchCapabilities `json:"capabilities"`
	Candidates        []storedResourceCandidate  `json:"candidates"`
}

type storedResourceCandidate struct {
	ResourceSearchCandidate
	CandidateID string `json:"candidate_id"`
}

func NewResourceImportService(cfg config.ResourceImportConfig, log *zap.Logger, repos *repository.Container, ctx context.Context) (*ResourceImportService, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	client, err := newResourcePipelineHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	return newResourceImportServiceWithClient(cfg, log, repos, ctx, client), nil
}

func newResourceImportServiceWithClient(cfg config.ResourceImportConfig, log *zap.Logger, repos *repository.Container, ctx context.Context, client resourcePipelineClient) *ResourceImportService {
	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	if cfg.MaxConcurrentPerUser <= 0 {
		cfg.MaxConcurrentPerUser = 2
	}
	if cfg.PollSeconds <= 0 {
		cfg.PollSeconds = 5
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &ResourceImportService{
		cfg:       cfg,
		log:       log,
		repos:     repos,
		client:    client,
		ctx:       ctx,
		globalSem: make(chan struct{}, maxConcurrent),
		userSems:  make(map[string]chan struct{}),
		executing: make(map[string]struct{}),
	}
}

func (s *ResourceImportService) SetSubscriptionFailureHandler(handler func(context.Context, model.ResourceImportJob) error) {
	if s == nil {
		return
	}
	s.subscriptionFailureHandler = handler
}

func (s *ResourceImportService) SetSubscriptionCompletionHandler(handler func(context.Context, model.ResourceImportJob) error) {
	if s == nil {
		return
	}
	s.subscriptionCompletionHandler = handler
}

func (s *ResourceImportService) Search(ctx context.Context, userID string, library model.Library, root model.LibraryRoot, in ResourceSearchInput) (ResourceSearchResponse, error) {
	if s == nil || s.client == nil || s.repos == nil || s.repos.DB == nil {
		return ResourceSearchResponse{}, errors.New("resource import service unavailable")
	}
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return ResourceSearchResponse{}, errors.New("query is required")
	}
	if len(query) > 512 {
		return ResourceSearchResponse{}, errors.New("query is too long")
	}
	if err := validateResourceSearchView(in); err != nil {
		return ResourceSearchResponse{}, err
	}
	source := strings.ToLower(strings.TrimSpace(in.Source))
	if source == "" {
		source = "default"
	}
	if source != "default" && source != "pansou" && source != "bt4g" {
		return ResourceSearchResponse{}, errors.New("unsupported resource search source")
	}
	category, _, _ := resourceTargetMetadata(library.Type)
	if _, err := resourceRootOpenListPath(root.Path); err != nil {
		return ResourceSearchResponse{}, err
	}
	if err := s.repos.DB.WithContext(ctx).Unscoped().Where("expires_at <= ?", time.Now()).Delete(&model.ResourceSearchSession{}).Error; err != nil {
		return ResourceSearchResponse{}, err
	}

	if cached, ok, err := s.findCachedSearch(ctx, userID, library.ID, root.ID, query, source, false); err != nil {
		return ResourceSearchResponse{}, err
	} else if ok && !in.SubscriptionFollow {
		var stored storedResourceSearch
		if err := json.Unmarshal([]byte(cached.ResultsJSON), &stored); err != nil {
			return ResourceSearchResponse{}, errors.New("cached resource search session data is invalid")
		}
		if storedResourceSearchValid(stored) {
			return resourceSearchPage(cached, stored, root, in), nil
		}
		if err := s.repos.DB.WithContext(ctx).Unscoped().Delete(&cached).Error; err != nil {
			return ResourceSearchResponse{}, err
		}
		if s.log != nil {
			s.log.Warn("discarded invalid resource search cache", zap.String("session_id", cached.ID))
		}
	}

	pipeline, err := s.client.Search(ctx, resourcePipelineSearchRequest{
		OwnerID:            userID,
		Query:              query,
		Category:           category,
		Source:             source,
		Limit:              resourceSearchLimit,
		SubscriptionFollow: in.SubscriptionFollow,
	})
	if err != nil {
		var pipelineErr *resourcePipelineError
		if errors.As(err, &pipelineErr) {
			return ResourceSearchResponse{}, &ResourceSearchError{
				StatusCode:   pipelineErr.StatusCode,
				Code:         pipelineErr.Code,
				Message:      pipelineErr.Message,
				Capabilities: pipelineErr.Capabilities,
			}
		}
		return ResourceSearchResponse{}, err
	}
	if strings.TrimSpace(pipeline.SessionID) == "" {
		return ResourceSearchResponse{}, errors.New("media-pipeline search returned no session_id")
	}
	return s.persistResourceSearch(ctx, userID, library, root, query, source, pipeline, in)
}

func (s *ResourceImportService) PrepareManual(ctx context.Context, userID string, library model.Library, root model.LibraryRoot, input, title string) (ResourceSearchResponse, error) {
	return s.prepareManual(ctx, userID, library, root, input, title, false)
}

func (s *ResourceImportService) prepareManual(
	ctx context.Context,
	userID string,
	library model.Library,
	root model.LibraryRoot,
	input, title string,
	subscriptionFollow bool,
) (ResourceSearchResponse, error) {
	if s == nil || s.client == nil || s.repos == nil || s.repos.DB == nil {
		return ResourceSearchResponse{}, errors.New("resource import service unavailable")
	}
	input = strings.TrimSpace(input)
	title = strings.TrimSpace(title)
	if input == "" {
		return ResourceSearchResponse{}, errors.New("input is required")
	}
	if len([]rune(input)) > 4096 {
		return ResourceSearchResponse{}, errors.New("input is too long")
	}
	if title == "" {
		return ResourceSearchResponse{}, errors.New("title is required")
	}
	if len([]rune(title)) > 200 {
		return ResourceSearchResponse{}, errors.New("title is too long")
	}
	category, _, _ := resourceTargetMetadata(library.Type)
	if _, err := resourceRootOpenListPath(root.Path); err != nil {
		return ResourceSearchResponse{}, err
	}
	client, ok := s.client.(resourcePipelineManualClient)
	if !ok {
		return ResourceSearchResponse{}, errors.New("manual resource task is unavailable")
	}
	pipeline, err := client.PrepareManual(ctx, resourcePipelineManualRequest{
		OwnerID:  userID,
		Input:    input,
		Title:    title,
		Category: category,
	})
	if err != nil {
		return ResourceSearchResponse{}, err
	}
	if strings.TrimSpace(pipeline.SessionID) == "" {
		return ResourceSearchResponse{}, errors.New("media-pipeline manual candidate returned no session_id")
	}
	if len(pipeline.Items) != 1 {
		return ResourceSearchResponse{}, errors.New("media-pipeline manual candidate returned an invalid item count")
	}
	candidateTitle := resourceString(pipeline.Items[0], "title", "name")
	if candidateTitle == "" {
		return ResourceSearchResponse{}, errors.New("media-pipeline manual candidate returned no title")
	}
	if candidateTitle != title {
		return ResourceSearchResponse{}, errors.New("media-pipeline manual candidate title mismatch")
	}
	return s.persistResourceSearch(
		ctx, userID, library, root, title, "manual", pipeline,
		ResourceSearchInput{Page: 1, PageSize: 1, SubscriptionFollow: subscriptionFollow},
	)
}

func (s *ResourceImportService) ReplenishEpisodes(ctx context.Context, userID, mediaID, input string) (ResourceImportTask, error) {
	target, err := s.resolveEpisodeReplenishmentTarget(ctx, mediaID)
	if err != nil {
		return ResourceImportTask{}, err
	}
	preview, err := s.prepareManual(ctx, userID, target.library, target.root, input, target.context.Title, true)
	if err != nil {
		return ResourceImportTask{}, err
	}
	if len(preview.Results) != 1 || len(preview.Roots) != 1 {
		return ResourceImportTask{}, errors.New("补集候选响应无效")
	}
	return s.createEpisodeReplenishment(ctx, userID, target, preview.SessionID, preview.Results[0].Index)
}

func (s *ResourceImportService) EpisodeReplenishmentContext(ctx context.Context, mediaID string) (EpisodeReplenishmentContext, error) {
	target, err := s.resolveEpisodeReplenishmentTarget(ctx, mediaID)
	if err != nil {
		return EpisodeReplenishmentContext{}, err
	}
	return target.context, nil
}

func (s *ResourceImportService) SearchEpisodeReplenishment(ctx context.Context, userID, mediaID string, in ResourceSearchInput) (ResourceSearchResponse, error) {
	target, err := s.resolveEpisodeReplenishmentTarget(ctx, mediaID)
	if err != nil {
		return ResourceSearchResponse{}, err
	}
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" {
		in.Query = target.context.Title
	}
	in.RootID = target.root.ID
	in.SubscriptionFollow = true
	return s.Search(ctx, userID, target.library, target.root, in)
}

func (s *ResourceImportService) CreateEpisodeReplenishment(
	ctx context.Context,
	userID, mediaID, searchSessionID string,
	candidateIndex int,
) (ResourceImportTask, error) {
	target, err := s.resolveEpisodeReplenishmentTarget(ctx, mediaID)
	if err != nil {
		return ResourceImportTask{}, err
	}
	return s.createEpisodeReplenishment(ctx, userID, target, searchSessionID, candidateIndex)
}

func (s *ResourceImportService) createEpisodeReplenishment(
	ctx context.Context,
	userID string,
	target episodeReplenishmentTarget,
	searchSessionID string,
	candidateIndex int,
) (ResourceImportTask, error) {
	_, stored, err := s.loadOwnedSearch(ctx, userID, target.library.ID, target.root.ID, searchSessionID)
	if err != nil {
		return ResourceImportTask{}, err
	}
	if candidateIndex < 0 || candidateIndex >= len(stored.Candidates) {
		return ResourceImportTask{}, errors.New("candidate_index is out of range")
	}
	return s.Create(ctx, userID, target.library, target.root, ResourceImportCreateInput{
		SearchSessionID: strings.TrimSpace(searchSessionID), CandidateIndex: candidateIndex, RootID: target.root.ID,
		SubscriptionFollow: true, ManualReplenish: true,
		WorkKey: target.context.WorkKey, Season: target.context.Season,
		ExistingEpisodes: target.context.ExistingEpisodes, TargetOpenListPath: target.context.TargetOpenListPath,
		TitleClass: "unknown", IsAdmin: true,
	})
}

func (s *ResourceImportService) resolveEpisodeReplenishmentTarget(ctx context.Context, mediaID string) (episodeReplenishmentTarget, error) {
	if s == nil || s.repos == nil || s.repos.DB == nil || s.repos.Media == nil || s.repos.Library == nil {
		return episodeReplenishmentTarget{}, errors.New("resource import service unavailable")
	}
	media, err := s.repos.Media.FindByID(ctx, strings.TrimSpace(mediaID))
	if err != nil {
		return episodeReplenishmentTarget{}, err
	}
	if media == nil {
		return episodeReplenishmentTarget{}, errors.New("media not found")
	}
	if media.SeasonNum <= 0 || media.EpisodeNum <= 0 {
		return episodeReplenishmentTarget{}, errors.New("当前媒体不是具有明确作品、季和集号的剧集")
	}
	seriesID := strings.TrimSpace(media.SeriesID)
	workKey := "series:" + seriesID
	if seriesID == "" {
		workKey = mediaSeriesKey(*media)
		if workKey == "" {
			return episodeReplenishmentTarget{}, errors.New("当前媒体缺少可用的剧集分组信息")
		}
	}
	mediaPath := pipelineCloudPathToOpenListPath(media.Path)
	if mediaPath == "" {
		return episodeReplenishmentTarget{}, errors.New("当前剧集不是有效的 OpenList 云盘媒体")
	}
	targetPath := pipelineNormalizeOpenListPath(path.Dir(mediaPath))
	if targetPath == "" || targetPath == "/" {
		return episodeReplenishmentTarget{}, errors.New("当前剧集缺少有效的正式目录")
	}
	library, err := s.repos.Library.FindByID(ctx, media.LibraryID)
	if err != nil {
		return episodeReplenishmentTarget{}, err
	}
	if library == nil || !library.Enabled {
		return episodeReplenishmentTarget{}, errors.New("目标媒体库不存在或已停用")
	}
	category, _, _ := resourceTargetMetadata(library.Type)
	if category != "tv" && category != "anime" {
		return episodeReplenishmentTarget{}, errors.New("补集只支持电视剧或动漫媒体库")
	}
	root, err := s.repos.Library.FindRootByID(ctx, library.ID, media.LibraryRootID)
	if err != nil {
		return episodeReplenishmentTarget{}, err
	}
	if root == nil || !root.Enabled {
		return episodeReplenishmentTarget{}, errors.New("当前剧集缺少可用的媒体库目录")
	}
	rootPath, err := resourceRootOpenListPath(root.Path)
	if err != nil {
		return episodeReplenishmentTarget{}, err
	}
	if targetPath == rootPath || !pipelinePathIsSameOrChild(targetPath, rootPath) {
		return episodeReplenishmentTarget{}, errors.New("当前剧集正式目录不在媒体库目录下")
	}
	var seasonRows []model.Media
	seasonQuery := s.repos.DB.WithContext(ctx).
		Where("library_id = ? AND library_root_id = ? AND season_num = ? AND episode_num > 0", library.ID, root.ID, media.SeasonNum)
	if seriesID != "" {
		seasonQuery = seasonQuery.Where("series_id = ?", seriesID)
	}
	if err := seasonQuery.Find(&seasonRows).Error; err != nil {
		return episodeReplenishmentTarget{}, err
	}
	existingEpisodes := make([]int, 0, len(seasonRows))
	for _, row := range seasonRows {
		if seriesID == "" && mediaSeriesKey(row) != workKey {
			continue
		}
		rowPath := pipelineCloudPathToOpenListPath(row.Path)
		if rowPath != "" && pipelineNormalizeOpenListPath(path.Dir(rowPath)) == targetPath {
			existingEpisodes = append(existingEpisodes, row.EpisodeNum)
		}
	}
	existingEpisodes = uniqueSortedPositiveInts(existingEpisodes)
	title := strings.TrimSpace(media.OriginalName)
	if title == "" {
		title = strings.TrimSpace(media.Title)
	}
	upperBound := 0
	if len(existingEpisodes) > 0 {
		upperBound = existingEpisodes[len(existingEpisodes)-1]
	}
	return episodeReplenishmentTarget{
		context: EpisodeReplenishmentContext{
			MediaID: media.ID, LibraryID: library.ID, RootID: root.ID, Title: title, Category: category,
			WorkKey: workKey, Season: media.SeasonNum, TargetOpenListPath: targetPath,
			ExistingEpisodes: existingEpisodes, MissingEpisodes: missingEpisodesThrough(existingEpisodes, upperBound),
			KnownEpisodeUpperBound: upperBound,
		},
		library: *library,
		root:    *root,
	}, nil
}

func (s *ResourceImportService) persistResourceSearch(
	ctx context.Context,
	userID string,
	library model.Library,
	root model.LibraryRoot,
	query string,
	source string,
	pipeline resourcePipelineSearchResponse,
	in ResourceSearchInput,
) (ResourceSearchResponse, error) {
	candidates, err := normalizeResourceCandidates(pipeline.Items)
	if err != nil {
		return ResourceSearchResponse{}, err
	}
	stored := storedResourceSearch{
		PipelineSessionID: pipeline.SessionID,
		Capabilities:      pipeline.Capabilities,
		Candidates:        candidates,
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return ResourceSearchResponse{}, err
	}
	expiresAt := time.Now().Add(resourceSearchSessionTTL)
	if pipeline.ExpiresAt > 0 {
		pipelineExpiry := time.Unix(pipeline.ExpiresAt, 0)
		if pipelineExpiry.Before(expiresAt) {
			expiresAt = pipelineExpiry
		}
	}
	record := model.ResourceSearchSession{
		UserID: userID, LibraryID: library.ID, LibraryRootID: root.ID,
		Query: query, Source: source, SubscriptionFollow: in.SubscriptionFollow,
		ResultsJSON: string(encoded), ExpiresAt: expiresAt,
	}
	if err := s.repos.DB.WithContext(ctx).Create(&record).Error; err != nil {
		return ResourceSearchResponse{}, err
	}
	return resourceSearchPage(record, stored, root, in), nil
}

func (s *ResourceImportService) Create(ctx context.Context, userID string, library model.Library, root model.LibraryRoot, in ResourceImportCreateInput) (ResourceImportTask, error) {
	if s == nil || s.client == nil || s.repos == nil || s.repos.DB == nil {
		return ResourceImportTask{}, errors.New("resource import service unavailable")
	}
	session, stored, err := s.loadOwnedSearch(ctx, userID, library.ID, root.ID, in.SearchSessionID)
	if err != nil {
		return ResourceImportTask{}, err
	}
	if in.CandidateIndex < 0 || in.CandidateIndex >= len(stored.Candidates) {
		return ResourceImportTask{}, errors.New("candidate_index is out of range")
	}
	storedCandidate := stored.Candidates[in.CandidateIndex]
	candidate := storedCandidate.ResourceSearchCandidate
	category, provider, mediaType := resourceTargetMetadata(library.Type)
	rootOpenListPath, err := resourceRootOpenListPath(root.Path)
	if err != nil {
		return ResourceImportTask{}, err
	}
	upgradeMediaID := strings.TrimSpace(in.UpgradeMediaID)
	if err := s.validateUpgradeTarget(ctx, library, root, upgradeMediaID); err != nil {
		return ResourceImportTask{}, err
	}
	upgradeScope, err := normalizeResourceImportUpgradeScope(category, upgradeMediaID, in.UpgradeScope)
	if err != nil {
		return ResourceImportTask{}, err
	}
	keepOldVersion := true
	if upgradeMediaID != "" && in.KeepOldVersion != nil {
		keepOldVersion = *in.KeepOldVersion
	}
	if upgradeMediaID != "" && !keepOldVersion && upgradeScope == "work" && !in.IsAdmin {
		return ResourceImportTask{}, fmt.Errorf("%w: 整剧替换旧片源仅管理员可操作", ErrMediaVersionForbidden)
	}
	if upgradeMediaID != "" && !keepOldVersion && upgradeScope == "media" {
		allowed, err := userCanManageMediaVersion(ctx, s.repos, userID, false, upgradeMediaID)
		if err != nil {
			return ResourceImportTask{}, err
		}
		if !allowed {
			return ResourceImportTask{}, fmt.Errorf("%w: 只有管理员或该片源的入库用户可以在升级成功后移除旧版本", ErrMediaVersionForbidden)
		}
	}
	subscriptionFollow := in.SubscriptionFollow
	manualReplenish := in.ManualReplenish
	workKey := strings.TrimSpace(in.WorkKey)
	targetOpenListPath := pipelineNormalizeOpenListPath(in.TargetOpenListPath)
	seasonNumber := in.Season
	if manualReplenish && !subscriptionFollow {
		return ResourceImportTask{}, errors.New("补集任务必须使用逐集校验链路")
	}
	if subscriptionFollow {
		if (!manualReplenish && strings.TrimSpace(in.SubscriptionID) == "") || workKey == "" || seasonNumber <= 0 || targetOpenListPath == "" {
			return ResourceImportTask{}, errors.New("追更任务缺少订阅、作品季或正式目录上下文")
		}
		if in.ForceDuplicate {
			return ResourceImportTask{}, errors.New("自动追更禁止 force_duplicate")
		}
		if upgradeMediaID != "" {
			return ResourceImportTask{}, errors.New("自动追更不能作为片源升级任务")
		}
		if rootOpenListPath == targetOpenListPath || !pipelinePathIsSameOrChild(targetOpenListPath, rootOpenListPath) {
			return ResourceImportTask{}, errors.New("追更正式目录必须位于当前媒体库根目录下")
		}
	}
	idempotencyKey := resourceImportIdempotencyKey(
		userID, library.ID, root.ID, session.ID, in.CandidateIndex, in.SubscriptionID, in.ForceDuplicate, upgradeMediaID, upgradeScope, keepOldVersion,
	)
	if existing, found, err := s.findJobByIdempotencyKey(ctx, idempotencyKey); err != nil {
		return ResourceImportTask{}, err
	} else if found {
		return s.taskDTO(ctx, existing, false)
	}
	candidateJSON, err := json.Marshal(candidate)
	if err != nil {
		return ResourceImportTask{}, err
	}
	existingEpisodesJSON, err := json.Marshal(uniqueSortedPositiveInts(in.ExistingEpisodes))
	if err != nil {
		return ResourceImportTask{}, err
	}
	reservedEpisodesJSON, err := json.Marshal(uniqueSortedPositiveInts(in.ReservedEpisodes))
	if err != nil {
		return ResourceImportTask{}, err
	}
	expectedEpisodes := uniqueSortedPositiveInts(in.ExpectedEpisodes)
	expectedEpisodesJSON, err := json.Marshal(expectedEpisodes)
	if err != nil {
		return ResourceImportTask{}, err
	}
	reservationKey := (*string)(nil)
	if subscriptionFollow {
		value := resourceImportReservationKey(library.ID, root.ID, workKey, seasonNumber)
		reservationKey = &value
	}
	record := model.ResourceImportJob{
		UserID: userID, SubscriptionID: strings.TrimSpace(in.SubscriptionID),
		SubscriptionFollow: subscriptionFollow, ManualReplenish: manualReplenish,
		WorkKey: workKey, SeasonNumber: seasonNumber,
		TitleClass: strings.TrimSpace(in.TitleClass), TargetOpenListPath: targetOpenListPath,
		ExistingEpisodesJSON: string(existingEpisodesJSON), ReservedEpisodesJSON: string(reservedEpisodesJSON), ExpectedEpisodesJSON: string(expectedEpisodesJSON),
		ActiveReservationKey: reservationKey,
		LibraryID:            library.ID, LibraryRootID: root.ID,
		SearchSessionID: session.ID, CandidateIndex: in.CandidateIndex,
		PipelineSearchSessionID: stored.PipelineSessionID, PipelineCandidateID: storedCandidate.CandidateID,
		CandidateJSON: string(candidateJSON), CandidateTitle: candidate.Title,
		CandidateSource: candidate.Source, CandidateSize: candidate.SizeBytes,
		Attempt: 1, IdempotencyKey: idempotencyKey, ForceDuplicate: in.ForceDuplicate,
		UpgradeMediaID: upgradeMediaID, UpgradeScope: upgradeScope, KeepOldVersion: keepOldVersion,
		Status: ResourceImportStatusQueued, Stage: "duplicate_check", Message: "等待 media-pipeline 接收任务",
	}
	reservedRecordCreated := false
	if subscriptionFollow {
		if err := s.repos.DB.WithContext(ctx).Create(&record).Error; err != nil {
			if existing, found, lookupErr := s.findJobByIdempotencyKey(ctx, idempotencyKey); lookupErr == nil && found {
				return s.taskDTO(ctx, existing, false)
			}
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return ResourceImportTask{}, errors.New("同一作品季已有追更任务正在处理")
			}
			return ResourceImportTask{}, err
		}
		reservedRecordCreated = true
	}

	pipelineTask, err := s.client.CreateImport(ctx, userID, idempotencyKey, resourcePipelineCreateRequest{
		SearchSessionID:    stored.PipelineSessionID,
		CandidateID:        storedCandidate.CandidateID,
		Category:           category,
		LibraryID:          library.ID,
		RootID:             root.ID,
		RootOpenListPath:   rootOpenListPath,
		Provider:           provider,
		MediaType:          mediaType,
		ForceDuplicate:     in.ForceDuplicate,
		UpgradeMediaID:     upgradeMediaID,
		UpgradeScope:       upgradeScope,
		KeepOldVersion:     keepOldVersion,
		SubscriptionFollow: subscriptionFollow,
		ManualReplenish:    manualReplenish,
		SubscriptionID:     strings.TrimSpace(in.SubscriptionID),
		WorkKey:            workKey,
		Season:             seasonNumber,
		ExistingEpisodes:   uniqueSortedPositiveInts(in.ExistingEpisodes),
		ReservedEpisodes:   uniqueSortedPositiveInts(in.ReservedEpisodes),
		ExpectedEpisodes:   expectedEpisodes,
		TargetOpenListPath: targetOpenListPath,
		TitleClass:         strings.TrimSpace(in.TitleClass),
	})
	if err != nil {
		if reservedRecordCreated {
			_ = s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", record.ID).Updates(map[string]any{
				"status": ResourceImportStatusFailed, "stage": "failed", "public_error": safePipelineMessage(err.Error()),
				"error": err.Error(), "finished_at": time.Now(), "active_reservation_key": nil,
			}).Error
		}
		var pipelineErr *resourcePipelineError
		if errors.As(err, &pipelineErr) && pipelineErr.StatusCode == 409 && pipelineErr.Duplicate != nil {
			return ResourceImportTask{}, &ResourceImportDuplicateError{Message: pipelineErr.Message, Duplicate: *pipelineErr.Duplicate}
		}
		return ResourceImportTask{}, err
	}
	if strings.TrimSpace(pipelineTask.ID) == "" {
		if reservedRecordCreated {
			_ = s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", record.ID).Updates(map[string]any{
				"status": ResourceImportStatusFailed, "stage": "failed", "public_error": "media-pipeline import returned no task id",
				"error": "media-pipeline import returned no task id", "finished_at": time.Now(), "active_reservation_key": nil,
			}).Error
		}
		return ResourceImportTask{}, errors.New("media-pipeline import returned no task id")
	}
	now := time.Now()
	status, stage := mapPipelineImportState(pipelineTask)
	if status == "" || stage == "" {
		_, _ = s.client.CancelImport(context.Background(), userID, pipelineTask.ID)
		if reservedRecordCreated {
			_ = s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", record.ID).Updates(map[string]any{
				"status": ResourceImportStatusFailed, "stage": "failed", "public_error": "media-pipeline import returned an invalid status or stage",
				"error": "media-pipeline import returned an invalid status or stage", "finished_at": time.Now(), "active_reservation_key": nil,
			}).Error
		}
		return ResourceImportTask{}, errors.New("media-pipeline import returned an invalid status or stage")
	}
	record.Status, record.Stage = status, stage
	record.Message = safePipelineMessage(pipelineTask.Message)
	record.PipelineJobID, record.MediaID = pipelineTask.ID, pipelineTask.MsgMediaID
	record.MediaTitle, record.CancelRequested = pipelineTask.MsgMediaTitle, pipelineTask.CancelRequested
	if status == ResourceImportStatusRunning {
		record.StartedAt = &now
	}
	if resourceImportStatusFinal(status) {
		record.FinishedAt = &now
	}
	if !reservedRecordCreated {
		if err := s.repos.DB.WithContext(ctx).Create(&record).Error; err != nil {
			if existing, found, lookupErr := s.findJobByIdempotencyKey(ctx, idempotencyKey); lookupErr == nil && found {
				return s.taskDTO(ctx, existing, false)
			}
			_, cancelErr := s.client.CancelImport(context.Background(), userID, pipelineTask.ID)
			if cancelErr != nil && s.log != nil {
				s.log.Error("orphan pipeline import cancel failed", zap.String("pipeline_job_id", pipelineTask.ID), zap.Error(cancelErr))
			}
			return ResourceImportTask{}, err
		}
	} else {
		updates := map[string]any{
			"status": record.Status, "stage": record.Stage, "message": record.Message,
			"pipeline_job_id": record.PipelineJobID, "media_id": record.MediaID, "media_title": record.MediaTitle,
			"cancel_requested": record.CancelRequested, "started_at": record.StartedAt, "finished_at": record.FinishedAt,
		}
		if resourceImportStatusFinal(status) {
			updates["active_reservation_key"] = nil
		}
		if err := s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", record.ID).Updates(updates).Error; err != nil {
			_, cancelErr := s.client.CancelImport(context.Background(), userID, pipelineTask.ID)
			if cancelErr != nil && s.log != nil {
				s.log.Error("orphan pipeline import cancel failed", zap.String("pipeline_job_id", pipelineTask.ID), zap.Error(cancelErr))
			}
			return ResourceImportTask{}, err
		}
	}
	s.schedule(record.ID)
	return s.taskDTO(ctx, record, false)
}

func (s *ResourceImportService) validateUpgradeTarget(ctx context.Context, library model.Library, root model.LibraryRoot, mediaID string) error {
	if mediaID == "" {
		return nil
	}
	if s.repos.Media == nil {
		return errors.New("upgrade_media_id 无效：媒体服务不可用")
	}
	media, err := s.repos.Media.FindByID(ctx, mediaID)
	if err != nil {
		return err
	}
	if media == nil {
		return errors.New("upgrade_media_id 无效：目标作品不存在")
	}
	if media.LibraryID != library.ID {
		return errors.New("upgrade_media_id 无效：目标作品不属于当前媒体库")
	}
	if media.LibraryRootID != "" && media.LibraryRootID != root.ID {
		return errors.New("upgrade_media_id 无效：目标作品不属于当前入库目录")
	}
	groupKey := mediaVersionGroupKey(*media)
	if groupKey == "" {
		return nil
	}
	var candidates []model.Media
	if err := s.repos.DB.WithContext(ctx).
		Where("library_id = ?", media.LibraryID).
		Find(&candidates).Error; err != nil {
		return err
	}
	primary := *media
	for _, candidate := range candidates {
		if mediaVersionGroupKey(candidate) == groupKey && betterMediaVersion(candidate, primary) {
			primary = candidate
		}
	}
	if primary.ID != media.ID {
		return errors.New("upgrade_media_id 无效：目标不是作品主片源，请刷新详情后重新发起升级")
	}
	return nil
}

func (s *ResourceImportService) Recover(ctx context.Context) (int, error) {
	if s == nil || s.repos == nil || s.repos.DB == nil {
		return 0, nil
	}
	var jobs []model.ResourceImportJob
	err := s.repos.DB.WithContext(ctx).
		Where("status NOT IN ?", resourceImportFinalStatuses).
		Order("created_at ASC").Find(&jobs).Error
	if err != nil {
		return 0, err
	}
	for i := range jobs {
		if strings.TrimSpace(jobs[i].PipelineJobID) == "" {
			var recovered resourcePipelineTask
			var recoverErr error
			if jobs[i].SubscriptionFollow {
				recovered, recoverErr = s.recreateReservedPipelineImport(ctx, &jobs[i])
			}
			if recoverErr != nil || strings.TrimSpace(recovered.ID) == "" {
				message := "missing pipeline_job_id during recovery"
				if recoverErr != nil {
					message = recoverErr.Error()
				}
				_ = s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", jobs[i].ID).Updates(map[string]any{
					"status": ResourceImportStatusFailed, "stage": "failed", "public_error": safePipelineMessage(message),
					"error": message, "finished_at": time.Now(), "active_reservation_key": nil,
				}).Error
				continue
			}
			jobs[i].PipelineJobID = recovered.ID
			jobs[i].Status, jobs[i].Stage = mapPipelineImportState(recovered)
			jobs[i].Message = safePipelineMessage(recovered.Message)
			if err := s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", jobs[i].ID).Updates(map[string]any{
				"pipeline_job_id": jobs[i].PipelineJobID, "status": jobs[i].Status, "stage": jobs[i].Stage, "message": jobs[i].Message,
			}).Error; err != nil {
				continue
			}
		}
		s.schedule(jobs[i].ID)
	}
	return len(jobs), nil
}

func (s *ResourceImportService) recreateReservedPipelineImport(ctx context.Context, job *model.ResourceImportJob) (resourcePipelineTask, error) {
	if s == nil || job == nil || !job.SubscriptionFollow {
		return resourcePipelineTask{}, errors.New("reserved pipeline import is not recoverable")
	}
	library, err := s.repos.Library.FindByID(ctx, job.LibraryID)
	if err != nil || library == nil {
		return resourcePipelineTask{}, firstNonNilError(err, errors.New("目标媒体库不存在"))
	}
	root, err := s.repos.Library.FindRootByID(ctx, job.LibraryID, job.LibraryRootID)
	if err != nil || root == nil {
		return resourcePipelineTask{}, firstNonNilError(err, errors.New("目标入库目录不存在"))
	}
	if strings.TrimSpace(job.PipelineSearchSessionID) == "" || strings.TrimSpace(job.PipelineCandidateID) == "" {
		return resourcePipelineTask{}, errors.New("追更恢复缺少 pipeline 搜索候选身份")
	}
	rootPath, err := resourceRootOpenListPath(root.Path)
	if err != nil {
		return resourcePipelineTask{}, err
	}
	category, provider, mediaType := resourceTargetMetadata(library.Type)
	return s.client.CreateImport(ctx, job.UserID, job.IdempotencyKey, resourcePipelineCreateRequest{
		SearchSessionID: job.PipelineSearchSessionID, CandidateID: job.PipelineCandidateID,
		Category: category, LibraryID: job.LibraryID, RootID: job.LibraryRootID,
		RootOpenListPath: rootPath, Provider: provider, MediaType: mediaType, KeepOldVersion: true,
		SubscriptionFollow: true, SubscriptionID: job.SubscriptionID, WorkKey: job.WorkKey,
		ManualReplenish: job.ManualReplenish,
		Season:          job.SeasonNumber, ExistingEpisodes: decodeEpisodeList(job.ExistingEpisodesJSON),
		ReservedEpisodes: decodeEpisodeList(job.ReservedEpisodesJSON), ExpectedEpisodes: decodeEpisodeList(job.ExpectedEpisodesJSON), TargetOpenListPath: job.TargetOpenListPath,
		TitleClass: job.TitleClass,
	})
}

func (s *ResourceImportService) Get(ctx context.Context, requesterID string, isAdmin bool, id string) (ResourceImportTask, error) {
	job, err := s.loadOwnedJob(ctx, requesterID, isAdmin, id)
	if err != nil {
		return ResourceImportTask{}, err
	}
	return s.taskDTO(ctx, job, isAdmin)
}

func (s *ResourceImportService) List(ctx context.Context, requesterID string, isAdmin bool, filter ResourceImportListFilter) (ResourceImportListResult, error) {
	if s == nil || s.repos == nil || s.repos.DB == nil {
		return ResourceImportListResult{}, errors.New("resource import service unavailable")
	}
	page, pageSize := normalizeResourceImportPage(filter.Page, filter.PageSize)
	query := s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{})
	if !isAdmin {
		query = query.Where("user_id = ?", requesterID)
	} else if strings.TrimSpace(filter.UserID) != "" {
		query = query.Where("user_id = ?", strings.TrimSpace(filter.UserID))
	}
	if strings.TrimSpace(filter.LibraryID) != "" {
		query = query.Where("library_id = ?", strings.TrimSpace(filter.LibraryID))
	}
	if strings.TrimSpace(filter.Status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(filter.Status))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return ResourceImportListResult{}, err
	}
	var rows []model.ResourceImportJob
	if err := query.Order("updated_at DESC, created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return ResourceImportListResult{}, err
	}
	items := make([]ResourceImportTask, 0, len(rows))
	for i := range rows {
		item, err := s.taskDTO(ctx, rows[i], isAdmin)
		if err != nil {
			return ResourceImportListResult{}, err
		}
		items = append(items, item)
	}
	return ResourceImportListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ResourceImportService) Cancel(ctx context.Context, requesterID string, isAdmin bool, id string) (ResourceImportTask, error) {
	job, err := s.loadOwnedJob(ctx, requesterID, isAdmin, id)
	if err != nil {
		return ResourceImportTask{}, err
	}
	if resourceImportStatusFinal(job.Status) {
		return ResourceImportTask{}, errors.New("resource import task is already final")
	}
	if err := s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", job.ID).Update("cancel_requested", true).Error; err != nil {
		return ResourceImportTask{}, err
	}
	task, err := s.client.CancelImport(ctx, job.UserID, job.PipelineJobID)
	if err != nil {
		return ResourceImportTask{}, err
	}
	if err := s.applyPipelineTask(ctx, &job, task); err != nil {
		return ResourceImportTask{}, err
	}
	return s.taskDTO(ctx, job, isAdmin)
}

func (s *ResourceImportService) Retry(ctx context.Context, requesterID string, isAdmin bool, id string) (ResourceImportTask, error) {
	job, err := s.loadOwnedJob(ctx, requesterID, isAdmin, id)
	if err != nil {
		return ResourceImportTask{}, err
	}
	if job.Status != ResourceImportStatusFailed && job.Status != ResourceImportStatusCanceled && job.Status != ResourceImportStatusCompletedWithWarning {
		return ResourceImportTask{}, errors.New("resource import task is not retryable")
	}
	var task resourcePipelineTask
	if job.SubscriptionFollow {
		var active int64
		if err := s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).
			Where("subscription_follow = ? AND work_key = ? AND season_number = ? AND library_id = ? AND library_root_id = ? AND status NOT IN ? AND id <> ?",
				true, job.WorkKey, job.SeasonNumber, job.LibraryID, job.LibraryRootID, resourceImportFinalStatuses, job.ID).
			Count(&active).Error; err != nil {
			return ResourceImportTask{}, err
		}
		if active > 0 {
			return ResourceImportTask{}, errors.New("同一作品季已有追更任务正在处理")
		}
		reservationValue := resourceImportReservationKey(job.LibraryID, job.LibraryRootID, job.WorkKey, job.SeasonNumber)
		reservation := s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).
			Where("id = ? AND active_reservation_key IS NULL", job.ID).
			Update("active_reservation_key", reservationValue)
		if reservation.Error != nil {
			return ResourceImportTask{}, reservation.Error
		}
		if reservation.RowsAffected != 1 {
			return ResourceImportTask{}, errors.New("当前追更任务正在重试")
		}
		job.ActiveReservationKey = &reservationValue
	}
	task, err = s.client.RetryImport(ctx, job.UserID, job.PipelineJobID)
	if err != nil {
		if job.SubscriptionFollow {
			_ = s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", job.ID).
				Update("active_reservation_key", nil).Error
		}
		return ResourceImportTask{}, err
	}
	if strings.TrimSpace(task.ID) != strings.TrimSpace(job.PipelineJobID) {
		if job.SubscriptionFollow {
			_ = s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", job.ID).
				Update("active_reservation_key", nil).Error
		}
		return ResourceImportTask{}, errors.New("media-pipeline retry returned a different task id")
	}
	job.Attempt++
	job.CancelRequested = false
	job.StartedAt = nil
	job.FinishedAt = nil
	if job.SubscriptionFollow {
		job.Message, job.PublicError, job.Error = "", "", ""
		job.MediaID, job.MediaTitle, job.ResultJSON, job.Outcome = "", "", "", ""
		if err := s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"message": "", "public_error": "", "error": "", "media_id": "", "media_title": "",
			"result_json": "", "outcome": "", "started_at": nil, "finished_at": nil,
		}).Error; err != nil {
			return ResourceImportTask{}, err
		}
	}
	if err := s.applyPipelineTask(ctx, &job, task); err != nil {
		return ResourceImportTask{}, err
	}
	s.schedule(job.ID)
	return s.taskDTO(ctx, job, isAdmin)
}

func (s *ResourceImportService) DeleteFailed(ctx context.Context, requesterID string, isAdmin bool, id string) error {
	job, err := s.loadOwnedJob(ctx, requesterID, isAdmin, id)
	if err != nil {
		return err
	}
	if job.Status != ResourceImportStatusFailed {
		return ErrResourceImportDeleteNotAllowed
	}
	result := s.repos.DB.WithContext(ctx).Unscoped().
		Where("id = ? AND status = ?", job.ID, ResourceImportStatusFailed).
		Delete(&model.ResourceImportJob{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrResourceImportDeleteNotAllowed
	}
	return nil
}

func (s *ResourceImportService) schedule(id string) {
	if s == nil || strings.TrimSpace(id) == "" {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if _, ok := s.executing[id]; ok {
		s.mu.Unlock()
		return
	}
	s.executing[id] = struct{}{}
	s.mu.Unlock()
	go s.monitor(id)
}

func (s *ResourceImportService) monitor(id string) {
	defer func() {
		s.mu.Lock()
		delete(s.executing, id)
		s.mu.Unlock()
	}()
	ctx := s.ctx
	job, err := s.loadOwnedJob(ctx, "", true, id)
	if err != nil {
		return
	}
	if !s.acquire(ctx, job.UserID) {
		return
	}
	defer s.release(job.UserID)
	ticker := time.NewTicker(time.Duration(s.cfg.PollSeconds) * time.Second)
	defer ticker.Stop()
	invalidStateCount := 0
	for {
		waitForStateRecovery := false
		job, err = s.loadOwnedJob(ctx, "", true, id)
		if err != nil || resourceImportStatusFinal(job.Status) {
			return
		}
		var child resourcePipelineTask
		if job.CancelRequested {
			child, err = s.client.CancelImport(ctx, job.UserID, job.PipelineJobID)
		} else {
			child, err = s.client.GetImport(ctx, job.UserID, job.PipelineJobID)
		}
		if err != nil {
			invalidStateCount = 0
			var pipelineErr *resourcePipelineError
			if job.CancelRequested && errors.As(err, &pipelineErr) && pipelineErr.StatusCode == 409 {
				_ = s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", job.ID).Updates(map[string]any{
					"cancel_requested": false,
					"message":          "任务已进入媒体库同步阶段，无法取消；已转存的115文件会保留",
				}).Error
			}
			if s.log != nil {
				s.log.Warn("resource import poll failed", zap.String("job_id", job.ID), zap.Error(err))
			}
		} else if applyErr := s.applyPipelineTask(ctx, &job, child); applyErr != nil {
			if s.log != nil {
				s.log.Error(
					"resource import state persistence failed",
					zap.String("job_id", job.ID),
					zap.String("pipeline_status", child.Status),
					zap.String("pipeline_stage", child.Stage),
					zap.Error(applyErr),
				)
			}
			var stateErr *resourcePipelineStateError
			if errors.As(applyErr, &stateErr) {
				invalidStateCount++
				if invalidStateCount < 3 {
					waitForStateRecovery = true
				} else {
					_ = s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", job.ID).Updates(map[string]any{
						"status": ResourceImportStatusFailed, "stage": "failed",
						"public_error": applyErr.Error(), "error": applyErr.Error(), "finished_at": time.Now(),
						"active_reservation_key": nil,
					}).Error
					return
				}
			} else {
				invalidStateCount = 0
			}
		} else {
			invalidStateCount = 0
		}
		if !waitForStateRecovery {
			// The subscription failure handler may retry this same durable job
			// before applyPipelineTask returns. Reload the row so that the old
			// in-memory terminal status cannot stop monitoring the queued retry.
			current, loadErr := s.loadOwnedJob(ctx, "", true, id)
			if loadErr != nil || resourceImportStatusFinal(current.Status) {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *ResourceImportService) acquire(ctx context.Context, userID string) bool {
	s.mu.Lock()
	userSem := s.userSems[userID]
	if userSem == nil {
		userSem = make(chan struct{}, s.cfg.MaxConcurrentPerUser)
		s.userSems[userID] = userSem
	}
	s.mu.Unlock()
	select {
	case s.globalSem <- struct{}{}:
	case <-ctx.Done():
		return false
	}
	select {
	case userSem <- struct{}{}:
		return true
	case <-ctx.Done():
		<-s.globalSem
		return false
	}
}

func (s *ResourceImportService) release(userID string) {
	s.mu.Lock()
	userSem := s.userSems[userID]
	s.mu.Unlock()
	if userSem != nil {
		<-userSem
	}
	<-s.globalSem
}

func (s *ResourceImportService) applyPipelineTask(ctx context.Context, job *model.ResourceImportJob, child resourcePipelineTask) error {
	if job == nil {
		return errors.New("resource import job is nil")
	}
	status, stage := mapPipelineImportState(child)
	if status == "" || stage == "" {
		return &resourcePipelineStateError{Status: child.Status, Stage: child.Stage}
	}
	outcome := resourceImportOutcome(child.Result)
	completedWithoutMedia := (status == ResourceImportStatusCompleted || status == ResourceImportStatusCompletedWithWarning) && strings.TrimSpace(child.MsgMediaID) == ""
	if completedWithoutMedia && !(job.ManualReplenish && outcome == "no_new_episodes") {
		status, stage = ResourceImportStatusFailed, "failed"
		child.Error = "media-pipeline completed without msg_media_id"
	}
	now := time.Now()
	updates := map[string]any{
		"status": status, "stage": stage, "message": safePipelineMessage(child.Message),
		"public_error": safePipelineMessage(child.Error), "error": child.Error,
		"media_id": strings.TrimSpace(child.MsgMediaID), "media_title": strings.TrimSpace(child.MsgMediaTitle),
		"cancel_requested": child.CancelRequested, "attempt": job.Attempt,
	}
	if outcome != "" {
		updates["outcome"] = outcome
		job.Outcome = outcome
	}
	if status == ResourceImportStatusRunning && job.StartedAt == nil {
		updates["started_at"] = now
		job.StartedAt = &now
	}
	if resourceImportStatusFinal(status) {
		updates["finished_at"] = now
		updates["active_reservation_key"] = nil
		job.FinishedAt = &now
	}
	if child.Result != nil {
		if encoded, err := json.Marshal(child.Result); err == nil {
			updates["result_json"] = string(encoded)
			job.ResultJSON = string(encoded)
		}
	}
	if err := s.repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).Where("id = ?", job.ID).Updates(updates).Error; err != nil {
		return err
	}
	job.Status, job.Stage = status, stage
	job.Message, job.PublicError, job.Error = safePipelineMessage(child.Message), safePipelineMessage(child.Error), child.Error
	job.MediaID, job.MediaTitle, job.CancelRequested = strings.TrimSpace(child.MsgMediaID), strings.TrimSpace(child.MsgMediaTitle), child.CancelRequested
	if (status == ResourceImportStatusCompleted || status == ResourceImportStatusCompletedWithWarning) && job.SubscriptionFollow && strings.TrimSpace(job.SubscriptionID) != "" && s.subscriptionCompletionHandler != nil {
		if err := s.subscriptionCompletionHandler(ctx, *job); err != nil && s.log != nil {
			s.log.Error("subscription follow completion notification failed", zap.String("job_id", job.ID), zap.String("subscription_id", job.SubscriptionID), zap.Error(err))
		}
	}
	if status == ResourceImportStatusFailed && job.SubscriptionFollow && strings.TrimSpace(job.SubscriptionID) != "" && s.subscriptionFailureHandler != nil {
		if err := s.subscriptionFailureHandler(ctx, *job); err != nil && s.log != nil {
			s.log.Error("subscription follow stop after import failure failed", zap.String("job_id", job.ID), zap.String("subscription_id", job.SubscriptionID), zap.Error(err))
		}
	}
	return nil
}

func (s *ResourceImportService) findCachedSearch(ctx context.Context, userID, libraryID, rootID, query, source string, subscriptionFollow bool) (model.ResourceSearchSession, bool, error) {
	var row model.ResourceSearchSession
	err := s.repos.DB.WithContext(ctx).
		Where("user_id = ? AND library_id = ? AND library_root_id = ? AND query = ? AND source = ? AND subscription_follow = ? AND expires_at > ?", userID, libraryID, rootID, query, source, subscriptionFollow, time.Now()).
		Order("created_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ResourceSearchSession{}, false, nil
	}
	return row, err == nil, err
}

func (s *ResourceImportService) loadOwnedSearch(ctx context.Context, userID, libraryID, rootID, id string) (model.ResourceSearchSession, storedResourceSearch, error) {
	var row model.ResourceSearchSession
	err := s.repos.DB.WithContext(ctx).
		Where("id = ? AND user_id = ? AND library_id = ? AND library_root_id = ?", strings.TrimSpace(id), userID, libraryID, rootID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, storedResourceSearch{}, errors.New("resource search session not found")
	}
	if err != nil {
		return row, storedResourceSearch{}, err
	}
	if !row.ExpiresAt.After(time.Now()) {
		return row, storedResourceSearch{}, errors.New("resource search session expired")
	}
	var stored storedResourceSearch
	if err := json.Unmarshal([]byte(row.ResultsJSON), &stored); err != nil {
		return row, stored, errors.New("resource search session data is invalid")
	}
	if !storedResourceSearchValid(stored) {
		return row, stored, errors.New("resource search session data is invalid; search again")
	}
	return row, stored, nil
}

func (s *ResourceImportService) findJobByIdempotencyKey(ctx context.Context, key string) (model.ResourceImportJob, bool, error) {
	var row model.ResourceImportJob
	err := s.repos.DB.WithContext(ctx).Where("idempotency_key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, false, nil
	}
	return row, err == nil, err
}

func (s *ResourceImportService) loadOwnedJob(ctx context.Context, requesterID string, isAdmin bool, id string) (model.ResourceImportJob, error) {
	var row model.ResourceImportJob
	query := s.repos.DB.WithContext(ctx).Where("id = ?", strings.TrimSpace(id))
	if !isAdmin {
		query = query.Where("user_id = ?", requesterID)
	}
	err := query.First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, errors.New("resource import task not found")
	}
	return row, err
}

func (s *ResourceImportService) taskDTO(ctx context.Context, job model.ResourceImportJob, includeCreator bool) (ResourceImportTask, error) {
	item := ResourceImportTask{
		ID: job.ID, SubscriptionID: job.SubscriptionID, SubscriptionFollow: job.SubscriptionFollow,
		ManualReplenish: job.ManualReplenish,
		WorkKey:         job.WorkKey, SeasonNumber: job.SeasonNumber, TitleClass: job.TitleClass,
		TargetOpenListPath: job.TargetOpenListPath, Outcome: job.Outcome,
		LibraryID: job.LibraryID, RootID: job.LibraryRootID,
		SearchSessionID: job.SearchSessionID, CandidateIndex: job.CandidateIndex,
		CandidateTitle: job.CandidateTitle, Source: job.CandidateSource,
		Status: job.Status, Stage: job.Stage, Progress: resourceImportProgress(job.Status, job.Stage),
		Message: job.Message, Error: job.PublicError,
		MediaID: job.MediaID, MediaTitle: job.MediaTitle, UpgradeMediaID: job.UpgradeMediaID, UpgradeScope: job.UpgradeScope,
		KeepOldVersion:  job.KeepOldVersion,
		CancelRequested: job.CancelRequested,
		Attempt:         job.Attempt, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
		StartedAt: job.StartedAt, FinishedAt: job.FinishedAt,
	}
	if job.SubscriptionFollow {
		item.ExistingEpisodes = decodeEpisodeList(job.ExistingEpisodesJSON)
		upperBound := 0
		if len(item.ExistingEpisodes) > 0 {
			upperBound = item.ExistingEpisodes[len(item.ExistingEpisodes)-1]
		}
		item.MissingEpisodes = missingEpisodesThrough(item.ExistingEpisodes, upperBound)
		item.SelectedEpisodes, item.MovedEpisodes, item.VerifiedEpisodes, item.ScanAdded = resourceImportSubscriptionProjection(job.ResultJSON)
	}
	if s.repos != nil && s.repos.Library != nil {
		if library, err := s.repos.Library.FindByID(ctx, job.LibraryID); err != nil {
			return item, err
		} else if library != nil {
			item.LibraryName = library.Name
			for _, root := range library.Roots {
				if root.ID == job.LibraryRootID {
					item.RootName = root.Name
					break
				}
			}
		}
	}
	if includeCreator {
		item.UserID = job.UserID
		item.PipelineJobID = job.PipelineJobID
		if s.repos != nil && s.repos.User != nil {
			user, err := s.repos.User.FindByID(ctx, job.UserID)
			if err != nil {
				return item, err
			}
			if user != nil {
				item.CreatorUsername = user.Username
			}
		}
	}
	return item, nil
}

func resourceSearchPage(record model.ResourceSearchSession, stored storedResourceSearch, root model.LibraryRoot, in ResourceSearchInput) ResourceSearchResponse {
	page, pageSize := in.Page, in.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > resourceSearchLimit {
		pageSize = resourceSearchLimit
	}
	candidates := filterResourceSearchCandidates(stored.Candidates, in)
	sortResourceSearchCandidates(candidates, in.SortBy)
	total := len(candidates)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}
	results := make([]ResourceSearchCandidate, 0, end-start)
	for _, candidate := range candidates[start:end] {
		results = append(results, candidate.ResourceSearchCandidate)
	}
	return ResourceSearchResponse{
		SessionID: record.ID, Query: record.Query, Page: page, PageSize: pageSize,
		Total: total, UnfilteredTotal: len(stored.Candidates), TotalPages: totalPages,
		Roots:        []ResourceSearchRoot{{ID: root.ID, Name: root.Name, Path: root.Path, Enabled: root.Enabled}},
		Capabilities: stored.Capabilities, Facets: resourceSearchFacets(stored.Candidates), Results: results,
	}
}

func validateResourceSearchView(in ResourceSearchInput) error {
	if len([]rune(strings.TrimSpace(in.ResultQuery))) > 200 {
		return errors.New("result_query is too long")
	}
	resolution := strings.ToLower(strings.TrimSpace(in.ResolutionFilter))
	if resolution != "" && resolution != "all" && resolution != "2160p" && resolution != "1080p" && resolution != "720p" && resolution != "other" {
		return errors.New("unsupported resolution_filter")
	}
	subtitle := strings.ToLower(strings.TrimSpace(in.SubtitleFilter))
	if subtitle != "" && subtitle != "all" && subtitle != "chinese" && subtitle != "with_subtitle" {
		return errors.New("unsupported subtitle_filter")
	}
	sortBy := strings.ToLower(strings.TrimSpace(in.SortBy))
	if sortBy != "" && sortBy != "relevance" && sortBy != "size_desc" && sortBy != "size_asc" && sortBy != "seeders_desc" && sortBy != "resolution_desc" {
		return errors.New("unsupported sort_by")
	}
	return nil
}

func filterResourceSearchCandidates(candidates []storedResourceCandidate, in ResourceSearchInput) []storedResourceCandidate {
	query := strings.ToLower(strings.TrimSpace(in.ResultQuery))
	source := strings.ToLower(strings.TrimSpace(in.SourceFilter))
	resolution := strings.ToLower(strings.TrimSpace(in.ResolutionFilter))
	subtitle := strings.ToLower(strings.TrimSpace(in.SubtitleFilter))
	out := make([]storedResourceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if query != "" && !strings.Contains(strings.ToLower(resourceCandidateSearchText(candidate.ResourceSearchCandidate)), query) {
			continue
		}
		if source != "" && source != "all" && !strings.EqualFold(strings.TrimSpace(candidate.Source), source) {
			continue
		}
		if resolution != "" && resolution != "all" && resourceCandidateResolution(candidate.ResourceSearchCandidate) != resolution {
			continue
		}
		if subtitle == "chinese" && !resourceCandidateHasChineseSubtitle(candidate.ResourceSearchCandidate) {
			continue
		}
		if subtitle == "with_subtitle" && !resourceCandidateHasSubtitle(candidate.ResourceSearchCandidate) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func sortResourceSearchCandidates(candidates []storedResourceCandidate, sortBy string) {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "size_desc":
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].SizeBytes > candidates[j].SizeBytes })
	case "size_asc":
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].SizeBytes < candidates[j].SizeBytes })
	case "seeders_desc":
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Seeders > candidates[j].Seeders })
	case "resolution_desc":
		sort.SliceStable(candidates, func(i, j int) bool {
			return resourceResolutionScore(resourceCandidateResolution(candidates[i].ResourceSearchCandidate)) >
				resourceResolutionScore(resourceCandidateResolution(candidates[j].ResourceSearchCandidate))
		})
	}
}

func resourceSearchFacets(candidates []storedResourceCandidate) ResourceSearchFacets {
	sources := make(map[string]string)
	resolutions := make(map[string]bool)
	for _, candidate := range candidates {
		if source := strings.TrimSpace(candidate.Source); source != "" {
			key := strings.ToLower(source)
			if _, exists := sources[key]; !exists {
				sources[key] = source
			}
		}
		resolutions[resourceCandidateResolution(candidate.ResourceSearchCandidate)] = true
	}
	sourceValues := make([]string, 0, len(sources))
	for _, source := range sources {
		sourceValues = append(sourceValues, source)
	}
	sort.Slice(sourceValues, func(i, j int) bool { return strings.ToLower(sourceValues[i]) < strings.ToLower(sourceValues[j]) })
	resolutionValues := make([]string, 0, 4)
	for _, value := range []string{"2160p", "1080p", "720p", "other"} {
		if resolutions[value] {
			resolutionValues = append(resolutionValues, value)
		}
	}
	return ResourceSearchFacets{Sources: sourceValues, Resolutions: resolutionValues}
}

func resourceCandidateSearchText(candidate ResourceSearchCandidate) string {
	return strings.Join([]string{
		candidate.Title, candidate.Source, candidate.Resolution, candidate.Subtitle,
		candidate.ResourceType, candidate.Summary, candidate.SizeText,
	}, " ")
}

func resourceCandidateCompatibilityWarning(candidate ResourceSearchCandidate) string {
	text := strings.ToLower(resourceCandidateSearchText(candidate))
	hasVision := strings.Contains(text, "dolby vision") || strings.Contains(text, "杜比视界") ||
		resourceCandidateHasToken(text, "dovi") || resourceCandidateHasToken(text, "dv")
	hasAtmos := strings.Contains(text, "atmos") || strings.Contains(text, "杜比全景声")
	hasDolbyAudio := hasAtmos || strings.Contains(text, "truehd") || strings.Contains(text, "true hd") ||
		strings.Contains(text, "eac3") || strings.Contains(text, "e-ac-3") ||
		strings.Contains(text, "ddp") || strings.Contains(text, "dd+") ||
		resourceCandidateHasToken(text, "ac3") || strings.Contains(text, "ac-3") ||
		strings.Contains(text, "dolby digital") || strings.Contains(text, "杜比音频")

	parts := make([]string, 0, 2)
	if hasVision {
		parts = append(parts, "杜比视界")
	}
	if hasAtmos {
		parts = append(parts, "杜比全景声")
	} else if hasDolbyAudio {
		parts = append(parts, "杜比音频")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " + ") + "，部分设备可能无法兼容"
}

func resourceCandidateHasToken(text, target string) bool {
	for _, token := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if token == target {
			return true
		}
	}
	return false
}

func resourceCandidateResolution(candidate ResourceSearchCandidate) string {
	text := strings.ToLower(resourceCandidateSearchText(candidate))
	switch {
	case strings.Contains(text, "2160") || strings.Contains(text, "4k") || strings.Contains(text, "uhd"):
		return "2160p"
	case strings.Contains(text, "1080") || strings.Contains(text, "fullhd") || strings.Contains(text, "full hd"):
		return "1080p"
	case strings.Contains(text, "720"):
		return "720p"
	default:
		return "other"
	}
}

func resourceResolutionScore(value string) int {
	switch value {
	case "2160p":
		return 4
	case "1080p":
		return 3
	case "720p":
		return 2
	default:
		return 1
	}
}

func resourceCandidateHasSubtitle(candidate ResourceSearchCandidate) bool {
	if strings.TrimSpace(candidate.Subtitle) != "" {
		return true
	}
	text := strings.ToLower(resourceCandidateSearchText(candidate))
	return strings.Contains(text, "subtitle") || strings.Contains(text, "subbed") || strings.Contains(text, "字幕") || strings.Contains(text, "中字")
}

func resourceCandidateHasChineseSubtitle(candidate ResourceSearchCandidate) bool {
	text := strings.ToLower(resourceCandidateSearchText(candidate))
	for _, marker := range []string{"中文字幕", "中字", "简中", "繁中", "中文", "chinese", "chs", "cht", "zh-cn", "zh-tw"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func normalizeResourceCandidates(items []map[string]any) ([]storedResourceCandidate, error) {
	if len(items) > resourceSearchLimit {
		items = items[:resourceSearchLimit]
	}
	out := make([]storedResourceCandidate, 0, len(items))
	for index, raw := range items {
		title := resourceString(raw, "title", "name")
		candidateID := resourceString(raw, "candidate_id")
		if title == "" || candidateID == "" {
			return nil, errors.New("media-pipeline search returned an invalid candidate")
		}
		candidate := ResourceSearchCandidate{
			Index:        index,
			Title:        title,
			SizeBytes:    resourceInt64(raw, "size_bytes", "size"),
			SizeText:     resourceString(raw, "size_text", "size_label"),
			Source:       resourceString(raw, "source", "indexer", "source_name"),
			Seeders:      int(resourceInt64(raw, "seeders", "seed")),
			Resolution:   resourceString(raw, "resolution", "quality"),
			Subtitle:     resourceString(raw, "subtitle", "subtitles"),
			ResourceType: resourceString(raw, "resource_type", "type"),
			Summary:      truncateResourceText(resourceString(raw, "summary", "description"), 600),
		}
		candidate.CompatibilityWarning = resourceCandidateCompatibilityWarning(candidate)
		out = append(out, storedResourceCandidate{
			ResourceSearchCandidate: candidate,
			CandidateID:             candidateID,
		})
	}
	return out, nil
}

func storedResourceSearchValid(stored storedResourceSearch) bool {
	if strings.TrimSpace(stored.PipelineSessionID) == "" {
		return false
	}
	for _, candidate := range stored.Candidates {
		if strings.TrimSpace(candidate.CandidateID) == "" {
			return false
		}
	}
	return true
}

func resourceTargetMetadata(libraryType string) (category, provider, mediaType string) {
	switch strings.ToLower(strings.TrimSpace(libraryType)) {
	case "adult":
		return "adult", "adult", "adult"
	case "anime":
		return "anime", "tmdb", "anime"
	case "tv", "series", "show":
		return "tv", "tmdb", "tv"
	case "movie":
		return "movie", "tmdb", "movie"
	default:
		return "other", "tmdb", "movie"
	}
}

func resourceRootOpenListPath(value string) (string, error) {
	raw := strings.TrimSpace(value)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "cloud" || parsed.Host != "openlist" {
		return "", errors.New("当前仅支持 OpenList 云盘媒体库入库")
	}
	cloudPath := "cloud://openlist" + parsed.EscapedPath()
	openListPath := pipelineCloudPathToOpenListPath(cloudPath)
	if openListPath == "" {
		return "", errors.New("OpenList 媒体库目录无效")
	}
	return openListPath, nil
}

func resourceImportIdempotencyKey(userID, libraryID, rootID, sessionID string, candidateIndex int, subscriptionID string, force bool, upgradeMediaID, upgradeScope string, keepOldVersion bool) string {
	raw := strings.Join([]string{
		userID, libraryID, rootID, sessionID, strconv.Itoa(candidateIndex), strings.TrimSpace(subscriptionID), strconv.FormatBool(force),
		strings.TrimSpace(upgradeMediaID), strings.TrimSpace(upgradeScope), strconv.FormatBool(keepOldVersion),
	}, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return "msg-resource-import:" + hex.EncodeToString(sum[:])
}

func resourceImportReservationKey(libraryID, rootID, workKey string, season int) string {
	raw := strings.Join([]string{
		strings.TrimSpace(libraryID), strings.TrimSpace(rootID), strings.TrimSpace(workKey), strconv.Itoa(season),
	}, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return "subscription-follow:" + hex.EncodeToString(sum[:])
}

func uniqueSortedPositiveInts(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func missingEpisodesThrough(existing []int, upperBound int) []int {
	upperBound = max(0, upperBound)
	if upperBound == 0 {
		return nil
	}
	present := make(map[int]struct{}, len(existing))
	for _, episode := range existing {
		if episode > 0 {
			present[episode] = struct{}{}
		}
	}
	missing := make([]int, 0)
	for episode := 1; episode <= upperBound; episode++ {
		if _, ok := present[episode]; !ok {
			missing = append(missing, episode)
		}
	}
	return missing
}

func resourceImportOutcome(result map[string]any) string {
	follow, ok := result["subscription_follow"].(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(follow["outcome"]))
}

func resourceImportSubscriptionProjection(raw string) (selected, moved, verified []int, scanAdded int) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil, nil, 0
	}
	var result struct {
		SubscriptionFollow struct {
			SelectedEpisodes []int `json:"selected_episodes"`
			MovedEpisodes    []int `json:"moved_episodes"`
			VerifiedEpisodes []int `json:"verified_episodes"`
			ScanAdded        int   `json:"scan_added"`
			MSGVerification  struct {
				VerifiedEpisodes []int `json:"verified_episodes"`
			} `json:"msg_verification"`
		} `json:"subscription_follow"`
	}
	if json.Unmarshal([]byte(raw), &result) != nil {
		return nil, nil, nil, 0
	}
	follow := result.SubscriptionFollow
	verified = follow.MSGVerification.VerifiedEpisodes
	if len(verified) == 0 {
		verified = follow.VerifiedEpisodes
	}
	return uniqueSortedPositiveInts(follow.SelectedEpisodes), uniqueSortedPositiveInts(follow.MovedEpisodes), uniqueSortedPositiveInts(verified), max(0, follow.ScanAdded)
}

func decodeEpisodeList(raw string) []int {
	var values []int
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &values) != nil {
		return nil
	}
	return uniqueSortedPositiveInts(values)
}

func normalizeResourceImportUpgradeScope(category, upgradeMediaID, requested string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if strings.TrimSpace(upgradeMediaID) == "" {
		if requested != "" {
			return "", errors.New("upgrade_scope 只能与 upgrade_media_id 一起使用")
		}
		return "", nil
	}
	if category == "tv" || category == "anime" {
		if requested != "" && requested != "work" {
			return "", errors.New("剧集只支持整剧升级")
		}
		return "work", nil
	}
	if requested != "" && requested != "media" {
		return "", errors.New("当前媒体类型只支持单作品升级")
	}
	return "media", nil
}

func mapPipelineImportState(task resourcePipelineTask) (string, string) {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	stage := strings.ToLower(strings.TrimSpace(task.Stage))
	switch status {
	case ResourceImportStatusQueued:
		return ResourceImportStatusQueued, mapPipelineActiveStage(stage)
	case ResourceImportStatusRunning:
		return ResourceImportStatusRunning, mapPipelineActiveStage(stage)
	case ResourceImportStatusCompleted:
		return ResourceImportStatusCompleted, "completed"
	case ResourceImportStatusCompletedWithWarning:
		return ResourceImportStatusCompletedWithWarning, "completed"
	case "cancelled", ResourceImportStatusCanceled:
		return ResourceImportStatusCanceled, "canceled"
	case ResourceImportStatusFailed:
		return ResourceImportStatusFailed, "failed"
	case "needs_attention":
		// pipeline 的 needs_attention 是"需要用户关注"的终态（115 授权/链接等问题），
		// 复用 failed 以保留删除/重试/前端展示能力，具体原因由 error 字段透传。
		return ResourceImportStatusFailed, "failed"
	default:
		return "", ""
	}
}

func mapPipelineActiveStage(stage string) string {
	if mapped := mapPipelineImportStage(stage); mapped != "" {
		return mapped
	}
	if strings.TrimSpace(stage) != "" {
		return "running"
	}
	return ""
}

func mapPipelineImportStage(stage string) string {
	switch stage {
	case "queued":
		return "duplicate_check"
	case "starting", "submitted":
		return "submitting"
	case "staging", "waiting_download":
		return "transferring"
	case "verifying_staging", "promoting", "syncing":
		return "preparing_openlist"
	case "scanning", "verifying_scan":
		return "scanning"
	case "scraping":
		return "scraping"
	case "subtitles":
		return "matching_subtitle"
	case "removing_old_version":
		return "finalizing_upgrade"
	case "cleanup":
		return "cleanup"
	case "completed", "completed_with_warning":
		return "completed"
	case "failed", "canceled", "cancelled":
		return stage
	default:
		return ""
	}
}

func resourceImportProgress(status, stage string) int {
	if status == ResourceImportStatusCompleted || status == ResourceImportStatusCompletedWithWarning {
		return 100
	}
	switch stage {
	case "duplicate_check":
		return 5
	case "submitting":
		return 15
	case "transferring":
		return 35
	case "preparing_openlist":
		return 55
	case "scanning":
		return 70
	case "scraping":
		return 85
	case "matching_subtitle":
		return 95
	case "finalizing_upgrade":
		return 98
	case "running":
		return 25
	default:
		return 0
	}
}

func resourceImportStatusFinal(status string) bool {
	for _, value := range resourceImportFinalStatuses {
		if status == value {
			return true
		}
	}
	return false
}

func normalizeResourceImportPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func resourceString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok && value != nil {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func resourceInt64(values map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int64(typed)
		case int64:
			return typed
		case int:
			return int64(typed)
		case json.Number:
			parsed, _ := typed.Int64()
			return parsed
		default:
			parsed, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
			return parsed
		}
	}
	return 0
}

func truncateResourceText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func safePipelineMessage(value string) string {
	return truncateResourceText(value, 1000)
}
