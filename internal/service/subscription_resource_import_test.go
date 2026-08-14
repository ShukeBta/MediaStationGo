package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubscriptionTargetLocalAvailabilityScopesRootAndSeason(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	library := model.Library{Name: "Anime", Path: "cloud://openlist/115%2Fanime", Type: "anime", Enabled: true}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: library.ID, Name: "Main", Path: library.Path, Enabled: true}
	otherRoot := model.LibraryRoot{LibraryID: library.ID, Name: "Other", Path: "cloud://openlist/115%2Fother", Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherRoot).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: library.ID, LibraryRootID: root.ID, Title: "Test Show", Path: root.Path + "/Test Show/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1},
		{LibraryID: library.ID, LibraryRootID: root.ID, Title: "Test Show", Path: root.Path + "/Test Show/S02E01.mkv", SeasonNum: 2, EpisodeNum: 1},
		{LibraryID: library.ID, LibraryRootID: otherRoot.ID, Title: "Test Show", Path: otherRoot.Path + "/Test Show/S02E02.mkv", SeasonNum: 2, EpisodeNum: 2},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	sub := &model.Subscription{
		Name: "Test Show", Filter: "Test Show", MediaType: "anime", TotalEpisodes: 2,
		LibraryID: library.ID, LibraryRootID: root.ID, SeasonNumber: 2,
	}

	got, err := SubscriptionTargetLocalAvailability(t.Context(), repos, sub)
	if err != nil {
		t.Fatal(err)
	}
	if got.LocalMediaCount != 2 || got.DownloadedEpisodes != 1 || len(got.MissingEpisodes) != 1 || got.MissingEpisodes[0] != 2 {
		t.Fatalf("availability = %+v", got)
	}
	if _, ok := got.ExistingEpisodeKeys[episodeKey(2, 1)]; !ok {
		t.Fatalf("season 2 episode 1 not found: %+v", got.ExistingEpisodeKeys)
	}
	if _, ok := got.ExistingEpisodeKeys[episodeKey(1, 1)]; ok {
		t.Fatalf("season 1 leaked into target season: %+v", got.ExistingEpisodeKeys)
	}
}

func TestSelectResourceImportSubscriptionCandidatesKeepsOnlyMissingSingles(t *testing.T) {
	sub := &model.Subscription{
		Name: "Test Show", Filter: "Test Show", MediaType: "tv", DeliveryMode: subscriptionDeliveryResourceImport,
		SeasonNumber: 1, TotalEpisodes: 3,
	}
	local := LocalAvailability{
		LocalMediaCount: 1, DownloadedEpisodes: 1, TotalEpisodes: 3,
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}},
		MissingEpisodes:     []int{2, 3},
	}
	items := []ResourceSearchCandidate{
		{Index: 0, Title: "Test Show S01E01 1080p"},
		{Index: 1, Title: "Test Show S01E02 1080p"},
		{Index: 2, Title: "Test Show Complete 1080p"},
	}

	got := selectResourceImportSubscriptionCandidates(items, sub, local)
	if len(got) != 1 || got[0].Episode != 2 || got[0].Item.SiteID != "1" {
		t.Fatalf("selected candidates = %+v", got)
	}
	if !resourceCandidateIsExplicitlyMissing(got[0], local) {
		t.Fatal("missing episode should be allowed to bypass the title-level duplicate check")
	}
}

