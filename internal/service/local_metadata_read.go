package service

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ReadLocalMetadata reads sidecar NFO files for a media path. For TV/anime it
// merges show-level tvshow.nfo with episode-level sidecar metadata.
func ReadLocalMetadata(mediaPath, libraryRoot string, seriesLike bool) (*LocalMetadata, error) {
	if seriesLike {
		return readSeriesMetadata(mediaPath, libraryRoot)
	}
	doc, path, err := findMovieNFO(mediaPath, libraryRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return metadataFromArtwork(mediaPath, ""), nil
		}
		return nil, err
	}
	meta := metadataFromDoc(doc, filepath.Dir(path), false)
	mergeArtworkMetadata(meta, mediaPath, filepath.Dir(path))
	return meta, nil
}

func findMovieNFO(mediaPath, libraryRoot string) (*nfoDocument, string, error) {
	mediaDir := filepath.Dir(mediaPath)
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	adultCode := AdultCodeFromMediaPath(mediaPath)
	names := []string{
		base + ".nfo",
		"movie.nfo",
		filepath.Base(mediaDir) + ".nfo",
	}
	if adultCode != "" {
		names = append([]string{adultCode + ".nfo", strings.ReplaceAll(adultCode, "-", "") + ".nfo"}, names...)
	}
	seen := map[string]struct{}{}
	for _, name := range names {
		if name == ".nfo" || name == "" {
			continue
		}
		path := filepath.Join(mediaDir, name)
		key := strings.ToLower(filepath.Clean(path))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if doc, _, err := readNFO(path); err == nil {
			return doc, path, nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
	}
	if libraryRoot == "" || !samePath(mediaDir, filepath.Clean(libraryRoot)) {
		matches, _ := filepath.Glob(filepath.Join(mediaDir, "*.nfo"))
		if adultCode != "" {
			codeKey := strings.ToLower(strings.ReplaceAll(adultCode, "-", ""))
			for _, match := range matches {
				baseKey := strings.ToLower(strings.ReplaceAll(strings.TrimSuffix(filepath.Base(match), filepath.Ext(match)), "-", ""))
				if strings.Contains(baseKey, codeKey) || strings.Contains(codeKey, baseKey) {
					if doc, _, err := readNFO(match); err == nil {
						return doc, match, nil
					} else if err != nil && !errors.Is(err, os.ErrNotExist) {
						return nil, "", err
					}
				}
			}
		}
		if len(matches) == 1 {
			if doc, _, err := readNFO(matches[0]); err == nil {
				return doc, matches[0], nil
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, "", err
			}
		}
	}
	return nil, "", os.ErrNotExist
}

func readSeriesMetadata(mediaPath, libraryRoot string) (*LocalMetadata, error) {
	var meta *LocalMetadata
	showBaseDir := ""
	if showDoc, showPath, err := findShowNFO(mediaPath, libraryRoot); err == nil && showDoc != nil {
		showBaseDir = filepath.Dir(showPath)
		meta = metadataFromDoc(showDoc, showBaseDir, true)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if episodeDoc, episodePath, err := readNFO(nfoPath(mediaPath)); err == nil {
		episodeMeta := metadataFromDoc(episodeDoc, filepath.Dir(episodePath), true)
		if meta == nil {
			meta = &LocalMetadata{}
		}
		mergeEpisodeMetadata(meta, episodeMeta, episodeDoc)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if meta == nil {
		meta = metadataFromArtwork(mediaPath, showBaseDir)
	} else {
		mergeArtworkMetadata(meta, mediaPath, showBaseDir)
	}
	return meta, nil
}

func readNFO(path string) (*nfoDocument, string, error) {
	body, err := os.ReadFile(path) // #nosec G304 -- path is a discovered NFO sidecar under the configured library root.
	if err != nil {
		return nil, "", err
	}
	var doc nfoDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, "", err
	}
	return &doc, path, nil
}

func findShowNFO(mediaPath, libraryRoot string) (*nfoDocument, string, error) {
	dir := filepath.Dir(mediaPath)
	root := filepath.Clean(libraryRoot)
	for {
		names := []string{"tvshow.nfo", "series.nfo"}
		base := filepath.Base(dir)
		if _, ok := seasonFromDir(base); ok {
			parentBase := filepath.Base(filepath.Dir(dir))
			names = append(names, parentBase+".nfo")
		}
		names = append(names, base+".nfo")
		for _, name := range names {
			path := filepath.Join(dir, name)
			if doc, _, err := readNFO(path); err == nil {
				return doc, path, nil
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, "", err
			}
		}
		if samePath(dir, root) {
			return nil, "", os.ErrNotExist
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, "", os.ErrNotExist
		}
		dir = parent
	}
}
