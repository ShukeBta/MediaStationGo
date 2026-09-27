package service

import "testing"

func TestCloudScanTargetsAllowExactVideoAtLibraryRoot(t *testing.T) {
	mount := CloudMountInfo{Provider: "openlist", DisplayDir: "115/电影", ScanDir: "115/电影"}
	target, ok := cloudScanTargetForResolvedOpenListPath(mount, "/115/电影/Movie.mkv", false)

	if !ok {
		t.Fatal("expected exact file target")
	}
	if target.scanDir != "115/电影" || target.displayDir != "115/电影" || target.exactFileName != "Movie.mkv" {
		t.Fatalf("target = %#v", target)
	}
}

func TestCloudScanTargetsStillRejectWholeLibraryRoot(t *testing.T) {
	mount := CloudMountInfo{Provider: "openlist", DisplayDir: "115/电影", ScanDir: "115/电影"}
	_, ok := cloudScanTargetForResolvedOpenListPath(mount, "/115/电影", true)

	if ok {
		t.Fatal("whole library root should be rejected")
	}
}

func TestCloudScanTargetsKeepExactFileInsideChildDirectory(t *testing.T) {
	mount := CloudMountInfo{Provider: "openlist", DisplayDir: "115/电影", ScanDir: "115/电影"}
	target, ok := cloudScanTargetForResolvedOpenListPath(mount, "/115/电影/Movie/Movie.mp4", false)

	if !ok {
		t.Fatal("expected child file target")
	}
	if target.scanDir != "115/电影/Movie" || target.displayDir != "115/电影/Movie" || target.exactFileName != "Movie.mp4" {
		t.Fatalf("target = %#v", target)
	}
}

func TestCloudScanTargetsKeepVideoNamedDirectoryAsDirectory(t *testing.T) {
	mount := CloudMountInfo{Provider: "openlist", DisplayDir: "115/电影", ScanDir: "115/电影"}
	target, ok := cloudScanTargetForResolvedOpenListPath(mount, "/115/电影/Movie.mp4", true)

	if !ok {
		t.Fatal("expected directory target")
	}
	if target.scanDir != "115/电影/Movie.mp4" || target.displayDir != "115/电影/Movie.mp4" || target.exactFileName != "" {
		t.Fatalf("target = %#v", target)
	}
}
