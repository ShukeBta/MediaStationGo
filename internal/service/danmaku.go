package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// 弹幕状态：unmatched 也要落库，否则每次播放都会重新回源第三方。
const (
	DanmakuStatusMatched   = "matched"
	DanmakuStatusUnmatched = "unmatched"
	DanmakuStatusFailed    = "failed"
)

// DanmakuMatchModeManual 表示用户手动指定的关联，自动匹配不得覆盖它。
const DanmakuMatchModeManual = "manual"

// ErrDanmakuUnavailable 表示弹幕模块不可用（未配置 / 未启用 / 管线不可达），
// 必须与「这一集没有弹幕」区分开，避免把配置问题伪装成内容问题。
var ErrDanmakuUnavailable = errors.New("danmaku is unavailable")

// ErrDanmakuUnmatched 表示自动匹配失败，调用方应展示 attempts 并引导手动匹配。
var ErrDanmakuUnmatched = errors.New("danmaku has no match for this media")

// ErrDanmakuInvalidInput 表示调用方参数不合法（映射为 400，而不是 500）。
var ErrDanmakuInvalidInput = errors.New("invalid danmaku input")

// ErrDanmakuPrewarmNotFound 表示管线没有这个预热任务（映射为 404）。
var ErrDanmakuPrewarmNotFound = errors.New("danmaku prewarm task not found")

// danmakuMaxOffsetSeconds 与客户端、管线的上限保持一致：超过这个范围的偏移
// 一定是参数错误，而不是"让管线去拒绝"。
const danmakuMaxOffsetSeconds = 600.0

func validateDanmakuOffset(offsetSeconds float64) error {
	if math.IsNaN(offsetSeconds) || math.IsInf(offsetSeconds, 0) ||
		offsetSeconds < -danmakuMaxOffsetSeconds || offsetSeconds > danmakuMaxOffsetSeconds {
		return fmt.Errorf("%w: offset_seconds must be within -600..600", ErrDanmakuInvalidInput)
	}
	return nil
}

// DanmakuComment 是对客户端下发的单条弹幕（弹弹play 字段 + 结构化补充）。
type DanmakuComment struct {
	CID      string  `json:"cid"`
	P        string  `json:"p"`
	M        string  `json:"m"`
	Time     float64 `json:"time"`
	Mode     int     `json:"mode"`
	ModeName string  `json:"mode_name,omitempty"`
	Color    int     `json:"color"`
	Size     int     `json:"size,omitempty"`
	User     string  `json:"user,omitempty"`
}

// DanmakuPayload 是 GET /api/media/:id/danmaku 的响应体。
type DanmakuPayload struct {
	MediaID              string           `json:"media_id"`
	Source               string           `json:"source"`
	EpisodeID            string           `json:"episode_id"`
	AnimeTitle           string           `json:"anime_title,omitempty"`
	EpisodeTitle         string           `json:"episode_title,omitempty"`
	MatchMode            string           `json:"match_mode,omitempty"`
	ProviderShiftSeconds float64          `json:"provider_shift_seconds"`
	OffsetSeconds        float64          `json:"offset_seconds"`
	ChConvert            int              `json:"ch_convert"`
	Count                int              `json:"count"`
	Total                int              `json:"total"`
	Filtered             int              `json:"filtered"`
	Skipped              int              `json:"skipped"`
	Truncated            bool             `json:"truncated"`
	Comments             []DanmakuComment `json:"comments"`
}

// DanmakuMatchCandidate 是匹配候选（供手动选择）。
type DanmakuMatchCandidate struct {
	EpisodeID       string  `json:"episode_id"`
	AnimeID         string  `json:"anime_id,omitempty"`
	AnimeTitle      string  `json:"anime_title,omitempty"`
	EpisodeTitle    string  `json:"episode_title,omitempty"`
	EpisodeNumber   string  `json:"episode_number,omitempty"`
	Type            string  `json:"type,omitempty"`
	TypeDescription string  `json:"type_description,omitempty"`
	ImageURL        string  `json:"image_url,omitempty"`
	Shift           float64 `json:"shift"`
}

// DanmakuAttempt 记录一次匹配尝试，未匹配时用于解释原因。
type DanmakuAttempt struct {
	Source         string `json:"source"`
	Mode           string `json:"mode"`
	Outcome        string `json:"outcome"`
	Error          string `json:"error,omitempty"`
	CandidateCount int    `json:"candidate_count,omitempty"`
}

