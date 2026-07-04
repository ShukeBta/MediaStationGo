package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAccessibleLibraryPathMapsConfiguredHostMediaDir(t *testing.T) {
	root := t.TempDir()
	hostRoot := filepath.Join(root, "nas", "host", "media")
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_MEDIA_DIR", hostRoot)
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerRoot)

	got, err := resolveAccessibleLibraryPath(filepath.Join(hostRoot, "电视剧", "国产剧"))
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

func TestResolveAccessibleLibraryPathMapsWindowsDriveBeforeLinuxAbs(t *testing.T) {
	root := t.TempDir()
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_MEDIA_DIR", `Q:\media`)
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerRoot)

	for _, input := range []string{
		`Q:\media\电视剧\国产剧`,
		`Q:/media/电视剧/国产剧`,
		`/app/Q:\media\电视剧\国产剧`,
		`/app/Q:/media/电视剧/国产剧`,
	} {
		t.Run(input, func(t *testing.T) {
			got, err := resolveAccessibleLibraryPath(input)
			if err != nil {
				t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
			}
			if got != filepath.Clean(containerLibrary) {
				t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
			}
		})
	}
}

func TestResolveAccessibleLibraryPathRecoversDockerPollutedWindowsDrive(t *testing.T) {
	root := t.TempDir()
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_MEDIA_DIR", `F:\media`)
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerRoot)

	got, err := resolveAccessibleLibraryPath(`/app/F:\media\电视剧\国产剧`)
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

func TestResolveAccessibleLibraryPathKeepsAccessibleContainerPath(t *testing.T) {
	containerLibrary := filepath.Join(t.TempDir(), "media", "电影")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveAccessibleLibraryPath(containerLibrary)
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

func TestResolveAccessibleLibraryPathMapsRelativeDockerMediaMarker(t *testing.T) {
	root := t.TempDir()
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerRoot)

	got, err := resolveAccessibleLibraryPath(filepath.Join("media", "电视剧", "国产剧"))
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

// 旧库把宿主机绝对路径整段写进来时（moviepilot 布局：/vol1/.../media/电视剧/国产剧），
// /media 出现在路径中段而非开头；解析必须能把最后一个 /media 段之后的尾巴映射到容器媒体
// 目录，否则新版容器只挂了 /media 就永远扫不出这类旧库。钉死该行为，防止再退化。
func TestResolveAccessibleLibraryPathMapsEmbeddedHostMediaMarker(t *testing.T) {
	root := t.TempDir()
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerRoot)

	got, err := resolveAccessibleLibraryPath("/vol1/1000/Docker/moviepilot-v2/media/电视剧/国产剧")
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

func TestResolveAccessibleMappedPathMapsEmbeddedHostDownloadMarker(t *testing.T) {
	root := t.TempDir()
	containerDownloads := filepath.Join(root, "container", "downloads")
	containerItem := filepath.Join(containerDownloads, "国产剧")
	if err := os.MkdirAll(containerItem, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", containerDownloads)

	got, _, err := resolveAccessibleMappedPath("/vol1/1000/Docker/qbittorrent/downloads/国产剧")
	if err != nil {
		t.Fatalf("resolveAccessibleMappedPath() error = %v", err)
	}
	if got != filepath.Clean(containerItem) {
		t.Fatalf("resolveAccessibleMappedPath() = %q, want %q", got, filepath.Clean(containerItem))
	}
}

// 中段 marker 启发式绝不能污染目的地解析：resolveMappedDestinationPath 不校验存在性、
// 会返回首个候选，一旦启发式并进来，就会把形如 <tmp>/media/... 的合法整理目的地错误
// 重写到容器根 /media（曾导致 organizer 跨盘 hardlink 失败）。钉死该边界。
func TestResolveMappedDestinationPathIgnoresEmbeddedMediaMarker(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "001", "media", "电视剧", "国产剧", "Some Show")
	if got := resolveMappedDestinationPath(dst); got != filepath.Clean(dst) {
		t.Fatalf("resolveMappedDestinationPath() = %q, want %q (embedded /media must not remap destinations)", got, filepath.Clean(dst))
	}
}

func TestInferLibraryKindFromCategoryPathOverridesMovieDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{name: "国产剧", path: `/media/电视剧/国产剧`, want: "tv"},
		{name: "日漫", path: `/media/电视剧/日漫`, want: "anime"},
		{name: "综艺", path: `/media/电视剧/综艺`, want: "variety"},
		{name: "成人", path: `/media/成人`, want: "adult"},
		{name: "动画电影", path: `/media/电影/动画电影`, want: "movie"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferLibraryKind(tc.name, tc.path, "movie"); got != tc.want {
				t.Fatalf("inferLibraryKind() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMappedPathCandidatesMapWindowsDriveDownloadMarker(t *testing.T) {
	root := t.TempDir()
	containerDownloads := filepath.Join(root, "container", "downloads")
	containerLibrary := filepath.Join(containerDownloads, "国产剧")
	t.Setenv("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", containerDownloads)

	want := filepath.Clean(containerLibrary)
	for _, got := range mappedPathCandidates(`F:\downloads\国产剧`) {
		if got == want {
			return
		}
	}
	t.Fatalf("mappedPathCandidates() missing %q", want)
}

func TestResolveMappedDestinationPathPrefersConfiguredContainerMapping(t *testing.T) {
	root := t.TempDir()
	containerMedia := filepath.Join(root, "container", "media")
	if err := os.MkdirAll(containerMedia, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_MEDIA_DIR", `Q:\media`)
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerMedia)

	for _, input := range []string{`Q:\media`, `Q:/media`, `/app/Q:\media`} {
		t.Run(input, func(t *testing.T) {
			got := resolveMappedDestinationPath(input)
			if got != filepath.Clean(containerMedia) {
				t.Fatalf("resolveMappedDestinationPath() = %q, want %q", got, filepath.Clean(containerMedia))
			}
		})
	}
}

func TestResolveMappedDestinationPathPrefersContainerForRelativeMediaMarker(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	containerMedia := filepath.Join(root, "container", "media")
	if err := os.MkdirAll(filepath.Join(workDir, "media", "电视剧", "国产剧"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(containerMedia, "电视剧", "国产剧"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workDir)
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerMedia)

	got := resolveMappedDestinationPath(filepath.Join("media", "电视剧", "国产剧"))
	want := filepath.Join(containerMedia, "电视剧", "国产剧")
	if got != filepath.Clean(want) {
		t.Fatalf("resolveMappedDestinationPath() = %q, want %q", got, filepath.Clean(want))
	}
}

func TestResolveAccessibleMappedPathMapsWindowsDownloadVariants(t *testing.T) {
	root := t.TempDir()
	containerDownloads := filepath.Join(root, "container", "downloads")
	containerSource := filepath.Join(containerDownloads, "国产剧")
	if err := os.MkdirAll(containerSource, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_DOWNLOAD_DIR", `Q:\downloads`)
	t.Setenv("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", containerDownloads)

	for _, input := range []string{`Q:\downloads\国产剧`, `Q:/downloads/国产剧`, `/app/Q:\downloads\国产剧`} {
		t.Run(input, func(t *testing.T) {
			got, info, err := resolveAccessibleMappedPath(input)
			if err != nil {
				t.Fatalf("resolveAccessibleMappedPath() error = %v", err)
			}
			if !info.IsDir() {
				t.Fatalf("resolved path is not dir")
			}
			if got != filepath.Clean(containerSource) {
				t.Fatalf("resolveAccessibleMappedPath() = %q, want %q", got, filepath.Clean(containerSource))
			}
		})
	}
}
