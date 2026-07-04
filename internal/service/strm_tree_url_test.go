package service

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestGenerateSTRMFromTreeUsesMediaPathFromURLQuery(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider: "openlist",
		Paths: []string{
			"https://openlist.example.com/api/fs/get?path=%2FMovies%2FDune.Part.Two.2024.mkv",
			"https://openlist.example.com/api/fs/get?path=%2FMovies%2FA%2BB.2026.mkv",
			"https://openlist.example.com/api/fs/get?path=/Movies/A+B.Raw.2026.mkv",
			"https://openlist.example.com/api/raw?ref=/Shows/Some.Show/S01E01.mp4",
			"https://cdn.example.com/media/Movies/Direct.Movie.2026.mkv?token=secret",
			"https://cdn.example.com/media/Movies/A+B.Direct.2026.mkv?token=secret",
			"https://openlist.example.com/api/fs/get?id=12345",
		},
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 6 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want six generated videos and non-media API URL ignored", res)
	}
	movie := readSTRM(t, filepath.Join(outDir, "Movies", "Dune.Part.Two.2024.strm"))
	if !strings.Contains(movie, "ref=%2FMovies%2FDune.Part.Two.2024.mkv") || strings.Contains(movie, "api%2Ffs%2Fget") {
		t.Fatalf("movie strm url = %q, want query media path as cloud ref", movie)
	}
	plusMovie := readSTRM(t, filepath.Join(outDir, "Movies", "A+B.2026.strm"))
	if !strings.Contains(plusMovie, "ref=%2FMovies%2FA%2BB.2026.mkv") || strings.Contains(plusMovie, "A+B.2026.mkv") {
		t.Fatalf("plus movie strm url = %q, want literal plus preserved and encoded in ref", plusMovie)
	}
	rawPlusMovie := readSTRM(t, filepath.Join(outDir, "Movies", "A+B.Raw.2026.strm"))
	if !strings.Contains(rawPlusMovie, "ref=%2FMovies%2FA%2BB.Raw.2026.mkv") || strings.Contains(rawPlusMovie, "A+B.Raw.2026.mkv") {
		t.Fatalf("raw plus movie strm url = %q, want raw query plus preserved and encoded in ref", rawPlusMovie)
	}
	show := readSTRM(t, filepath.Join(outDir, "Shows", "Some.Show", "S01E01.strm"))
	if !strings.Contains(show, "ref=%2FShows%2FSome.Show%2FS01E01.mp4") {
		t.Fatalf("show strm url = %q, want ref query media path", show)
	}
	direct := readSTRM(t, filepath.Join(outDir, "media", "Movies", "Direct.Movie.2026.strm"))
	if !strings.Contains(direct, "ref=%2Fmedia%2FMovies%2FDirect.Movie.2026.mkv") {
		t.Fatalf("direct url strm = %q, want normal URL path media ref", direct)
	}
	plusDirect := readSTRM(t, filepath.Join(outDir, "media", "Movies", "A+B.Direct.2026.strm"))
	if !strings.Contains(plusDirect, "ref=%2Fmedia%2FMovies%2FA%2BB.Direct.2026.mkv") {
		t.Fatalf("plus direct url strm = %q, want URL path plus preserved", plusDirect)
	}
	if _, err := os.Stat(filepath.Join(outDir, "api", "fs", "get.strm")); !os.IsNotExist(err) {
		t.Fatalf("non-media API URL should not generate STRM, stat err=%v", err)
	}
}

