package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type fakeEmptyParentCloudProvider struct {
	entries     map[string][]cloud.FileEntry
	listErrs    map[string]error
	listCalls   []string
	deleteCalls []string
	deleteErr   error
}

func (f *fakeEmptyParentCloudProvider) Type() string               { return cloud.TypeOpenList }
func (f *fakeEmptyParentCloudProvider) Ping(context.Context) error { return nil }
func (f *fakeEmptyParentCloudProvider) Resolve(context.Context, string) (*cloud.DirectLink, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeEmptyParentCloudProvider) List(_ context.Context, ref string) ([]cloud.FileEntry, error) {
	ref = normalizeCloudPath(ref)
	f.listCalls = append(f.listCalls, ref)
	if err := f.listErrs[ref]; err != nil {
		return nil, err
	}
	return append([]cloud.FileEntry(nil), f.entries[ref]...), nil
}
func (f *fakeEmptyParentCloudProvider) ListRefresh(ctx context.Context, ref string) ([]cloud.FileEntry, error) {
	return f.List(ctx, ref)
}
func (f *fakeEmptyParentCloudProvider) Delete(_ context.Context, ref string) error {
	ref = normalizeCloudPath(ref)
	f.deleteCalls = append(f.deleteCalls, ref)
	return f.deleteErr
}

func TestPruneEmptyCloudParentDirectoriesStopsAtLibraryRoot(t *testing.T) {
	provider := &fakeEmptyParentCloudProvider{entries: map[string][]cloud.FileEntry{
		"/115/剧集/Show/Season 1": {},
		"/115/剧集/Show":          {},
	}}
	err := pruneEmptyCloudParentDirectories(
		t.Context(), provider, provider,
		"115/剧集/Show/Season 1/Show.S01E01.mkv", "115/剧集",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/115/剧集/Show/Season 1", "/115/剧集/Show"}
	if !reflect.DeepEqual(provider.listCalls, want) || !reflect.DeepEqual(provider.deleteCalls, want) {
		t.Fatalf("list=%v delete=%v want=%v", provider.listCalls, provider.deleteCalls, want)
	}
}

func TestPruneEmptyCloudParentDirectoriesKeepsNonEmptyDirectory(t *testing.T) {
	provider := &fakeEmptyParentCloudProvider{entries: map[string][]cloud.FileEntry{
		"/115/剧集/Show": {{Name: "Show.zh-CN.ass"}},
	}}
	err := pruneEmptyCloudParentDirectories(
		t.Context(), provider, provider,
		"115/剧集/Show/Show.mkv", "115/剧集",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.deleteCalls) != 0 {
		t.Fatalf("non-empty directory was deleted: %v", provider.deleteCalls)
	}
}

func TestPruneEmptyCloudParentDirectoriesTreatsMissingParentAsPruned(t *testing.T) {
	provider := &fakeEmptyParentCloudProvider{
		listErrs: map[string]error{
			"/115/剧集/Show/Season 1": errors.New("openlist: api list failed: object not found"),
		},
		entries: map[string][]cloud.FileEntry{
			"/115/剧集/Show": {},
		},
	}
	err := pruneEmptyCloudParentDirectories(
		t.Context(), provider, provider,
		"115/剧集/Show/Season 1/Show.S01E01.mkv", "115/剧集",
	)
	if err != nil {
		t.Fatal(err)
	}
	wantLists := []string{"/115/剧集/Show/Season 1", "/115/剧集/Show"}
	wantDeletes := []string{"/115/剧集/Show"}
	if !reflect.DeepEqual(provider.listCalls, wantLists) || !reflect.DeepEqual(provider.deleteCalls, wantDeletes) {
		t.Fatalf("list=%v delete=%v want lists=%v deletes=%v", provider.listCalls, provider.deleteCalls, wantLists, wantDeletes)
	}
}

func TestPruneEmptyCloudParentDirectoriesKeepsPendingDeleteFailure(t *testing.T) {
	provider := &fakeEmptyParentCloudProvider{
		listErrs: map[string]error{
			"/115/剧集/Show": errors.New("code: 990019, message: 删除操作尚未执行完成，请稍后再试"),
		},
	}
	err := pruneEmptyCloudParentDirectories(
		t.Context(), provider, provider,
		"115/剧集/Show/Show.mkv", "115/剧集",
	)
	if err == nil {
		t.Fatal("pending upstream delete must remain an error")
	}
	if len(provider.deleteCalls) != 0 {
		t.Fatalf("pending delete error caused directory deletion: %v", provider.deleteCalls)
	}
}

func TestPruneEmptyCloudParentDirectoriesRejectsOutsideRoot(t *testing.T) {
	provider := &fakeEmptyParentCloudProvider{}
	err := pruneEmptyCloudParentDirectories(
		t.Context(), provider, provider,
		"115/电影/Show/Show.mkv", "115/剧集",
	)
	if err == nil {
		t.Fatal("path outside library root should be rejected")
	}
	if len(provider.listCalls) != 0 || len(provider.deleteCalls) != 0 {
		t.Fatalf("outside-root path caused provider calls: list=%v delete=%v", provider.listCalls, provider.deleteCalls)
	}
}
