package service

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type GenerateSTRMTreeOptions struct {
	Provider          string   `json:"provider"`
	TreeText          string   `json:"tree_text,omitempty"`
	Paths             []string `json:"paths,omitempty"`
	SourceRoot        string   `json:"source_root,omitempty"`
	OutputPrefix      string   `json:"output_prefix,omitempty"`
	OutputDir         string   `json:"output_dir"`
	BaseURL           string   `json:"base_url,omitempty"`
	Overwrite         bool     `json:"overwrite"`
	Cleanup           bool     `json:"cleanup"`
	DryRun            bool     `json:"dry_run"`
	BatchLimit        int      `json:"batch_limit,omitempty"`
	RecognizeRename   bool     `json:"recognize_rename,omitempty"`
	TransferSubtitles bool     `json:"transfer_subtitles,omitempty"`
	MissingOnly       bool     `json:"missing_only,omitempty"`
}

type strmTreeSource struct {
	Provider string
	Path     string
	RefPath  string
	Kind     string
}

const (
	strmTreeSourceKindVideo    = "video"
	strmTreeSourceKindSubtitle = "subtitle"
)

type strmTreeSourceCollection struct {
	sources      []strmTreeSource
	ignored      []string
	ignoredCount int
}

type strmTreeSourceCollector struct {
	fallbackProvider  string
	transferSubtitles bool
	sources           []strmTreeSource
	subtitles         []strmTreeSource
	ignored           []string
	ignoredCount      int
	seen              map[string]struct{}
	seenIgnored       map[string]struct{}
}

func (s *STRMService) GenerateFromTree(ctx context.Context, opts GenerateSTRMTreeOptions) (*GenerateSTRMResult, error) {
	provider := normalizeSTRMTreeProvider(opts.Provider)
	if provider == "" {
		return nil, errors.New("provider required")
	}
	outputDir := resolveMappedDestinationPath(strings.TrimSpace(opts.OutputDir))
	if outputDir == "" || outputDir == "." {
		return nil, errors.New("output_dir required")
	}
	if !opts.DryRun {
		if err := os.MkdirAll(outputDir, 0o755); err != nil { // #nosec G301 -- STRM output directories must be readable by media players.
			return nil, err
		}
	}
	result := &GenerateSTRMResult{LibraryID: provider, OutputDir: outputDir}
	collection := collectSTRMTreeSources(opts)
	sources := collection.sources
	result.Total = len(sources)
	result.Ignored = collection.ignoredCount
	result.IgnoredItems = collection.ignored
	existingRefs, err := s.strmTreeExistingCloudRefs(ctx, opts)
	if err != nil {
		return nil, err
	}
	expectedFiles := make(map[string]struct{})
	for i, source := range sources {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		item := s.generateTreeSTRMItem(ctx, outputDir, source, opts, existingRefs)
		if item.FilePath != "" && item.Action != "error" {
			expectedFiles[filepath.Clean(item.FilePath)] = struct{}{}
		}
		result.addItem(item)
		if strmTreeBatchLimitReached(result, opts.BatchLimit) {
			result.Remaining = len(sources) - i - 1
			result.BatchLimited = result.Remaining > 0
			break
		}
	}
	if opts.Cleanup && !opts.DryRun && opts.BatchLimit <= 0 && len(expectedFiles) > 0 {
		cleanupDir := outputDir
		if prefix, err := strmTreeOutputPrefixPath(opts.OutputPrefix); err == nil && prefix != "" {
			cleanupDir = filepath.Join(outputDir, prefix)
		}
		cleaned, err := removeStaleSTRMFiles(cleanupDir, expectedFiles)
		result.Cleaned += cleaned
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
	}
	return result, nil
}

func strmTreeBatchLimitReached(result *GenerateSTRMResult, limit int) bool {
	if result == nil || limit <= 0 {
		return false
	}
	return result.Generated+result.Updated+result.Previewed >= limit
}

