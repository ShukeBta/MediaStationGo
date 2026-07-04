package service

import (
	"context"
	"errors"
	"fmt"
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
