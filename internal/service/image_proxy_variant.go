package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"math"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxImageVariantInputBytes = 32 << 20

const imageVariantFFmpegTimeout = 4 * time.Second

var errImageVariantDecode = errors.New("image variant source is not decodable")

type imageVariantFallbackFunc func(context.Context, []byte, imageVariantOptions) ([]byte, string, error)

type imageVariantOptions struct {
	maxWidth   int
	maxHeight  int
	quality    int
	hasQuality bool
}

func imageVariantFromRequest(r *http.Request) imageVariantOptions {
	if r == nil || r.URL == nil {
		return imageVariantOptions{}
	}
	q := r.URL.Query()
	return imageVariantOptions{
		maxWidth:   minPositiveQueryInt(q["maxWidth"], q["MaxWidth"], q["width"], q["Width"]),
		maxHeight:  minPositiveQueryInt(q["maxHeight"], q["MaxHeight"], q["height"], q["Height"]),
		quality:    firstPositiveQueryInt(q["quality"], q["Quality"]),
		hasQuality: hasPositiveQueryInt(q["quality"], q["Quality"]),
	}
}

func (o imageVariantOptions) enabled() bool {
	return o.maxWidth > 0 || o.maxHeight > 0 || o.hasQuality
}

func minPositiveQueryInt(values ...[]string) int {
	out := 0
	for _, group := range values {
		for _, value := range group {
			n, ok := parsePositiveImageVariantInt(value)
			if !ok {
				continue
			}
			if out == 0 || n < out {
				out = n
			}
		}
	}
	return out
}

func firstPositiveQueryInt(values ...[]string) int {
	for _, group := range values {
		for _, value := range group {
			if n, ok := parsePositiveImageVariantInt(value); ok {
				return n
			}
		}
	}
	return 0
}

func hasPositiveQueryInt(values ...[]string) bool {
	return firstPositiveQueryInt(values...) > 0
}

func parsePositiveImageVariantInt(raw string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	return n, err == nil && n > 0
}

func (p *ImageProxy) serveImageVariant(w http.ResponseWriter, r *http.Request, key string, modTime time.Time, sourceSize int64, data []byte, contentType, cacheControl string, variant imageVariantOptions) error {
	variantKey, variantCachePath := p.imageVariantCachePaths(key, modTime, sourceSize, variant)
	if p.serveCachedImageVariantFile(w, r, variantKey, variantCachePath, cacheControl) {
		return nil
	}

	out, outType, err := buildImageVariant(data, contentType, variant)
	if err != nil && p != nil && p.variantFallback != nil {
		out, outType, err = p.variantFallback(r.Context(), data, variant)
	}
	if err != nil {
		return err
	}
	p.writeImageVariantCache(variantCachePath, out)
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Type", outType)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", imageVariantCacheETag(variantKey))
	http.ServeContent(w, r, variantKey, modTime, bytes.NewReader(out))
	return nil
}

func buildImageVariant(data []byte, contentType string, variant imageVariantOptions) ([]byte, string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", errImageVariantDecode, err)
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, "", fmt.Errorf("%w: invalid dimensions %dx%d", errImageVariantDecode, width, height)
	}
	targetW, targetH := imageVariantTargetSize(width, height, variant)
	if targetW == width && targetH == height && !variant.hasQuality {
		return data, normalizedImageContentType(contentType, data), nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Over, nil)

	quality := variant.quality
	if quality <= 0 {
		quality = defaultImageVariantQuality
	}
	quality = clampImageQuality(quality)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, "", fmt.Errorf("encode image variant: %w", err)
	}
	return buf.Bytes(), "image/jpeg", nil
}

func normalizedImageContentType(contentType string, data []byte) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if isImageContentType(contentType) {
		return contentType
	}
	return detectContentType(data)
}

func (p *ImageProxy) transcodeImageVariantWithFFmpeg(ctx context.Context, data []byte, variant imageVariantOptions) ([]byte, string, error) {
	configured := ""
	if p != nil && p.cfg != nil {
		configured = p.cfg.App.FFmpegPath
	}
	bin, err := resolveLocalExecutable(configured, "ffmpeg")
	if err != nil {
		return nil, "", fmt.Errorf("image variant ffmpeg unavailable: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, imageVariantFFmpegTimeout)
	defer cancel()

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-threads", "1",
		"-f", "image2pipe",
		"-i", "pipe:0",
		"-an", "-sn", "-dn",
	}
	if filter := imageVariantFFmpegScaleFilter(variant); filter != "" {
		args = append(args, "-vf", filter)
	}
	args = append(args,
		"-frames:v", "1",
		"-q:v", strconv.Itoa(imageVariantFFmpegQuality(variant)),
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"pipe:1",
	)

	cmd := exec.CommandContext(runCtx, bin, args...) // #nosec G204 -- executable is resolved locally and all arguments are numeric constants derived from validated query parameters.
	cmd.Stdin = bytes.NewReader(data)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			return nil, "", fmt.Errorf("image variant ffmpeg timed out: %w", runCtx.Err())
		}
		return nil, "", fmt.Errorf("image variant ffmpeg failed: %w: %s", err, truncateImageVariantError(stderr.String()))
	}
	out := stdout.Bytes()
	if len(out) == 0 || len(out) > maxImageVariantInputBytes {
		return nil, "", fmt.Errorf("image variant ffmpeg returned invalid size %d", len(out))
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		return nil, "", fmt.Errorf("image variant ffmpeg returned invalid jpeg: %w", err)
	}
	return append([]byte(nil), out...), "image/jpeg", nil
}

func imageVariantFFmpegScaleFilter(variant imageVariantOptions) string {
	switch {
	case variant.maxWidth > 0 && variant.maxHeight > 0:
		return fmt.Sprintf("scale=min(iw\\,%d):min(ih\\,%d):force_original_aspect_ratio=decrease", variant.maxWidth, variant.maxHeight)
	case variant.maxWidth > 0:
		return fmt.Sprintf("scale=min(iw\\,%d):-2", variant.maxWidth)
	case variant.maxHeight > 0:
		return fmt.Sprintf("scale=-2:min(ih\\,%d)", variant.maxHeight)
	default:
		return ""
	}
}

func imageVariantFFmpegQuality(variant imageVariantOptions) int {
	quality := variant.quality
	if quality <= 0 {
		quality = defaultImageVariantQuality
	}
	quality = clampImageQuality(quality)
	return 2 + int(math.Round(float64(95-quality)*8.0/60.0))
}

func truncateImageVariantError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 512 {
		return value
	}
	return value[:512]
}

func imageVariantTargetSize(width, height int, variant imageVariantOptions) (int, int) {
	scale := 1.0
	if variant.maxWidth > 0 && width > variant.maxWidth {
		scale = math.Min(scale, float64(variant.maxWidth)/float64(width))
	}
	if variant.maxHeight > 0 && height > variant.maxHeight {
		scale = math.Min(scale, float64(variant.maxHeight)/float64(height))
	}
	if scale >= 1 {
		return width, height
	}
	targetW := int(math.Round(float64(width) * scale))
	targetH := int(math.Round(float64(height) * scale))
	if targetW < 1 {
		targetW = 1
	}
	if targetH < 1 {
		targetH = 1
	}
	return targetW, targetH
}

func clampImageQuality(value int) int {
	if value < 35 {
		return 35
	}
	if value > 95 {
		return 95
	}
	return value
}

const defaultImageVariantQuality = 82
