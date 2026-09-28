package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/ShukeBta/MediaStationGo/internal/config"
	"go.uber.org/zap"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func variantPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 100, A: 128})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestImageVariantA10FormatsCropAndValidation(t *testing.T) {
	for _, tt := range []struct {
		query, format string
		width, height int
	}{
		{"?maxWidth=30&format=webp", "webp", 30, 15},
		{"?WIDTH=30&MAXHEIGHT=10&FORMAT=PNG", "png", 20, 10},
		{"?fillWidth=20&fillHeight=20&format=webp", "webp", 20, 20},
		{"?fillWidth=200&fillHeight=200&format=png", "png", 60, 60},
		{"?maxWidth=30&format=jpg", "jpeg", 30, 15},
		{"?maxWidth=30", "jpeg", 30, 15},
	} {
		req := httptest.NewRequest("GET", "/"+tt.query, nil)
		options := imageVariantFromRequest(req)
		if options.err != nil {
			t.Fatal(options.err)
		}
		data, ctype, err := buildImageVariant(variantPNG(t, 120, 60), "image/png", options)
		if err != nil {
			t.Fatal(err)
		}
		img, format, err := image.Decode(bytes.NewReader(data))
		if err != nil || format != tt.format || img.Bounds().Dx() != tt.width || img.Bounds().Dy() != tt.height {
			t.Fatalf("%s: image=%v format=%s err=%v", tt.query, img, format, err)
		}
		if ctype != "image/"+tt.format {
			t.Fatalf("content type=%s", ctype)
		}
		if format == "png" || format == "webp" {
			_, _, _, alpha := img.At(0, 0).RGBA()
			if alpha == 65535 {
				t.Fatal("transparency lost")
			}
		}
	}
	// Center crop excludes the differently colored outer quarters.
	img := image.NewNRGBA(image.Rect(0, 0, 120, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 120; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 30 && x < 90 {
				c = color.NRGBA{G: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	data, _, err := buildImageVariant(buf.Bytes(), "image/png", imageVariantOptions{fillWidth: 60, fillHeight: 60, format: "png"})
	if err != nil {
		t.Fatal(err)
	}
	cropped, _, _ := image.Decode(bytes.NewReader(data))
	r, g, _, _ := cropped.At(0, 0).RGBA()
	if r != 0 || g != 65535 {
		t.Fatal("crop is not centered")
	}
	for _, query := range []string{"?width=0", "?width=4097", "?quality=101", "?fillWidth=20", "?format=avif", "?width=10&Width=20", "?width=10&width=10", "?quality=abc"} {
		if options := imageVariantFromRequest(httptest.NewRequest("GET", "/"+query, nil)); options.err == nil {
			t.Errorf("accepted %s", query)
		}
	}
}

func TestImageVariantA10HTTPWebPCacheAndInvalidParameters(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "poster.png")
	if err := os.WriteFile(path, variantPNG(t, 120, 60), 0600); err != nil {
		t.Fatal(err)
	}
	proxy := NewImageProxy(&config.Config{App: config.AppConfig{DataDir: root}, Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	serve := func(target, etag, method string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		w := httptest.NewRecorder()
		if err := proxy.Serve(req.Context(), w, req, path); err != nil {
			t.Fatal(err)
		}
		return w
	}
	first := serve("/?fillWidth=24&fillHeight=24&format=webp", "", http.MethodGet)
	if first.Code != 200 || first.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("first=%d %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	if next := serve("/?fillWidth=24&fillHeight=24&format=webp", etag, http.MethodGet); next.Code != 304 {
		t.Fatalf("cache=%d", next.Code)
	}
	if next := serve("/?fillWidth=24&fillHeight=24&format=webp", "", http.MethodHead); next.Code != 200 || next.Body.Len() != 0 {
		t.Fatal("HEAD body/status")
	}
	pngResult := serve("/?fillWidth=24&fillHeight=24&format=png", etag, http.MethodGet)
	if pngResult.Code != 200 || pngResult.Header().Get("ETag") == etag {
		t.Fatal("format cache collision")
	}
	if err := os.WriteFile(path, variantPNG(t, 80, 60), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(time.Second)
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
	if next := serve("/?fillWidth=24&fillHeight=24&format=webp", etag, http.MethodGet); next.Code != 200 || next.Header().Get("ETag") == etag {
		t.Fatal("source update did not invalidate variant")
	}
	for _, target := range []string{"/?format=invalid", "/?fillWidth=20", "/?width=999999"} {
		if result := serve(target, "", http.MethodGet); result.Code != 400 || result.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("invalid status=%d", result.Code)
		}
	}
}

func TestImageVariantA10ConcurrentCancellation(t *testing.T) {
	p := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	p.variantFallback = func(ctx context.Context, _ []byte, _ imageVariantOptions) ([]byte, string, error) {
		calls.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return testJPEGBytes(t, 10, 10), "image/jpeg", nil
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	req := httptest.NewRequest("GET", "/?maxWidth=10", nil)
	started := make(chan struct{})
	var wg sync.WaitGroup
	var bad atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-started
			w := httptest.NewRecorder()
			if err := p.serveImageVariant(w, req, "same", time.Unix(1, 0), 3, []byte("bad"), "image/jpeg", "public", imageVariantOptions{maxWidth: 10}); err != nil || w.Code != 200 {
				bad.Add(1)
			}
		}()
	}
	close(started)
	<-entered
	// A waiting client may leave while the shared generation continues.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := p.serveImageVariant(httptest.NewRecorder(), req.WithContext(ctx), "same", time.Unix(1, 0), 3, []byte("bad"), "image/jpeg", "public", imageVariantOptions{maxWidth: 10}); err == nil {
		t.Fatal("cancellation ignored")
	}
	close(release)
	wg.Wait()
	if bad.Load() != 0 || calls.Load() != 1 {
		t.Fatalf("calls=%d failures=%d", calls.Load(), bad.Load())
	}
}

func TestImageVariantA10OrientationAndAnimation(t *testing.T) {
	// Little-endian TIFF，IFD0 的 Orientation=6（顺时针 90 度）。
	exif := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewNRGBA(image.Rect(0, 0, 120, 60)), nil); err != nil {
		t.Fatal(err)
	}
	app1 := append([]byte("Exif\x00\x00"), exif...)
	withExif := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(app1) + 2)}, app1...)
	withExif = append(withExif, jpegData.Bytes()[2:]...)
	pngChunk := func(kind string, payload []byte) []byte {
		chunk := make([]byte, len(payload)+12)
		binary.BigEndian.PutUint32(chunk[:4], uint32(len(payload)))
		copy(chunk[4:8], kind)
		copy(chunk[8:], payload)
		binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
		return chunk
	}
	pngData := variantPNG(t, 120, 60)
	pngExif := append(append(append([]byte{}, pngData[:33]...), pngChunk("eXIf", exif)...), pngData[33:]...)
	for _, source := range [][]byte{withExif, pngExif} {
		data, _, err := buildImageVariant(source, "", imageVariantOptions{maxWidth: 20, quality: 80, format: "webp"})
		if err != nil {
			t.Fatal(err)
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != 20 || cfg.Height != 40 {
			t.Fatalf("orientation: %v %v", cfg, err)
		}
	}
	apng := append(append(append([]byte{}, pngData[:33]...), pngChunk("acTL", []byte{0, 0, 0, 2, 0, 0, 0, 0})...), pngData[33:]...)
	if out, _, err := buildImageVariant(apng, "image/png", imageVariantOptions{quality: 80, format: "webp"}); err != nil || !bytes.Equal(out, apng) {
		t.Fatal("APNG was flattened")
	}
	for _, invalid := range [][]byte{nil, exif[:7], {'I', 'I', 42, 0, 255, 255, 255, 255}} {
		if variantTIFFOrientation(invalid) != 1 {
			t.Fatal("invalid EXIF")
		}
	}
	source := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	source.Set(0, 0, color.White)
	for i, point := range []image.Point{{0, 0}, {2, 0}, {2, 1}, {0, 1}, {0, 0}, {1, 0}, {1, 2}, {0, 2}} {
		got := orientVariantImage(source, i+1)
		if got.At(point.X, point.Y) != (color.NRGBA{255, 255, 255, 255}) {
			t.Fatalf("orientation %d wrong pixel mapping", i+1)
		}
	}
}

