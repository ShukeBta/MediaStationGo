package service

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

func (s *ScannerService) resolveCloudSTRMTarget(ctx context.Context, typ, ref string) (string, error) {
	if s.storage == nil {
		return "", nil
	}
	content, err := s.storage.CloudReadText(ctx, typ, ref, 64<<10)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(content, "\n") {
		candidate := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if candidate == "" || strings.HasPrefix(candidate, "#") {
			continue
		}
		u, err := url.Parse(candidate)
		if err != nil {
			continue
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https", "webdav", "davs", "alist", "alists", "openlist", "openlists":
			return candidate, nil
		}
	}
	return "", nil
}

func readLocalSTRMTarget(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a discovered .strm file under the configured library root.
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		candidate := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if candidate == "" || strings.HasPrefix(candidate, "#") {
			continue
		}
		if strings.HasPrefix(candidate, "/api/") || strings.HasPrefix(candidate, "/Videos/") || strings.HasPrefix(candidate, "/videos/") {
			return candidate, nil
		}
		u, err := url.Parse(candidate)
		if err != nil {
			continue
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https", "webdav", "davs", "alist", "alists", "openlist", "openlists":
			return candidate, nil
		}
	}
	return "", nil
}

func (s *ScannerService) maybeGenerateSTRMAfterScan(libraryID string) {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return
	}
	value, err := s.repo.Setting.Get(context.Background(), "strm.auto_generate_enabled")
	if err != nil || !parseBoolSetting(value, false) {
		return
	}
	go func() {
		ctx := context.Background()
		strmSvc := NewSTRMService(s.log, s.repo, s.cfg)
		opts := GenerateSTRMOptions{
			LibraryID:        libraryID,
			Enabled:          true,
			IncludeLocal:     true,
			Overwrite:        true,
			PreserveTree:     s.autoSTRMPreserveTree(ctx),
			SkipSettingsSave: true,
		}
		if outDir, scope := s.autoSTRMOutputDir(ctx); outDir != "" {
			opts.OutputDir = outDir
			if scope == "all" {
				if lib, err := s.repo.Library.FindByID(ctx, libraryID); err == nil && lib != nil {
					opts.OutputDir = filepath.Join(outDir, strmLibraryOutputSubdir(*lib))
				}
			}
		}
		if _, err := strmSvc.GenerateForLibrary(ctx, opts); err != nil && s.log != nil {
			s.log.Warn("auto generate strm failed", zap.String("library_id", libraryID), zap.Error(err))
		}
	}()
}

func (s *ScannerService) autoSTRMOutputDir(ctx context.Context) (string, string) {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return "", ""
	}
	outDir, err := s.repo.Setting.Get(ctx, "strm.output_dir")
	if err != nil {
		return "", ""
	}
	scope, _ := s.repo.Setting.Get(ctx, "strm.output_scope")
	return resolveMappedDestinationPath(strings.TrimSpace(outDir)), strings.ToLower(strings.TrimSpace(scope))
}

func (s *ScannerService) autoSTRMPreserveTree(ctx context.Context) bool {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return false
	}
	value, err := s.repo.Setting.Get(ctx, "strm.preserve_tree")
	return err == nil && parseBoolSetting(value, false)
}
