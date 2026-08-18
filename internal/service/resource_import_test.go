package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type fakeResourcePipeline struct {
	mu             sync.Mutex
	duplicate      *ResourceImportDuplicate
	searchErr      error
	searchRequests []resourcePipelineSearchRequest
	manualRequests []resourcePipelineManualRequest
	createCalls    int
	getDelay       time.Duration
	active         int
	maxActive      int
	activeByOwner  map[string]int
	maxByOwner     map[string]int
	createdOwners  []string
	createRequests []resourcePipelineCreateRequest
	canceledOwners []string
	retriedOwners  []string
}

func (f *fakeResourcePipeline) PrepareManual(_ context.Context, in resourcePipelineManualRequest) (resourcePipelineSearchResponse, error) {
	f.mu.Lock()
	f.manualRequests = append(f.manualRequests, in)
	f.mu.Unlock()
	return resourcePipelineSearchResponse{
		SessionID: "manual-session-" + in.OwnerID,
		ExpiresAt: time.Now().Add(15 * time.Minute).Unix(),
		Items: []map[string]any{{
			"candidate_id": "manual-candidate-1", "title": in.Title,
			"indexer": "115分享", "resource_type": "115_share",
			"summary":      "任务名称由用户填写",
			"download_uri": "https://115.com/s/swabc123?password=secret",
		}},
	}, nil
}

func (f *fakeResourcePipeline) Search(_ context.Context, in resourcePipelineSearchRequest) (resourcePipelineSearchResponse, error) {
	f.mu.Lock()
	f.searchRequests = append(f.searchRequests, in)
	f.mu.Unlock()
	if f.searchErr != nil {
		return resourcePipelineSearchResponse{}, f.searchErr
	}
	return resourcePipelineSearchResponse{
		SessionID: "pipeline-session-" + in.OwnerID,
		ExpiresAt: time.Now().Add(15 * time.Minute).Unix(),
		Items: []map[string]any{{
			"candidate_id": "candidate-1", "title": in.Query, "size": float64(1024), "indexer": "test",
		}},
		Capabilities: ResourceSearchCapabilities{Pansou: true},
	}, nil
}

func TestResourceImportSubscriptionSearchBypassesManualCacheAndMarksPipelineRequest(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, _, library, root, _, user := newResourceImportTestService(t, pipeline)
	input := ResourceSearchInput{Query: "凡人修仙传", RootID: root.ID}

	manual, err := svc.Search(t.Context(), user.ID, library, root, input)
	if err != nil {
		t.Fatal(err)
	}
	input.SubscriptionFollow = true
	follow, err := svc.Search(t.Context(), user.ID, library, root, input)
	if err != nil {
		t.Fatal(err)
	}
	input.SubscriptionFollow = false
	cached, err := svc.Search(t.Context(), user.ID, library, root, input)
	if err != nil {
		t.Fatal(err)
	}

	pipeline.mu.Lock()
	requests := append([]resourcePipelineSearchRequest(nil), pipeline.searchRequests...)
	pipeline.mu.Unlock()
	if len(requests) != 2 || requests[0].SubscriptionFollow || !requests[1].SubscriptionFollow {
		t.Fatalf("pipeline search requests = %+v", requests)
	}
	if manual.SessionID == follow.SessionID {
		t.Fatal("subscription search reused the manual search session")
	}
	if cached.SessionID != manual.SessionID {
		t.Fatalf("manual cache session = %q, want original manual session %q", cached.SessionID, manual.SessionID)
	}
}

func TestResourceImportSearchErrorPreservesCapabilities(t *testing.T) {
	pipeline := &fakeResourcePipeline{searchErr: &resourcePipelineError{
		StatusCode:   502,
		Code:         "search_failed",
		Message:      "BT4G timed out",
		Capabilities: ResourceSearchCapabilities{Pansou: true},
	}}
	svc, _, library, root, _, user := newResourceImportTestService(t, pipeline)

	_, err := svc.Search(t.Context(), user.ID, library, root, ResourceSearchInput{Query: "Missing", RootID: root.ID})

	var searchErr *ResourceSearchError
	if !errors.As(err, &searchErr) {
		t.Fatalf("search error = %#v", err)
	}
	if searchErr.Code != "search_failed" || !searchErr.Capabilities.Pansou || searchErr.HTTPStatus() != 502 {
		t.Fatalf("search error details = %+v", searchErr)
	}
}

func (f *fakeResourcePipeline) CreateImport(_ context.Context, owner, _ string, in resourcePipelineCreateRequest) (resourcePipelineTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	f.createdOwners = append(f.createdOwners, owner)
	f.createRequests = append(f.createRequests, in)
	if f.duplicate != nil {
		return resourcePipelineTask{}, &resourcePipelineError{
			StatusCode: 409, Code: "duplicate_media", Message: "duplicate",
			Duplicate: f.duplicate,
		}
	}
	return resourcePipelineTask{ID: "pipeline-job-1", OwnerID: owner, Status: "queued", Stage: "queued", Message: "queued"}, nil
}

