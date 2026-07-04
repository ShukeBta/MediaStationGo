package service

import (
	"context"
	"encoding/xml"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type cloudSidecarSet struct {
	nfoByName   map[string]string
	nfoByBase   map[string]string
	jsonByName  map[string]string
	jsonByBase  map[string]string
	imageByName map[string]string
	imageByBase map[string]string
}

func newCloudSidecarSet(typ string, entries []cloud.FileEntry) cloudSidecarSet {
	set := cloudSidecarSet{
		nfoByName:   make(map[string]string),
		nfoByBase:   make(map[string]string),
		jsonByName:  make(map[string]string),
		jsonByBase:  make(map[string]string),
		imageByName: make(map[string]string),
		imageByBase: make(map[string]string),
	}
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		ref := cloudEntryRef(typ, entry.ID, entry.PickCode)
		if ref == "" {
			continue
		}
		name := strings.TrimSpace(entry.Name)
		ext := strings.ToLower(filepath.Ext(name))
		base := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(name, ext)))
		if name == "" || base == "" {
			continue
		}
		switch ext {
		case ".nfo":
			set.nfoByName[strings.ToLower(name)] = ref
			set.nfoByBase[base] = ref
		case ".json":
			set.jsonByName[strings.ToLower(name)] = ref
			set.jsonByBase[base] = ref
		case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tbn":
			set.imageByName[strings.ToLower(name)] = ref
			set.imageByBase[base] = ref
		}
	}
	return set
}

func (s *ScannerService) cloudDirectoryMetadata(ctx context.Context, typ, displayDir string, sidecars cloudSidecarSet, inherited *LocalMetadata) *LocalMetadata {
	meta := cloneLocalMetadata(inherited)
	if hinted, _ := pathHintMetadata(displayDir, true); hinted != nil {
		meta = mergeCloudMetadata(meta, hinted)
	}
	for _, name := range cloudShowNFOCandidates(displayDir) {
		ref := sidecars.nfoByName[strings.ToLower(name)]
		if ref == "" {
			ref = sidecars.nfoByBase[strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))]
		}
		if ref == "" {
			continue
		}
		if local, doc, err := s.readCloudNFO(ctx, typ, ref, true); err == nil && local != nil {
			local = applyCloudNFOArtwork(typ, sidecars, local, doc)
			meta = mergeCloudMetadata(meta, local)
			break
		}
	}
	for _, name := range cloudDirectoryJSONCandidates(displayDir) {
		ref := cloudJSONRefByName(sidecars, name)
		if ref == "" {
			continue
		}
		if local, err := s.readCloudJSONMetadata(ctx, typ, ref, sidecars); err == nil && local != nil {
			meta = mergeCloudMetadata(meta, local)
			break
		}
	}
	meta = applyCloudDirectoryArtwork(typ, displayDir, sidecars, meta)
	if !cloudMetadataUseful(meta) {
		return nil
	}
	return meta
}

func (s *ScannerService) cloudFileMetadata(ctx context.Context, typ, displayPath, fileName string, sidecars cloudSidecarSet, inherited *LocalMetadata, seriesLike bool) *LocalMetadata {
	season, episode := ParseEpisode(displayPath)
	seriesLike = seriesLike || season > 0 || episode > 0
	meta := cloneLocalMetadata(inherited)
	if hinted, _ := pathHintMetadata(displayPath, seriesLike); hinted != nil {
		meta = mergeCloudPathHintMetadata(meta, hinted)
	}
	base := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(fileName, filepath.Ext(fileName))))
	if ref := sidecars.nfoByBase[base]; ref != "" {
		if local, doc, err := s.readCloudNFO(ctx, typ, ref, seriesLike); err == nil && local != nil {
			local = applyCloudNFOArtwork(typ, sidecars, local, doc)
			if seriesLike && doc != nil {
				if meta == nil {
					meta = &LocalMetadata{}
				}
				mergeEpisodeMetadata(meta, local, doc)
				meta.HasNFO = true
			} else {
				meta = mergeCloudMetadata(meta, local)
			}
		}
	}
	for _, name := range cloudFileJSONCandidates(fileName, base) {
		ref := cloudJSONRefByName(sidecars, name)
		if ref == "" {
			continue
		}
		if local, err := s.readCloudJSONMetadata(ctx, typ, ref, sidecars); err == nil && local != nil {
			if cloudFileJSONIsEpisodeMetadata(seriesLike, season, episode, local) {
				meta = mergeCloudEpisodeMetadata(meta, local)
			} else {
				meta = mergeCloudMetadata(meta, local)
			}
			break
		}
	}
	meta = applyCloudFileArtwork(typ, sidecars, displayPath, fileName, base, meta)
	if !cloudMetadataUseful(meta) {
		return nil
	}
	return meta
}

func (s *ScannerService) readCloudJSONMetadata(ctx context.Context, typ, ref string, sidecars cloudSidecarSet) (*LocalMetadata, error) {
	if s.storage == nil {
		return nil, nil
	}
	body, err := s.storage.CloudReadText(ctx, typ, ref, 512<<10)
	if err != nil {
		return nil, err
	}
	meta, artwork := metadataFromCloudJSON([]byte(body))
	if meta == nil {
		return nil, nil
	}
	meta = applyCloudJSONArtwork(typ, sidecars, meta, artwork)
	return meta, nil
}

func (s *ScannerService) readCloudNFO(ctx context.Context, typ, ref string, seriesLike bool) (*LocalMetadata, *nfoDocument, error) {
	if s.storage == nil {
		return nil, nil, nil
	}
	body, err := s.storage.CloudReadText(ctx, typ, ref, 512<<10)
	if err != nil {
		return nil, nil, err
	}
	var doc nfoDocument
	if err := xml.Unmarshal([]byte(body), &doc); err != nil {
		return nil, nil, err
	}
	meta := metadataFromDoc(&doc, "", seriesLike)
	return meta, &doc, nil
}