func TestSelectResourceImportSubscriptionCandidatesTreatsUpdatedToAsCumulativePack(t *testing.T) {
	sub := &model.Subscription{
		Name: "凡人修仙传", Filter: "凡人修仙传", MediaType: "anime", DeliveryMode: subscriptionDeliveryResourceImport,
		SeasonNumber: 1, TotalEpisodes: 124,
	}
	existing := map[string]struct{}{}
	for episode := 1; episode <= 114; episode++ {
		existing[episodeKey(1, episode)] = struct{}{}
	}
	local := LocalAvailability{
		LocalMediaCount: 114, DownloadedEpisodes: 114, TotalEpisodes: 124,
		ExistingEpisodeKeys: existing, MissingEpisodes: []int{115, 116, 117, 118, 119, 120, 121, 122, 123, 124},
	}
	for _, end := range []int{121, 122, 123, 124} {
		items := []ResourceSearchCandidate{{Index: 0, Title: fmt.Sprintf("凡人修仙传 更新至%d集 1080p", end)}}
		got := selectResourceImportSubscriptionCandidates(items, sub, local)
		if len(got) != 1 || got[0].Episode != 1 || len(got[0].Episodes) != end || !got[0].Pack {
			t.Fatalf("end=%d selected=%+v", end, got)
		}
		if class := resourceImportCandidateTitleClass(got[0]); class != "cumulative_pack" {
			t.Fatalf("end=%d title class=%q", end, class)
		}
	}
}

func TestSelectResourceImportSubscriptionCandidatesPrioritizesCompleteMissingCoverage(t *testing.T) {
	sub := &model.Subscription{
		Name: "凡人修仙传", Filter: "凡人修仙传", MediaType: "anime", DeliveryMode: subscriptionDeliveryResourceImport,
		SeasonNumber: 1, TotalEpisodes: 120,
	}
	existing := map[string]struct{}{}
	for episode := 1; episode <= 114; episode++ {
		existing[episodeKey(1, episode)] = struct{}{}
	}
	local := LocalAvailability{
		LocalMediaCount: 114, DownloadedEpisodes: 114, TotalEpisodes: 120,
		ExistingEpisodeKeys: existing, MissingEpisodes: []int{115, 116, 117, 118, 119, 120},
	}
	items := []ResourceSearchCandidate{
		{Index: 0, Title: "凡人修仙传 更新至143集 2160p", Seeders: 999},
		{Index: 1, Title: "凡人修仙传 S01E115-E120 1080p", Seeders: 1},
		{Index: 2, Title: "凡人修仙传 120集全 1080p", Seeders: 2},
		{Index: 3, Title: "凡人修仙传 S01E115 1080p", Seeders: 50},
	}
	got := selectResourceImportSubscriptionCandidates(items, sub, local)
	if len(got) != 4 {
		t.Fatalf("selected candidates = %+v", got)
	}
	for index, siteID := range []string{"1", "2", "0", "3"} {
		if got[index].Item.SiteID != siteID {
			t.Fatalf("candidate order = %+v", got)
		}
	}
}

func TestSelectResourceImportSubscriptionCandidatesUnknownTotalRequiresFrontier(t *testing.T) {
	sub := &model.Subscription{
		Name: "凡人修仙传", Filter: "凡人修仙传", MediaType: "anime", DeliveryMode: subscriptionDeliveryResourceImport,
		SeasonNumber: 1,
	}
	existing := map[string]struct{}{}
	for episode := 1; episode <= 114; episode++ {
		existing[episodeKey(1, episode)] = struct{}{}
	}
	local := LocalAvailability{LocalMediaCount: 114, DownloadedEpisodes: 114, ExistingEpisodeKeys: existing}
	items := []ResourceSearchCandidate{
		{Index: 0, Title: "凡人修仙传 S01E143 1080p", Seeders: 999},
		{Index: 1, Title: "凡人修仙传 S01E115 1080p", Seeders: 1},
		{Index: 2, Title: "凡人修仙传 120集全 1080p", Seeders: 2},
	}

	got := selectResourceImportSubscriptionCandidates(items, sub, local)
	if len(got) != 2 || got[0].Item.SiteID != "2" || got[1].Item.SiteID != "1" {
		t.Fatalf("candidate order = %+v, want full pack then E115 and no E143", got)
	}
}

