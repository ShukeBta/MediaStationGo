package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrSTRMTargetNotDeletable = errors.New("STRM target is not a deletable local file")
var ErrSTRMTargetChanged = errors.New("STRM target changed; preview the target again")

type STRMDeleteTarget struct {
	TargetPath   string `json:"target_path"`
	ParentPath   string `json:"parent_path,omitempty"`
	Confirmation string `json:"confirmation"`
	trustedRoot  string
}

// ResolveSTRMDeleteTarget 从具体媒体的 sidecar 和当前路径映射解析可安全删除的本地目标。
func (s *FileManagerService) ResolveSTRMDeleteTarget(ctx context.Context, mediaID string) (*STRMDeleteTarget, error) {
	if s == nil || s.repo == nil || s.repo.Media == nil {
		return nil, ErrMediaNotFound
	}
	media, err := s.repo.Media.FindByID(ctx, strings.TrimSpace(mediaID))
	if err != nil {
		return nil, err
	}
	if media == nil {
		return nil, ErrMediaNotFound
	}
	if !isLocalSTRMFile(media.Path) {
		return nil, ErrSTRMTargetNotDeletable
	}
	rawTarget, err := readLocalSTRMTarget(media.Path)
	if err != nil {
		return nil, err
	}

	var target string
	var trustedRoots []string
	if local, ok := localSTRMTargetPath(media.Path, rawTarget); ok {
		target = local
		roots, _, rootsErr := s.allowedRootList()
		if rootsErr != nil {
			return nil, rootsErr
		}
		for _, root := range roots {
			trustedRoots = append(trustedRoots, root)
		}
	} else if isHTTPPlaybackTarget(rawTarget) {
		rawMappings := ""
		if s.repo.Setting != nil {
			rawMappings, _ = s.repo.Setting.Get(ctx, FFprobePathMappingsSettingKey)
		}
		var mappingRoot string
		target, mappingRoot = mapRemoteProbePathWithRoot(rawMappings, rawTarget)
		if mappingRoot != "" {
			trustedRoots = []string{mappingRoot}
		}
	}
	if target == "" || len(trustedRoots) == 0 {
		return nil, ErrSTRMTargetNotDeletable
	}

	target, trustedRoot, err := secureSTRMDeleteTarget(target, trustedRoots)
	if err != nil {
		return nil, err
	}
	if _, ok := videoExtensions[strings.ToLower(filepath.Ext(target))]; !ok || isLocalSTRMFile(target) {
		return nil, ErrSTRMTargetNotDeletable
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	result := &STRMDeleteTarget{TargetPath: target, trustedRoot: trustedRoot}
	parent := filepath.Dir(target)
	sidecar, _ := filepath.Abs(media.Path)
	realSidecar, _ := filepath.EvalSymlinks(sidecar)
	sidecarInsideParent := pathWithin(sidecar, parent) || (realSidecar != "" && pathWithin(realSidecar, parent))
	if parent != target && !strings.EqualFold(parent, trustedRoot) && pathWithin(parent, trustedRoot) && !sidecarInsideParent {
		result.ParentPath = parent
	}
	result.Confirmation = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d", rawTarget, target, result.ParentPath, info.Size(), info.ModTime().UnixNano()))))
	return result, nil
}

// DeleteSTRMTarget 只删除 STRM 指向的本地文件或其父目录，不修改 sidecar 和数据库。
func (s *FileManagerService) DeleteSTRMTarget(ctx context.Context, mediaID string, deleteParent bool, confirmation string) (string, error) {
	resolved, err := s.ResolveSTRMDeleteTarget(ctx, mediaID)
	if err != nil {
		return "", err
	}
	if confirmation == "" || confirmation != resolved.Confirmation {
		return "", ErrSTRMTargetChanged
	}
	root, err := os.OpenRoot(resolved.trustedRoot)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if !deleteParent {
		relative, err := filepath.Rel(resolved.trustedRoot, resolved.TargetPath)
		if err != nil {
			return "", err
		}
		info, err := root.Lstat(relative)
		if err != nil || !info.Mode().IsRegular() {
			if err != nil {
				return "", err
			}
			return "", ErrSTRMTargetNotDeletable
		}
		return resolved.TargetPath, root.Remove(relative)
	}
	if resolved.ParentPath == "" {
		return "", ErrRootMutation
	}
	relative, err := filepath.Rel(resolved.trustedRoot, resolved.ParentPath)
	if err != nil || relative == "." || !probePathWithin(resolved.trustedRoot, resolved.ParentPath) {
		return "", ErrRootMutation
	}
	return resolved.ParentPath, root.RemoveAll(relative)
}

func secureSTRMDeleteTarget(target string, trustedRoots []string) (string, string, error) {
	absTarget, err := filepath.Abs(strings.TrimSpace(target))
	if err != nil {
		return "", "", err
	}
	info, err := os.Lstat(absTarget)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		if err != nil {
			return "", "", err
		}
		return "", "", ErrPathOutOfBounds
	}
	realTarget, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		return "", "", err
	}
	info, err = os.Stat(realTarget)
	if err != nil || !info.Mode().IsRegular() {
		if err != nil {
			return "", "", err
		}
		return "", "", ErrSTRMTargetNotDeletable
	}

	bestRoot := ""
	for _, root := range trustedRoots {
		absRoot, err := filepath.Abs(strings.TrimSpace(root))
		if err != nil {
			continue
		}
		realRoot, err := filepath.EvalSymlinks(absRoot)
		if err != nil || filepath.Dir(realRoot) == realRoot || !pathWithin(realTarget, realRoot) {
			continue
		}
		if len(realRoot) > len(bestRoot) {
			bestRoot = filepath.Clean(realRoot)
		}
	}
	if bestRoot == "" {
		return "", "", ErrPathOutOfBounds
	}
	return filepath.Clean(realTarget), bestRoot, nil
}
