package service

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func metadataFromArtwork(mediaPath, showBaseDir string) *LocalMetadata {
	meta := &LocalMetadata{}
	mergeArtworkMetadata(meta, mediaPath, showBaseDir)
	if meta.PosterURL == "" && meta.BackdropURL == "" {
		return nil
	}
	return meta
}

func mergeArtworkMetadata(meta *LocalMetadata, mediaPath, showBaseDir string) {
	if meta == nil {
		return
	}
	mediaDir := filepath.Dir(mediaPath)
	if localPoster := firstLocalPoster(mediaPath, showBaseDir); localPoster != "" {
		meta.PosterURL = localPoster
	} else if meta.PosterURL == "" {
		meta.PosterURL = firstAdultLooseImage(mediaDir, "poster")
	}
	dirs := []string{mediaDir, showBaseDir}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if img := firstExistingImage(dir, localBackdropCandidates(mediaPath)...); img != "" {
			meta.BackdropURL = img
			break
		}
		if meta.BackdropURL == "" {
			meta.BackdropURL = firstAdultLooseImage(dir, "backdrop")
		}
	}
	if !isLocalPath(meta.PosterURL) {
		if dmmPoster := adultDMMPosterFromSampleURL(meta.BackdropURL); dmmPoster != "" {
			meta.PosterURL = dmmPoster
		}
	}
	if meta.PosterURL != "" || meta.BackdropURL != "" {
		meta.HasArtwork = true
	}
}

func localPosterCandidates(mediaPath string) []string {
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	names := []string{
		base + "-poster",
		base + ".poster",
		"poster",
		"folder",
		"cover",
		"movie",
		"show",
		base + "-cover",
		base + ".cover",
		base,
		base + "-thumb",
		base + ".thumb",
		"thumb",
	}
	return append(adultArtworkNameCandidates(mediaPath, "poster"), names...)
}

func localBackdropCandidates(mediaPath string) []string {
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	names := []string{
		base + "-fanart",
		base + ".fanart",
		base + "-backdrop",
		base + ".backdrop",
		base + "-background",
		"fanart",
		"backdrop",
		"background",
		"landscape",
		"banner",
		"clearart",
	}
	return append(adultArtworkNameCandidates(mediaPath, "backdrop"), names...)
}

func adultArtworkNameCandidates(mediaPath, kind string) []string {
	code := AdultCodeFromMediaPath(mediaPath)
	if code == "" {
		return nil
	}
	compact := strings.ReplaceAll(code, "-", "")
	bases := []string{code, compact}
	bases = append(bases, adultDMMNameCandidates(code)...)
	out := make([]string, 0, len(bases)*6)
	for _, base := range bases {
		if base == "" {
			continue
		}
		if kind == "poster" {
			out = append(out, base, base+"-poster", base+".poster", base+"-cover", base+".cover", base+"-thumb", base+".thumb", base+"pl", base+"-pl")
		} else {
			out = append(out, base+"-fanart", base+".fanart", base+"-backdrop", base+".backdrop", base+"-background", base+"-landscape", base+"jp", base+"jp-1")
		}
	}
	return out
}

func adultDMMNameCandidates(code string) []string {
	parts := adultStandardPattern.FindStringSubmatch(code)
	if len(parts) < 3 {
		return nil
	}
	prefix := strings.ToLower(parts[1])
	num := strings.TrimLeft(parts[2], "0")
	if num == "" {
		num = "0"
	}
	padded := num
	for len(padded) < 5 {
		padded = "0" + padded
	}
	return []string{prefix + padded}
}

func firstExistingImage(dir string, names ...string) string {
	if dir == "" {
		return ""
	}
	for _, name := range names {
		for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tbn"} {
			path := filepath.Join(dir, name+ext)
			if fileExists(path) {
				return filepath.Clean(path)
			}
		}
	}
	return ""
}

