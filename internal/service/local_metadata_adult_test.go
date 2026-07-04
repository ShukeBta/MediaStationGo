package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAdultLocalMetadataAndArtwork(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "SSIS-001")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(dir, "SSIS-001.mp4")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	nfo := `<?xml version="1.0" encoding="UTF-8"?>
<movie>
  <title>成人影片标题</title>
  <originaltitle>SSIS-001</originaltitle>
  <num>SSIS-001</num>
  <releasedate>2024-05-01</releasedate>
  <plot>本地简介</plot>
  <poster>SSIS-001-poster.jpg</poster>
  <fanart><thumb>SSIS-001-fanart.jpg</thumb></fanart>
  <studio>测试片商</studio>
  <genre>剧情</genre>
  <tag>中文字幕</tag>
  <actor><name>演员A</name></actor>
</movie>`
	if err := os.WriteFile(nfoPath(mediaPath), []byte(nfo), 0o644); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(dir, "SSIS-001-poster.jpg")
	if err := os.WriteFile(poster, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	fanart := filepath.Join(dir, "SSIS-001-fanart.jpg")
	if err := os.WriteFile(fanart, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Title != "成人影片标题" || got.AdultCode != "SSIS-001" || !got.NSFW {
		t.Fatalf("unexpected adult metadata: %+v", got)
	}
	if got.OriginalName != "SSIS-001" || got.Year != 2024 || got.Overview != "本地简介" {
		t.Fatalf("unexpected adult fields: %+v", got)
	}
	if got.PosterURL != poster || got.BackdropURL != fanart {
		t.Fatalf("artwork poster=%q fanart=%q", got.PosterURL, got.BackdropURL)
	}
	if got.Genres != "剧情,中文字幕,测试片商,演员A" {
		t.Fatalf("genres = %q", got.Genres)
	}
}

func TestReadAdultMovieNFOFallbackInSingleMovieFolder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "MIDV-123")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "movie.nfo"), []byte(`<movie><title>本地番号电影</title><num>MIDV-123</num></movie>`), 0o644); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(dir, "video-cover.jpg")
	if err := os.WriteFile(poster, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Title != "本地番号电影" || got.AdultCode != "MIDV-123" || got.PosterURL != poster {
		t.Fatalf("unexpected fallback metadata: %+v", got)
	}
}

func TestReadAdultNFOByCodeForStackedFile(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "SSIS-001-C.mp4")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SSIS-001.nfo"), []byte(`<movie>
  <title>按番号命中的本地 NFO</title>
  <originaltitle>SSIS-001</originaltitle>
  <art><poster>SSIS-001-poster.jpg</poster><fanart>SSIS-001-fanart.jpg</fanart></art>
</movie>`), 0o644); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(root, "SSIS-001-poster.jpg")
	fanart := filepath.Join(root, "SSIS-001-fanart.jpg")
	if err := os.WriteFile(poster, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fanart, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Title != "按番号命中的本地 NFO" || got.AdultCode != "SSIS-001" || !got.HasNFO {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if got.PosterURL != poster || got.BackdropURL != fanart {
		t.Fatalf("artwork poster=%q fanart=%q", got.PosterURL, got.BackdropURL)
	}
}

func TestReadAdultMetadataPrefersLocalDMMPosterOverRemoteNFO(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "IPX-641-C")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(dir, "ipx-641-C.mp4")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nfoPath(mediaPath), []byte(`<movie>
  <title>本地标题</title>
  <originaltitle>IPX-641</originaltitle>
  <thumb>https://www.javbus.com/pics/cover/remote.jpg</thumb>
  <fanart>https://pics.dmm.co.jp/digital/video/ipx00641/ipx00641jp-1.jpg</fanart>
</movie>`), 0o644); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(dir, "ipx00641pl.jpg")
	if err := os.WriteFile(poster, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PosterURL != poster {
		t.Fatalf("poster_url = %q, want local %q", got.PosterURL, poster)
	}
	if got.BackdropURL != "https://pics.dmm.co.jp/digital/video/ipx00641/ipx00641jp-1.jpg" {
		t.Fatalf("backdrop should keep remote NFO fallback, got %q", got.BackdropURL)
	}
}

func TestReadAdultMetadataDerivesDMMPosterFromRemoteFanart(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "NACR-833")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(dir, "NACR-833.mp4")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nfoPath(mediaPath), []byte(`<movie>
  <title>本地标题</title>
  <originaltitle>NACR-833</originaltitle>
  <thumb>https://www.javbus.com/pics/cover/an5p_b.jpg</thumb>
  <fanart>https://pics.dmm.co.jp/digital/video/h_237nacr00833/h_237nacr00833jp-1.jpg</fanart>
</movie>`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PosterURL != "https://pics.dmm.co.jp/digital/video/h_237nacr00833/h_237nacr00833pl.jpg" {
		t.Fatalf("unexpected metadata: %+v", got)
	}
}

func TestReadAdultArtworkByCodeWithoutNFO(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "SSIS-001-CD1.mp4")
	if err := os.WriteFile(mediaPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(root, "SSIS-001-poster.jpg")
	if err := os.WriteFile(poster, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadLocalMetadata(mediaPath, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PosterURL != poster || !got.HasArtwork || got.HasNFO {
		t.Fatalf("unexpected artwork metadata: %+v", got)
	}
}
