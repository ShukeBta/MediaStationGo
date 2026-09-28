package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gen2brain/webp"
	_ "golang.org/x/image/bmp"
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
	fillWidth  int
	fillHeight int
	format     string
	err        error
}

// Existing Emby clients retain JPEG output unless a format is requested.
// All parameter names are case insensitive; ambiguous duplicates are rejected.
func imageVariantFromRequest(r *http.Request) imageVariantOptions {
	o := imageVariantOptions{}
	if r == nil || r.URL == nil {
		return o
	}
	seen := make(map[string]bool)
	for name, values := range r.URL.Query() {
		name = strings.ToLower(name)
		switch name {
		case "width", "maxwidth", "height", "maxheight", "fillwidth", "fillheight", "quality", "format":
		default:
			continue
		}
		if seen[name] || len(values) != 1 {
			o.err = fmt.Errorf("duplicate image parameter: %s", name)
			return o
		}
		seen[name] = true
		raw := strings.TrimSpace(values[0])
		if name == "format" {
			o.format = strings.ToLower(raw)
			if o.format == "jpg" {
				o.format = "jpeg"
			}
			if o.format != "jpeg" && o.format != "webp" && o.format != "png" {
				o.err = errors.New("unsupported image format")
				return o
			}
			continue
		}
		value, err := strconv.Atoi(raw)
		limit := 4096
		if name == "quality" {
			limit = 100
		}
		if err != nil || value < 1 || value > limit {
			o.err = fmt.Errorf("invalid image parameter: %s", name)
			return o
		}
		switch name {
		case "width", "maxwidth":
			if o.maxWidth == 0 || value < o.maxWidth {
				o.maxWidth = value
			}
		case "height", "maxheight":
			if o.maxHeight == 0 || value < o.maxHeight {
				o.maxHeight = value
			}
		case "fillwidth":
			o.fillWidth = value
		case "fillheight":
			o.fillHeight = value
		case "quality":
			o.quality, o.hasQuality = value, true
		}
	}
	if (o.fillWidth == 0) != (o.fillHeight == 0) {
		o.err = errors.New("fillWidth and fillHeight are required together")
	}
	return o
}

func (o imageVariantOptions) enabled() bool {
	return o.maxWidth > 0 || o.maxHeight > 0 || o.hasQuality || o.fillWidth > 0 || o.format != "" || o.err != nil
}

func (p *ImageProxy) serveImageVariant(w http.ResponseWriter, r *http.Request, key string, modTime time.Time, sourceSize int64, data []byte, contentType, cacheControl string, variant imageVariantOptions) error {
	variantKey, variantCachePath := p.imageVariantCachePaths(key, modTime, sourceSize, variant)
	if p.serveCachedImageVariantFile(w, r, variantKey, variantCachePath, cacheControl) {
		return nil
	}

	result := p.variantBuildGroup.DoChan(variantKey, func() (any, error) {
		p.variantBuildOnce.Do(func() { p.variantBuildSlots = make(chan struct{}, 2) })
		select {
		case p.variantBuildSlots <- struct{}{}:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		defer func() { <-p.variantBuildSlots }()
		out, outType, err := buildImageVariant(data, contentType, variant)
		if errors.Is(err, errImageVariantDecode) && p.variantFallback != nil {
			out, outType, err = p.variantFallback(r.Context(), data, variant)
		}
		if err != nil {
			return nil, err
		}
		if err := r.Context().Err(); err != nil {
			return nil, err
		}
		p.writeImageVariantCache(variantCachePath, out)
		return imageVariantResult{out, outType}, nil
	})
	var generated imageVariantResult
	select {
	case <-r.Context().Done():
		return r.Context().Err()
	case result := <-result:
		if result.Err != nil {
			return result.Err
		}
		generated = result.Val.(imageVariantResult)
	}
	out, outType := generated.data, generated.contentType
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Type", outType)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", imageVariantCacheETag(variantKey))
	http.ServeContent(w, r, variantKey, modTime, bytes.NewReader(out))
	return nil
}

type imageVariantResult struct {
	data        []byte
	contentType string
}