func (s *STRMService) generateTreeSTRMItem(ctx context.Context, outputDir string, source strmTreeSource, opts GenerateSTRMTreeOptions, existingRefs map[string]struct{}) GenerateSTRMItem {
	relSource := strmTreeRelativeSource(source.Path, opts.SourceRoot)
	relPath, err := strmTreeOutputRelativePath(relSource)
	if opts.RecognizeRename && source.Kind != strmTreeSourceKindSubtitle {
		if renamed, renameErr := s.strmTreeRecognizedOutputRelativePath(ctx, source.Path, opts.SourceRoot); renameErr == nil && renamed != "" {
			relPath = renamed
			err = nil
		}
	}
	if source.Kind == strmTreeSourceKindSubtitle {
		relPath, err = strmTreeOutputSubtitleLinkRelativePath(relSource)
	}
	item := GenerateSTRMItem{Title: strings.TrimSuffix(path.Base(source.Path), path.Ext(source.Path))}
	if err != nil {
		item.Action = "error"
		item.Reason = err.Error()
		return item
	}
	prefix, err := strmTreeOutputPrefixPath(opts.OutputPrefix)
	if err != nil {
		item.Action = "error"
		item.Reason = err.Error()
		return item
	}
	filePath := filepath.Join(outputDir, prefix, relPath)
	item.FilePath = filePath
	item.URL = absolutizeSTRMURL(BuildRelativeCloudPlayURL(source.Provider, strmTreeCloudRef(source.cloudRefPath(), opts.SourceRoot)), opts.BaseURL)
	if strmTreeSourceAlreadyInLibrary(source, opts, existingRefs) {
		item.Action = "skipped"
		item.Reason = "already in media library"
		return item
	}
	if _, err := os.Stat(filePath); err == nil && !opts.Overwrite {
		item.Action = "skipped"
		item.Reason = "target exists"
		return item
	}
	action := "generated"
	if _, err := os.Stat(filePath); err == nil {
		action = "updated"
	}
	if opts.DryRun {
		item.Action = "preview"
		item.Reason = action
		return item
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil { // #nosec G301 -- STRM output directories must be readable by media players.
		item.Action = "error"
		item.Reason = err.Error()
		return item
	}
	if err := os.WriteFile(filePath, []byte(item.URL+"\n"), 0o644); err != nil { // #nosec G306 -- STRM files are media sidecars intended to be readable by players.
		item.Action = "error"
		item.Reason = err.Error()
		return item
	}
	item.Action = action
	return item
}

func collectSTRMTreeSources(opts GenerateSTRMTreeOptions) strmTreeSourceCollection {
	collector := newSTRMTreeSourceCollector(opts)
	for _, value := range opts.Paths {
		collector.add(value)
	}
	treeSources, treeIgnored := parseSTRMTreeTextWithIgnored(opts.TreeText)
	for _, value := range treeSources {
		collector.add(value)
	}
	for _, value := range treeIgnored {
		collector.addIgnoredOrSubtitle(value)
	}
	collector.finalizeSubtitles()
	return collector.collection()
}

func newSTRMTreeSourceCollector(opts GenerateSTRMTreeOptions) *strmTreeSourceCollector {
	return &strmTreeSourceCollector{
		fallbackProvider:  normalizeSTRMTreeProvider(opts.Provider),
		transferSubtitles: opts.TransferSubtitles,
		sources:           make([]strmTreeSource, 0, len(opts.Paths)),
		subtitles:         make([]strmTreeSource, 0),
		ignored:           make([]string, 0),
		seen:              map[string]struct{}{},
		seenIgnored:       map[string]struct{}{},
	}
}

func (c *strmTreeSourceCollector) add(value string) {
	source := normalizeSTRMTreeSourceWithProvider(value, c.fallbackProvider)
	if source.Provider == "" || source.Path == "" || !strmTreeSourceIsVideo(source.Path) {
		c.addIgnoredOrSubtitle(value)
		return
	}
	source.Kind = strmTreeSourceKindVideo
	c.addSource(source)
}

func (c *strmTreeSourceCollector) addIgnoredOrSubtitle(value string) {
	if c.transferSubtitles && c.addSubtitleCandidate(value) {
		return
	}
	c.addIgnored(value)
}

func (c *strmTreeSourceCollector) addSource(source strmTreeSource) {
	if source.Kind == "" {
		source.Kind = strmTreeSourceKindVideo
	}
	key := strings.ToLower(source.Provider) + "\x00" + strings.ToLower(source.Kind) + "\x00" + strings.ToLower(source.Path) + "\x00" + strings.ToLower(source.cloudRefPath())
	if _, ok := c.seen[key]; ok {
		return
	}
	c.seen[key] = struct{}{}
	c.sources = append(c.sources, source)
}

func (c *strmTreeSourceCollector) addSubtitleCandidate(value string) bool {
	source := normalizeSTRMTreeSubtitleSourceWithProvider(value, c.fallbackProvider)
	if source.Provider == "" || source.Path == "" {
		return false
	}
	c.subtitles = append(c.subtitles, source)
	return true
}

func (c *strmTreeSourceCollector) addIgnored(value string) {
	if ignoredPath, ok := strmTreeIgnoredFileLikeSource(value); ok {
		key := strings.ToLower(ignoredPath)
		if _, exists := c.seenIgnored[key]; exists {
			return
		}
		c.seenIgnored[key] = struct{}{}
		c.ignoredCount++
		if len(c.ignored) < strmTreeIgnoredItemSampleLimit {
			c.ignored = append(c.ignored, ignoredPath)
		}
	}
}

func (c *strmTreeSourceCollector) finalizeSubtitles() {
	if !c.transferSubtitles {
		return
	}
	for _, source := range c.subtitles {
		if strmTreeSubtitleMatchesVideo(source, c.sources) {
			c.addSource(source)
			continue
		}
		c.addIgnored(source.Path)
	}
}

func (c *strmTreeSourceCollector) collection() strmTreeSourceCollection {
	return strmTreeSourceCollection{sources: c.sources, ignored: c.ignored, ignoredCount: c.ignoredCount}
}

func (s strmTreeSource) cloudRefPath() string {
	if strings.TrimSpace(s.RefPath) != "" {
		return s.RefPath
	}
	return s.Path
}