// DanmakuMatchResult 是匹配结果（来自管线，或本地持久化的关联）。
type DanmakuMatchResult struct {
	Matched      bool                    `json:"matched"`
	Provider     string                  `json:"source,omitempty"`
	MatchMode    string                  `json:"match_mode,omitempty"`
	EpisodeID    string                  `json:"episode_id,omitempty"`
	AnimeTitle   string                  `json:"anime_title,omitempty"`
	EpisodeTitle string                  `json:"episode_title,omitempty"`
	Shift        float64                 `json:"shift"`
	Status       string                  `json:"status"`
	Ambiguous    bool                    `json:"ambiguous,omitempty"`
	Candidates   []DanmakuMatchCandidate `json:"candidates"`
	Attempts     []DanmakuAttempt        `json:"attempts"`
}

// DanmakuSearchResult 是手动匹配用的搜索结果。
type DanmakuSearchResult struct {
	Keyword string                `json:"keyword"`
	Results []DanmakuSearchSource `json:"results"`
	Errors  []DanmakuSearchError  `json:"errors"`
}

type DanmakuSearchSource struct {
	Source string               `json:"source"`
	Animes []DanmakuSearchAnime `json:"animes"`
}

type DanmakuSearchAnime struct {
	AnimeID    string                 `json:"anime_id"`
	AnimeTitle string                 `json:"anime_title"`
	Type       string                 `json:"type"`
	TypeDesc   string                 `json:"type_description"`
	ImageURL   string                 `json:"image_url"`
	Episodes   []DanmakuSearchEpisode `json:"episodes"`
}

type DanmakuSearchEpisode struct {
	EpisodeID     string `json:"episode_id"`
	EpisodeTitle  string `json:"episode_title"`
	EpisodeNumber string `json:"episode_number"`
}

type DanmakuSearchError struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

// DanmakuOptions 控制单次取弹幕的行为。
type DanmakuOptions struct {
	ChConvert     int
	OffsetSeconds float64
	WithRelated   bool
	ForceRefresh  bool
}

// DanmakuFetchRequest 是发给管线的取弹幕请求。
type DanmakuFetchRequest struct {
	MediaID              string  `json:"media_id"`
	EpisodeID            string  `json:"episode_id,omitempty"`
	Source               string  `json:"source,omitempty"`
	ChConvert            int     `json:"ch_convert"`
	OffsetSeconds        float64 `json:"offset_seconds"`
	ProviderShiftSeconds float64 `json:"provider_shift_seconds"`
	AnimeTitle           string  `json:"anime_title,omitempty"`
	EpisodeTitle         string  `json:"episode_title,omitempty"`
	MatchMode            string  `json:"match_mode,omitempty"`
	WithRelated          bool    `json:"with_related"`
}

// DanmakuPrewarmEpisode 是整季预热里的一个分集。
type DanmakuPrewarmEpisode struct {
	MediaID    string `json:"media_id"`
	EpisodeKey string `json:"episode_key,omitempty"`
}

// DanmakuPrewarmDetail 是一集的预热结果（失败必须能被看见）。
type DanmakuPrewarmDetail struct {
	MediaID    string `json:"media_id"`
	EpisodeKey string `json:"episode_key,omitempty"`
	Status     string `json:"status"`
	Count      int    `json:"count"`
	Cached     bool   `json:"cached"`
	Error      string `json:"error,omitempty"`
}

// DanmakuPrewarmTask 是整季预热任务的进度快照。
type DanmakuPrewarmTask struct {
	TaskID         string                 `json:"task_id"`
	Status         string                 `json:"status"`
	MediaID        string                 `json:"media_id"`
	Season         int                    `json:"season"`
	Total          int                    `json:"total"`
	Processed      int                    `json:"processed"`
	Matched        int                    `json:"matched"`
	Empty          int                    `json:"empty"`
	Cached         int                    `json:"cached"`
	Failed         int                    `json:"failed"`
	CurrentEpisode string                 `json:"current_episode,omitempty"`
	Error          string                 `json:"error,omitempty"`
	Details        []DanmakuPrewarmDetail `json:"details"`
	CreatedAt      float64                `json:"created_at,omitempty"`
	UpdatedAt      float64                `json:"updated_at,omitempty"`
}

type danmakuPipelineClient interface {
	MatchDanmaku(context.Context, string) (DanmakuMatchResult, error)
	FetchDanmaku(context.Context, DanmakuFetchRequest) (DanmakuPayload, error)
	SearchDanmaku(context.Context, string, int) (DanmakuSearchResult, error)
	StartDanmakuPrewarm(context.Context, DanmakuPrewarmRequest) (DanmakuPrewarmTask, error)
	GetDanmakuPrewarm(context.Context, string) (DanmakuPrewarmTask, error)
}

