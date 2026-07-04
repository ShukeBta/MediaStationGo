package service

import (
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestGenerateSTRMFromTreeTextPreservesRootTree(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)
	tree := strings.Join([]string{
		"电视剧",
		"├── 国产剧",
		"│   └── 南部档案",
		"│       ├── Archives.S01E01.mkv",
		"│       └── Archives.S01E01.nfo",
	}, "\n")

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		TreeText:  tree,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want one generated video", res)
	}
	got := readSTRM(t, filepath.Join(outDir, "电视剧", "国产剧", "南部档案", "Archives.S01E01.strm"))
	if got != "/api/cloud/play/openlist?ref=%2F%E7%94%B5%E8%A7%86%E5%89%A7%2F%E5%9B%BD%E4%BA%A7%E5%89%A7%2F%E5%8D%97%E9%83%A8%E6%A1%A3%E6%A1%88%2FArchives.S01E01.mkv" {
		t.Fatalf("strm url = %q", got)
	}
}

func TestGenerateSTRMFromTreeTextSupportsSingleLineTreeMarkers(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)
	tree := strings.Join([]string{
		"动漫",
		"├─ 国漫",
		"│  └─ 凡人修仙传",
		"│     └─ Season 01",
		"│        └─ Mortal.Journey.S01E01.mp4",
	}, "\n")

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "115",
		TreeText:  tree,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want one generated video", res)
	}
	got := readSTRM(t, filepath.Join(outDir, "动漫", "国漫", "凡人修仙传", "Season 01", "Mortal.Journey.S01E01.strm"))
	if !strings.Contains(got, "/api/cloud/play/cloud115?") {
		t.Fatalf("strm url = %q, want cloud115 play url", got)
	}
}

func TestGenerateSTRMFromTreeTextSupportsConnectorURLSources(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)
	tree := strings.Join([]string{
		"电影",
		"├── https://media.example.com/api/cloud/play/openlist?ref=%2FMovies%2FLinked.Movie.2026.mkv",
		"└── cloud://openlist/%E7%94%B5%E5%BD%B1/%E5%88%AB%E5%90%8D/Cloud.Query.2026.mkv?dir=%2Factual%2Fcloud%2FCloud.Query.2026.mkv",
	}, "\n")

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "115",
		TreeText:  tree,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 2 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want two generated videos from connector URL rows", res)
	}
	linked := readSTRM(t, filepath.Join(outDir, "Movies", "Linked.Movie.2026.strm"))
	if !strings.Contains(linked, "/api/cloud/play/openlist?") || !strings.Contains(linked, "ref=%2FMovies%2FLinked.Movie.2026.mkv") {
		t.Fatalf("cloud play connector url = %q, want preserved provider/ref", linked)
	}
	cloud := readSTRM(t, filepath.Join(outDir, "电影", "别名", "Cloud.Query.2026.strm"))
	if !strings.Contains(cloud, "/api/cloud/play/openlist?") || !strings.Contains(cloud, "ref=%2Factual%2Fcloud%2FCloud.Query.2026.mkv") {
		t.Fatalf("cloud mount connector url = %q, want display path output and scan dir ref", cloud)
	}
}

func TestGenerateSTRMFromTreeTextSupportsPlainIndentedTree(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)
	tree := strings.Join([]string{
		"电视剧",
		"  欧美剧",
		"    House of the Dragon",
		"      Season 03",
		"        House.of.the.Dragon.S03E01.mkv",
		"        House.of.the.Dragon.S03E02.mkv",
		"    The Last of Us",
		"      Season 02",
		"        The.Last.of.Us.S02E01.mkv",
	}, "\n")

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		TreeText:  tree,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 3 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want three generated videos from plain indented tree", res)
	}
	got := readSTRM(t, filepath.Join(outDir, "电视剧", "欧美剧", "House of the Dragon", "Season 03", "House.of.the.Dragon.S03E01.strm"))
	if !strings.Contains(got, "House.of.the.Dragon.S03E01.mkv") {
		t.Fatalf("strm url = %q, want first plain-indented source ref", got)
	}
	got = readSTRM(t, filepath.Join(outDir, "电视剧", "欧美剧", "The Last of Us", "Season 02", "The.Last.of.Us.S02E01.strm"))
	if !strings.Contains(got, "The.Last.of.Us.S02E01.mkv") {
		t.Fatalf("strm url = %q, want sibling folder source ref", got)
	}
}