func TestSelectResourceImportSubscriptionCandidatesKnownTotalRequiresFrontier(t *testing.T) {
	sub := &model.Subscription{
		Name: "凡人修仙传", Filter: "凡人修仙传", MediaType: "anime", DeliveryMode: subscriptionDeliveryResourceImport,
		SeasonNumber: 1, TotalEpisodes: 177,
	}
	existing := map[string]struct{}{}
	for episode := 1; episode <= 114; episode++ {
		existing[episodeKey(1, episode)] = struct{}{}
	}
	missing := make([]int, 0, 63)
	for episode := 115; episode <= 177; episode++ {
		missing = append(missing, episode)
	}
	local := LocalAvailability{
		LocalMediaCount: 114, DownloadedEpisodes: 114, TotalEpisodes: 177,
		ExistingEpisodeKeys: existing, MissingEpisodes: missing,
	}
	items := []ResourceSearchCandidate{
		{Index: 0, Title: "凡人修仙传 S01E177 2160p", Seeders: 999},
		{Index: 1, Title: "凡人修仙传 S01E115 1080p", Seeders: 1},
	}

	got := selectResourceImportSubscriptionCandidates(items, sub, local)
	if len(got) != 1 || got[0].Item.SiteID != "1" {
		t.Fatalf("candidate order = %+v, want E115 and no E177", got)
	}
}

func TestSelectResourceImportSubscriptionCandidatesExactSingleWinsWhenOnlyOneMissing(t *testing.T) {
	sub := &model.Subscription{
		Name: "凡人修仙传", Filter: "凡人修仙传", MediaType: "anime", DeliveryMode: subscriptionDeliveryResourceImport,
		SeasonNumber: 1, TotalEpisodes: 115,
	}
	existing := map[string]struct{}{}
	for episode := 1; episode <= 114; episode++ {
		existing[episodeKey(1, episode)] = struct{}{}
	}
	local := LocalAvailability{
		LocalMediaCount: 114, DownloadedEpisodes: 114, TotalEpisodes: 115,
		ExistingEpisodeKeys: existing, MissingEpisodes: []int{115},
	}
	items := []ResourceSearchCandidate{
		{Index: 0, Title: "凡人修仙传 115集全 2160p", Seeders: 999},
		{Index: 1, Title: "凡人修仙传 S01E115 1080p", Seeders: 1},
	}

	got := selectResourceImportSubscriptionCandidates(items, sub, local)
	if len(got) != 2 || got[0].Item.SiteID != "1" {
		t.Fatalf("candidate order = %+v, want exact E115 first", got)
	}
}

func TestSelectResourceImportSubscriptionCandidatesKeepsAlternativeLinksForSameEpisode(t *testing.T) {
	sub := &model.Subscription{
		Name: "凡人修仙传", Filter: "凡人修仙传", MediaType: "anime", DeliveryMode: subscriptionDeliveryResourceImport,
		SeasonNumber: 1, TotalEpisodes: 115,
	}
	existing := map[string]struct{}{}
	for episode := 1; episode <= 114; episode++ {
		existing[episodeKey(1, episode)] = struct{}{}
	}
	local := LocalAvailability{
		LocalMediaCount: 114, DownloadedEpisodes: 114, TotalEpisodes: 115,
		ExistingEpisodeKeys: existing, MissingEpisodes: []int{115},
	}
	items := []ResourceSearchCandidate{
		{Index: 0, Title: "凡人修仙传 S01E115 1080p WEB-DL", Seeders: 1},
		{Index: 1, Title: "凡人修仙传 S01E115 1080p WEB-DL", Seeders: 2},
	}

	got := selectResourceImportSubscriptionCandidates(items, sub, local)
	if len(got) != 2 || got[0].Item.SiteID != "1" || got[1].Item.SiteID != "0" {
		t.Fatalf("candidate alternatives = %+v", got)
	}
}