// DanmakuPrewarmRequest 是发给管线的整季预热请求。
type DanmakuPrewarmRequest struct {
	OwnerID  string                  `json:"owner_id"`
	MediaID  string                  `json:"media_id"`
	Season   int                     `json:"season"`
	Episodes []DanmakuPrewarmEpisode `json:"episodes"`
}

// DanmakuService 负责媒体与弹幕库的关联、持久化与下发。
type DanmakuService struct {
	log      *zap.Logger
	repos    *repository.Container
	pipeline danmakuPipelineClient
}

func NewDanmakuService(log *zap.Logger, repos *repository.Container) *DanmakuService {
	if log == nil {
		log = zap.NewNop()
	}
	return &DanmakuService{log: log, repos: repos}
}

func (s *DanmakuService) SetPipelineClient(client danmakuPipelineClient) {
	s.pipeline = client
}

func (s *DanmakuService) Available() bool {
	return s != nil && s.pipeline != nil
}

func (s *DanmakuService) Association(ctx context.Context, mediaID string) (*model.MediaDanmaku, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return nil, fmt.Errorf("%w: media id is required", ErrDanmakuInvalidInput)
	}
	var row model.MediaDanmaku
	err := s.repos.DB.WithContext(ctx).Where("media_id = ?", mediaID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// Match 执行一次自动匹配并落库。手动指定的关联不会被自动匹配覆盖。
func (s *DanmakuService) Match(ctx context.Context, mediaID string) (DanmakuMatchResult, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return DanmakuMatchResult{}, fmt.Errorf("%w: media id is required", ErrDanmakuInvalidInput)
	}
	if !s.Available() {
		return DanmakuMatchResult{}, ErrDanmakuUnavailable
	}
	existing, err := s.Association(ctx, mediaID)
	if err != nil {
		return DanmakuMatchResult{}, err
	}
	if existing != nil && existing.MatchMode == DanmakuMatchModeManual && existing.Status == DanmakuStatusMatched {
		return matchResultFromRow(existing), nil
	}
	result, err := s.pipeline.MatchDanmaku(ctx, mediaID)
	if err != nil {
		// 回源失败也要落库，避免把「上游挂了」当成「这集没弹幕」从而反复重试。
		if storeErr := s.storeResult(ctx, mediaID, DanmakuMatchResult{Status: DanmakuStatusFailed}, ""); storeErr != nil {
			s.log.Warn("persist failed danmaku match", zap.String("media_id", mediaID), zap.Error(storeErr))
		}
		if errors.Is(err, ErrDanmakuUnavailable) {
			return DanmakuMatchResult{}, err
		}
		return DanmakuMatchResult{}, fmt.Errorf("%w: %v", ErrDanmakuUnavailable, err)
	}
	if err := s.storeResult(ctx, mediaID, result, encodeDanmakuAttempts(result.Attempts)); err != nil {
		return DanmakuMatchResult{}, err
	}
	return result, nil
}

func (s *DanmakuService) storeResult(ctx context.Context, mediaID string, result DanmakuMatchResult, attempts string) error {
	status := result.Status
	if status == "" {
		if result.Matched {
			status = DanmakuStatusMatched
		} else {
			status = DanmakuStatusUnmatched
		}
	}
	var row model.MediaDanmaku
	err := s.repos.DB.WithContext(ctx).Where("media_id = ?", mediaID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = model.MediaDanmaku{
			MediaID:              mediaID,
			Provider:             result.Provider,
			EpisodeID:            result.EpisodeID,
			AnimeTitle:           result.AnimeTitle,
			EpisodeTitle:         result.EpisodeTitle,
			MatchMode:            result.MatchMode,
			ProviderShiftSeconds: result.Shift,
			Status:               status,
			Attempts:             attempts,
		}
		return s.repos.DB.WithContext(ctx).Create(&row).Error
	}
	if err != nil {
		return err
	}
	row.Provider = result.Provider
	row.EpisodeID = result.EpisodeID
	row.AnimeTitle = result.AnimeTitle
	row.EpisodeTitle = result.EpisodeTitle
	row.MatchMode = result.MatchMode
	row.ProviderShiftSeconds = result.Shift
	row.Status = status
	row.Attempts = attempts
	return s.repos.DB.WithContext(ctx).Save(&row).Error
}