func nfoPosterValues(doc *nfoDocument) []string {
	if doc == nil {
		return nil
	}
	values := []string{doc.Poster, doc.Art.Poster}
	for _, thumb := range doc.Thumbs {
		aspect := strings.ToLower(strings.TrimSpace(thumb.Aspect))
		if aspect == "" || aspect == "poster" || aspect == "cover" || aspect == "default" {
			values = append(values, thumb.Value)
		}
	}
	values = append(values, doc.Art.Thumb)
	return values
}

func nfoBackdropValues(doc *nfoDocument) []string {
	if doc == nil {
		return nil
	}
	values := []string{doc.Fanart.Value, doc.Art.Fanart, doc.Art.Backdrop, doc.Art.Background, doc.Art.Landscape, doc.Art.Banner}
	for _, thumb := range doc.Thumbs {
		aspect := strings.ToLower(strings.TrimSpace(thumb.Aspect))
		if aspect == "fanart" || aspect == "backdrop" || aspect == "background" || aspect == "landscape" {
			values = append(values, thumb.Value)
		}
	}
	values = append(values, doc.Fanart.Thumbs...)
	return values
}

func firstLocalPoster(mediaPath, showBaseDir string) string {
	mediaDir := filepath.Dir(mediaPath)
	dirs := []string{}
	if showBaseDir != "" && !samePath(showBaseDir, mediaDir) {
		dirs = append(dirs, showBaseDir)
	}
	dirs = append(dirs, mediaDir)
	for _, dir := range dirs {
		if localPoster := firstExistingPosterImage(dir, localPosterCandidates(mediaPath)...); localPoster != "" {
			return localPoster
		}
	}
	return ""
}

func firstExistingPosterImage(dir string, names ...string) string {
	if dir == "" {
		return ""
	}
	for _, name := range names {
		if isRejectedPosterName(name) {
			continue
		}
		for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tbn"} {
			path := filepath.Join(dir, name+ext)
			if fileExists(path) && likelyPosterImage(path) {
				return filepath.Clean(path)
			}
		}
	}
	return ""
}

func firstAdultLooseImage(dir, kind string) string {
	if dir == "" {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*"))
	preferred := []string{}
	fallback := []string{}
	for _, path := range matches {
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".gif" && ext != ".bmp" && ext != ".tbn" {
			continue
		}
		name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), ext))
		if kind == "poster" {
			if isRejectedPosterName(name) {
				continue
			}
			if strings.Contains(name, "poster") || strings.Contains(name, "cover") || strings.Contains(name, "folder") || strings.Contains(name, "movie") || strings.HasSuffix(name, "pl") {
				preferred = append(preferred, path)
			}
		} else if strings.Contains(name, "fanart") || strings.Contains(name, "backdrop") || strings.Contains(name, "background") || strings.Contains(name, "landscape") || strings.Contains(name, "jp") {
			preferred = append(preferred, path)
		}
		if kind != "poster" || likelyPosterImage(path) {
			fallback = append(fallback, path)
		}
	}
	if len(preferred) > 0 {
		return filepath.Clean(preferred[0])
	}
	if kind == "poster" && len(fallback) == 1 {
		return filepath.Clean(fallback[0])
	}
	return ""
}

func isRejectedPosterName(name string) bool {
	name = strings.ToLower(name)
	rejected := []string{
		"actor", "actors", "actress", "cast", "avatar", "portrait", "person",
		"sample", "screenshot", "screen", "still", "scene", "extrafanart", "extrathumb",
		"fanart", "backdrop", "background", "landscape", "banner", "clearlogo", "clearart", "logo", "disc",
	}
	for _, token := range rejected {
		if strings.Contains(name, token) {
			return true
		}
	}
	return false
}

func likelyPosterImage(path string) bool {
	file, err := os.Open(path) // #nosec G304 -- path is a discovered artwork sidecar under the configured library root.
	if err != nil {
		return false
	}
	defer file.Close()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return true
	}
	return cfg.Height >= cfg.Width
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func isLocalPath(raw string) bool {
	raw = strings.TrimSpace(raw)
	return raw != "" && !isHTTPURL(raw)
}
