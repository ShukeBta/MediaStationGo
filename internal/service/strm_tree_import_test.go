package service

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestGenerateSTRMFromTreePaths(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:   "115",
		Paths:      []string{"/电视剧/国产剧/南部档案/Season 01/Archives.S01E01.mkv", "/电视剧/国产剧/南部档案/poster.jpg", "/电视剧/国产剧/南部档案/Existing.strm"},
		TreeText:   "电视剧\n└── 国产剧\n    └── 南部档案\n        └── Existing.Tree.strm",
		SourceRoot: "/电视剧",
		OutputDir:  outDir,
		BaseURL:    "https://media.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || res.Skipped != 0 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want one generated video and ignored sidecar", res)
	}
	path := filepath.Join(outDir, "国产剧", "南部档案", "Season 01", "Archives.S01E01.strm")
	got := readSTRM(t, path)
	if !strings.HasPrefix(got, "https://media.example.com/api/cloud/play/cloud115?") {
		t.Fatalf("strm url = %q, want cloud115 play url", got)
	}
	if !strings.Contains(got, "ref=%2F%E7%94%B5%E8%A7%86%E5%89%A7%2F%E5%9B%BD%E4%BA%A7%E5%89%A7%2F%E5%8D%97%E9%83%A8%E6%A1%A3%E6%A1%88%2FSeason+01%2FArchives.S01E01.mkv") {
		t.Fatalf("strm url = %q, missing encoded source ref", got)
	}
	if _, err := os.Stat(filepath.Join(outDir, "国产剧", "南部档案", "Existing.strm")); !os.IsNotExist(err) {
		t.Fatalf("existing .strm source should be ignored by tree generator, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "电视剧", "国产剧", "南部档案", "Existing.Tree.strm")); !os.IsNotExist(err) {
		t.Fatalf("tree .strm source should be ignored by tree generator, stat err=%v", err)
	}
}

func TestGenerateSTRMFromTreeRecognizeRenameOutputPaths(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:        "openlist",
		Paths:           []string{"/电视剧/国产剧/南部档案/Season 01/Archives.The.Nanyang.Mystery.S01E02.2160p.WEB-DL.mkv", "/电影/Dune.Part.Two.2024.2160p.WEB-DL.mkv"},
		SourceRoot:      "/电视剧",
		OutputDir:       outDir,
		RecognizeRename: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 2 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want two generated renamed STRM files", res)
	}
	episode := readSTRM(t, filepath.Join(outDir, "南部档案", "Season 01", "南部档案 S01E02.strm"))
	if !strings.Contains(episode, "ref=%2F%E7%94%B5%E8%A7%86%E5%89%A7%2F%E5%9B%BD%E4%BA%A7%E5%89%A7%2F%E5%8D%97%E9%83%A8%E6%A1%A3%E6%A1%88%2FSeason+01%2FArchives.The.Nanyang.Mystery.S01E02.2160p.WEB-DL.mkv") {
		t.Fatalf("episode strm URL = %q, want original cloud ref preserved", episode)
	}
	movie := readSTRM(t, filepath.Join(outDir, "Dune Part Two (2024)", "Dune Part Two (2024).strm"))
	if !strings.Contains(movie, "ref=%2F%E7%94%B5%E5%BD%B1%2FDune.Part.Two.2024.2160p.WEB-DL.mkv") {
		t.Fatalf("movie strm URL = %q, want original cloud ref preserved", movie)
	}
	if _, err := os.Stat(filepath.Join(outDir, "国产剧", "南部档案", "Season 01", "Archives.The.Nanyang.Mystery.S01E02.2160p.WEB-DL.strm")); !os.IsNotExist(err) {
		t.Fatalf("recognize rename should not leave original episode output path, stat err=%v", err)
	}
}