// SetManual 手动指定关联；同时清掉自动匹配的 shift，避免叠加两次时间轴修正。
func (s *DanmakuService) SetManual(ctx context.Context, mediaID, provider, episodeID, animeTitle, episodeTitle string, offsetSeconds float64) (*model.MediaDanmaku, error) {
	mediaID = strings.TrimSpace(mediaID)
	episodeID = strings.TrimSpace(episodeID)
	if mediaID == "" {
		return nil, fmt.Errorf("%w: media id is required", ErrDanmakuInvalidInput)
	}
	if episodeID == "" {
		return nil, fmt.Errorf("%w: episode id is required", ErrDanmakuInvalidInput)
	}
	if _, err := strconv.ParseInt(episodeID, 10, 64); err != nil {
		return nil, fmt.Errorf("%w: episode id must be numeric", ErrDanmakuInvalidInput)
	}
	if strings.TrimSpace(provider) == "" {
		provider = "dandanplay"
	}
	if err := validateDanmakuOffset(offsetSeconds); err != nil {
		return nil, err
	}
	var row model.MediaDanmaku
	err := s.repos.DB.WithContext(ctx).Where("media_id = ?", mediaID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = model.MediaDanmaku{
			MediaID:       mediaID,
			Provider:      provider,
			EpisodeID:     episodeID,
			AnimeTitle:    animeTitle,
			EpisodeTitle:  episodeTitle,
			MatchMode:     DanmakuMatchModeManual,
			OffsetSeconds: offsetSeconds,
			Status:        DanmakuStatusMatched,
		}
		if err := s.repos.DB.WithContext(ctx).Create(&row).Error; err != nil {
			return nil, err
		}
		return &row, nil
	}
	if err != nil {
		return nil, err
	}
	row.Provider = provider
	row.EpisodeID = episodeID
	row.AnimeTitle = animeTitle
	row.EpisodeTitle = episodeTitle
	row.MatchMode = DanmakuMatchModeManual
	row.ProviderShiftSeconds = 0
	row.OffsetSeconds = offsetSeconds
	row.Status = DanmakuStatusMatched
	row.Attempts = ""
	if err := s.repos.DB.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return s.Association(ctx, mediaID)
}

func (s *DanmakuService) SetOffset(ctx context.Context, mediaID string, offsetSeconds float64) (*model.MediaDanmaku, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return nil, fmt.Errorf("%w: media id is required", ErrDanmakuInvalidInput)
	}
	// 先校验参数再查关联：越界偏移是调用方的错误，与当前有没有匹配无关。
	if err := validateDanmakuOffset(offsetSeconds); err != nil {
		return nil, err
	}
	row, err := s.Association(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrDanmakuUnmatched
	}
	if err := s.repos.DB.WithContext(ctx).Model(row).Update("offset_seconds", offsetSeconds).Error; err != nil {
		return nil, err
	}
	return s.Association(ctx, mediaID)
}

func (s *DanmakuService) Clear(ctx context.Context, mediaID string) error {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return fmt.Errorf("%w: media id is required", ErrDanmakuInvalidInput)
	}
	return s.repos.DB.WithContext(ctx).Where("media_id = ?", mediaID).Delete(&model.MediaDanmaku{}).Error
}

func (s *DanmakuService) Search(ctx context.Context, keyword string, episode int) (DanmakuSearchResult, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return DanmakuSearchResult{}, fmt.Errorf("%w: keyword is required", ErrDanmakuInvalidInput)
	}
	if !s.Available() {
		return DanmakuSearchResult{}, ErrDanmakuUnavailable
	}
	result, err := s.pipeline.SearchDanmaku(ctx, keyword, episode)
	if err != nil {
		return DanmakuSearchResult{}, fmt.Errorf("%w: %v", ErrDanmakuUnavailable, err)
	}
	return result, nil
}