func TestSubscriptionTargetOpenListPathUsesExistingSeasonDirectory(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	library := model.Library{Name: "Anime", Path: "cloud://openlist/115%2F动漫", Type: "anime", Enabled: true}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: library.ID, Path: library.Path, Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	for episode := 1; episode <= 2; episode++ {
		if err := db.Create(&model.Media{
			LibraryID: library.ID, LibraryRootID: root.ID, Title: "凡人修仙传", SeasonNum: 1, EpisodeNum: episode,
			Path: fmt.Sprintf("cloud://openlist/115/动漫/凡人修仙传 (2020)/Season 1/凡人修仙传.S01E%02d.mkv", episode),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	sub := &model.Subscription{Name: "凡人修仙传", Filter: "凡人修仙传", LibraryID: library.ID, LibraryRootID: root.ID, SeasonNumber: 1}
	got, err := SubscriptionTargetOpenListPath(t.Context(), repos, sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/115/动漫/凡人修仙传 (2020)/Season 1" {
		t.Fatalf("target path = %q", got)
	}
}

type subscriptionResourcePipeline struct {
	mu           sync.Mutex
	items        []map[string]any
	searches     []resourcePipelineSearchRequest
	creates      []resourcePipelineCreateRequest
	createErrors map[string]error
	nextTask     int
}

func (f *subscriptionResourcePipeline) Search(_ context.Context, in resourcePipelineSearchRequest) (resourcePipelineSearchResponse, error) {
	f.mu.Lock()
	f.searches = append(f.searches, in)
	f.mu.Unlock()
	return resourcePipelineSearchResponse{
		SessionID:    "pipeline-" + in.OwnerID,
		ExpiresAt:    time.Now().Add(15 * time.Minute).Unix(),
		Items:        f.items,
		Capabilities: ResourceSearchCapabilities{Pansou: true},
	}, nil
}

func (f *subscriptionResourcePipeline) CreateImport(_ context.Context, owner, _ string, in resourcePipelineCreateRequest) (resourcePipelineTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextTask++
	f.creates = append(f.creates, in)
	if err := f.createErrors[in.CandidateID]; err != nil {
		return resourcePipelineTask{}, err
	}
	return resourcePipelineTask{ID: fmt.Sprintf("job-%d", f.nextTask), OwnerID: owner, Status: "queued", Stage: "queued"}, nil
}

func (f *subscriptionResourcePipeline) GetImport(_ context.Context, owner, id string) (resourcePipelineTask, error) {
	return resourcePipelineTask{ID: id, OwnerID: owner, Status: "completed", Stage: "completed", MsgMediaID: "media-" + id}, nil
}

func (f *subscriptionResourcePipeline) CancelImport(_ context.Context, owner, id string) (resourcePipelineTask, error) {
	return resourcePipelineTask{ID: id, OwnerID: owner, Status: "canceled", Stage: "canceled"}, nil
}

func (f *subscriptionResourcePipeline) RetryImport(_ context.Context, owner, id string) (resourcePipelineTask, error) {
	return resourcePipelineTask{ID: id, OwnerID: owner, Status: "queued", Stage: "queued"}, nil
}

func TestRunResourceImportSubscriptionQueuesMissingEpisodesThroughExistingService(t *testing.T) {
	db := newServiceTestDB(t,
		&model.User{}, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Subscription{},
		&model.ResourceSearchSession{}, &model.ResourceImportJob{},
	)
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	repos := repository.New(db)
	user := model.User{Username: "admin", PasswordHash: "x", Role: "admin", IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	library := model.Library{Name: "TV", Path: "cloud://openlist/115%2Ftv", Type: "tv", Enabled: true}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: library.ID, Name: "TV", Path: library.Path, Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Media{
		LibraryID: library.ID, LibraryRootID: root.ID, Title: "Test Show",
		Path: root.Path + "/Test Show/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	sub := &model.Subscription{
		UserID: user.ID, Name: "Test Show", FeedURL: "resource-import://pansou", Filter: "Test Show",
		DeliveryMode: subscriptionDeliveryResourceImport, ResourceSource: "pansou",
		LibraryID: library.ID, LibraryRootID: root.ID, MediaType: "tv", SeasonNumber: 1,
		TotalEpisodes: 3, MaxImportsPerRun: 2, Enabled: true,
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	pipeline := &subscriptionResourcePipeline{items: []map[string]any{
		{"candidate_id": "c1", "title": "Test Show S01E01 1080p"},
		{"candidate_id": "c2", "title": "Test Show S01E02 1080p"},
		{"candidate_id": "c3", "title": "Test Show S01E03 1080p"},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resourceImport := newResourceImportServiceWithClient(config.ResourceImportConfig{
		Enabled: true, MaxConcurrent: 1, MaxConcurrentPerUser: 1, PollSeconds: 1,
	}, nil, repos, ctx, pipeline)
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	svc.SetResourceImport(resourceImport)

	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 || len(pipeline.creates) != 1 {
		t.Fatalf("queued=%d creates=%d", queued, len(pipeline.creates))
	}
	for _, request := range pipeline.creates {
		if request.ForceDuplicate || !request.SubscriptionFollow {
			t.Fatalf("automatic follow contract = %+v", request)
		}
	}
	if len(pipeline.searches) == 0 || !pipeline.searches[0].SubscriptionFollow {
		t.Fatalf("subscription search contract = %+v", pipeline.searches)
	}
	var jobs []model.ResourceImportJob
	if err := db.Where("subscription_id = ?", sub.ID).Find(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("subscription jobs = %d", len(jobs))
	}
}

func TestRunResourceImportSubscriptionSkipsBlockedSourceAndQueuesNextCandidate(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Subscription{}, &model.ResourceSearchSession{}, &model.ResourceImportJob{})
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	repos := repository.New(db)
	user := model.User{Username: "admin", PasswordHash: "x", Role: "admin", IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	library := model.Library{Name: "TV", Path: "cloud://openlist/115%2Ftv", Type: "tv", Enabled: true}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: library.ID, Name: "TV", Path: library.Path, Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Media{LibraryID: library.ID, LibraryRootID: root.ID, Title: "Test Show", Path: root.Path + "/Test Show/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1}).Error; err != nil {
		t.Fatal(err)
	}
	sub := &model.Subscription{UserID: user.ID, Name: "Test Show", FeedURL: "resource-import://pansou", Filter: "Test Show", DeliveryMode: subscriptionDeliveryResourceImport, ResourceSource: "pansou", LibraryID: library.ID, LibraryRootID: root.ID, MediaType: "tv", SeasonNumber: 1, TotalEpisodes: 3, MaxImportsPerRun: 2, Enabled: true}
	if err := db.Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	pipeline := &subscriptionResourcePipeline{
		items:        []map[string]any{{"candidate_id": "blocked", "title": "Test Show S01E02 1080p", "seeders": 10}, {"candidate_id": "usable", "title": "Test Show S01E03 1080p", "seeders": 1}},
		createErrors: map[string]error{"blocked": &resourcePipelineError{StatusCode: 409, Code: "subscription_source_blocked", Message: "blocked"}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resourceImport := newResourceImportServiceWithClient(config.ResourceImportConfig{Enabled: true, MaxConcurrent: 1, MaxConcurrentPerUser: 1, PollSeconds: 1}, nil, repos, ctx, pipeline)
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	svc.SetResourceImport(resourceImport)
	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 || len(pipeline.creates) != 2 || pipeline.creates[1].CandidateID != "usable" {
		t.Fatalf("queued=%d creates=%+v", queued, pipeline.creates)
	}
	var jobs []model.ResourceImportJob
	if err := db.Where("subscription_id = ?", sub.ID).Find(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	statuses := map[int]string{}
	for _, job := range jobs {
		statuses[job.CandidateIndex] = job.Status
	}
	if len(jobs) != 2 || statuses[0] != ResourceImportStatusFailed || (statuses[1] != ResourceImportStatusQueued && statuses[1] != ResourceImportStatusCompleted) {
		t.Fatalf("subscription jobs = %+v", jobs)
	}
}
