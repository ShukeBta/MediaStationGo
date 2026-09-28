package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

type NFOEditTarget struct {
	Path        string `json:"path"`
	Exists      bool   `json:"exists"`
	RootElement string `json:"root_element"`
	root        string
}

func (s *MediaService) NFOEditTarget(ctx context.Context, id, scope string) (*NFOEditTarget, error) {
	m, err := s.repo.Media.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrMediaNotFound
	}
	return resolveNFOEditTarget(ctx, s.repo.DB, m, scope)
}

func resolveNFOEditTarget(ctx context.Context, db *gorm.DB, m *model.Media, scope string) (*NFOEditTarget, error) {
	if scope != "" && scope != "media" && scope != "series" {
		return nil, errors.New("invalid NFO scope")
	}
	mediaPath := resolveMappedDestinationPath(m.Path)
	if !filepath.IsAbs(mediaPath) {
		return nil, errors.New("NFO editing requires a local library file")
	}
	var lib model.Library
	if err := db.WithContext(ctx).First(&lib, "id = ?", m.LibraryID).Error; err != nil {
		return nil, err
	}
	paths := []string{lib.Path}
	if m.LibraryRootID != "" {
		var libraryRoot model.LibraryRoot
		if err := db.WithContext(ctx).First(&libraryRoot, "id = ? AND library_id = ?", m.LibraryRootID, m.LibraryID).Error; err != nil {
			return nil, err
		}
		paths = append(paths, libraryRoot.Path)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(mediaPath))
	if err != nil {
		return nil, err
	}
	root := ""
	for _, path := range paths {
		realRoot, err := filepath.EvalSymlinks(resolveMappedDestinationPath(path))
		if err == nil && filepath.Dir(realRoot) != realRoot && pathWithin(parent, realRoot) && len(realRoot) > len(root) {
			root = realRoot
		}
	}
	if root == "" {
		return nil, ErrPathOutOfBounds
	}
	mediaPath = filepath.Join(parent, filepath.Base(mediaPath))
	target := &NFOEditTarget{Path: nfoPath(mediaPath), RootElement: "movie", root: root}
	series := m.SeasonNum > 0 || m.EpisodeNum > 0 || lib.Type == "tv" || lib.Type == "anime"
	if scope == "series" {
		if !series {
			return nil, errors.New("series NFO requires a TV or anime item")
		}
		_, path, err := findShowNFO(mediaPath, root)
		if err == nil {
			target.Path = path
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		} else {
			dir := parent
			if _, ok := seasonFromDir(filepath.Base(dir)); ok {
				dir = filepath.Dir(dir)
			}
			target.Path = filepath.Join(dir, "tvshow.nfo")
		}
		target.RootElement = "tvshow"
	} else if series {
		target.RootElement = "episodedetails"
	} else {
		_, path, err := findMovieNFO(mediaPath, root)
		if err == nil {
			target.Path = path
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(target.Path))
	if err != nil || !pathWithin(realParent, root) {
		return nil, ErrPathOutOfBounds
	}
	target.Path = filepath.Join(realParent, filepath.Base(target.Path))
	info, err := os.Lstat(target.Path)
	if err == nil {
		if !info.Mode().IsRegular() || info.Size() > 16<<20 {
			return nil, errors.New("NFO must be a regular XML file smaller than 16 MiB")
		}
		target.Exists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return target, nil
}

func (s *MediaService) writeEditedNFO(ctx context.Context, db *gorm.DB, before, after *model.Media, scope string, updates map[string]any) (func() error, error) {
	if before.Path != after.Path || before.LibraryID != after.LibraryID || before.LibraryRootID != after.LibraryRootID {
		return nil, errors.New("media source changed; reopen the metadata editor")
	}
	target, err := resolveNFOEditTarget(ctx, db, before, scope)
	if err != nil {
		return nil, err
	}
	original, err := os.ReadFile(target.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if target.Exists && err != nil {
		return nil, err
	}
	input := original
	if !target.Exists {
		input = []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<" + target.RootElement + "></" + target.RootElement + ">\n")
	}
	fields := nfoEditFields(before, after, target.RootElement, !target.Exists, updates)
	updated, err := rewriteEditedNFO(input, target.RootElement, fields)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(original, updated) {
		return nil, nil
	}
	mode := os.FileMode(0644)
	if info, err := os.Stat(target.Path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := replaceEditedNFO(target, updated, mode); err != nil {
		return nil, err
	}
	return func() error {
		current, err := os.ReadFile(target.Path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, updated) {
			return errors.New("NFO changed during rollback; preserved newer contents")
		}
		if target.Exists {
			return replaceEditedNFO(target, original, mode)
		}
		root, err := os.OpenRoot(target.root)
		if err != nil {
			return err
		}
		defer root.Close()
		rel, err := filepath.Rel(target.root, target.Path)
		if err != nil {
			return err
		}
		return root.Remove(rel)
	}, nil
}

func replaceEditedNFO(target *NFOEditTarget, data []byte, mode os.FileMode) error {
	root, err := os.OpenRoot(target.root)
	if err != nil {
		return err
	}
	defer root.Close()
	rel, err := filepath.Rel(target.root, target.Path)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(rel), fmt.Sprintf(".mediastationgo-%x.nfo.tmp", rand.Text()))
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	err = errors.Join(writeErr, f.Close())
	if err != nil {
		return err
	}
	return root.Rename(tmp, rel)
}

func nfoScalar(value any) string { return strings.TrimSpace(fmt.Sprint(value)) }
