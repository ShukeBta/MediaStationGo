package service

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func (s *SystemUpdateService) check(ctx context.Context) SystemUpdateStatus {
	status := s.baseStatus(ctx)
	now := time.Now().Format(time.RFC3339)
	status.CheckedAt = &now

	customCommand := s.rawUpdateCommand(ctx)
	composeTarget, composeErr := s.resolveComposeTarget(ctx)
	status.ComposeDir = composeTarget.Dir
	status.ComposeFile = composeTarget.File
	status.ComposeCommand = composeTarget.Command

	if strings.TrimSpace(customCommand) != "" {
		status.CanApply = true
		status.Message = "将执行自定义更新命令"
	}

	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		if status.CanApply {
			status.Details = err.Error()
			return status
		}
		status.Message = "当前环境未安装 docker CLI，无法执行 Docker Compose 更新"
		status.Details = err.Error()
		return status
	}
	if status.ComposeCommand == "" {
		status.ComposeCommand = availableComposeCommand(ctx, dockerPath)
	}
	if status.ComposeCommand == "" && strings.TrimSpace(customCommand) == "" {
		status.Message = "当前环境未安装 docker compose 插件或 docker-compose 命令"
		return status
	}

	if strings.TrimSpace(customCommand) == "" {
		if composeErr != "" {
			status.Message = composeErr
			return status
		}
		status.CanApply = true
		status.Message = "已识别 Docker Compose 安装目录，可执行一键更新"
	}

	checkCtx, cancel := context.WithTimeout(ctx, systemUpdateCheckTimeout)
	defer cancel()
	if out, err := runSystemUpdateCommand(checkCtx, dockerPath, "version", "--format", "{{.Server.Version}}"); err != nil {
		details := strings.TrimSpace(out + "\n" + err.Error())
		status.Details = details
		if status.CanApply {
			status.Message = "Docker Compose 更新命令已就绪；当前无法读取 Docker 引擎摘要，执行时如失败请检查 Docker 权限"
			return status
		}
		status.Message = "无法连接 Docker 引擎，请检查 Docker 权限"
		return status
	}
	status.DockerAvailable = true

	containerID := currentContainerID()
	status.ContainerID = containerID
	if containerID == "" {
		if status.CanApply {
			status.Message = "已识别 Docker Compose 安装目录；无法识别当前容器 ID，将使用默认容器名 mediastation-go 重启"
		}
		return status
	}

	populateSystemUpdateDockerMetadata(checkCtx, dockerPath, &status)
	status.UpdateAvailable = compareDockerDigests(status.LocalDigest, status.RemoteDigest)
	status.CanApply = true
	status.Message = systemUpdateCheckMessage(status)
	return status
}

func availableComposeCommand(ctx context.Context, dockerPath string) string {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := runSystemUpdateCommand(checkCtx, dockerPath, "compose", "version"); err == nil {
		return "docker compose"
	}
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return "docker-compose"
	}
	return ""
}

type systemUpdateFallback struct {
	command        string
	customMessage  string
	customDetails  string
	defaultMessage string
	defaultDetails string
}

func systemUpdateCustomFallback(status SystemUpdateStatus, fallback systemUpdateFallback) SystemUpdateStatus {
	if fallback.command != "" {
		status.CanApply = true
		status.Message = fallback.customMessage
		status.Details = fallback.customDetails
		return status
	}
	status.Message = fallback.defaultMessage
	status.Details = fallback.defaultDetails
	return status
}

func populateSystemUpdateDockerMetadata(ctx context.Context, dockerPath string, status *SystemUpdateStatus) {
	if status == nil {
		return
	}
	if out, err := runSystemUpdateCommand(ctx, dockerPath, "inspect", status.ContainerID, "--format", "{{.Name}}|{{.Image}}"); err == nil {
		name, imageID := parseContainerInspectLine(out)
		status.ContainerName = name
		status.CurrentImageID = imageID
	}
	if out, err := runSystemUpdateCommand(ctx, dockerPath, "image", "inspect", status.Image, "--format", "{{json .RepoDigests}}"); err == nil {
		status.LocalDigest = firstDockerDigest(out)
	}
	if out, err := runSystemUpdateCommand(ctx, dockerPath, "manifest", "inspect", "--verbose", status.Image); err == nil {
		status.RemoteDigest = firstDockerDigest(out)
	} else if status.Details == "" {
		status.Details = strings.TrimSpace(out + "\n" + err.Error())
	}
}

func currentContainerID() string {
	if value := strings.TrimSpace(os.Getenv("HOSTNAME")); looksLikeContainerID(value) {
		return value
	}
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	host = strings.TrimSpace(host)
	if looksLikeContainerID(host) {
		return host
	}
	return ""
}

func looksLikeContainerID(value string) bool {
	if len(value) < 12 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'f') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func parseContainerInspectLine(raw string) (string, string) {
	line := strings.TrimSpace(raw)
	parts := strings.SplitN(line, "|", 2)
	if len(parts) != 2 {
		return strings.TrimPrefix(line, "/"), ""
	}
	return strings.TrimPrefix(strings.TrimSpace(parts[0]), "/"), strings.TrimSpace(parts[1])
}

var dockerDigestPattern = regexp.MustCompile(`sha256:[a-fA-F0-9]{64}`)

func firstDockerDigest(raw string) string {
	match := dockerDigestPattern.FindString(raw)
	return strings.ToLower(strings.TrimSpace(match))
}

func compareDockerDigests(localDigest, remoteDigest string) *bool {
	localDigest = strings.ToLower(strings.TrimSpace(localDigest))
	remoteDigest = strings.ToLower(strings.TrimSpace(remoteDigest))
	if localDigest == "" || remoteDigest == "" {
		return nil
	}
	available := localDigest != remoteDigest
	return &available
}

func systemUpdateCheckMessage(status SystemUpdateStatus) string {
	if status.UpdateAvailable == nil {
		return "已连接 Docker，可执行更新；当前环境无法精确比较本地与远端镜像摘要"
	}
	if *status.UpdateAvailable {
		return "检测到远端镜像摘要与本地不同，可以执行更新"
	}
	return "当前本地镜像摘要与远端一致"
}