func buildImageVariant(data []byte, contentType string, variant imageVariantOptions) ([]byte, string, error) {
	if len(data) > maxImageVariantInputBytes {
		return nil, "", errors.New("image input limit exceeded")
	}
	if isTransparentPlaceholderData(data) {
		return data, "image/png", nil
	}
	cfg, sourceFormat, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", errImageVariantDecode, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 32_000_000 {
		return nil, "", errors.New("image pixel limit exceeded")
	}
	orientation, animated := variantMetadata(data, sourceFormat)
	if animated || sourceFormat == "gif" || (sourceFormat == "webp" && len(data) >= 21 && string(data[12:16]) == "VP8X" && data[20]&2 != 0) {
		return data, normalizedImageContentType(contentType, data), nil
	}
	var img image.Image
	if sourceFormat == "webp" {
		img, err = webp.Decode(bytes.NewReader(data), webp.Options{AutoRotate: true})
	} else {
		img, _, err = image.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", errImageVariantDecode, err)
	}
	img = orientVariantImage(img, orientation)
	bounds := imageVariantCropBounds(img.Bounds(), variant)
	targetW, targetH := imageVariantTargetSize(bounds.Dx(), bounds.Dy(), variant)
	if targetW == cfg.Width && targetH == cfg.Height && !variant.hasQuality && variant.format == "" && variant.fillWidth == 0 && orientation < 2 {
		return data, normalizedImageContentType(contentType, data), nil
	}
	dst := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Src, nil)
	quality := variant.quality
	if quality <= 0 {
		quality = defaultImageVariantQuality
	}
	quality = clampImageQuality(quality)
	var buf bytes.Buffer
	outType := "image/jpeg"
	switch variant.format {
	case "webp":
		outType = "image/webp"
		err = webp.Encode(&buf, dst, webp.Options{Quality: quality, Method: 4})
	case "png":
		outType = "image/png"
		err = png.Encode(&buf, dst)
	default:
		background := image.NewRGBA(dst.Bounds())
		xdraw.Draw(background, background.Bounds(), image.NewUniform(color.White), image.Point{}, xdraw.Src)
		xdraw.Draw(background, background.Bounds(), dst, image.Point{}, xdraw.Over)
		err = jpeg.Encode(&buf, background, &jpeg.Options{Quality: quality})
	}
	if err != nil {
		return nil, "", fmt.Errorf("encode image variant: %w", err)
	}
	return buf.Bytes(), outType, nil
}

func imageVariantCropBounds(bounds image.Rectangle, variant imageVariantOptions) image.Rectangle {
	if variant.fillWidth <= 0 || variant.fillHeight <= 0 {
		return bounds
	}
	if int64(bounds.Dx())*int64(variant.fillHeight) > int64(bounds.Dy())*int64(variant.fillWidth) {
		width := max(1, int(int64(bounds.Dy())*int64(variant.fillWidth)/int64(variant.fillHeight)))
		bounds.Min.X += (bounds.Dx() - width) / 2
		bounds.Max.X = bounds.Min.X + width
	} else {
		height := max(1, int(int64(bounds.Dx())*int64(variant.fillHeight)/int64(variant.fillWidth)))
		bounds.Min.Y += (bounds.Dy() - height) / 2
		bounds.Max.Y = bounds.Min.Y + height
	}
	return bounds
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
	if filter := imageVariantFFmpegScaleFilter(variant); filter != "" && variant.fillWidth == 0 {
		args = append(args, "-vf", filter)
	}
	extended := variant.fillWidth > 0 || (variant.format != "" && variant.format != "jpeg")
	codec := "mjpeg"
	if extended {
		codec = "png"
	}
	args = append(args, "-frames:v", "1", "-q:v", strconv.Itoa(imageVariantFFmpegQuality(variant)), "-f", "image2pipe", "-vcodec", codec, "pipe:1")

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
	if extended {
		return buildImageVariant(out, "image/png", variant)
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
	scale := math.Min(1, math.Min(4096/float64(width), 4096/float64(height)))
	if variant.fillWidth > 0 && variant.fillHeight > 0 {
		scale = math.Min(scale, math.Min(float64(variant.fillWidth)/float64(width), float64(variant.fillHeight)/float64(height)))
	}
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
