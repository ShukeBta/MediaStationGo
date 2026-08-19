package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// writeTempVideoWithSubtitle creates a temp directory with a fake video and a
// same-name external subtitle, and inserts the media row into the test DB. It
// returns the media id. The subtitle content is valid ASS for .ass/.ssa and
// valid SRT for .srt.
func writeTempVideoWithSubtitle(t *testing.T, svc *EmbyService, lib *model.Library, container, subExt string) string {
	t.Helper()
	dir := t.TempDir()
	video := "MovieName" + container
	sub := "MovieName" + subExt
	if err := os.WriteFile(filepath.Join(dir, video), []byte("video"), 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	var subBody string
	switch strings.ToLower(subExt) {
	case ".srt":
		subBody = "1\n00:00:01,000 --> 00:00:02,000\nhello\n"
	default:
		subBody = "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,hello\n"
	}
	if err := os.WriteFile(filepath.Join(dir, sub), []byte(subBody), 0o644); err != nil {
		t.Fatalf("write subtitle: %v", err)
	}
	m := model.Media{
		LibraryID:  lib.ID,
		Title:      "MovieName",
		Path:       filepath.Join(dir, video),
		Container:  container,
		VideoCodec: "h264",
		AudioCodec: "aac",
	}
	if err := svc.repo.DB.Create(&m).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	return m.ID
}

func newTestSubtitleService(t *testing.T, svc *EmbyService) *SubtitleService {
	t.Helper()
	return NewSubtitleService(zap.NewNop(), svc.repo)
}

func TestEmbyMediaStreamsAttachSameNameSubtitle(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	mediaID := writeTempVideoWithSubtitle(t, svc, &lib, ".mp4", ".ass")
	svc.SetSubtitleService(newTestSubtitleService(t, svc))

	m, err := svc.repo.Media.FindByID(t.Context(), mediaID)
	if err != nil || m == nil {
		t.Fatalf("find media: %v", err)
	}
	streams := svc.mediaStreams(t.Context(), m)

	// Video (0) + Audio (1) + one Subtitle (2)
	if len(streams) != 3 {
		t.Fatalf("expected 3 streams (video/audio/subtitle), got %d: %#v", len(streams), streams)
	}
	sub := streams[2]
	if sub["Type"] != "Subtitle" {
		t.Fatalf("stream[2] type = %v, want Subtitle", sub["Type"])
	}
	if sub["Index"] != 2 {
		t.Fatalf("subtitle index = %v, want 2", sub["Index"])
	}
	// Source codec (ASS) must be reported, and IsDefault must be false to match
	// the official Emby external-subtitle contract.
	if sub["Codec"] != "ass" {
		t.Fatalf("subtitle codec = %v, want ass", sub["Codec"])
	}
	if sub["DeliveryFormat"] != "ass" {
		t.Fatalf("subtitle delivery format = %v, want ass", sub["DeliveryFormat"])
	}
	if sub["IsDefault"] != false {
		t.Fatalf("subtitle IsDefault = %v, want false", sub["IsDefault"])
	}
	if sub["DeliveryMethod"] != "External" {
		t.Fatalf("subtitle DeliveryMethod = %v, want External", sub["DeliveryMethod"])
	}
	if got, _ := sub["Path"].(string); got == "" {
		t.Fatalf("subtitle Path should not be empty: %#v", sub)
	}
	// Official Emby shape: /Videos/{Id}/{MediaSourceId}/Subtitles/{Index}/Stream.{Format}
	// (no "mediasource_" prefix; that prefix belongs to the Id value, not the route)
	wantURL := "/Videos/" + mediaID + "/" + mediaID + "/Subtitles/2/Stream.ass"
	if sub["DeliveryUrl"] != wantURL {
		t.Fatalf("DeliveryUrl = %v, want %v", sub["DeliveryUrl"], wantURL)
	}
	if sub["IsTextSubtitleStream"] != true {
		t.Fatalf("IsTextSubtitleStream = %v, want true", sub["IsTextSubtitleStream"])
	}
	if sub["SupportsExternalStream"] != true {
		t.Fatalf("SupportsExternalStream = %v, want true", sub["SupportsExternalStream"])
	}
}

func TestEmbyMediaStreamsSubtitleDisplayTitleFriendly(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	mediaID := writeTempVideoWithSubtitle(t, svc, &lib, ".mp4", ".ass")
	svc.SetSubtitleService(newTestSubtitleService(t, svc))

	m, err := svc.repo.Media.FindByID(t.Context(), mediaID)
	if err != nil || m == nil {
		t.Fatalf("find media: %v", err)
	}
	streams := svc.mediaStreams(t.Context(), m)
	sub := streams[2]
	// No language tag on the filename -> should fall back to "字幕 (ASS)",
	// matching official Emby's friendly label rather than a bare "und".
	if sub["DisplayTitle"] != "字幕 (ASS)" {
		t.Fatalf("DisplayTitle = %v, want 字幕 (ASS)", sub["DisplayTitle"])
	}
}

func TestEmbyServeSubtitleStreamServesRawSource(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	mediaID := writeTempVideoWithSubtitle(t, svc, &lib, ".mp4", ".ass")
	svc.SetSubtitleService(newTestSubtitleService(t, svc))

	var buf bytes.Buffer
	if err := svc.ServeSubtitleStream(t.Context(), &buf, mediaID, "2", ""); err != nil {
		t.Fatalf("serve subtitle: %v", err)
	}
	// Emby must get the RAW ASS source, NOT a WebVTT conversion.
	if bytes.Contains(buf.Bytes(), []byte("WEBVTT")) {
		t.Fatalf("served subtitle should be raw ASS, got WebVTT: %q", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("Dialogue:")) {
		t.Fatalf("served subtitle should contain raw ASS Dialogue lines: %q", buf.String())
	}
}

func TestEmbyServeSubtitleStreamBadIndexNotFound(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	mediaID := writeTempVideoWithSubtitle(t, svc, &lib, ".mp4", ".ass")
	svc.SetSubtitleService(newTestSubtitleService(t, svc))

	var buf bytes.Buffer
	if err := svc.ServeSubtitleStream(t.Context(), &buf, mediaID, "99", ""); err != ErrSubtitleNotFound {
		t.Fatalf("expected ErrSubtitleNotFound for index 99, got %v", err)
	}
}

func TestEmbyMediaStreamsNoSubtitleServiceKeepsVideoAudio(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	// Intentionally do NOT SetSubtitleService.
	mediaID := writeTempVideoWithSubtitle(t, svc, &lib, ".mp4", ".srt")

	m, err := svc.repo.Media.FindByID(t.Context(), mediaID)
	if err != nil || m == nil {
		t.Fatalf("find media: %v", err)
	}
	streams := svc.mediaStreams(t.Context(), m)
	if len(streams) != 2 {
		t.Fatalf("expected 2 streams (video/audio) without subtitle service, got %d: %#v", len(streams), streams)
	}
}
