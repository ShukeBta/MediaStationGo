package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

const (
	SystemUpdateImageSettingKey           = "system.update.image"
	SystemUpdateWatchtowerImageSettingKey = "system.update.watchtower_image"
	SystemUpdateCommandSettingKey         = "system.update.command"
	SystemUpdateComposeDirSettingKey      = "system.update.compose_dir"

	DefaultSystemUpdateImage           = "ghcr.io/shukebta/mediastation-go:latest"
	DefaultSystemUpdateWatchtowerImage = "containrrr/watchtower:latest"
)

const (
	systemUpdateCheckTimeout = 20 * time.Second
	systemUpdateRunTimeout   = 15 * time.Minute
)

var ErrSystemUpdateRunning = errors.New("system update already running")

type SystemUpdateStatus struct {
	Image           string  `json:"image"`
	CurrentVersion  string  `json:"current_version,omitempty"`
	WatchtowerImage string  `json:"watchtower_image,omitempty"`
	UpdateMode      string  `json:"update_mode,omitempty"`
	ComposeDir      string  `json:"compose_dir,omitempty"`
	ComposeFile     string  `json:"compose_file,omitempty"`
	ComposeCommand  string  `json:"compose_command,omitempty"`
	ContainerID     string  `json:"container_id,omitempty"`
	ContainerName   string  `json:"container_name,omitempty"`
	CurrentImageID  string  `json:"current_image_id,omitempty"`
	LocalDigest     string  `json:"local_digest,omitempty"`
	RemoteDigest    string  `json:"remote_digest,omitempty"`
	DockerAvailable bool    `json:"docker_available"`
	CanApply        bool    `json:"can_apply"`
	UpdateAvailable *bool   `json:"update_available,omitempty"`
	Running         bool    `json:"running"`
	TaskID          string  `json:"task_id,omitempty"`
	Message         string  `json:"message,omitempty"`
	Details         string  `json:"details,omitempty"`
	CheckedAt       *string `json:"checked_at,omitempty"`
	StartedAt       *string `json:"started_at,omitempty"`
}

type SystemUpdateService struct {
	cfg     *config.Config
	log     *zap.Logger
	repo    *repository.Container
	tasks   *TaskTrackerService
	version string

	mu      sync.Mutex
	running bool
	last    *SystemUpdateStatus
}

func NewSystemUpdateService(cfg *config.Config, log *zap.Logger, repo *repository.Container, tasks *TaskTrackerService, version string) *SystemUpdateService {
	return &SystemUpdateService{cfg: cfg, log: log, repo: repo, tasks: tasks, version: normalizeSystemUpdateVersion(version)}
}

func (s *SystemUpdateService) Status(ctx context.Context) SystemUpdateStatus {
	s.mu.Lock()
	if s.last != nil {
		status := cloneSystemUpdateStatus(*s.last)
		status.Running = s.running
		s.mu.Unlock()
		return status
	}
	running := s.running
	s.mu.Unlock()

	status := s.baseStatus(ctx)
	status.Running = running
	status.Message = "尚未检查更新"
	return status
}

func (s *SystemUpdateService) Check(ctx context.Context) SystemUpdateStatus {
	status := s.check(ctx)
	s.mu.Lock()
	status.Running = s.running
	s.last = &status
	s.mu.Unlock()
	return status
}

func (s *SystemUpdateService) Apply(ctx context.Context) (SystemUpdateStatus, error) {
	s.mu.Lock()
	if s.running {
		if s.last != nil {
			status := cloneSystemUpdateStatus(*s.last)
			status.Running = true
			s.mu.Unlock()
			return status, ErrSystemUpdateRunning
		}
		s.mu.Unlock()
		status := s.baseStatus(ctx)
		status.Running = true
		return status, ErrSystemUpdateRunning
	}
	s.running = true
	s.mu.Unlock()

	status := s.check(ctx)
	if !status.CanApply {
		s.finishWithoutTask(status)
		return status, errors.New(firstNonEmpty(status.Message, "当前环境不支持自动更新"))
	}

	task := s.startUpdateTask(status)
	if task != nil {
		status.TaskID = task.id
	}
	now := time.Now().Format(time.RFC3339)
	status.StartedAt = &now
	status.Running = true
	status.Message = "已开始后台更新，容器会在更新完成后重启"
	s.mu.Lock()
	s.last = &status
	s.mu.Unlock()

	go s.runUpdate(context.Background(), status, task)
	return status, nil
}