func (f *fakeResourcePipeline) GetImport(_ context.Context, owner, id string) (resourcePipelineTask, error) {
	f.mu.Lock()
	if f.activeByOwner == nil {
		f.activeByOwner = map[string]int{}
		f.maxByOwner = map[string]int{}
	}
	f.active++
	f.activeByOwner[owner]++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	if f.activeByOwner[owner] > f.maxByOwner[owner] {
		f.maxByOwner[owner] = f.activeByOwner[owner]
	}
	f.mu.Unlock()
	if f.getDelay > 0 {
		time.Sleep(f.getDelay)
	}
	f.mu.Lock()
	f.active--
	f.activeByOwner[owner]--
	f.mu.Unlock()
	return resourcePipelineTask{
		ID: id, OwnerID: owner, Status: "completed", Stage: "completed",
		Message: "done", MsgMediaID: "media-" + id, MsgMediaTitle: "Done",
	}, nil
}

func (f *fakeResourcePipeline) CancelImport(_ context.Context, owner, id string) (resourcePipelineTask, error) {
	f.mu.Lock()
	f.canceledOwners = append(f.canceledOwners, owner)
	f.mu.Unlock()
	return resourcePipelineTask{ID: id, OwnerID: owner, Status: "canceled", Stage: "canceled", Message: "canceled"}, nil
}

func (f *fakeResourcePipeline) RetryImport(_ context.Context, owner, id string) (resourcePipelineTask, error) {
	f.mu.Lock()
	f.retriedOwners = append(f.retriedOwners, owner)
	f.mu.Unlock()
	return resourcePipelineTask{ID: id, OwnerID: owner, Status: "queued", Stage: "queued", Message: "queued"}, nil
}