func TestGenerateSTRMFromTreePreservesSourceProvider(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider: "115",
		Paths: []string{
			"cloud://openlist/Movies/OpenList.Movie.2026.mkv",
			"/api/cloud/play/cloud115?ref=%2FShows%2FCloud115.Show.S01E01.mkv",
			"/api/cloud/play/openlist?ref=%2FMovies%2FMy+Space.Movie.2026.mkv",
			"https://media.example.com/api/cloud/play/openlist?ref=%2FMovies%2FRemote.OpenList.Movie.2026.mkv",
			"/Movies/Fallback.115.Movie.2026.mkv",
		},
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 5 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want five generated videos", res)
	}
	openlist := readSTRM(t, filepath.Join(outDir, "Movies", "OpenList.Movie.2026.strm"))
	if !strings.Contains(openlist, "/api/cloud/play/openlist?") || !strings.Contains(openlist, "ref=%2FMovies%2FOpenList.Movie.2026.mkv") {
		t.Fatalf("cloud:// source url = %q, want openlist provider and original ref", openlist)
	}
	cloud115 := readSTRM(t, filepath.Join(outDir, "Shows", "Cloud115.Show.S01E01.strm"))
	if !strings.Contains(cloud115, "/api/cloud/play/cloud115?") || !strings.Contains(cloud115, "ref=%2FShows%2FCloud115.Show.S01E01.mkv") {
		t.Fatalf("cloud play source url = %q, want cloud115 provider preserved", cloud115)
	}
	spaceMoviePath := filepath.Join(outDir, "Movies", "My Space.Movie.2026.strm")
	spaceMovie := readSTRM(t, spaceMoviePath)
	if _, err := os.Stat(filepath.Join(outDir, "Movies", "My+Space.Movie.2026.strm")); !os.IsNotExist(err) {
		t.Fatalf("cloud play source should not create literal-plus local path, stat err=%v", err)
	}
	spaceURL, err := url.Parse(spaceMovie)
	if err != nil {
		t.Fatalf("parse cloud play source url %q: %v", spaceMovie, err)
	}
	if got := spaceURL.Query().Get("ref"); got != "/Movies/My Space.Movie.2026.mkv" {
		t.Fatalf("cloud play source ref = %q, want decoded space path", got)
	}
	remoteOpenlist := readSTRM(t, filepath.Join(outDir, "Movies", "Remote.OpenList.Movie.2026.strm"))
	if !strings.Contains(remoteOpenlist, "/api/cloud/play/openlist?") || !strings.Contains(remoteOpenlist, "ref=%2FMovies%2FRemote.OpenList.Movie.2026.mkv") {
		t.Fatalf("absolute cloud play source url = %q, want openlist provider preserved", remoteOpenlist)
	}
	fallback := readSTRM(t, filepath.Join(outDir, "Movies", "Fallback.115.Movie.2026.strm"))
	if !strings.Contains(fallback, "/api/cloud/play/cloud115?") || !strings.Contains(fallback, "ref=%2FMovies%2FFallback.115.Movie.2026.mkv") {
		t.Fatalf("plain source url = %q, want fallback cloud115 provider", fallback)
	}
}

func TestGenerateSTRMFromTreeCloudMountUsesScanDirForRef(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider: "115",
		Paths: []string{
			"cloud://openlist/%E7%94%B5%E5%BD%B1/%E5%88%AB%E5%90%8D%E7%9B%AE%E5%BD%95/Alias.Movie.2026.mkv?dir=%2Factual%2Fcloud%2Fmovies%2FAlias.Movie.2026.mkv",
		},
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want one generated video", res)
	}
	got := readSTRM(t, filepath.Join(outDir, "电影", "别名目录", "Alias.Movie.2026.strm"))
	if !strings.Contains(got, "/api/cloud/play/openlist?") {
		t.Fatalf("strm url = %q, want provider from cloud mount", got)
	}
	if !strings.Contains(got, "ref=%2Factual%2Fcloud%2Fmovies%2FAlias.Movie.2026.mkv") {
		t.Fatalf("strm url = %q, want dir scan path as playable ref", got)
	}
	if strings.Contains(got, "%E5%88%AB%E5%90%8D%E7%9B%AE%E5%BD%95") {
		t.Fatalf("strm url = %q, display path leaked into playable ref", got)
	}
}

func TestGenerateSTRMFromTreeDoesNotDedupeDifferentProviders(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "115",
		Paths:     []string{"cloud://openlist/Movies/Same.Movie.mkv", "cloud://cloud115/Movies/Same.Movie.mkv"},
		OutputDir: outDir,
		Overwrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || res.Updated != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want both provider-specific sources processed", res)
	}
	got := readSTRM(t, filepath.Join(outDir, "Movies", "Same.Movie.strm"))
	if !strings.Contains(got, "/api/cloud/play/cloud115?") || !strings.Contains(got, "ref=%2FMovies%2FSame.Movie.mkv") {
		t.Fatalf("final strm url = %q, want second provider write to prove it was not deduped", got)
	}
}