func TestImageVariantA10PixelLimitSkipsDecoderFallback(t *testing.T) {
	data := variantPNG(t, 2, 2)
	binary.BigEndian.PutUint32(data[16:20], 10000)
	binary.BigEndian.PutUint32(data[20:24], 10000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	p := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	var fallback atomic.Bool
	p.variantFallback = func(context.Context, []byte, imageVariantOptions) ([]byte, string, error) {
		fallback.Store(true)
		return nil, "", nil
	}
	req := httptest.NewRequest("GET", "/?format=webp", nil)
	if err := p.serveImageVariant(httptest.NewRecorder(), req, "huge", time.Unix(1, 0), int64(len(data)), data, "image/png", "public", imageVariantOptions{format: "webp"}); err == nil {
		t.Fatal("oversized decoded image accepted")
	}
	if fallback.Load() {
		t.Fatal("pixel limit bypassed through FFmpeg")
	}
}

func TestImageVariantA10FFmpegFallbackKeepsCropAndFormat(t *testing.T) {
	if _, err := resolveLocalExecutable("", "ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	p := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	data, ctype, err := p.transcodeImageVariantWithFFmpeg(t.Context(), variantPNG(t, 120, 60), imageVariantOptions{fillWidth: 24, fillHeight: 24, format: "webp"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "webp" || ctype != "image/webp" || cfg.Width != 24 || cfg.Height != 24 {
		t.Fatalf("fallback output=%v %s %s %v", cfg, format, ctype, err)
	}
}