// PrewarmSeason 触发一次整季预热。
//
// 只在管理员显式调用时发生：预热会为每一集回源第三方，逐集串行且带延迟，
// 绝不自动排期整库预热（弹弹play 的使用约定禁止规模化抓取）。
func (s *DanmakuService) PrewarmSeason(ctx context.Context, mediaID string, season int) (DanmakuPrewarmTask, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return DanmakuPrewarmTask{}, fmt.Errorf("%w: media id is required", ErrDanmakuInvalidInput)
	}
	if !s.Available() {
		return DanmakuPrewarmTask{}, ErrDanmakuUnavailable
	}
	var media model.Media
	if err := s.repos.DB.WithContext(ctx).Where("id = ?", mediaID).First(&media).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DanmakuPrewarmTask{}, fmt.Errorf("%w: media not found", ErrDanmakuInvalidInput)
		}
		return DanmakuPrewarmTask{}, err
	}
	if season <= 0 {
		season = media.SeasonNum
	}
	if season <= 0 {
		return DanmakuPrewarmTask{}, fmt.Errorf("%w: season is required for prewarm", ErrDanmakuInvalidInput)
	}
	episodes, err := s.seasonEpisodes(ctx, media, season)
	if err != nil {
		return DanmakuPrewarmTask{}, err
	}
	if len(episodes) == 0 {
		return DanmakuPrewarmTask{}, fmt.Errorf("%w: no episodes found for season %d", ErrDanmakuInvalidInput, season)
	}
	task, err := s.pipeline.StartDanmakuPrewarm(ctx, DanmakuPrewarmRequest{
		OwnerID:  mediaID,
		MediaID:  mediaID,
		Season:   season,
		Episodes: episodes,
	})
	if err != nil {
		if errors.Is(err, ErrDanmakuUnavailable) {
			return DanmakuPrewarmTask{}, err
		}
		return DanmakuPrewarmTask{}, fmt.Errorf("%w: %v", ErrDanmakuUnavailable, err)
	}
	return task, nil
}

// seasonEpisodes 按媒体所属的剧集/季解析分集，供整季预热使用。
func (s *DanmakuService) seasonEpisodes(ctx context.Context, media model.Media, season int) ([]DanmakuPrewarmEpisode, error) {
	query := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
		Where("season_num = ? AND episode_num > 0", season)
	if libraryID := strings.TrimSpace(media.LibraryID); libraryID != "" {
		query = query.Where("library_id = ?", libraryID)
	}
	if seriesID := strings.TrimSpace(media.SeriesID); seriesID != "" {
		query = query.Where("series_id = ?", seriesID)
	} else {
		// 没有剧集归属时只预热这一条，避免把同名作品的其他季混进来。
		query = query.Where("id = ?", media.ID)
	}
	var rows []model.Media
	if err := query.Order("episode_num asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	episodes := make([]DanmakuPrewarmEpisode, 0, len(rows))
	for _, row := range rows {
		episodes = append(episodes, DanmakuPrewarmEpisode{
			MediaID:    row.ID,
			EpisodeKey: fmt.Sprintf("S%02dE%02d", row.SeasonNum, row.EpisodeNum),
		})
	}
	return episodes, nil
}

func (s *DanmakuService) PrewarmTask(ctx context.Context, taskID string) (DanmakuPrewarmTask, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return DanmakuPrewarmTask{}, fmt.Errorf("%w: task id is required", ErrDanmakuInvalidInput)
	}
	if !s.Available() {
		return DanmakuPrewarmTask{}, ErrDanmakuUnavailable
	}
	task, err := s.pipeline.GetDanmakuPrewarm(ctx, taskID)
	if err != nil {
		var pipelineErr *resourcePipelineError
		if errors.As(err, &pipelineErr) && pipelineErr.StatusCode == 404 {
			return DanmakuPrewarmTask{}, fmt.Errorf("%w: %s", ErrDanmakuPrewarmNotFound, pipelineErr.Message)
		}
		if errors.Is(err, ErrDanmakuPrewarmNotFound) || errors.Is(err, ErrDanmakuUnavailable) {
			return DanmakuPrewarmTask{}, err
		}
		return DanmakuPrewarmTask{}, fmt.Errorf("%w: %v", ErrDanmakuUnavailable, err)
	}
	return task, nil
}

