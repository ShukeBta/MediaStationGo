package handler

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/webp"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const (
	embyFolderCoverGridLimit    = 4
	embyFolderCoverDefaultWidth = 960
	embyFolderCoverMinWidth     = 160
	embyFolderCoverMaxWidth     = 1200
	embyFolderCoverAspectRatio  = 16.0 / 9.0
)

func serveEmbyFolderCoverImage(svc *service.Container, c *gin.Context, id, imageType string) bool {
	if svc == nil || svc.Emby == nil || svc.ImageProxy == nil {
		return false
	}
	artworks, err := svc.Emby.FolderCoverArtwork(c.Request.Context(), id, imageType, embyFolderCoverGridLimit)
	if err != nil || len(artworks) == 0 {
		return false
	}
	images := make([]image.Image, 0, len(artworks))
	for _, artwork := range artworks {
		img, err := fetchEmbyFolderArtwork(c.Request.Context(), svc, artwork.URL)
		if err != nil {
			continue
		}
		images = append(images, img)
		if len(images) >= embyFolderCoverGridLimit {
			break
		}
	}
	if len(images) == 0 {
		return false
	}
	width, height := embyFolderCoverDimensions(c)
	body, err := buildEmbyFolderCoverGrid(images, width, height)
	if err != nil {
		return false
	}
	tag := service.EmbyFolderCoverTag(id, artworks)
	setEmbyFolderCoverHeaders(c, tag)
	c.Header("Content-Length", strconv.Itoa(len(body)))
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return true
	}
	c.Data(http.StatusOK, "image/png", body)
	return true
}

func fetchEmbyFolderArtwork(ctx context.Context, svc *service.Container, raw string) (image.Image, error) {
	raw = strings.TrimSpace(raw)
	if typ, ref, ok := service.ParseCloudArtworkURL(raw); ok {
		stableKey := typ + ":" + ref
		if data, _, cached := svc.ImageProxy.FetchCloudCached(stableKey); cached {
			return decodeEmbyFolderArtwork(data)
		}
		if svc.StorageCfg == nil {
			return nil, http.ErrMissingFile
		}
		link, err := svc.StorageCfg.CloudResolve(ctx, typ, ref, "")
		if err != nil {
			return nil, err
		}
		data, _, err := svc.ImageProxy.FetchCloudResolved(ctx, stableKey, link)
		if err != nil {
			return nil, err
		}
		return decodeEmbyFolderArtwork(data)
	}
	data, _, err := svc.ImageProxy.Fetch(ctx, raw)
	if err != nil {
		return nil, err
	}
	return decodeEmbyFolderArtwork(data)
}

func decodeEmbyFolderArtwork(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

func buildEmbyFolderCoverGrid(images []image.Image, width, height int) ([]byte, error) {
	if width <= 0 {
		width = embyFolderCoverDefaultWidth
	}
	if height <= 0 {
		height = int(math.Round(float64(width) / embyFolderCoverAspectRatio))
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.RGBA{18, 18, 18, 255}}, image.Point{}, draw.Src)
	rects := embyFolderCoverCellRects(width, height, len(images))
	for i, rect := range rects {
		drawCoverFit(dst, rect, images[i])
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func embyFolderCoverCellRects(width, height, count int) []image.Rectangle {
	columnCount := count
	if columnCount > embyFolderCoverGridLimit {
		columnCount = embyFolderCoverGridLimit
	}
	if columnCount < 1 {
		columnCount = 1
	}
	rects := make([]image.Rectangle, 0, columnCount)
	x := 0
	for i := 0; i < columnCount; i++ {
		nextX := width
		if i != columnCount-1 {
			nextX = int(math.Round(float64(width) * float64(i+1) / float64(columnCount)))
		}
		rects = append(rects, image.Rect(x, 0, nextX, height))
		x = nextX
	}
	return rects
}

func drawCoverFit(dst draw.Image, rect image.Rectangle, src image.Image) {
	if src == nil || rect.Empty() {
		return
	}
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dw, dh := rect.Dx(), rect.Dy()
	if sw <= 0 || sh <= 0 || dw <= 0 || dh <= 0 {
		return
	}
	srcRatio := float64(sw) / float64(sh)
	dstRatio := float64(dw) / float64(dh)
	cropW, cropH := sw, sh
	if srcRatio > dstRatio {
		cropW = int(math.Round(float64(sh) * dstRatio))
	} else if srcRatio < dstRatio {
		cropH = int(math.Round(float64(sw) / dstRatio))
	}
	if cropW < 1 {
		cropW = 1
	}
	if cropH < 1 {
		cropH = 1
	}
	sx0 := sb.Min.X + (sw-cropW)/2
	sy0 := sb.Min.Y + (sh-cropH)/2
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		sy := sy0 + (y-rect.Min.Y)*cropH/dh
		for x := rect.Min.X; x < rect.Max.X; x++ {
			sx := sx0 + (x-rect.Min.X)*cropW/dw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
}

func embyFolderCoverDimensions(c *gin.Context) (int, int) {
	width := queryMinPositiveInt(c, "maxWidth", "MaxWidth", "width", "Width")
	maxHeight := queryMinPositiveInt(c, "maxHeight", "MaxHeight", "height", "Height")
	if width <= 0 {
		width = embyFolderCoverDefaultWidth
	}
	if maxHeight > 0 {
		heightBoundedWidth := int(float64(maxHeight) * embyFolderCoverAspectRatio)
		if heightBoundedWidth > 0 && heightBoundedWidth < width {
			width = heightBoundedWidth
		}
	}
	width = clampInt(width, embyFolderCoverMinWidth, embyFolderCoverMaxWidth)
	height := int(math.Round(float64(width) / embyFolderCoverAspectRatio))
	if height < 1 {
		height = 1
	}
	return width, height
}

func queryMinPositiveInt(c *gin.Context, keys ...string) int {
	out := 0
	for _, key := range keys {
		for _, value := range c.QueryArray(key) {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				continue
			}
			if out == 0 || n < out {
				out = n
			}
		}
	}
	return out
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func setEmbyFolderCoverHeaders(c *gin.Context, tag string) {
	now := time.Now().UTC()
	c.Header("Content-Type", "image/png")
	c.Header("Cache-Control", "public, max-age=31536000")
	if tag != "" {
		c.Header("ETag", `"`+tag+`"`)
	}
	c.Header("Last-Modified", now.Format(http.TimeFormat))
	c.Header("Expires", now.Add(365*24*time.Hour).Format(http.TimeFormat))
	c.Header("Accept-Ranges", "bytes")
	c.Header("Cross-Origin-Resource-Policy", "cross-origin")
	c.Header("Access-Control-Allow-Origin", "*")
}