func newResourceImportTestService(t *testing.T, pipeline *fakeResourcePipeline) (*ResourceImportService, *repository.Container, model.Library, model.LibraryRoot, model.User, model.User) {
	t.Helper()
	db := newServiceTestDB(t,
		&model.User{}, &model.Library{}, &model.LibraryRoot{},
		&model.Media{}, &model.ResourceSearchSession{}, &model.ResourceImportJob{},
	)
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	repos := repository.New(db)
	admin := model.User{Username: "admin", PasswordHash: "x", Role: "admin", IsActive: true}
	user := model.User{Username: "user-a", PasswordHash: "x", Role: "user", IsActive: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	library := model.Library{Name: "Movies", Path: "cloud://openlist/115%2F电影", Type: "movie", Enabled: true}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: library.ID, Name: "115 Movies", Path: "cloud://openlist/115%2F电影?dir=电影&auto_category=1", Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	library.Roots = []model.LibraryRoot{root}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc := newResourceImportServiceWithClient(config.ResourceImportConfig{
		Enabled: true, MaxConcurrent: 3, MaxConcurrentPerUser: 2, PollSeconds: 1,
	}, nil, repos, ctx, pipeline)
	return svc, repos, library, root, admin, user
}

func TestResourceImportSearchAndSessionAreOwnerScoped(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, _, library, root, _, user := newResourceImportTestService(t, pipeline)
	search, err := svc.Search(t.Context(), user.ID, library, root, ResourceSearchInput{Query: "Sintel", RootID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	if search.Total != 1 || search.Results[0].Title != "Sintel" {
		t.Fatalf("unexpected search response: %+v", search)
	}
	publicJSON, err := json.Marshal(search.Results[0])
	if err != nil || strings.Contains(string(publicJSON), "candidate-1") {
		t.Fatalf("public candidate leaked pipeline id: %s, err=%v", publicJSON, err)
	}
	_, err = svc.Create(t.Context(), "another-user", library, root, ResourceImportCreateInput{
		SearchSessionID: search.SessionID, CandidateIndex: 0, RootID: root.ID,
	})
	if err == nil || err.Error() != "resource search session not found" {
		t.Fatalf("cross-owner session create err = %v", err)
	}
	if pipeline.createCalls != 0 {
		t.Fatalf("pipeline create calls = %d", pipeline.createCalls)
	}
}

func TestResourceImportManualPreviewUsesDedicatedPipelinePath(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, _, library, root, _, user := newResourceImportTestService(t, pipeline)
	title := "手动任务名称"

	preview, err := svc.PrepareManual(t.Context(), user.ID, library, root, "https://115.com/s/swabc123", title)
	if err != nil {
		t.Fatal(err)
	}

	if preview.Total != 1 || preview.Query != title || preview.Results[0].ResourceType != "115_share" {
		t.Fatalf("unexpected manual preview: %+v", preview)
	}
	if preview.Results[0].Title != title || preview.Results[0].Summary != "任务名称由用户填写" {
		t.Fatalf("manual preview did not keep user supplied title: %+v", preview.Results[0])
	}
	if len(preview.Roots) != 1 || preview.Roots[0].ID != root.ID {
		t.Fatalf("unexpected manual root: %+v", preview.Roots)
	}
	publicJSON, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicJSON), "download_uri") || strings.Contains(string(publicJSON), "password=secret") || strings.Contains(string(publicJSON), "manual-candidate-1") {
		t.Fatalf("manual preview leaked private candidate data: %s", publicJSON)
	}
	pipeline.mu.Lock()
	manualRequests := append([]resourcePipelineManualRequest(nil), pipeline.manualRequests...)
	searchRequests := append([]resourcePipelineSearchRequest(nil), pipeline.searchRequests...)
	pipeline.mu.Unlock()
	if len(manualRequests) != 1 || manualRequests[0].OwnerID != user.ID || manualRequests[0].Category != "movie" || manualRequests[0].Title != title {
		t.Fatalf("manual requests = %+v", manualRequests)
	}
	if len(searchRequests) != 0 {
		t.Fatalf("manual preview called resource search: %+v", searchRequests)
	}

	task, err := svc.Create(t.Context(), user.ID, library, root, ResourceImportCreateInput{
		SearchSessionID: preview.SessionID, CandidateIndex: 0, RootID: root.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != ResourceImportStatusQueued || task.CandidateTitle != title {
		t.Fatalf("manual task = %+v", task)
	}
	pipeline.mu.Lock()
	createRequests := append([]resourcePipelineCreateRequest(nil), pipeline.createRequests...)
	pipeline.mu.Unlock()
	if len(createRequests) != 1 || createRequests[0].SearchSessionID != "manual-session-"+user.ID || createRequests[0].CandidateID != "manual-candidate-1" {
		t.Fatalf("manual create requests = %+v", createRequests)
	}
}

func TestResourceImportManualReplenishUsesExactSeriesSeasonDirectoryBaseline(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	library.Type = "anime"
	if err := repos.DB.Model(&model.Library{}).Where("id = ?", library.ID).Update("type", library.Type).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: library.ID, LibraryRootID: root.ID, SeriesID: "series-1", Title: "凡人修仙传 第1集", OriginalName: "A Record of a Mortal's Journey to Immortality", SeasonNum: 1, EpisodeNum: 1, Path: "cloud://openlist/115/电影/凡人修仙传/Season%201/E01.mkv"},
		{LibraryID: library.ID, LibraryRootID: root.ID, SeriesID: "series-1", Title: "凡人修仙传 第2集", SeasonNum: 1, EpisodeNum: 2, Path: "cloud://openlist/115/电影/凡人修仙传/Season%201/E02.mkv"},
		{LibraryID: library.ID, LibraryRootID: root.ID, SeriesID: "series-1", Title: "其他目录第3集", SeasonNum: 1, EpisodeNum: 3, Path: "cloud://openlist/115/电影/其他目录/Season%201/E03.mkv"},
	}
	for i := range rows {
		if err := repos.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	task, err := svc.ReplenishEpisodes(t.Context(), user.ID, rows[0].ID, "https://115.com/s/swabc123")
	if err != nil {
		t.Fatal(err)
	}
	if !task.SubscriptionFollow || !task.ManualReplenish || task.SubscriptionID != "" {
		t.Fatalf("unexpected replenishment task: %+v", task)
	}
	pipeline.mu.Lock()
	defer pipeline.mu.Unlock()
	if len(pipeline.manualRequests) != 1 || pipeline.manualRequests[0].Title != rows[0].OriginalName || pipeline.manualRequests[0].Category != "anime" {
		t.Fatalf("manual requests = %+v", pipeline.manualRequests)
	}
	if len(pipeline.createRequests) != 1 {
		t.Fatalf("create requests = %+v", pipeline.createRequests)
	}
	request := pipeline.createRequests[0]
	if !request.SubscriptionFollow || !request.ManualReplenish || request.SubscriptionID != "" || request.Season != 1 {
		t.Fatalf("unexpected pipeline replenishment request: %+v", request)
	}
	if request.TargetOpenListPath != "/115/电影/凡人修仙传/Season 1" {
		t.Fatalf("target path = %q", request.TargetOpenListPath)
	}
	if len(request.ExistingEpisodes) != 2 || request.ExistingEpisodes[0] != 1 || request.ExistingEpisodes[1] != 2 {
		t.Fatalf("existing episodes = %+v", request.ExistingEpisodes)
	}
}

func TestResourceImportManualReplenishAllowsCompletedNoNewEpisodesWithoutMedia(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	job := model.ResourceImportJob{
		UserID: user.ID, SubscriptionFollow: true, ManualReplenish: true,
		LibraryID: library.ID, LibraryRootID: root.ID, SearchSessionID: "search-manual-replenish",
		CandidateIndex: 0, CandidateJSON: `{}`, CandidateTitle: "补集", IdempotencyKey: "manual-replenish-no-new",
		Status: ResourceImportStatusRunning, Stage: "verifying_staging", Attempt: 1,
	}
	if err := repos.DB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	child := resourcePipelineTask{
		Status: "completed", Stage: "completed",
		Result: map[string]any{"subscription_follow": map[string]any{"outcome": "no_new_episodes"}},
	}
	if err := svc.applyPipelineTask(t.Context(), &job, child); err != nil {
		t.Fatal(err)
	}
	var stored model.ResourceImportJob
	if err := repos.DB.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != ResourceImportStatusCompleted || stored.Outcome != "no_new_episodes" || stored.MediaID != "" {
		t.Fatalf("stored manual replenishment = %+v", stored)
	}
}

func TestResourceImportDuplicateDoesNotCreateParentJob(t *testing.T) {
	pipeline := &fakeResourcePipeline{duplicate: &ResourceImportDuplicate{CanForce: false, MediaID: "media-existing", Title: "Existing"}}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	search, err := svc.Search(t.Context(), user.ID, library, root, ResourceSearchInput{Query: "Sintel", RootID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(t.Context(), user.ID, library, root, ResourceImportCreateInput{
		SearchSessionID: search.SessionID, CandidateIndex: 0, RootID: root.ID,
	})
	var duplicate *ResourceImportDuplicateError
	if !errors.As(err, &duplicate) || duplicate.Duplicate.MediaID != "media-existing" || duplicate.Duplicate.CanForce {
		t.Fatalf("duplicate error = %#v", err)
	}
	var count int64
	if err := repos.DB.Model(&model.ResourceImportJob{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("parent job count = %d", count)
	}
}

func TestResourceImportListAndActionsAreOwnerScoped(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, admin, user := newResourceImportTestService(t, pipeline)
	other := model.User{Username: "user-b", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	jobs := []model.ResourceImportJob{
		{UserID: user.ID, LibraryID: library.ID, LibraryRootID: root.ID, SearchSessionID: "s1", CandidateJSON: "{}", CandidateTitle: "A", IdempotencyKey: "key-a", Status: "running", Stage: "transferring", PipelineJobID: "p-a", Attempt: 1},
		{UserID: other.ID, LibraryID: library.ID, LibraryRootID: root.ID, SearchSessionID: "s2", CandidateJSON: "{}", CandidateTitle: "B", IdempotencyKey: "key-b", Status: "failed", Stage: "failed", PipelineJobID: "p-b", Attempt: 1},
	}
	if err := repos.DB.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	own, err := svc.List(t.Context(), user.ID, false, ResourceImportListFilter{UserID: other.ID})
	if err != nil || own.Total != 1 || own.Items[0].CandidateTitle != "A" || own.Items[0].UserID != "" {
		t.Fatalf("owner list = %+v, err=%v", own, err)
	}
	all, err := svc.List(t.Context(), admin.ID, true, ResourceImportListFilter{})
	if err != nil || all.Total != 2 || all.Items[0].UserID == "" || all.Items[0].CreatorUsername == "" {
		t.Fatalf("admin list = %+v, err=%v", all, err)
	}
	if _, err := svc.Get(t.Context(), user.ID, false, jobs[1].ID); err == nil {
		t.Fatal("user A read user B task")
	}
	if _, err := svc.Cancel(t.Context(), user.ID, false, jobs[1].ID); err == nil {
		t.Fatal("user A canceled user B task")
	}
	if _, err := svc.Retry(t.Context(), user.ID, false, jobs[1].ID); err == nil {
		t.Fatal("user A retried user B task")
	}
	if _, err := svc.Retry(t.Context(), admin.ID, true, jobs[1].ID); err != nil {
		t.Fatalf("admin retry: %v", err)
	}
	if len(pipeline.retriedOwners) != 1 || pipeline.retriedOwners[0] != other.ID {
		t.Fatalf("pipeline retry owners = %v", pipeline.retriedOwners)
	}
}

func TestDeleteFailedResourceImportIsOwnerScopedAndStatusRestricted(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, admin, user := newResourceImportTestService(t, pipeline)
	other := model.User{Username: "delete-other", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	failed := model.ResourceImportJob{
		UserID: user.ID, LibraryID: library.ID, LibraryRootID: root.ID,
		SearchSessionID: "delete-failed", CandidateJSON: "{}", CandidateTitle: "Failed",
		IdempotencyKey: "delete-failed", Status: ResourceImportStatusFailed, Stage: "failed", PipelineJobID: "pipeline-failed", Attempt: 1,
	}
	running := model.ResourceImportJob{
		UserID: user.ID, LibraryID: library.ID, LibraryRootID: root.ID,
		SearchSessionID: "delete-running", CandidateJSON: "{}", CandidateTitle: "Running",
		IdempotencyKey: "delete-running", Status: ResourceImportStatusRunning, Stage: "running", PipelineJobID: "pipeline-running", Attempt: 1,
	}
	adminTarget := model.ResourceImportJob{
		UserID: other.ID, LibraryID: library.ID, LibraryRootID: root.ID,
		SearchSessionID: "delete-admin", CandidateJSON: "{}", CandidateTitle: "Admin target",
		IdempotencyKey: "delete-admin", Status: ResourceImportStatusFailed, Stage: "failed", PipelineJobID: "pipeline-admin", Attempt: 1,
	}
	jobs := []model.ResourceImportJob{failed, running, adminTarget}
	if err := repos.DB.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	failed, running, adminTarget = jobs[0], jobs[1], jobs[2]

	if err := svc.DeleteFailed(t.Context(), other.ID, false, failed.ID); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("other user delete error = %v", err)
	}
	if err := svc.DeleteFailed(t.Context(), user.ID, false, running.ID); !errors.Is(err, ErrResourceImportDeleteNotAllowed) {
		t.Fatalf("running task delete error = %v", err)
	}
	if err := svc.DeleteFailed(t.Context(), user.ID, false, failed.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteFailed(t.Context(), admin.ID, true, adminTarget.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{failed.ID, adminTarget.ID} {
		var count int64
		if err := repos.DB.Unscoped().Model(&model.ResourceImportJob{}).Where("id = ?", id).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("deleted task %s still exists", id)
		}
	}
}

func TestResourceImportRecoveryHonorsGlobalAndPerUserLimits(t *testing.T) {
	pipeline := &fakeResourcePipeline{getDelay: 80 * time.Millisecond}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	other := model.User{Username: "user-b", PasswordHash: "x", Role: "user", IsActive: true}
	third := model.User{Username: "user-c", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&third).Error; err != nil {
		t.Fatal(err)
	}
	owners := []string{user.ID, user.ID, user.ID, other.ID, third.ID}
	for i, owner := range owners {
		job := model.ResourceImportJob{
			UserID: owner, LibraryID: library.ID, LibraryRootID: root.ID,
			SearchSessionID: "session", CandidateJSON: "{}", CandidateTitle: "Task",
			IdempotencyKey: "recover-" + string(rune('a'+i)), Status: "queued", Stage: "duplicate_check",
			PipelineJobID: "pipeline-" + string(rune('a'+i)), Attempt: 1,
		}
		if err := repos.DB.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
	}
	if count, err := svc.Recover(t.Context()); err != nil || count != len(owners) {
		t.Fatalf("recover count=%d err=%v", count, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	completed := int64(0)
	for time.Now().Before(deadline) {
		if err := repos.DB.Model(&model.ResourceImportJob{}).Where("status = ?", "completed").Count(&completed).Error; err != nil {
			t.Fatal(err)
		}
		if completed == int64(len(owners)) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if completed != int64(len(owners)) {
		t.Fatalf("completed jobs = %d, want %d", completed, len(owners))
	}
	pipeline.mu.Lock()
	defer pipeline.mu.Unlock()
	if pipeline.maxActive > 3 {
		t.Fatalf("global max active = %d", pipeline.maxActive)
	}
	if pipeline.maxByOwner[user.ID] > 2 {
		t.Fatalf("owner max active = %d", pipeline.maxByOwner[user.ID])
	}
}

func TestResourceRootOpenListPathIgnoresQueryAndDecodesEscapedSlash(t *testing.T) {
	got, err := resourceRootOpenListPath("cloud://openlist/115%2F成人?dir=成人&auto_category=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/115/成人" {
		t.Fatalf("root path = %q", got)
	}
}

func TestStoredResourceSearchDoesNotExposePipelineCandidateID(t *testing.T) {
	candidate := ResourceSearchCandidate{Index: 0, Title: "Movie", CandidateID: "secret-candidate"}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || string(encoded) == "null" {
		t.Fatal("candidate marshal failed")
	}
	if string(encoded) != `{"index":0,"title":"Movie"}` {
		t.Fatalf("public candidate JSON = %s", encoded)
	}
}

func TestResourceCandidateCompatibilityWarningMarksDolbyVideoAndAudio(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Movie.2160p.DV.HDR.TrueHD.Atmos", "杜比视界 + 杜比全景声，部分设备可能无法兼容"},
		{"Movie.2160p.DoVi.HDR", "杜比视界，部分设备可能无法兼容"},
		{"Movie.1080p.EAC3.DDP", "杜比音频，部分设备可能无法兼容"},
		{"Movie.DVD.1080p.DTS", ""},
	}

	for _, test := range tests {
		candidate := ResourceSearchCandidate{Title: test.title}
		if got := resourceCandidateCompatibilityWarning(candidate); got != test.want {
			t.Fatalf("warning for %q = %q, want %q", test.title, got, test.want)
		}
	}
}

func TestResourceImportPersistsPipelineCandidateIDForCreate(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	search, err := svc.Search(t.Context(), user.ID, library, root, ResourceSearchInput{Query: "Sintel", RootID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	var session model.ResourceSearchSession
	if err := repos.DB.First(&session, "id = ?", search.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(session.ResultsJSON, `"candidate_id":"candidate-1"`) {
		t.Fatalf("stored search session has no pipeline candidate id: %s", session.ResultsJSON)
	}
	if _, err := svc.Create(t.Context(), user.ID, library, root, ResourceImportCreateInput{
		SearchSessionID: search.SessionID, CandidateIndex: 0, RootID: root.ID,
	}); err != nil {
		t.Fatal(err)
	}
	pipeline.mu.Lock()
	defer pipeline.mu.Unlock()
	if len(pipeline.createRequests) != 1 || pipeline.createRequests[0].CandidateID != "candidate-1" {
		t.Fatalf("pipeline create requests = %#v", pipeline.createRequests)
	}
}

func TestResourceImportSubscriptionReservationAllowsOnlyOneActiveWorkSeason(t *testing.T) {
	pipeline := &fakeResourcePipeline{getDelay: 500 * time.Millisecond}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	library.Type = "anime"
	if err := repos.DB.Model(&model.Library{}).Where("id = ?", library.ID).Update("type", library.Type).Error; err != nil {
		t.Fatal(err)
	}
	searches := make([]ResourceSearchResponse, 0, 2)
	for _, query := range []string{"Test Show 更新至2集", "Test Show 更新至3集"} {
		search, err := svc.Search(t.Context(), user.ID, library, root, ResourceSearchInput{Query: query, RootID: root.ID})
		if err != nil {
			t.Fatal(err)
		}
		searches = append(searches, search)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, search := range searches {
		search := search
		go func() {
			<-start
			_, err := svc.Create(t.Context(), user.ID, library, root, ResourceImportCreateInput{
				SearchSessionID: search.SessionID, CandidateIndex: 0, RootID: root.ID,
				SubscriptionID: "subscription-show", SubscriptionFollow: true,
				WorkKey: "test-show", Season: 1, ExistingEpisodes: []int{1},
				TargetOpenListPath: "/115/电影/Test Show/Season 1", TitleClass: "cumulative_pack",
			})
			errs <- err
		}()
	}
	close(start)
	successes, conflicts := 0, 0
	for range searches {
		err := <-errs
		switch {
		case err == nil:
			successes++
		case strings.Contains(err.Error(), "同一作品季已有追更任务正在处理"):
			conflicts++
		default:
			t.Fatalf("unexpected create error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	var count int64
	if err := repos.DB.Model(&model.ResourceImportJob{}).Where("subscription_follow = ?", true).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 || pipeline.createCalls != 1 {
		t.Fatalf("job count=%d pipeline creates=%d", count, pipeline.createCalls)
	}
}

func TestRetrySubscriptionFollowCreatesNewAuditRowAndKeepsOriginal(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	library.Type = "anime"
	if err := repos.DB.Model(&model.Library{}).Where("id = ?", library.ID).Update("type", library.Type).Error; err != nil {
		t.Fatal(err)
	}
	original := model.ResourceImportJob{
		UserID: user.ID, SubscriptionID: "subscription-show", SubscriptionFollow: true,
		WorkKey: "test-show", SeasonNumber: 1, TitleClass: "cumulative_pack",
		TargetOpenListPath:   "/115/电影/Test Show/Season 1",
		ExistingEpisodesJSON: `[1]`, ReservedEpisodesJSON: `[]`,
		LibraryID: library.ID, LibraryRootID: root.ID, SearchSessionID: "search-original",
		PipelineSearchSessionID: "pipeline-search", PipelineCandidateID: "pipeline-candidate",
		CandidateIndex: 0, CandidateJSON: `{}`, CandidateTitle: "Test Show 更新至2集",
		IdempotencyKey: "subscription-original", Attempt: 1,
		Status: ResourceImportStatusFailed, Stage: "failed", Outcome: "rejected", PublicError: "unknown video",
		PipelineJobID: "pipeline-original",
	}
	if err := repos.DB.Create(&original).Error; err != nil {
		t.Fatal(err)
	}

	retried, err := svc.Retry(t.Context(), user.ID, false, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID == original.ID || retried.Attempt != 2 {
		t.Fatalf("retried task = %+v", retried)
	}
	var rows []model.ResourceImportJob
	if err := repos.DB.Where("subscription_id = ?", original.SubscriptionID).Order("attempt asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != original.ID || rows[0].Status != ResourceImportStatusFailed || rows[0].Outcome != "rejected" {
		t.Fatalf("audit rows = %+v", rows)
	}
	if rows[1].RetryOfJobID != original.ID || rows[1].Attempt != 2 || rows[1].PipelineJobID == "" {
		t.Fatalf("retry row = %+v", rows[1])
	}
	if len(pipeline.createRequests) != 1 || !pipeline.createRequests[0].SubscriptionFollow {
		t.Fatalf("pipeline create requests = %+v", pipeline.createRequests)
	}
}

func TestResourceImportUpgradePersistsAndForwardsTargetMedia(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	target := model.Media{
		LibraryID: library.ID, LibraryRootID: root.ID,
		Title: "Sintel", Path: "cloud://openlist/115/电影/Sintel/Sintel.mkv",
	}
	if err := repos.DB.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	search, err := svc.Search(t.Context(), user.ID, library, root, ResourceSearchInput{Query: "Sintel 4K", RootID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	task, err := svc.Create(t.Context(), user.ID, library, root, ResourceImportCreateInput{
		SearchSessionID: search.SessionID, CandidateIndex: 0, RootID: root.ID, UpgradeMediaID: target.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.UpgradeMediaID != target.ID {
		t.Fatalf("task upgrade_media_id = %q, want %q", task.UpgradeMediaID, target.ID)
	}
	if task.UpgradeScope != "media" {
		t.Fatalf("task upgrade_scope = %q, want media", task.UpgradeScope)
	}
	pipeline.mu.Lock()
	if len(pipeline.createRequests) != 1 || pipeline.createRequests[0].UpgradeMediaID != target.ID {
		pipeline.mu.Unlock()
		t.Fatalf("pipeline create requests = %#v", pipeline.createRequests)
	}
	if !pipeline.createRequests[0].KeepOldVersion {
		pipeline.mu.Unlock()
		t.Fatal("default upgrade should keep the old version")
	}
	if pipeline.createRequests[0].UpgradeScope != "media" {
		pipeline.mu.Unlock()
		t.Fatalf("pipeline upgrade_scope = %q, want media", pipeline.createRequests[0].UpgradeScope)
	}
	pipeline.mu.Unlock()
	var job model.ResourceImportJob
	if err := repos.DB.First(&job, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.UpgradeMediaID != target.ID {
		t.Fatalf("persisted upgrade_media_id = %q, want %q", job.UpgradeMediaID, target.ID)
	}
	if job.UpgradeScope != "media" {
		t.Fatalf("persisted upgrade_scope = %q, want media", job.UpgradeScope)
	}
	if !job.KeepOldVersion {
		t.Fatal("persisted upgrade should keep the old version by default")
	}
}

func TestResourceImportTVUpgradeAlwaysUsesWorkScopeAndRequiresAdminForReplacement(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	library.Type = "tv"
	if err := repos.DB.Model(&model.Library{}).Where("id = ?", library.ID).Update("type", "tv").Error; err != nil {
		t.Fatal(err)
	}
	target := model.Media{
		LibraryID: library.ID, LibraryRootID: root.ID, Title: "Show", TMDbID: 303143,
		SeasonNum: 1, EpisodeNum: 1, Path: "cloud://openlist/115/电影/Show-old/Show.S01E01.mkv",
	}
	if err := repos.DB.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	search, err := svc.Search(t.Context(), user.ID, library, root, ResourceSearchInput{Query: "Show", RootID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	keepOld := false
	_, err = svc.Create(t.Context(), user.ID, library, root, ResourceImportCreateInput{
		SearchSessionID: search.SessionID, CandidateIndex: 0, RootID: root.ID,
		UpgradeMediaID: target.ID, KeepOldVersion: &keepOld,
	})
	if !errors.Is(err, ErrMediaVersionForbidden) {
		t.Fatalf("non-admin work replacement err = %v", err)
	}

	task, err := svc.Create(t.Context(), user.ID, library, root, ResourceImportCreateInput{
		SearchSessionID: search.SessionID, CandidateIndex: 0, RootID: root.ID,
		UpgradeMediaID: target.ID, KeepOldVersion: &keepOld, IsAdmin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.UpgradeScope != "work" {
		t.Fatalf("task upgrade_scope = %q, want work", task.UpgradeScope)
	}
	pipeline.mu.Lock()
	defer pipeline.mu.Unlock()
	if len(pipeline.createRequests) != 1 || pipeline.createRequests[0].UpgradeScope != "work" {
		t.Fatalf("pipeline create requests = %#v", pipeline.createRequests)
	}
}

func TestResourceImportOwnerCanReplaceOwnVersionButOtherUserCannot(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	target := model.Media{
		LibraryID: library.ID, LibraryRootID: root.ID,
		Title: "Sintel", Path: "cloud://openlist/115/电影/Sintel/Sintel.mkv",
	}
	if err := repos.DB.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	ownerJob := model.ResourceImportJob{
		UserID: user.ID, LibraryID: library.ID, LibraryRootID: root.ID,
		SearchSessionID: "owner-session", CandidateJSON: "{}", CandidateTitle: "Sintel",
		IdempotencyKey: "owner-job", Status: ResourceImportStatusCompleted, Stage: "completed",
		PipelineJobID: "owner-pipeline", MediaID: target.ID, Attempt: 1,
	}
	if err := repos.DB.Create(&ownerJob).Error; err != nil {
		t.Fatal(err)
	}
	search, err := svc.Search(t.Context(), user.ID, library, root, ResourceSearchInput{Query: "Sintel 4K", RootID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	keepOld := false
	if _, err := svc.Create(t.Context(), user.ID, library, root, ResourceImportCreateInput{
		SearchSessionID: search.SessionID, CandidateIndex: 0, RootID: root.ID,
		UpgradeMediaID: target.ID, KeepOldVersion: &keepOld,
	}); err != nil {
		t.Fatalf("owner replace: %v", err)
	}
	pipeline.mu.Lock()
	if len(pipeline.createRequests) != 1 || pipeline.createRequests[0].KeepOldVersion {
		pipeline.mu.Unlock()
		t.Fatalf("pipeline create requests = %#v", pipeline.createRequests)
	}
	pipeline.mu.Unlock()

	other := model.User{Username: "user-b", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	otherSearch, err := svc.Search(t.Context(), other.ID, library, root, ResourceSearchInput{Query: "Sintel 8K", RootID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(t.Context(), other.ID, library, root, ResourceImportCreateInput{
		SearchSessionID: otherSearch.SessionID, CandidateIndex: 0, RootID: root.ID,
		UpgradeMediaID: target.ID, KeepOldVersion: &keepOld,
	})
	if !errors.Is(err, ErrMediaVersionForbidden) {
		t.Fatalf("non-owner replace err = %v", err)
	}
}

func TestResourceImportUpgradeRejectsMediaOutsideTargetLibraryOrRoot(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	otherLibrary := model.Library{Name: "Other", Path: "cloud://openlist/115%2F其他", Type: "movie", Enabled: true}
	if err := repos.DB.Create(&otherLibrary).Error; err != nil {
		t.Fatal(err)
	}
	wrongLibrary := model.Media{LibraryID: otherLibrary.ID, Title: "Sintel", Path: "cloud://openlist/115/其他/Sintel.mkv"}
	wrongRoot := model.Media{LibraryID: library.ID, LibraryRootID: "other-root", Title: "Sintel", Path: "cloud://openlist/115/电影/Sintel-2.mkv"}
	if err := repos.DB.Create(&wrongLibrary).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&wrongRoot).Error; err != nil {
		t.Fatal(err)
	}
	search, err := svc.Search(t.Context(), user.ID, library, root, ResourceSearchInput{Query: "Sintel 4K", RootID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		mediaID string
		message string
	}{
		{name: "library", mediaID: wrongLibrary.ID, message: "不属于当前媒体库"},
		{name: "root", mediaID: wrongRoot.ID, message: "不属于当前入库目录"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := svc.Create(t.Context(), user.ID, library, root, ResourceImportCreateInput{
				SearchSessionID: search.SessionID, CandidateIndex: 0, RootID: root.ID, UpgradeMediaID: test.mediaID,
			})
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("upgrade error = %v, want %q", err, test.message)
			}
		})
	}
	pipeline.mu.Lock()
	defer pipeline.mu.Unlock()
	if pipeline.createCalls != 0 {
		t.Fatalf("pipeline create calls = %d", pipeline.createCalls)
	}
}

func TestResourceImportUpgradeRejectsNonPrimaryMediaVersion(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, repos, library, root, _, user := newResourceImportTestService(t, pipeline)
	primary := model.Media{
		LibraryID: library.ID, LibraryRootID: root.ID, Title: "MIMK-267", TMDbID: 267,
		Path: "cloud://openlist/115/成人/MIMK-267/MIMK-267.mp4", SizeBytes: 4 << 30,
	}
	auxiliary := model.Media{
		LibraryID: library.ID, LibraryRootID: root.ID, Title: "MIMK-267", TMDbID: 267,
		Path: "cloud://openlist/115/成人/MIMK-267/ad.mp4", SizeBytes: 2 << 20,
	}
	if err := repos.DB.Create(&primary).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&auxiliary).Error; err != nil {
		t.Fatal(err)
	}
	search, err := svc.Search(t.Context(), user.ID, library, root, ResourceSearchInput{Query: "MIMK-267", RootID: root.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(t.Context(), user.ID, library, root, ResourceImportCreateInput{
		SearchSessionID: search.SessionID, CandidateIndex: 0, RootID: root.ID, UpgradeMediaID: auxiliary.ID,
	})
	if err == nil || !strings.Contains(err.Error(), "目标不是作品主片源") {
		t.Fatalf("upgrade error = %v", err)
	}
	pipeline.mu.Lock()
	defer pipeline.mu.Unlock()
	if pipeline.createCalls != 0 {
		t.Fatalf("pipeline create calls = %d", pipeline.createCalls)
	}
}

func TestMapPipelineImportStateKeepsUnknownActiveStageRunning(t *testing.T) {
	status, stage := mapPipelineImportState(resourcePipelineTask{Status: "running", Stage: "future_stage"})
	if status != ResourceImportStatusRunning || stage != "running" {
		t.Fatalf("mapped state = %q/%q", status, stage)
	}
}

func TestApplyPipelineTaskReportsRawInvalidState(t *testing.T) {
	pipeline := &fakeResourcePipeline{}
	svc, _, _, _, _, _ := newResourceImportTestService(t, pipeline)
	job := model.ResourceImportJob{Attempt: 1}
	job.ID = "job-1"
	err := svc.applyPipelineTask(t.Context(), &job, resourcePipelineTask{})
	var stateErr *resourcePipelineStateError
	if !errors.As(err, &stateErr) || stateErr.Status != "" || stateErr.Stage != "" {
		t.Fatalf("state error = %#v", err)
	}
}
