package service

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const (
	adultDerivedPosterStrategy      = "adult-right-half-poster-v1"
	adultNormalizedBackdropStrategy = "adult-backdrop-normalize-v1"
)

var adultDerivedPosterSize = image.Pt(600, 900)

var errAdultPosterSourceNotLandscape = errors.New("adult poster source is not landscape")

var jpegJFIFAPP0 = []byte{
	0xff, 0xe0, 0x00, 0x10,
	'J', 'F', 'I', 'F', 0x00,
	0x01, 0x01,
	0x00,
	0x00, 0x01,
	0x00, 0x01,
	0x00, 0x00,
}

func (s *ScraperService) deriveAdultPosterIfNeeded(ctx context.Context, m *model.Media, lib *model.Library, match *Match) {
	if s == nil || s.images == nil || match == nil || !adultScrapeNeedsDerivedPoster(lib, match) {
		return
	}
	backdropNormalized := s.normalizeAdultBackdropIfNeeded(ctx, m, match)
	posterURL := strings.TrimSpace(match.PosterURL)
	backdropURL := strings.TrimSpace(match.BackdropURL)
	sourceURL, data, ok := s.fetchAdultPosterDerivationSource(ctx, m, posterURL, backdropURL)
	if !ok {
		return
	}
	if !backdropNormalized {
		if backdropPath, err := s.writeAdultBackdrop(sourceURL, data); err == nil {
			match.BackdropURL = backdropPath
		} else if !errors.Is(err, errAdultPosterSourceNotLandscape) {
			s.log.Warn("adult backdrop fallback skipped; backdrop generation failed",
				zap.String("media_id", mediaIDForLog(m)),
				zap.String("source", sourceURL),
				zap.Error(err))
		}
	}
	posterPath, err := s.writeAdultPosterFromBackdrop(sourceURL, data)
	if err != nil {
		if errors.Is(err, errAdultPosterSourceNotLandscape) {
			return
		}
		s.log.Warn("adult poster derivation skipped; poster generation failed",
			zap.String("media_id", mediaIDForLog(m)),
			zap.String("source", sourceURL),
			zap.Error(err))
		return
	}
	match.PosterURL = posterPath
}

func (s *ScraperService) normalizeAdultBackdropIfNeeded(ctx context.Context, m *model.Media, match *Match) bool {
	if s == nil || s.images == nil || match == nil {
		return false
	}
	backdropURL := strings.TrimSpace(match.BackdropURL)
	if backdropURL == "" || !isHTTPish(backdropURL) {
		return false
	}
	data, err := s.fetchAdultPosterSource(ctx, backdropURL)
	if err != nil {
		s.log.Warn("adult backdrop normalization skipped; backdrop fetch failed",
			zap.String("media_id", mediaIDForLog(m)),
			zap.String("backdrop", backdropURL),
			zap.Error(err))
		return false
	}
	backdropPath, err := s.writeAdultBackdrop(backdropURL, data)
	if err != nil {
		s.log.Warn("adult backdrop normalization skipped; backdrop generation failed",
			zap.String("media_id", mediaIDForLog(m)),
			zap.String("backdrop", backdropURL),
			zap.Error(err))
		return false
	}
	match.BackdropURL = backdropPath
	return true
}

func (s *ScraperService) fetchAdultPosterDerivationSource(ctx context.Context, m *model.Media, posterURL, backdropURL string) (string, []byte, bool) {
	posterURL = strings.TrimSpace(posterURL)
	backdropURL = strings.TrimSpace(backdropURL)
	if posterURL != "" && posterURL != backdropURL {
		if data, err := s.fetchAdultPosterSource(ctx, posterURL); err == nil {
			if imageDataIsLandscape(data) {
				return posterURL, data, true
			}
			return "", nil, false
		} else {
			if backdropURL == "" {
				s.log.Warn("adult poster derivation skipped; poster fetch failed",
					zap.String("media_id", mediaIDForLog(m)),
					zap.String("source", posterURL),
					zap.Error(err))
				return "", nil, false
			}
			s.log.Warn("adult poster derivation source fetch failed; trying backdrop",
				zap.String("media_id", mediaIDForLog(m)),
				zap.String("source", posterURL),
				zap.Error(err))
		}
	}
	if backdropURL == "" {
		return "", nil, false
	}
	data, err := s.fetchAdultPosterSource(ctx, backdropURL)
	if err != nil {
		s.log.Warn("adult poster derivation skipped; backdrop fetch failed",
			zap.String("media_id", mediaIDForLog(m)),
			zap.String("backdrop", backdropURL),
			zap.Error(err))
		return "", nil, false
	}
	return backdropURL, data, true
}