func TestGenerateSTRMFromTreeSupportsCommonVideoExtensions(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider: "openlist",
		Paths: []string{
			"/Movies/BluRay.Stream.2026.m2ts",
			"/Movies/Camera.Source.2026.MTS",
			"/Movies/DVD.Feature.2026.vob",
			"/Movies/Legacy.Video.2026.wmv",
			"/Movies/Web.Legacy.2026.flv",
			"/Movies/Disc.Image.2026.iso",
		},
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 6 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want six common video sources generated", res)
	}
	for _, name := range []string{
		"BluRay.Stream.2026",
		"Camera.Source.2026",
		"DVD.Feature.2026",
		"Legacy.Video.2026",
		"Web.Legacy.2026",
		"Disc.Image.2026",
	} {
		got := readSTRM(t, filepath.Join(outDir, "Movies", name+".strm"))
		if !strings.Contains(got, "/api/cloud/play/openlist?") {
			t.Fatalf("%s strm url = %q, want cloud play url", name, got)
		}
	}
}

func TestGenerateSTRMFromTreeReportsIgnoredFileLikeRows(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider: "openlist",
		Paths: []string{
			"/Movies/A.mkv",
			"/Movies/poster.jpg",
			"/Movies/fanart.jpg (cover image)",
			"/Movies/Disc.Image.2026.iso",
			"/Movies/Existing.strm",
		},
		TreeText: strings.Join([]string{
			"电视剧",
			"└── Show.Name.2026",
			"    ├── Show.S01E01.mp4",
			"    └── Show.S01E01.nfo",
			"    └── Show.S01E01.srt 72 KB",
		}, "\n"),
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 3 || res.Ignored != 5 || len(res.IgnoredItems) != 5 {
		t.Fatalf("result = %#v, want three generated videos and five ignored sidecars", res)
	}
	for _, item := range res.IgnoredItems {
		if item == "Show.Name.2026" || item == "电视剧/Show.Name.2026" {
			t.Fatalf("directory-like dotted title should not be reported as ignored: %#v", res.IgnoredItems)
		}
	}
	if !strings.Contains(strings.Join(res.IgnoredItems, "\n"), "Show.S01E01.nfo") {
		t.Fatalf("directory-like dotted title should not be reported as ignored: %#v", res.IgnoredItems)
	}
}

func TestGenerateSTRMFromTreeTransfersMatchingSubtitleLinks(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider: "openlist",
		Paths: []string{
			"/Movies/A.mkv",
			"/Movies/A.zh.srt",
			"/Movies/Orphan.srt",
		},
		TreeText: strings.Join([]string{
			"电视剧",
			"└── Show.Name.2026",
			"    ├── Show.S01E01.mp4",
			"    ├── Show.S01E01.ass",
			"    └── Show.S01E02.srt",
		}, "\n"),
		OutputDir:         outDir,
		BaseURL:           "https://media.example.com",
		TransferSubtitles: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 4 || res.Ignored != 2 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want two video links, two matching subtitle links, and two orphan subtitles ignored", res)
	}
	movieSubtitle := readSTRM(t, filepath.Join(outDir, "Movies", "A.zh.srt.strm"))
	if !strings.HasPrefix(movieSubtitle, "https://media.example.com/api/cloud/play/openlist?") ||
		!strings.Contains(movieSubtitle, "ref=%2FMovies%2FA.zh.srt") {
		t.Fatalf("subtitle link = %q, want cloud play URL for subtitle source", movieSubtitle)
	}
	showSubtitle := readSTRM(t, filepath.Join(outDir, "电视剧", "Show.Name.2026", "Show.S01E01.ass.strm"))
	if !strings.Contains(showSubtitle, "ref=%2F%E7%94%B5%E8%A7%86%E5%89%A7%2FShow.Name.2026%2FShow.S01E01.ass") {
		t.Fatalf("tree subtitle link = %q, want full tree subtitle ref", showSubtitle)
	}
	if _, err := os.Stat(filepath.Join(outDir, "Movies", "Orphan.srt.strm")); !os.IsNotExist(err) {
		t.Fatalf("orphan subtitle should not generate a link, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "电视剧", "Show.Name.2026", "Show.S01E02.srt.strm")); !os.IsNotExist(err) {
		t.Fatalf("subtitle without matching episode should not generate a link, stat err=%v", err)
	}
}

