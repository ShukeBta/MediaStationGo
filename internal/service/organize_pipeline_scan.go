package service

import (
	"path/filepath"
	"strings"
)

func organizeScanRoot(res *OrganizeResult, path string) string {
	if res == nil {
		if strings.TrimSpace(path) == "" {
			return ""
		}
		return filepath.Dir(path)
	}
	var root string
	for _, item := range res.Items {
		if !organizeItemNeedsVisibilitySync(item) {
			continue
		}
		target := strings.TrimSpace(item.Target)
		if target == "" {
			continue
		}
		dir := filepath.Dir(target)
		if root == "" {
			root = dir
			continue
		}
		root = commonPathRoot(root, dir)
	}
	if root != "" {
		return root
	}
	if strings.TrimSpace(path) != "" {
		return filepath.Dir(path)
	}
	return strings.TrimSpace(res.DestPath)
}

func organizeItemNeedsVisibilitySync(item OrganizePreviewItem) bool {
	switch item.Action {
	case "organize", "replace", "reclassify", "cleanup":
		return true
	case "skip":
		switch item.Reason {
		case organizeSkipAlreadyOrganized, organizeSkipTargetExists, "duplicate exists", "target exists":
			return true
		}
	}
	return false
}

func commonPathRoot(a, b string) string {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if a == "" || a == "." {
		return b
	}
	if b == "" || b == "." {
		return a
	}
	if pathWithin(a, b) {
		return b
	}
	if pathWithin(b, a) {
		return a
	}
	for {
		parent := filepath.Dir(a)
		if parent == a || parent == "." {
			return parent
		}
		if pathWithin(b, parent) {
			return parent
		}
		a = parent
	}
}
