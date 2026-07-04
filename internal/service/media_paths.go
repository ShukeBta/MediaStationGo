package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func inferLibraryKind(name, path, requested string) string {
	requested = normalizeOrganizeMediaType(requested)
	text := strings.ToLower(name + " " + filepath.ToSlash(path))
	switch {
	case containsAnyText(text, "成人", "番号", "jav", "9kg", "adult", "nsfw"):
		return "adult"
	case containsAnyText(text, "综艺", "真人秀", "variety"):
		return "variety"
	case containsAnyText(text, "国漫", "日漫", "日番", "动漫", "动画", "anime", "bangumi") && !containsAnyText(text, "动画电影"):
		return "anime"
	case containsAnyText(text, "电视剧", "剧集", "连续剧", "短剧", "国产剧", "国剧", "大陆剧", "华语剧", "国产电视剧", "大陆电视剧", "华语电视剧", "欧美剧", "欧美电视剧", "美剧", "英剧", "日韩剧", "日韩电视剧", "日剧", "韩剧", "港剧", "台剧", "港台剧", "泰剧", "tv", "series"):
		return "tv"
	case containsAnyText(text, "电影", "movie", "film"):
		return "movie"
	}
	if requested != "" {
		return requested
	}
	return "movie"
}

func resolveAccessibleLibraryPath(path string) (string, error) {
	input := strings.TrimSpace(path)
	if input == "" {
		return "", errors.New("path required")
	}
	for _, candidate := range mappedPathCandidates(input) {
		if isAccessibleDir(candidate) {
			return filepath.Clean(candidate), nil
		}
	}
	// 兜底：旧库把宿主机绝对路径整段写进来时 /media 出现在路径中段（moviepilot 布局），
	// 常规候选覆盖不到。只在这里按 isAccessibleDir 校验后采用，避免污染不校验存在性的
	// 目的地解析（resolveMappedDestinationPath）。见 embeddedContainerMarkerCandidates。
	for _, candidate := range embeddedContainerMarkerCandidates(input) {
		if isAccessibleDir(candidate) {
			return filepath.Clean(candidate), nil
		}
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	return "", fmt.Errorf("path is not an accessible directory: %s", abs)
}

// embeddedContainerMarkerCandidates 处理旧库把宿主机绝对路径整段写进来的情况：
// /media 或 /downloads 出现在路径中段（如 moviepilot 的 /vol1/.../media/电视剧/国产剧），
// 取最后一个该段之后的尾巴拼到容器目录（默认 /media、/downloads，可用 *_CONTAINER_DIR 覆盖）。
//
// 这个启发式偏激进，只能在**按存在性校验**的读/扫描解析里使用；绝不能并入
// mappedPathCandidates——resolveMappedDestinationPath 不校验存在性、会返回首个候选，
// 那样会把形如 <tmp>/media/... 的合法目的地错误重写到容器根，破坏整理/硬链接。
func embeddedContainerMarkerCandidates(input string) []string {
	normalized := cleanPathForVolumeMapping(input)
	markerPath := pathAfterWindowsDrivePrefix(normalized)
	var candidates []string
	for _, marker := range []struct {
		part      string
		container string
	}{
		{part: "/media", container: envOrDefault("MEDIASTATION_MEDIA_CONTAINER_DIR", "/media")},
		{part: "/downloads", container: envOrDefault("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", "/downloads")},
	} {
		part := strings.TrimRight(marker.part, "/")
		container := strings.TrimRight(filepath.ToSlash(marker.container), "/")
		if idx := strings.LastIndex(markerPath, part+"/"); idx > 0 {
			candidates = append(candidates, filepath.Clean(filepath.FromSlash(container+markerPath[idx+len(part):])))
		}
	}
	return candidates
}

// describeUnresolvedLibraryPath 把「路径解析失败」变成可操作的诊断信息：列出尝试过的
// 候选路径与当前的宿主机→容器映射状态。旧库存的是宿主机路径，在新版/容器内常因为缺少
// MEDIASTATION_MEDIA_DIR 映射而扫不出媒体——这条诊断帮助用户直接定位到底哪一步断了。
// 仅用于日志与扫描错误提示，不改变解析逻辑本身。
func describeUnresolvedLibraryPath(rawPath string) string {
	rawPath = strings.TrimSpace(rawPath)
	candidates := mappedPathCandidates(rawPath)
	mediaHost := strings.TrimSpace(os.Getenv("MEDIASTATION_MEDIA_DIR"))
	mediaContainer := envOrDefault("MEDIASTATION_MEDIA_CONTAINER_DIR", "/media")
	var b strings.Builder
	fmt.Fprintf(&b, "媒体库路径无法解析为可访问目录：%s（已尝试候选：%s）", rawPath, strings.Join(candidates, " | "))
	if mediaHost == "" {
		b.WriteString("；未配置 MEDIASTATION_MEDIA_DIR / MEDIASTATION_MEDIA_CONTAINER_DIR。若为 Docker 部署且此库为旧宿主机路径，请设置这两个变量把宿主机路径映射到容器内路径，并确认对应 volume 已挂载。")
	} else {
		fmt.Fprintf(&b, "；当前映射 %s → %s。请确认该库路径位于此宿主机目录下，或补充对应的 volume 与路径映射。", mediaHost, mediaContainer)
	}
	return b.String()
}

func resolveAccessibleMappedPath(path string) (string, os.FileInfo, error) {
	input := strings.TrimSpace(path)
	if input == "" {
		return "", nil, errors.New("path required")
	}
	candidates := mappedPathCandidates(input)
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil {
			return filepath.Clean(candidate), info, nil
		}
	}
	// 兜底同 resolveAccessibleLibraryPath：中段 /media|/downloads 的旧宿主机路径，
	// 仅在 os.Stat 校验存在后采用。
	for _, candidate := range embeddedContainerMarkerCandidates(input) {
		if info, err := os.Stat(candidate); err == nil {
			return filepath.Clean(candidate), info, nil
		}
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", nil, fmt.Errorf("invalid path: %w", err)
	}
	return "", nil, fmt.Errorf("path is not accessible: %s", abs)
}

func resolveMappedDestinationPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if _, err := os.Stat(clean); err == nil && !isRelativeVolumeMarkerPath(clean) {
		return clean
	}
	for _, candidate := range mappedPathCandidates(clean) {
		if candidate == clean {
			continue
		}
		return filepath.Clean(candidate)
	}
	return clean
}

func mappedPathCandidates(input string) []string {
	var candidates []string
	add := func(candidate string) {
		candidate = filepath.Clean(filepath.FromSlash(strings.TrimSpace(candidate)))
		if candidate == "" || candidate == "." {
			return
		}
		for _, existing := range candidates {
			if sameLibraryPath(existing, candidate) {
				return
			}
		}
		candidates = append(candidates, candidate)
	}
	clean := filepath.Clean(input)
	if isRelativeVolumeMarkerPath(clean) {
		for _, candidate := range dockerVolumePathCandidates(clean) {
			add(candidate)
		}
	}
	add(clean)
	for _, candidate := range dockerVolumePathCandidates(input) {
		add(candidate)
	}
	for _, candidate := range dockerVolumePathCandidates(clean) {
		add(candidate)
	}
	if slashClean := cleanPathForVolumeMapping(input); slashClean != "" {
		add(slashClean)
	}
	if abs, err := filepath.Abs(input); err == nil {
		add(abs)
		for _, candidate := range dockerVolumePathCandidates(abs) {
			add(candidate)
		}
	}
	return candidates
}

func isAccessibleDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func dockerVolumePathCandidates(path string) []string {
	normalized := cleanPathForVolumeMapping(path)
	var candidates []string
	addCandidate := func(candidate string) {
		candidate = filepath.Clean(filepath.FromSlash(candidate))
		for _, existing := range candidates {
			if sameLibraryPath(existing, candidate) {
				return
			}
		}
		candidates = append(candidates, candidate)
	}

	for _, mapping := range []struct {
		env       string
		container string
	}{
		{env: "MEDIASTATION_MEDIA_DIR", container: envOrDefault("MEDIASTATION_MEDIA_CONTAINER_DIR", "/media")},
		{env: "MEDIASTATION_DOWNLOAD_DIR", container: envOrDefault("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", "/downloads")},
	} {
		container := cleanPathForVolumeMapping(mapping.container)
		if container == "." || container == "" || strings.HasPrefix(container, ".") {
			continue
		}
		relativeMarker := strings.TrimPrefix(normalized, "./")
		if !strings.HasPrefix(relativeMarker, "/") {
			containerBase := strings.TrimSpace(filepath.Base(container))
			if relativeMarker == containerBase {
				addCandidate(container)
				continue
			}
			if strings.HasPrefix(relativeMarker, containerBase+"/") {
				addCandidate(container + strings.TrimPrefix(relativeMarker, containerBase))
				continue
			}
		}
		host := cleanPathForVolumeMapping(os.Getenv(mapping.env))
		if host == "." || host == "" || strings.HasPrefix(host, ".") {
			continue
		}
		if normalized == host {
			addCandidate(mapping.container)
			continue
		}
		if strings.HasPrefix(normalized, host+"/") {
			addCandidate(mapping.container + strings.TrimPrefix(normalized, host))
		}
		if normalized == container {
			addCandidate(host)
			continue
		}
		if strings.HasPrefix(normalized, container+"/") {
			addCandidate(host + strings.TrimPrefix(normalized, container))
		}
	}

	for _, marker := range []struct {
		part      string
		container string
	}{
		{part: "/media", container: envOrDefault("MEDIASTATION_MEDIA_CONTAINER_DIR", "/media")},
		{part: "/downloads", container: envOrDefault("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", "/downloads")},
	} {
		part := strings.TrimRight(marker.part, "/")
		container := strings.TrimRight(filepath.ToSlash(marker.container), "/")
		markerPath := pathAfterWindowsDrivePrefix(normalized)
		if markerPath == part {
			addCandidate(container)
			continue
		}
		if strings.HasPrefix(markerPath, part+"/") {
			addCandidate(container + strings.TrimPrefix(markerPath, part))
		}
	}

	return candidates
}

func isRelativeVolumeMarkerPath(path string) bool {
	normalized := cleanPathForVolumeMapping(path)
	normalized = strings.TrimPrefix(normalized, "./")
	if normalized == "" || strings.HasPrefix(normalized, "/") || filepath.IsAbs(path) {
		return false
	}
	for _, marker := range []string{"media", "downloads"} {
		if normalized == marker || strings.HasPrefix(normalized, marker+"/") {
			return true
		}
	}
	return false
}
