package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type cloudTreeManifest struct {
	EntryCount     int    `json:"entry_count"`
	DirectoryCount int    `json:"directory_count"`
	FileCount      int    `json:"file_count"`
	TotalFileSize  int64  `json:"total_file_size"`
	Fingerprint    string `json:"fingerprint"`
}

type cloudTreeManifestBuilder struct {
	mu      sync.Mutex
	rootDir string
	records []string
	dirs    int
	files   int
	bytes   int64
}

func newCloudTreeManifestBuilder(rootDir string) *cloudTreeManifestBuilder {
	return &cloudTreeManifestBuilder{rootDir: normalizeCloudMountDir("", rootDir)}
}

func (b *cloudTreeManifestBuilder) add(displayDir string, entry cloud.FileEntry) {
	if b == nil {
		return
	}
	entryPath := normalizeCloudMountDir("", joinCloudDisplayPath(displayDir, entry.Name))
	relativePath := strings.Trim(strings.TrimPrefix(entryPath, b.rootDir), "/")
	if relativePath == "" {
		relativePath = strings.TrimSpace(entry.Name)
	}
	kind := "F"
	size := entry.Size
	if entry.IsDir {
		kind = "D"
		size = 0
	}
	record := fmt.Sprintf("%s\x00%s\x00%d", kind, relativePath, size)
	b.mu.Lock()
	b.records = append(b.records, record)
	if entry.IsDir {
		b.dirs++
	} else {
		b.files++
		b.bytes += entry.Size
	}
	b.mu.Unlock()
}

func (b *cloudTreeManifestBuilder) build() cloudTreeManifest {
	if b == nil {
		return cloudTreeManifest{}
	}
	b.mu.Lock()
	records := append([]string(nil), b.records...)
	dirs, files, bytes := b.dirs, b.files, b.bytes
	b.mu.Unlock()
	sort.Strings(records)
	sum := sha256.Sum256([]byte(strings.Join(records, "\n")))
	return cloudTreeManifest{
		EntryCount:     len(records),
		DirectoryCount: dirs,
		FileCount:      files,
		TotalFileSize:  bytes,
		Fingerprint:    hex.EncodeToString(sum[:]),
	}
}

func cloudTreeManifestsEqual(left, right cloudTreeManifest) bool {
	return left.Fingerprint != "" &&
		left.Fingerprint == right.Fingerprint &&
		left.EntryCount == right.EntryCount &&
		left.DirectoryCount == right.DirectoryCount &&
		left.FileCount == right.FileCount &&
		left.TotalFileSize == right.TotalFileSize
}

func combineCloudTreeManifests(items map[string]cloudTreeManifest) cloudTreeManifest {
	if len(items) == 0 {
		return cloudTreeManifest{}
	}
	keys := make([]string, 0, len(items))
	combined := cloudTreeManifest{}
	for key, item := range items {
		keys = append(keys, key)
		combined.EntryCount += item.EntryCount
		combined.DirectoryCount += item.DirectoryCount
		combined.FileCount += item.FileCount
		combined.TotalFileSize += item.TotalFileSize
	}
	sort.Strings(keys)
	records := make([]string, 0, len(keys))
	for _, key := range keys {
		records = append(records, key+"\x00"+items[key].Fingerprint)
	}
	sum := sha256.Sum256([]byte(strings.Join(records, "\n")))
	combined.Fingerprint = hex.EncodeToString(sum[:])
	return combined
}
