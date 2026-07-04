package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	CloudUploadAutoEnabledKey      = "cloud.upload_auto_enabled"
	CloudUploadProviderKey         = "cloud.upload_provider"
	CloudUploadSourceDirKey        = "cloud.upload_source_dir"
	CloudUploadDestPathKey         = "cloud.upload_dest_path"
	CloudUploadRecursiveKey        = "cloud.upload_recursive"
	CloudUploadSidecarsKey         = "cloud.upload_sidecars"
	CloudUploadOverwriteKey        = "cloud.upload_overwrite"
	CloudUploadTransferModeKey     = "cloud.upload_transfer_mode"
	CloudUploadIntervalSecondsKey  = "cloud.upload_interval_seconds"
	CloudUploadUnsupportedProvider = "本地文件直传目前支持 Alist / OpenList / WebDAV / CloudDrive2；115 原生上传需要分片上传私有接口，建议先用 CloudDrive2、OpenList 或 Alist 桥接后转存。"
)

type CloudUploadInput struct {
	Type            string `json:"type"`
	SourcePath      string `json:"source_path"`
	DestPath        string `json:"dest_path"`
	Recursive       bool   `json:"recursive"`
	IncludeSidecars bool   `json:"include_sidecars"`
	Overwrite       bool   `json:"overwrite"`
	TransferMode    string `json:"transfer_mode"`
}

type CloudUploadResult struct {
	SourcePath string                  `json:"source_path"`
	DestPath   string                  `json:"dest_path"`
	Uploaded   int                     `json:"uploaded"`
	Moved      int                     `json:"moved,omitempty"`
	Skipped    int                     `json:"skipped"`
	Bytes      int64                   `json:"bytes"`
	Errors     []string                `json:"errors,omitempty"`
	Items      []CloudUploadResultItem `json:"items,omitempty"`
}