func (s *ScraperService) fetchAdultPosterSource(ctx context.Context, sourceURL string) ([]byte, error) {
	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	data, _, err := s.images.Fetch(fetchCtx, sourceURL)
	return data, err
}

func imageDataIsLandscape(data []byte) bool {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	return err == nil && cfg.Width > cfg.Height && cfg.Width >= 320 && cfg.Height >= 180
}

func adultScrapeNeedsDerivedPoster(lib *model.Library, match *Match) bool {
	if match == nil {
		return false
	}
	if match.NSFW || strings.EqualFold(match.MediaType, "adult") {
		return true
	}
	return lib != nil && strings.EqualFold(lib.Type, "adult")
}

func (s *ScraperService) writeAdultBackdrop(backdropURL string, data []byte) (string, error) {
	if s == nil || s.images == nil {
		return "", errors.New("image proxy missing")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return "", errors.New("invalid image bounds")
	}
	if bounds.Dx() <= bounds.Dy() || bounds.Dx() < 320 || bounds.Dy() < 180 {
		return "", errAdultPosterSourceNotLandscape
	}
	sum := sha1.Sum([]byte(adultNormalizedBackdropStrategy + "\n" + backdropURL + "\n" + hex.EncodeToString(sha1Bytes(data))))
	name := hex.EncodeToString(sum[:])[:24] + ".jpg"
	dir := filepath.Join(s.images.cacheDir, "adult-backdrops")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return filepath.Abs(path)
	}
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.CatmullRom.Scale(out, out.Bounds(), img, bounds, draw.Over, nil)
	encoded, err := encodeAdultArtworkJPEG(out)
	if err != nil {
		return "", err
	}
	if err := writeAdultArtworkFile(path, encoded); err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func (s *ScraperService) writeAdultPosterFromBackdrop(backdropURL string, data []byte) (string, error) {
	if s == nil || s.images == nil {
		return "", errors.New("image proxy missing")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return "", errors.New("invalid image bounds")
	}
	if bounds.Dx() <= bounds.Dy() || bounds.Dx() < 320 || bounds.Dy() < 180 {
		return "", errAdultPosterSourceNotLandscape
	}
	crop := image.Rect(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y, bounds.Max.X, bounds.Max.Y)
	targetRatio := float64(adultDerivedPosterSize.X) / float64(adultDerivedPosterSize.Y)
	cropRatio := float64(crop.Dx()) / float64(crop.Dy())
	if cropRatio > targetRatio {
		desiredW := int(float64(crop.Dy()) * targetRatio)
		if desiredW < 1 {
			desiredW = 1
		}
		crop.Min.X = crop.Max.X - desiredW
	} else if cropRatio < targetRatio {
		desiredH := int(float64(crop.Dx()) / targetRatio)
		if desiredH < 1 {
			desiredH = 1
		}
		extra := crop.Dy() - desiredH
		if extra > 0 {
			crop.Min.Y += extra / 2
			crop.Max.Y = crop.Min.Y + desiredH
		}
	}

	sum := sha1.Sum([]byte(adultDerivedPosterStrategy + "\n" + backdropURL + "\n" + hex.EncodeToString(sha1Bytes(data))))
	name := hex.EncodeToString(sum[:])[:24] + ".jpg"
	dir := filepath.Join(s.images.cacheDir, "adult-posters")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return filepath.Abs(path)
	}

	out := image.NewRGBA(image.Rect(0, 0, adultDerivedPosterSize.X, adultDerivedPosterSize.Y))
	draw.CatmullRom.Scale(out, out.Bounds(), img, crop, draw.Over, nil)
	encoded, err := encodeAdultArtworkJPEG(out)
	if err != nil {
		return "", err
	}
	if err := writeAdultArtworkFile(path, encoded); err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func writeAdultArtworkFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "adult-artwork-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func encodeAdultArtworkJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return ensureJPEGJFIFHeader(buf.Bytes()), nil
}

func ensureJPEGJFIFHeader(data []byte) []byte {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return data
	}
	if len(data) >= 11 && data[2] == 0xff && data[3] == 0xe0 && string(data[6:11]) == "JFIF\x00" {
		return data
	}
	out := make([]byte, 0, len(data)+len(jpegJFIFAPP0))
	out = append(out, data[:2]...)
	out = append(out, jpegJFIFAPP0...)
	out = append(out, data[2:]...)
	return out
}

func sha1Bytes(data []byte) []byte {
	sum := sha1.Sum(data)
	return sum[:]
}

func mediaIDForLog(m *model.Media) string {
	if m == nil {
		return ""
	}
	return m.ID
}