func (s *SystemUpdateService) finishWithoutTask(status SystemUpdateStatus) {
	status.Running = false
	s.mu.Lock()
	s.running = false
	s.last = &status
	s.mu.Unlock()
}

func (s *SystemUpdateService) runUpdate(parent context.Context, status SystemUpdateStatus, task *TaskHandle) {
	runCtx, cancel := context.WithTimeout(parent, systemUpdateRunTimeout)
	defer cancel()

	command := s.updateCommand(runCtx, status)
	if task != nil {
		task.Update(TaskUpdate{
			Stage:   "pull_restart",
			Message: "正在拉取最新镜像并重建当前容器",
			Details: []string{
				"image=" + status.Image,
				"container=" + firstNonEmpty(status.ContainerName, status.ContainerID),
				"command=" + redactSystemUpdateCommand(command),
			},
		})
	}

	output, err := runSystemUpdateShell(runCtx, command)
	details := systemUpdateOutputDetails(output)
	if task != nil {
		task.Finish(err, TaskUpdate{
			Stage:   "completed",
			Message: "更新命令已执行；如果容器未自动重启，请查看 Docker 日志",
			Details: details,
		})
	}
	status.Running = false
	status.Message = "更新命令已执行；如果容器未自动重启，请查看 Docker 日志"
	status.Details = strings.Join(details, "\n")
	if err != nil {
		status.Message = "更新失败：" + err.Error()
	}
	s.mu.Lock()
	s.running = false
	s.last = &status
	s.mu.Unlock()
	if err != nil && s.log != nil {
		s.log.Warn("system update failed", zap.Error(err), zap.String("output", output))
	}
}

func (s *SystemUpdateService) startUpdateTask(status SystemUpdateStatus) *TaskHandle {
	if s == nil || s.tasks == nil {
		return nil
	}
	return s.tasks.Start(TaskKindUpdate, "Docker 镜像热更新", TaskUpdate{
		Stage:      "prepare",
		SourcePath: status.Image,
		DestPath:   firstNonEmpty(status.ContainerName, status.ContainerID),
		Message:    "准备拉取最新版 Docker 镜像并重启容器",
	})
}

func (s *SystemUpdateService) baseStatus(ctx context.Context) SystemUpdateStatus {
	image := s.setting(ctx, SystemUpdateImageSettingKey, os.Getenv("MEDIASTATION_UPDATE_IMAGE"))
	if image == "" {
		image = DefaultSystemUpdateImage
	}
	watchtowerImage := s.setting(ctx, SystemUpdateWatchtowerImageSettingKey, os.Getenv("MEDIASTATION_UPDATE_WATCHTOWER_IMAGE"))
	if watchtowerImage == "" {
		watchtowerImage = DefaultSystemUpdateWatchtowerImage
	}
	return SystemUpdateStatus{
		Image:           image,
		CurrentVersion:  s.version,
		WatchtowerImage: watchtowerImage,
		UpdateMode:      "compose",
		ContainerID:     currentContainerID(),
	}
}

func normalizeSystemUpdateVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "dev"
	}
	return version
}

func (s *SystemUpdateService) setting(ctx context.Context, key, fallback string) string {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return strings.TrimSpace(fallback)
	}
	if value, err := s.repo.Setting.Get(ctx, key); err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

func cloneSystemUpdateStatus(status SystemUpdateStatus) SystemUpdateStatus {
	if status.UpdateAvailable != nil {
		v := *status.UpdateAvailable
		status.UpdateAvailable = &v
	}
	if status.CheckedAt != nil {
		v := *status.CheckedAt
		status.CheckedAt = &v
	}
	if status.StartedAt != nil {
		v := *status.StartedAt
		status.StartedAt = &v
	}
	return status
}
