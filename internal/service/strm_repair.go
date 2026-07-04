package service

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type RepairSTRMOptions struct {
	OutputDir string `json:"output_dir"`
	BaseURL   string `json:"base_url,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

type RepairSTRMResult struct {
	OutputDir string             `json:"output_dir"`
	Repaired  int                `json:"repaired"`
	Previewed int                `json:"previewed,omitempty"`
	Skipped   int                `json:"skipped"`
	Errors    []string           `json:"errors,omitempty"`
	Items     []RepairSTRMItem   `json:"items,omitempty"`
	Refresh   *STRMRefreshResult `json:"refresh,omitempty"`
}

type RepairSTRMItem struct {
	FilePath string `json:"file_path"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
	Action   string `json:"action"`
	Reason   string `json:"reason,omitempty"`
}

func (s *STRMService) RepairFiles(ctx context.Context, opts RepairSTRMOptions) (*RepairSTRMResult, error) {
	outputDir := resolveMappedDestinationPath(strings.TrimSpace(opts.OutputDir))
	if outputDir == "" || outputDir == "." {
		return nil, errors.New("output_dir required")
	}
	info, err := os.Stat(outputDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("output_dir must be a directory")
	}
	result := &RepairSTRMResult{OutputDir: outputDir}
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	err = filepath.WalkDir(outputDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			result.Errors = append(result.Errors, filePath+": "+walkErr.Error())
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".strm") {
			return nil
		}
		result.addRepairItem(repairSTRMFile(filePath, baseURL, opts.DryRun))
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func repairSTRMFile(filePath, baseURL string, dryRun bool) RepairSTRMItem {
	item := RepairSTRMItem{FilePath: filePath}
	body, err := os.ReadFile(filePath) // #nosec G304 -- admin-selected STRM repair output directory.
	if err != nil {
		item.Action = "error"
		item.Reason = err.Error()
		return item
	}
	before := strings.TrimSpace(string(body))
	after, ok := repairedSTRMPlaybackURL(before, baseURL)
	if !ok {
		item.Action = "skipped"
		item.Reason = "unsupported strm target"
		return item
	}
	if after == before {
		item.Action = "skipped"
		item.Reason = "already current"
		return item
	}
	item.Before = before
	item.After = after
	if dryRun {
		item.Action = "preview"
		item.Reason = "repaired"
		return item
	}
	if err := os.WriteFile(filePath, []byte(after+"\n"), 0o644); err != nil { // #nosec G306 -- STRM files are player-readable sidecars.
		item.Action = "error"
		item.Reason = err.Error()
		return item
	}
	item.Action = "repaired"
	return item
}

func repairedSTRMPlaybackURL(raw, baseURL string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "//") {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strmRepairOwnsAPIPath(parsed.Path) {
		return "", false
	}
	apiPath := parsed.EscapedPath()
	if apiPath == "" {
		apiPath = parsed.Path
	}
	if parsed.RawQuery != "" {
		apiPath += "?" + parsed.RawQuery
	}
	return buildAbsoluteSTRMAPIURL(baseURL, apiPath, nil), true
}

func strmRepairOwnsAPIPath(apiPath string) bool {
	value := strings.ToLower(strings.TrimSpace(apiPath))
	return strings.HasPrefix(value, "/api/stream/") || strings.HasPrefix(value, "/api/cloud/play/")
}

func (r *RepairSTRMResult) addRepairItem(item RepairSTRMItem) {
	r.Items = append(r.Items, item)
	switch item.Action {
	case "repaired":
		r.Repaired++
	case "preview":
		r.Previewed++
	case "skipped":
		r.Skipped++
	case "error":
		r.Errors = append(r.Errors, item.FilePath+": "+item.Reason)
	}
}
