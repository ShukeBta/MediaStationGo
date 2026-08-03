package service

import (
	"path/filepath"
	"strings"
)

func preferISOParentScrapeIdentity(mediaPath, libraryRoot, title string, year int) (string, int) {
	if !strings.EqualFold(filepath.Ext(pathBaseSlash(mediaPath)), ".iso") || !genericISOImageTitle(title) {
		return title, year
	}
	parent := pathBaseSlash(parentSlashPath(mediaPath))
	if parent == "" || isGenericMediaCategoryFolder(parent) || isTechnicalMediaFolder(parent) {
		return title, year
	}
	// CleanQuery expects a filename and would otherwise treat a trailing
	// ".2024" folder suffix as a file extension.
	parentTitle, parentYear := CleanQuery(parent + ".mkv")
	if parentTitle == "" {
		parentTitle = mediaFolderTitle(mediaPath, libraryRoot)
	}
	if parentTitle != "" {
		title = parentTitle
	}
	if parentYear > 0 {
		year = parentYear
	}
	return title, year
}

func genericISOImageTitle(title string) bool {
	key := strings.ToLower(strings.TrimSpace(title))
	key = strings.NewReplacer(" ", "", "_", "", "-", "", ".", "").Replace(key)
	switch key {
	case "bdmv", "bluray", "bluraydisc", "disc", "disk", "dvd", "movie", "video", "videots", "image", "iso":
		return true
	default:
		return false
	}
}