func TestGenerateSTRMFromTreeTextSupportsWindowsTreeFileRows(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)
	tree := strings.Join([]string{
		"电视剧",
		"├─欧美剧",
		"│  ├─House of the Dragon",
		"│  │      House.of.the.Dragon.S03E01.mkv",
		"│  │      House.of.the.Dragon.S03E02.mkv",
		"│  └─The Last of Us",
		"│          The.Last.of.Us.S02E01.mkv",
	}, "\n")

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		TreeText:  tree,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 3 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want three generated videos from Windows tree rows", res)
	}
	got := readSTRM(t, filepath.Join(outDir, "电视剧", "欧美剧", "House of the Dragon", "House.of.the.Dragon.S03E02.strm"))
	if !strings.Contains(got, "House.of.the.Dragon.S03E02.mkv") {
		t.Fatalf("strm url = %q, want vertical-prefix file row ref", got)
	}
	got = readSTRM(t, filepath.Join(outDir, "电视剧", "欧美剧", "The Last of Us", "The.Last.of.Us.S02E01.strm"))
	if !strings.Contains(got, "The.Last.of.Us.S02E01.mkv") {
		t.Fatalf("strm url = %q, want blank-prefix sibling file row ref", got)
	}
}

func TestGenerateSTRMFromTreeStripsDecoratedTreeNames(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)
	tree := strings.Join([]string{
		"📁 电视剧",
		"├── [目录] 欧美剧",
		"│   └── (folder) House of the Dragon",
		"│   │   📄 House.of.the.Dragon.S03E01.mkv",
	}, "\n")

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:   "openlist",
		TreeText:   tree,
		Paths:      []string{"/[目录] 动漫/[folder] 日番/[文件] Frieren.S01E01.mp4"},
		SourceRoot: "/动漫",
		OutputDir:  outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 2 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want two generated videos with decorations stripped", res)
	}
	showPath := filepath.Join(outDir, "电视剧", "欧美剧", "House of the Dragon", "House.of.the.Dragon.S03E01.strm")
	show := readSTRM(t, showPath)
	if strings.Contains(showPath, "目录") || strings.Contains(showPath, "folder") || strings.Contains(showPath, "📄") {
		t.Fatalf("decorated local path was not cleaned: %q", showPath)
	}
	if !strings.Contains(show, "House.of.the.Dragon.S03E01.mkv") || strings.Contains(show, "%5B") || strings.Contains(show, "%F0%9F") {
		t.Fatalf("decorated tree ref was not cleaned: %q", show)
	}
	episode := readSTRM(t, filepath.Join(outDir, "日番", "Frieren.S01E01.strm"))
	if !strings.Contains(episode, "ref=%2F%E5%8A%A8%E6%BC%AB%2F%E6%97%A5%E7%95%AA%2FFrieren.S01E01.mp4") {
		t.Fatalf("decorated direct path ref was not cleaned: %q", episode)
	}
}

func TestGenerateSTRMFromTreeStripsExportedFileMetadata(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)
	tree := strings.Join([]string{
		"电影",
		"└── 欧美电影",
		"    └── Dune.Part.Two.2024.2160p.WEB-DL.mkv 18.6 GB 2024-04-01 12:30",
	}, "\n")

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:   "openlist",
		TreeText:   tree,
		Paths:      []string{"/电视剧/欧美剧/Show/Season 01/Show.S01E01.mp4 (2.1 GB)"},
		SourceRoot: "/电视剧",
		OutputDir:  outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 2 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want two generated videos with metadata suffix stripped", res)
	}
	movie := readSTRM(t, filepath.Join(outDir, "电影", "欧美电影", "Dune.Part.Two.2024.2160p.WEB-DL.strm"))
	if !strings.Contains(movie, "Dune.Part.Two.2024.2160p.WEB-DL.mkv") || strings.Contains(movie, "18.6") {
		t.Fatalf("movie strm url = %q, want clean media ref without size metadata", movie)
	}
	episode := readSTRM(t, filepath.Join(outDir, "欧美剧", "Show", "Season 01", "Show.S01E01.strm"))
	if !strings.Contains(episode, "Show.S01E01.mp4") || strings.Contains(episode, "2.1") {
		t.Fatalf("episode strm url = %q, want clean media ref without size metadata", episode)
	}
}
