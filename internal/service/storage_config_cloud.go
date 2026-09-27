package service

import (
	"context"
	"errors"
	"fmt"
	pathpkg "path"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

// CloudProvider constructs a cloud-disk provider from the saved (decrypted)
// config for the given type, or returns an error if not configured.
func (s *StorageConfigService) CloudProvider(ctx context.Context, typ string) (cloud.Provider, error) {
	if !cloud.IsCloudType(typ) {
		return nil, fmt.Errorf("not a cloud provider: %q", typ)
	}
	view, err := s.Get(ctx, typ)
	if err != nil {
		return nil, err
	}
	if view == nil {
		return nil, fmt.Errorf("%s storage not configured", typ)
	}
	if !view.Enabled {
		return nil, fmt.Errorf("%s storage disabled", typ)
	}
	return cloud.New(typ, view.Config, s.clientForConfig(view.Config))
}

// CloudList lists entries under dirID for the configured cloud provider.
func (s *StorageConfigService) CloudList(ctx context.Context, typ, dirID string) ([]cloud.FileEntry, error) {
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return nil, err
	}
	return p.List(ctx, dirID)
}

// CloudListRefresh lists entries while explicitly refreshing the upstream cache
// when the provider supports it.
func (s *StorageConfigService) CloudListRefresh(ctx context.Context, typ, dirID string) ([]cloud.FileEntry, error) {
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return nil, err
	}
	refresher, ok := p.(cloud.RefreshableProvider)
	if !ok {
		return nil, fmt.Errorf("%s storage does not support refresh list", typ)
	}
	return refresher.ListRefresh(ctx, dirID)
}

func (s *StorageConfigService) CloudMkdir(ctx context.Context, typ, parentDir, name string) (*cloud.FileEntry, error) {
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return nil, err
	}
	mutable, ok := p.(cloud.MutableProvider)
	if !ok {
		return nil, fmt.Errorf("%s does not support folder creation", typ)
	}
	return mutable.Mkdir(ctx, parentDir, name)
}

func (s *StorageConfigService) CloudRename(ctx context.Context, typ, ref, name string) (*cloud.FileEntry, error) {
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return nil, err
	}
	mutable, ok := p.(cloud.MutableProvider)
	if !ok {
		return nil, fmt.Errorf("%s does not support rename", typ)
	}
	return mutable.Rename(ctx, ref, name)
}

func (s *StorageConfigService) CloudMove(ctx context.Context, typ, ref, targetDir, name string) (*cloud.FileEntry, error) {
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return nil, err
	}
	movable, ok := p.(cloud.MovableProvider)
	if !ok {
		return nil, fmt.Errorf("%s does not support move", typ)
	}
	return movable.Move(ctx, ref, targetDir, name)
}

func (s *StorageConfigService) DeleteCloudFile(ctx context.Context, typ, ref string) error {
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return err
	}
	deletable, ok := p.(cloud.DeletableProvider)
	if !ok {
		return fmt.Errorf("%s does not support file deletion", typ)
	}
	if err := deletable.Delete(ctx, ref); err != nil {
		return err
	}
	s.clearResolveCacheForType(typ)
	return nil
}

// PruneEmptyCloudParents removes empty directories between a deleted cloud
// file and the owning library root. The root itself is never removed.
func (s *StorageConfigService) PruneEmptyCloudParents(ctx context.Context, typ, ref, rootRef string) error {
	if typ != cloud.TypeOpenList {
		return fmt.Errorf("empty cloud parent cleanup is unsupported for %s", typ)
	}
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return err
	}
	deletable, ok := p.(cloud.DeletableProvider)
	if !ok {
		return fmt.Errorf("%s does not support directory deletion", typ)
	}
	return pruneEmptyCloudParentDirectories(ctx, p, deletable, ref, rootRef)
}

func pruneEmptyCloudParentDirectories(ctx context.Context, p cloud.Provider, deletable cloud.DeletableProvider, ref, rootRef string) error {
	root := normalizeCloudPath(rootRef)
	current := normalizeCloudPath(pathpkg.Dir(normalizeCloudPath(ref)))
	if root == "/" || current == "/" || !cloudPathWithin(current, root) {
		return fmt.Errorf("refusing to prune cloud path outside library root: %q", ref)
	}
	for current != root {
		var entries []cloud.FileEntry
		var err error
		if refresher, ok := p.(cloud.RefreshableProvider); ok {
			entries, err = refresher.ListRefresh(ctx, current)
		} else {
			entries, err = p.List(ctx, current)
		}
		if err != nil {
			if cloud.IsOpenListAlreadyAbsentError(err) {
				current = normalizeCloudPath(pathpkg.Dir(current))
				continue
			}
			return fmt.Errorf("list cloud directory %s: %w", current, err)
		}
		if len(entries) > 0 {
			return nil
		}
		if err := deletable.Delete(ctx, current); err != nil {
			return fmt.Errorf("delete empty cloud directory %s: %w", current, err)
		}
		current = normalizeCloudPath(pathpkg.Dir(current))
	}
	return nil
}

func normalizeCloudPath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || value == "." {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	cleaned := pathpkg.Clean(value)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func cloudPathWithin(pathValue, root string) bool {
	pathValue = strings.TrimRight(normalizeCloudPath(pathValue), "/")
	root = strings.TrimRight(normalizeCloudPath(root), "/")
	return pathValue == root || strings.HasPrefix(pathValue, root+"/")
}

// cloudLibraryName maps a provider type to a friendly Chinese library name.
func cloudLibraryName(typ string) string {
	switch typ {
	case cloud.Type115:
		return "115 网盘"
	case cloud.TypeCloudDrive2:
		return "CloudDrive2"
	case cloud.TypeOpenList:
		return "OpenList"
	default:
		return typ
	}
}

// ensureCloudLibrary returns (creating if necessary) the per-provider cloud
// library that owns imported 302 media.
func (s *StorageConfigService) ensureCloudLibrary(ctx context.Context, typ string) (*model.Library, error) {
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	path := "cloud://" + typ
	for i := range libs {
		if libs[i].Path == path {
			return &libs[i], nil
		}
	}
	lib := &model.Library{Name: cloudLibraryName(typ), Path: path, Type: "movie", Enabled: true}
	if err := s.repo.Library.Create(ctx, lib); err != nil {
		return nil, err
	}
	return lib, nil
}

// CloudImport creates (or refreshes) a playable media row backed by a cloud
// file. Playback is served entirely via 302 redirect — the host never streams
// the bytes (unless the provider requires proxy mode).
func (s *StorageConfigService) CloudImport(ctx context.Context, typ, fileRef, name string, size int64) (*model.Media, error) {
	if !cloud.IsCloudType(typ) {
		return nil, fmt.Errorf("not a cloud provider: %q", typ)
	}
	if strings.TrimSpace(fileRef) == "" {
		return nil, errors.New("file reference required")
	}
	lib, err := s.ensureCloudLibrary(ctx, typ)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(name)
	container := ""
	if i := strings.LastIndex(title, "."); i > 0 {
		container = strings.ToLower(strings.TrimPrefix(title[i:], "."))
		title = title[:i]
	}
	if title == "" {
		title = fileRef
	}
	m := &model.Media{
		LibraryID:    lib.ID,
		Title:        title,
		Path:         cloudMediaPath(typ, fileRef),
		SizeBytes:    size,
		Container:    container,
		STRMURL:      BuildRelativeCloudPlayURL(typ, fileRef),
		ScrapeStatus: "pending",
	}
	if err := s.repo.Media.Upsert(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}
