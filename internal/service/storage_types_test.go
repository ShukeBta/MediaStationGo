package service

import "testing"

func TestAdminStorageConfigurableTypes(t *testing.T) {
	for _, typ := range []string{"openlist", "alist", "webdav", "clouddrive2", "cloud115"} {
		if !IsAdminStorageConfigurable(typ) {
			t.Fatalf("%s should be configurable", typ)
		}
	}
	for _, typ := range []string{"quark", "s3", "", "unknown"} {
		if IsAdminStorageConfigurable(typ) {
			t.Fatalf("%s should not be configurable", typ)
		}
	}
}

func TestAdminCloudConfigurableTypes(t *testing.T) {
	for _, typ := range []string{"openlist", "clouddrive2", "cloud115"} {
		if !IsAdminCloudConfigurable(typ) {
			t.Fatalf("%s should be cloud-configurable", typ)
		}
	}
	for _, typ := range []string{"quark", "alist", "webdav", "s3", ""} {
		if IsAdminCloudConfigurable(typ) {
			t.Fatalf("%s should not be cloud-configurable", typ)
		}
	}
}