func TestGenerateSTRMFromTreeMissingOnlySkipsMediaAlreadyInLibrary(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	rows := []model.Media{
		{
			Base:    model.Base{ID: "existing-strm"},
			Title:   "Existing STRM",
			Path:    "cloud://openlist/Movies/Existing.Path.Copy.mkv",
			STRMURL: "/api/cloud/play/openlist?ref=%2FMovies%2FExisting.Movie.mkv",
		},
		{
			Base:  model.Base{ID: "existing-cloud-path"},
			Title: "Existing Cloud Path",
			Path:  "cloud://cloud115/Shows/Existing.Show.S01E01.mkv",
		},
		{
			Base:  model.Base{ID: "different-provider"},
			Title: "Different Provider",
			Path:  "cloud://cloud115/Shows/Same.Provider.mkv",
		},
	}
	for i := range rows {
		if err := repos.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), repos, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider: "openlist",
		Paths: []string{
			"cloud://openlist/Movies/Existing.Movie.mkv",
			"cloud://openlist/Movies/New.Movie.mkv",
			"cloud://cloud115/Shows/Existing.Show.S01E01.mkv",
			"cloud://openlist/Shows/Same.Provider.mkv",
		},
		OutputDir:   outDir,
		MissingOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 2 || res.Skipped != 2 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want generated=2 skipped=2", res)
	}
	if _, err := os.Stat(filepath.Join(outDir, "Movies", "Existing.Movie.strm")); !os.IsNotExist(err) {
		t.Fatalf("existing media should not generate STRM, stat err=%v", err)
	}
	assertFileContains(t, filepath.Join(outDir, "Movies", "New.Movie.strm"), "/api/cloud/play/openlist?ref=%2FMovies%2FNew.Movie.mkv")
	assertFileContains(t, filepath.Join(outDir, "Shows", "Same.Provider.strm"), "/api/cloud/play/openlist?ref=%2FShows%2FSame.Provider.mkv")
	for _, item := range res.Items {
		if item.Title == "Existing.Movie" && item.Reason != "already in media library" {
			t.Fatalf("existing media skip reason = %q", item.Reason)
		}
	}
}

func TestGenerateSTRMFromTreeStripsPathListPrefixes(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)
	tree := strings.Join([]string{
		"- /Movies/Dune.Part.Two.2024.mkv",
		"* https://openlist.example.com/api/fs/get?path=%2FShows%2FSome.Show%2FS01E01.mp4",
		"1. /Anime/Frieren/Frieren.S01E01.mp4",
		"2) /Anime/Frieren/Frieren.S01E02.mp4",
		"• /Documentaries/Earth.2026.mkv",
	}, "\n")

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		TreeText:  tree,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 5 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want five generated videos with list prefixes stripped", res)
	}
	moviePath := filepath.Join(outDir, "Movies", "Dune.Part.Two.2024.strm")
	movie := readSTRM(t, moviePath)
	if strings.Contains(moviePath, "- ") || !strings.Contains(movie, "ref=%2FMovies%2FDune.Part.Two.2024.mkv") {
		t.Fatalf("markdown bullet prefix leaked into movie path/ref: path=%q url=%q", moviePath, movie)
	}
	show := readSTRM(t, filepath.Join(outDir, "Shows", "Some.Show", "S01E01.strm"))
	if strings.Contains(show, "api%2Ffs%2Fget") || !strings.Contains(show, "ref=%2FShows%2FSome.Show%2FS01E01.mp4") {
		t.Fatalf("bullet URL query source was not cleaned: %q", show)
	}
	if _, err := os.Stat(filepath.Join(outDir, "1. ", "Anime", "Frieren", "Frieren.S01E01.strm")); !os.IsNotExist(err) {
		t.Fatalf("numbered prefix should not create a literal prefix directory, stat err=%v", err)
	}
	if got := readSTRM(t, filepath.Join(outDir, "Documentaries", "Earth.2026.strm")); !strings.Contains(got, "ref=%2FDocumentaries%2FEarth.2026.mkv") {
		t.Fatalf("round bullet source ref = %q", got)
	}
}

func TestGenerateSTRMFromTreeOutputPrefixOnlyAffectsLocalPath(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:     "openlist",
		Paths:        []string{"/cloud/Dune.Part.Two.2024.mkv"},
		SourceRoot:   "/cloud",
		OutputPrefix: "电影/欧美电影",
		OutputDir:    outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want one generated video", res)
	}
	got := readSTRM(t, filepath.Join(outDir, "电影", "欧美电影", "Dune.Part.Two.2024.strm"))
	if strings.Contains(got, "%E7%94%B5%E5%BD%B1") || strings.Contains(got, "%E6%AC%A7%E7%BE%8E%E7%94%B5%E5%BD%B1") {
		t.Fatalf("strm url = %q, output prefix should not be injected into cloud ref", got)
	}
	if !strings.Contains(got, "ref=%2Fcloud%2FDune.Part.Two.2024.mkv") {
		t.Fatalf("strm url = %q, want original cloud ref", got)
	}
}

func TestGenerateSTRMFromTreeRejectsUnsafeOutputPrefix(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:     "openlist",
		Paths:        []string{"Movies/Movie.mkv"},
		OutputPrefix: "../escape",
		OutputDir:    outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 0 || len(res.Errors) != 1 {
		t.Fatalf("result = %#v, want unsafe prefix rejected", res)
	}
	if _, err := os.Stat(filepath.Join(outDir, "..", "escape", "Movies", "Movie.strm")); !os.IsNotExist(err) {
		t.Fatalf("unsafe prefixed strm should not be written, stat err=%v", err)
	}
}
