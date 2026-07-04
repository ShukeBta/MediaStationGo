package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadLocalMovieMetadata(t *testing.T) {
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "Inception.2010.mkv")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	nfo := `<?xml version="1.0" encoding="UTF-8"?>
<movie>
  <title>盗梦空间</title>
  <originaltitle>Inception</originaltitle>
  <year>2010</year>
  <plot>梦境盗窃。</plot>
  <rating>8.8</rating>
  <uniqueid type="tmdb">27205</uniqueid>
  <genre>科幻</genre>
  <genre>动作</genre>
</movie>`
	if err := os.WriteFile(nfoPath(mediaPath), []byte(nfo), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Title != "盗梦空间" || got.OriginalName != "Inception" || got.Year != 2010 || got.TMDbID != 27205 {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if got.Genres != "科幻,动作" {
		t.Fatalf("genres = %q", got.Genres)
	}
}

func TestReadLocalEpisodeMetadataMergesShowAndEpisode(t *testing.T) {
	root := t.TempDir()
	showDir := filepath.Join(root, "Show")
	seasonDir := filepath.Join(showDir, "Season 02")
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(seasonDir, "Show - EP03.mkv")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(`<tvshow><title>正确剧名</title><year>2024</year><tmdbid>123</tmdbid></tvshow>`), 0o644); err != nil {
		t.Fatal(err)
	}
	// 单集 NFO 携带【单集级】tmdb id(4375419)与单集名(第三集):二者都不得
	// 覆盖整剧字段,否则同剧各集 id/原名互不相同会被拆成多张卡。
	if err := os.WriteFile(nfoPath(mediaPath), []byte(`<episodedetails><title>第三集</title><season>2</season><episode>3</episode><plot>本集简介</plot><uniqueid type="tmdb">4375419</uniqueid></episodedetails>`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Title != "正确剧名" || got.SeasonNum != 2 || got.EpisodeNum != 3 {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	// 单集名不得写入整剧原名(分组键)。
	if got.OriginalName != "" {
		t.Fatalf("episode title must not pollute OriginalName, got %q", got.OriginalName)
	}
	if got.EpisodeTitle != "第三集" {
		t.Fatalf("episode title metadata = %q, want 第三集", got.EpisodeTitle)
	}
	// 单集级简介按集回填;整剧 tmdb 仍取 tvshow.nfo 的 123,单集 id 不得覆盖。
	if got.Overview != "本集简介" || got.TMDbID != 123 {
		t.Fatalf("episode/show merge failed: %+v", got)
	}
}

func TestReadLocalEpisodeMetadataWithoutShowTitleDoesNotUseEpisodeTitleAsSeries(t *testing.T) {
	root := t.TempDir()
	showDir := filepath.Join(root, "哈哈哈哈哈")
	seasonDir := filepath.Join(showDir, "Season 06")
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(seasonDir, "哈哈哈哈哈 - S06E11.mkv")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nfoPath(mediaPath), []byte(`<episodedetails><title>第 11 集</title><season>6</season><episode>11</episode><uniqueid type="tmdb">4375419</uniqueid></episodedetails>`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("metadata is nil")
	}
	if got.Title != "" {
		t.Fatalf("episode title must not become series title, got %q", got.Title)
	}
	if got.EpisodeTitle != "第 11 集" || got.SeasonNum != 6 || got.EpisodeNum != 11 {
		t.Fatalf("episode metadata not preserved: %+v", got)
	}
	if got.TMDbID != 0 {
		t.Fatalf("episode-level tmdb id must not become series tmdb id, got %d", got.TMDbID)
	}
}

func TestMergeEpisodeMetadataKeepsExistingSeasonWhenEpisodeNFOOmitsIt(t *testing.T) {
	dst := &LocalMetadata{Title: "哈哈哈哈哈", SeasonNum: 6}
	episodeDoc := &nfoDocument{Title: "第 11 集", Episode: 11}
	episode := metadataFromDoc(episodeDoc, "", true)

	mergeEpisodeMetadata(dst, episode, episodeDoc)

	if dst.SeasonNum != 6 || dst.EpisodeNum != 11 {
		t.Fatalf("episode NFO without season should keep parsed season, got %+v", dst)
	}
}

func TestReadLocalEpisodeMetadataIgnoresNoneNumericFields(t *testing.T) {
	root := t.TempDir()
	showDir := filepath.Join(root, "链锯人 总集篇 (2025)")
	seasonDir := filepath.Join(showDir, "Season 1")
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(seasonDir, "链锯人 总集篇 - S01E01.mkv")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(`<tvshow><title>链锯人 总集篇</title><year>2025</year></tvshow>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nfoPath(mediaPath), []byte(`<episodedetails><title>第 1 集</title><season>None</season><episode>None</episode><rating>None</rating></episodedetails>`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Title != "链锯人 总集篇" || got.Year != 2025 {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if got.SeasonNum != 0 || got.EpisodeNum != 0 || got.Rating != 0 {
		t.Fatalf("None numeric fields should parse as zero, got %+v", got)
	}
}

func TestReadLocalVarietyMetadataUsesLocalArtwork(t *testing.T) {
	root := t.TempDir()
	showDir := filepath.Join(root, "哈哈哈哈哈")
	seasonDir := filepath.Join(showDir, "Season 06")
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(seasonDir, "哈哈哈哈哈 - S06E17.mkv")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(showDir, "哈哈哈哈哈.nfo"), []byte(`<tvshow><title>哈哈哈哈哈</title><genre>综艺</genre></tvshow>`), 0o644); err != nil {
		t.Fatal(err)
	}
	showPoster := filepath.Join(showDir, "poster.jpg")
	if err := os.WriteFile(showPoster, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	episodeThumb := filepath.Join(seasonDir, "哈哈哈哈哈 - S06E17-thumb.jpg")
	if err := os.WriteFile(episodeThumb, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	backdrop := filepath.Join(showDir, "fanart.jpg")
	if err := os.WriteFile(backdrop, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("metadata is nil")
	}
	if got.Title != "哈哈哈哈哈" || got.Genres != "综艺" {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if got.PosterURL != showPoster {
		t.Fatalf("PosterURL = %q, want show poster %q, not episode thumb %q", got.PosterURL, showPoster, episodeThumb)
	}
	if got.BackdropURL != backdrop {
		t.Fatalf("BackdropURL = %q, want %q", got.BackdropURL, backdrop)
	}
}

func TestReadLocalMetadataPrioritizesPosterOverThumbAndStills(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	thumb := filepath.Join(root, "Movie-thumb.jpg")
	if err := os.WriteFile(thumb, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	still := filepath.Join(root, "Movie-still.jpg")
	if err := os.WriteFile(still, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(root, "poster.jpg")
	if err := os.WriteFile(poster, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PosterURL != poster {
		t.Fatalf("PosterURL = %q, want poster %q", got.PosterURL, poster)
	}
}

func TestReadLocalMetadataIgnoresActorAndStillArtworkOnly(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "SSIS-001.mp4")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"actor.jpg", "sample.jpg", "fanart.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("jpg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ReadLocalMetadata(mediaPath, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PosterURL != "" || got.BackdropURL == "" {
		t.Fatalf("expected backdrop-only metadata without actor/still poster, got %+v", got)
	}
}

func TestReadLocalMetadataWithoutNFOStillFindsArtwork(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(root, "Movie-poster.jpg")
	if err := os.WriteFile(poster, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PosterURL != poster {
		t.Fatalf("unexpected artwork metadata: %+v", got)
	}
}