// Payload 返回可直接渲染的弹幕；没有关联时先自动匹配，匹配不上时明确报错。
func (s *DanmakuService) Payload(ctx context.Context, mediaID string, options DanmakuOptions) (DanmakuPayload, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return DanmakuPayload{}, fmt.Errorf("%w: media id is required", ErrDanmakuInvalidInput)
	}
	if !s.Available() {
		return DanmakuPayload{}, ErrDanmakuUnavailable
	}
	row, err := s.Association(ctx, mediaID)
	if err != nil {
		return DanmakuPayload{}, err
	}
	if row == nil || row.Status != DanmakuStatusMatched || row.EpisodeID == "" {
		if _, err := s.Match(ctx, mediaID); err != nil {
			return DanmakuPayload{}, err
		}
		if row, err = s.Association(ctx, mediaID); err != nil {
			return DanmakuPayload{}, err
		}
	}
	if row == nil || row.Status != DanmakuStatusMatched || row.EpisodeID == "" {
		return DanmakuPayload{}, fmt.Errorf("%w: %s", ErrDanmakuUnmatched, rowAttempts(row))
	}
	offset := row.OffsetSeconds
	if options.OffsetSeconds != 0 {
		// 单次覆盖也要校验：否则管线会用 400 拒绝，而调用方只会看到 503。
		if err := validateDanmakuOffset(options.OffsetSeconds); err != nil {
			return DanmakuPayload{}, err
		}
		offset = options.OffsetSeconds
	}
	payload, err := s.pipeline.FetchDanmaku(ctx, DanmakuFetchRequest{
		MediaID:              mediaID,
		EpisodeID:            row.EpisodeID,
		Source:               row.Provider,
		ChConvert:            options.ChConvert,
		OffsetSeconds:        offset,
		ProviderShiftSeconds: row.ProviderShiftSeconds,
		AnimeTitle:           row.AnimeTitle,
		EpisodeTitle:         row.EpisodeTitle,
		MatchMode:            row.MatchMode,
		WithRelated:          options.WithRelated,
	})
	if err != nil {
		if errors.Is(err, ErrDanmakuUnavailable) {
			return DanmakuPayload{}, err
		}
		return DanmakuPayload{}, fmt.Errorf("%w: %v", ErrDanmakuUnavailable, err)
	}
	payload.MediaID = mediaID
	return payload, nil
}

// State 返回本地关联状态，不回源。
func (s *DanmakuService) State(ctx context.Context, mediaID string) (DanmakuMatchResult, error) {
	row, err := s.Association(ctx, mediaID)
	if err != nil {
		return DanmakuMatchResult{}, err
	}
	return matchResultFromRow(row), nil
}

func matchResultFromRow(row *model.MediaDanmaku) DanmakuMatchResult {
	if row == nil {
		return DanmakuMatchResult{
			Status:     DanmakuStatusUnmatched,
			Candidates: []DanmakuMatchCandidate{},
			Attempts:   []DanmakuAttempt{},
		}
	}
	return DanmakuMatchResult{
		Matched:      row.Status == DanmakuStatusMatched && row.EpisodeID != "",
		Provider:     row.Provider,
		MatchMode:    row.MatchMode,
		EpisodeID:    row.EpisodeID,
		AnimeTitle:   row.AnimeTitle,
		EpisodeTitle: row.EpisodeTitle,
		Shift:        row.ProviderShiftSeconds,
		Status:       row.Status,
		Candidates:   []DanmakuMatchCandidate{},
		Attempts:     decodeDanmakuAttempts(row.Attempts),
	}
}

func rowAttempts(row *model.MediaDanmaku) string {
	if row == nil {
		return ""
	}
	return row.Attempts
}

func encodeDanmakuAttempts(attempts []DanmakuAttempt) string {
	if len(attempts) == 0 {
		return ""
	}
	encoded, err := json.Marshal(attempts)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func decodeDanmakuAttempts(raw string) []DanmakuAttempt {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []DanmakuAttempt{}
	}
	var attempts []DanmakuAttempt
	if err := json.Unmarshal([]byte(raw), &attempts); err != nil {
		return []DanmakuAttempt{}
	}
	return attempts
}

// DanmakuXML 把归一化弹幕转成 B 站 XML，用于 Emby 兼容端点与导出。
func DanmakuXML(payload DanmakuPayload) []byte {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	builder.WriteString("<i>\n")
	builder.WriteString("  <chatserver>chat.bilibili.com</chatserver>\n")
	builder.WriteString("  <chatid>" + xmlEscape(payload.EpisodeID) + "</chatid>\n")
	builder.WriteString("  <mission>0</mission>\n")
	builder.WriteString("  <maxlimit>" + strconv.Itoa(payload.Count) + "</maxlimit>\n")
	builder.WriteString("  <source>MediaStationGo</source>\n")
	sentAt := time.Now().Unix()
	for _, comment := range payload.Comments {
		size := comment.Size
		if size <= 0 {
			size = 25
		}
		p := fmt.Sprintf("%.2f,%d,%d,%d,%d,0,%s,0",
			comment.Time,
			comment.Mode,
			size,
			comment.Color,
			sentAt,
			comment.CID,
		)
		builder.WriteString(`  <d p="` + xmlEscape(p) + `">` + xmlEscape(comment.M) + "</d>\n")
	}
	builder.WriteString("</i>\n")
	return []byte(builder.String())
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}