func TestGenerateSTRMFromTreeLimitsIgnoredItemSamples(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	paths := make([]string, 0, 25)
	for i := 0; i < 25; i++ {
		paths = append(paths, filepath.ToSlash(filepath.Join("/Movies", "sidecar-"+strconv.Itoa(i)+".nfo")))
	}
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		Paths:     paths,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ignored != 25 || len(res.IgnoredItems) != strmTreeIgnoredItemSampleLimit {
		t.Fatalf("ignored = %d samples = %d, want 25/%d", res.Ignored, len(res.IgnoredItems), strmTreeIgnoredItemSampleLimit)
	}
}

func TestGenerateSTRMFromTreeDryRunDoesNotWriteOrCleanup(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	stale := filepath.Join(outDir, "Movies", "Old.Movie.strm")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		Paths:     []string{"/Movies/New.Movie.2026.mkv"},
		OutputDir: outDir,
		Cleanup:   true,
		DryRun:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Previewed != 1 || res.Generated != 0 || res.Updated != 0 || res.Cleaned != 0 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want one preview and no writes", res)
	}
	if len(res.Items) != 1 || res.Items[0].Action != "preview" || res.Items[0].Reason != "generated" {
		t.Fatalf("preview item = %#v, want generated preview", res.Items)
	}
	if _, err := os.Stat(filepath.Join(outDir, "Movies", "New.Movie.2026.strm")); !os.IsNotExist(err) {
		t.Fatalf("dry run should not write new strm, stat err=%v", err)
	}
	if got := readSTRM(t, stale); got != "old" {
		t.Fatalf("dry run cleanup touched stale file: %q", got)
	}
}

func TestGenerateSTRMFromTreeDryRunDoesNotCreateOutputDir(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "missing-strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		Paths:     []string{"/Movies/New.Movie.2026.mkv"},
		OutputDir: outDir,
		DryRun:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Previewed != 1 {
		t.Fatalf("previewed = %d, want 1", res.Previewed)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("dry run should not create output dir, stat err=%v", err)
	}
}

func TestGenerateSTRMFromTreeBatchLimitContinuesAfterExistingFiles(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)
	opts := GenerateSTRMTreeOptions{
		Provider:   "openlist",
		Paths:      []string{"/Movies/A.mkv", "/Movies/B.mkv", "/Movies/C.mkv"},
		OutputDir:  outDir,
		BatchLimit: 2,
	}

	first, err := svc.GenerateFromTree(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.Generated != 2 || first.Skipped != 0 {
		t.Fatalf("first batch = %#v, want two generated", first)
	}
	if first.Total != 3 || first.Remaining != 1 || !first.BatchLimited {
		t.Fatalf("first batch progress = total %d remaining %d limited %v, want 3/1/true", first.Total, first.Remaining, first.BatchLimited)
	}
	if _, err := os.Stat(filepath.Join(outDir, "Movies", "C.strm")); !os.IsNotExist(err) {
		t.Fatalf("first batch should not write third item, stat err=%v", err)
	}

	second, err := svc.GenerateFromTree(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Generated != 1 || second.Skipped != 2 {
		t.Fatalf("second batch = %#v, want two existing skips then next generated", second)
	}
	if second.Total != 3 || second.Remaining != 0 || second.BatchLimited {
		t.Fatalf("second batch progress = total %d remaining %d limited %v, want 3/0/false", second.Total, second.Remaining, second.BatchLimited)
	}
	if got := readSTRM(t, filepath.Join(outDir, "Movies", "C.strm")); !strings.Contains(got, "C.mkv") {
		t.Fatalf("second batch C.strm = %q, want generated third item", got)
	}
}

func TestGenerateSTRMFromTreeBatchLimitSkipsCleanup(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	stale := filepath.Join(outDir, "Movies", "stale.strm")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:   "openlist",
		Paths:      []string{"/Movies/A.mkv", "/Movies/B.mkv"},
		OutputDir:  outDir,
		BatchLimit: 1,
		Cleanup:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || res.Cleaned != 0 {
		t.Fatalf("batch result = %#v, want one generated and no cleanup", res)
	}
	if got := readSTRM(t, stale); got != "keep" {
		t.Fatalf("batch cleanup should not touch stale file: %q", got)
	}
}
