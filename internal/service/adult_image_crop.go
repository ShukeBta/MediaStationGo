package service

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"strings"
)

// IsAdultMediaPathOrMetadata reports whether media is adult based on path, mediaType, nsfw flag, or adult code.
func IsAdultMediaPathOrMetadata(path, mediaType string, nsfw bool) bool {
	if nsfw {
		return true
	}
	if normalizeOrganizeMediaType(mediaType) == "adult" {
		return true
	}
	if AdultCodeFromMediaPath(path) != "" {
		return true
	}
	return false
}

// IsAdultArtworkURL reports whether the URL is likely an adult cover/poster image.
func IsAdultArtworkURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "javbus") ||
		strings.Contains(lower, "javdb") ||
		strings.Contains(lower, "jdbstatic") ||
		strings.Contains(lower, "busjav") ||
		strings.Contains(lower, "dmmbus") ||
		strings.Contains(lower, "cdnbus") ||
		strings.Contains(lower, "javsee") ||
		strings.Contains(lower, "/covers/") ||
		strings.Contains(lower, "pics.dmm.co.jp")
}

// CropAdultCoverPoster checks if the image data is a wide full-jacket DVD cover (width > height * 1.15).
// If so, it crops the right portion (the front cover poster) and returns the encoded JPEG bytes.
// If the image is already portrait or cannot be decoded, it safely returns the original data.
func CropAdultCoverPoster(data []byte) ([]byte, string, error) {
	if len(data) == 0 {
		return data, "", nil
	}
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, format, err
	}
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return data, format, nil
	}
	// If the image is already portrait or square, keep it as-is.
	if float64(width) <= float64(height)*1.15 {
		return data, format, nil
	}

	// Front cover width is typically ~0.70-0.72 of the height (or right ~50-52% of total width)
	cropWidth := int(float64(height) * 0.71)
	maxCropWidth := int(float64(width) * 0.53)
	if cropWidth > maxCropWidth {
		cropWidth = maxCropWidth
	}
	if cropWidth <= 0 {
		cropWidth = width / 2
	}
	minX := bounds.Max.X - cropWidth
	if minX < bounds.Min.X {
		minX = bounds.Min.X
	}

	cropRect := image.Rect(minX, bounds.Min.Y, bounds.Max.X, bounds.Max.Y)
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	var cropped image.Image
	if si, ok := src.(subImager); ok {
		cropped = si.SubImage(cropRect)
	} else {
		dst := image.NewRGBA(image.Rect(0, 0, cropRect.Dx(), cropRect.Dy()))
		draw.Draw(dst, dst.Bounds(), src, cropRect.Min, draw.Src)
		cropped = dst
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, cropped, &jpeg.Options{Quality: 92}); err != nil {
		return data, format, err
	}
	return buf.Bytes(), "image/jpeg", nil
}