type CloudUploadResultItem struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Action string `json:"action"` // upload / move / skip / error
	Size   int64  `json:"size,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type storageUploader interface {
	ensureDir(ctx context.Context, remoteDir string) error
	exists(ctx context.Context, remotePath string) (bool, error)
	upload(ctx context.Context, localPath, remotePath string, size int64) error
}

var cloudUploadSidecarExtensions = map[string]struct{}{
	".nfo": {}, ".jpg": {}, ".jpeg": {}, ".png": {}, ".webp": {},
	".srt": {}, ".ass": {}, ".ssa": {}, ".vtt": {}, ".sub": {}, ".idx": {},
}

// UploadLocal copies or moves local media files into an external storage
// backend. Cloud writes are intentionally gated by the per-storage
// transfer_enabled switch so mounting/scanning a cloud provider does not
// accidentally make it writable.
func (s *StorageConfigService) UploadLocal(ctx context.Context, in CloudUploadInput) (*CloudUploadResult, error) {
	in.Type = strings.TrimSpace(in.Type)
	in.SourcePath = strings.TrimSpace(in.SourcePath)
	in.DestPath = normalizeRemotePath(in.DestPath)
	if in.SourcePath == "" {
		return nil, errors.New("source_path required")
	}
	view, err := s.Get(ctx, in.Type)
	if err != nil {
		return nil, err
	}
	if view == nil || !view.Enabled {
		return nil, fmt.Errorf("%s storage not configured", in.Type)
	}
	if !parseBoolSetting(strr(view.Config["transfer_enabled"]), false) {
		return nil, errors.New("cloud transfer is disabled for this storage; enable transfer in external storage settings before writing to cloud")
	}
	mode := resolveCloudUploadTransferMode(in.TransferMode, strr(view.Config["transfer_mode"]), s.settingValue(ctx, CloudUploadTransferModeKey))
	if mode != TransferCopy && mode != TransferMove {
		return nil, errors.New("transfer_mode must be copy or move")
	}
	in.TransferMode = string(mode)
	uploader, err := s.uploaderForView(in.Type, view)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(in.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("source path not accessible: %w", err)
	}
	result := &CloudUploadResult{SourcePath: in.SourcePath, DestPath: in.DestPath}
	if !info.IsDir() {
		s.uploadOne(ctx, uploader, in, in.SourcePath, filepath.Base(in.SourcePath), info.Size(), result)
		return result, firstUploadError(result)
	}
	root := filepath.Clean(in.SourcePath)
	walkFn := func(localPath string, entryInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			addUploadError(result, localPath, "", walkErr)
			return nil
		}
		if entryInfo == nil || entryInfo.IsDir() {
			if !in.Recursive && filepath.Clean(localPath) != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !eligibleCloudUploadFile(localPath, in.IncludeSidecars) {
			return nil
		}
		rel, err := filepath.Rel(root, localPath)
		if err != nil {
			addUploadError(result, localPath, "", err)
			return nil
		}
		s.uploadOne(ctx, uploader, in, localPath, filepath.ToSlash(rel), entryInfo.Size(), result)
		return nil
	}
	if err := filepath.Walk(in.SourcePath, walkFn); err != nil {
		return result, err
	}
	return result, firstUploadError(result)
}

func (s *StorageConfigService) uploader(ctx context.Context, typ string) (storageUploader, error) {
	view, err := s.Get(ctx, typ)
	if err != nil {
		return nil, err
	}
	if view == nil || !view.Enabled {
		return nil, fmt.Errorf("%s storage not configured", typ)
	}
	return s.uploaderForView(typ, view)
}

func (s *StorageConfigService) uploaderForView(typ string, view *StorageView) (storageUploader, error) {
	if view == nil || !view.Enabled {
		return nil, fmt.Errorf("%s storage not configured", typ)
	}
	switch typ {
	case "alist":
		return newAlistUploader(view.Config), nil
	case "openlist":
		if strings.TrimSpace(strr(view.Config["server"])) != "" {
			return newNamedAlistUploader("openlist", view.Config), nil
		}
		return newWebDAVUploader(view.Config), nil
	case "webdav":
		return newWebDAVUploader(view.Config), nil
	case "clouddrive2":
		return newWebDAVUploader(view.Config), nil
	case "s3":
		return nil, errors.New("s3 local upload is not implemented yet")
	case "cloud115":
		return nil, errors.New(CloudUploadUnsupportedProvider)
	default:
		return nil, fmt.Errorf("unsupported storage type %q", typ)
	}
}

func (s *StorageConfigService) uploadOne(ctx context.Context, uploader storageUploader, in CloudUploadInput, localPath, rel string, size int64, result *CloudUploadResult) {
	remotePath := joinRemotePath(in.DestPath, rel)
	if err := uploader.ensureDir(ctx, path.Dir(remotePath)); err != nil {
		addUploadError(result, localPath, remotePath, err)
		return
	}
	if !in.Overwrite {
		exists, err := uploader.exists(ctx, remotePath)
		if err != nil {
			addUploadError(result, localPath, remotePath, err)
			return
		}
		if exists {
			result.Skipped++
			addUploadItem(result, CloudUploadResultItem{Source: localPath, Target: remotePath, Action: "skip", Size: size, Reason: "remote exists"})
			return
		}
	}
	if err := uploader.upload(ctx, localPath, remotePath, size); err != nil {
		addUploadError(result, localPath, remotePath, err)
		return
	}
	result.Uploaded++
	result.Bytes += size
	action := "upload"
	mode := resolveCloudUploadTransferMode(in.TransferMode)
	if mode == TransferMove {
		if err := os.Remove(localPath); err != nil {
			addUploadError(result, localPath, remotePath, fmt.Errorf("uploaded but failed to remove local source: %w", err))
			addUploadItem(result, CloudUploadResultItem{Source: localPath, Target: remotePath, Action: "upload", Size: size, Reason: "source remove failed"})
			return
		}
		result.Moved++
		action = "move"
	}
	addUploadItem(result, CloudUploadResultItem{Source: localPath, Target: remotePath, Action: action, Size: size})
}

func (s *StorageConfigService) settingValue(ctx context.Context, key string) string {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return ""
	}
	v, _ := s.repo.Setting.Get(ctx, key)
	return strings.TrimSpace(v)
}

func resolveCloudUploadTransferMode(values ...string) TransferMode {
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "move", "移动":
			return TransferMove
		case "copy", "复制":
			return TransferCopy
		}
	}
	return TransferCopy
}

func eligibleCloudUploadFile(localPath string, includeSidecars bool) bool {
	ext := strings.ToLower(filepath.Ext(localPath))
	if _, ok := videoExtensions[ext]; ok {
		return true
	}
	if includeSidecars {
		_, ok := cloudUploadSidecarExtensions[ext]
		return ok
	}
	return false
}

func addUploadError(result *CloudUploadResult, source, target string, err error) {
	result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", source, err))
	addUploadItem(result, CloudUploadResultItem{Source: source, Target: target, Action: "error", Reason: err.Error()})
}

func addUploadItem(result *CloudUploadResult, item CloudUploadResultItem) {
	if len(result.Items) < 200 {
		result.Items = append(result.Items, item)
	}
}

func firstUploadError(result *CloudUploadResult) error {
	if result.Uploaded > 0 || len(result.Errors) == 0 {
		return nil
	}
	return errors.New(result.Errors[0])
}

func normalizeRemotePath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	if p == "" || p == "." {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

func joinRemotePath(base, rel string) string {
	parts := []string{normalizeRemotePath(base)}
	for _, part := range strings.Split(strings.ReplaceAll(rel, "\\", "/"), "/") {
		part = strings.TrimSpace(part)
		if part != "" && part != "." {
			parts = append(parts, part)
		}
	}
	return path.Clean(path.Join(parts...))
}
