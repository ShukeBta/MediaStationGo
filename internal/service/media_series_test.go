package service

import (
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestListRecentSeriesCardsCountsAllEpisodesInSeries(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "国漫", Path: "/media/anime", Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	rows := make([]model.Media, 0, 40)
	for i := 1; i <= 40; i++ {
		created := now.Add(-48 * time.Hour)
		if i > 23 {
			created = now.Add(time.Duration(i) * time.Minute)
		}
		rows = append(rows, model.Media{
			Base:       model.Base{ID: fmt.Sprintf("recent-ep-%02d", i), CreatedAt: created, UpdatedAt: created},
			LibraryID:  lib.ID,
			Title:      "史上最强炼体老祖",
			Path:       fmt.Sprintf("/media/anime/国漫/史上最强炼体老祖/Season 01/史上最强炼体老祖.S01E%02d.mkv", i),
			SeasonNum:  1,
			EpisodeNum: i,
		})
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)

	cards, err := svc.ListRecentSeriesCards(t.Context(), 24, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 {
		t.Fatalf("recent cards = %#v, want one series card", cards)
	}
	if cards[0].Count != 40 {
		t.Fatalf("recent series count = %d, want full 40 episodes", cards[0].Count)
	}
}

func TestMediaSeriesKeyCollapsesNestedSpecialFolders(t *testing.T) {
	main := model.Media{
		LibraryID:  "lib-tv",
		Path:       `cloud://openlist/动漫/国漫/示例剧/Season 01/示例剧.S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	special := model.Media{
		LibraryID: "lib-tv",
		Path:      `cloud://openlist/动漫/国漫/示例剧/Extras/Season 01/示例剧.SP01.mkv`,
	}

	if got, want := mediaSeriesKey(special), mediaSeriesKey(main); got != want {
		t.Fatalf("special key=%q, want main key=%q", got, want)
	}

	cards := groupMediaSeriesCards([]model.Media{main, special})
	if len(cards) != 1 || cards[0].Count != 2 {
		t.Fatalf("cards=%#v, want one merged series card with two items", cards)
	}
}

func TestMediaSeriesKeyMergesMatchedSeasonsAcrossDifferentDirectories(t *testing.T) {
	seasonTwo := model.Media{
		LibraryID:    "lib-tv",
		Title:        "模范出租车",
		OriginalName: "모범택시",
		Path:         `cloud://openlist/115/剧集/模范出租车2 [import-29430b23083f]/【高清剧集网 www.BTHDTV.com】模范出租车2[全16集][简繁字幕].Taxi.Driver.S02.1080p.SBS.WEB-DL.AAC2.0.H.264-BlackTV/Taxi.Driver.S02E01.1080p.SBS.WEB-DL.AAC2.0.H.264-BlackTV.mkv`,
		SeasonNum:    2,
		EpisodeNum:   1,
		TMDbID:       119769,
		ScrapeStatus: "matched",
	}
	seasonThree := model.Media{
		LibraryID:    "lib-tv",
		Title:        "模范出租车",
		OriginalName: "모범택시",
		Path:         `cloud://openlist/115/剧集/模范出租车3/Taxi.Driver.S03E01.1080p.WEB-DL.AAC2.0.H.264-BlackTV.mkv`,
		SeasonNum:    3,
		EpisodeNum:   1,
		TMDbID:       119769,
		ScrapeStatus: "matched",
	}

	if seasonTwoPathTitle, seasonThreePathTitle := seriesTitleFromMediaPath(seasonTwo.Path), seriesTitleFromMediaPath(seasonThree.Path); seasonTwoPathTitle == seasonThreePathTitle {
		t.Fatalf("test requires distinct path titles, both were %q", seasonTwoPathTitle)
	}
	if got, want := mediaSeriesKey(seasonTwo), mediaSeriesKey(seasonThree); got != want {
		t.Fatalf("matched seasons split by directory: key=%q, want %q", got, want)
	}
	cards := groupMediaSeriesCards([]model.Media{seasonTwo, seasonThree})
	if len(cards) != 1 || cards[0].Count != 2 {
		t.Fatalf("cards=%#v, want one matched series card with two seasons", cards)
	}

	seasonTwo.ScrapeStatus = "pending"
	seasonThree.ScrapeStatus = "pending"
	if got, other := mediaSeriesKey(seasonTwo), mediaSeriesKey(seasonThree); got == other {
		t.Fatalf("pending seasons unexpectedly ignored distinct path identities: key=%q", got)
	}
}

func TestMediaSeriesKeyCollapsesSPsFolder(t *testing.T) {
	const seriesDirectory = `cloud://openlist/115/动漫/[Maho.sub&VCB-Studio] Aki Sora Yume no Naka [Hi10p_1080p]`
	main := model.Media{
		LibraryID:  "lib-anime",
		Title:      "Aki Sora OVA",
		Path:       seriesDirectory + `/[Maho.sub&VCB-Studio] Aki Sora Yume no Naka [01][Hi10p_1080p][x264_flac].mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
		TMDbID:     1281913,
	}
	special := model.Media{
		LibraryID:  "lib-anime",
		Title:      "Aki Sora OVA",
		Path:       seriesDirectory + `/SPs/[Maho.sub&VCB-Studio] Aki Sora Yume no Naka [NCED][Hi10p_1080p][x264_flac].mkv`,
		SeasonNum:  1,
		EpisodeNum: 2,
		TMDbID:     1281913,
	}

	if got, want := mediaSeriesKey(special), mediaSeriesKey(main); got != want {
		t.Fatalf("SPs key=%q, want main key=%q", got, want)
	}
	cards := groupMediaSeriesCards([]model.Media{main, special})
	if len(cards) != 1 || cards[0].Count != 2 {
		t.Fatalf("cards=%#v, want one merged Aki Sora card with two items", cards)
	}
}

func TestGroupMediaSeriesCardsSortsByLatestEpisodeTime(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	newerFirstEpisode := model.Media{
		Base:       model.Base{CreatedAt: now.Add(-72 * time.Hour), UpdatedAt: now.Add(-72 * time.Hour)},
		LibraryID:  "lib-tv",
		Title:      "更新合集",
		Path:       `F:\media\电视剧\国产剧\更新合集\Season 01\更新合集.S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	newerLatestEpisode := model.Media{
		Base:       model.Base{CreatedAt: now, UpdatedAt: now},
		LibraryID:  "lib-tv",
		Title:      "更新合集",
		Path:       `F:\media\电视剧\国产剧\更新合集\Season 01\更新合集.S01E02.mkv`,
		SeasonNum:  1,
		EpisodeNum: 2,
	}
	olderSeries := model.Media{
		Base:       model.Base{CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour)},
		LibraryID:  "lib-tv",
		Title:      "较早合集",
		Path:       `F:\media\电视剧\国产剧\较早合集\Season 01\较早合集.S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}

	cards := groupMediaSeriesCards([]model.Media{olderSeries, newerFirstEpisode, newerLatestEpisode})
	if len(cards) != 2 {
		t.Fatalf("cards=%#v, want two series cards", cards)
	}
	if cards[0].Key != mediaSeriesKey(newerFirstEpisode) {
		t.Fatalf("first card key=%q, want latest series key=%q", cards[0].Key, mediaSeriesKey(newerFirstEpisode))
	}
	if cards[0].Count != 2 {
		t.Fatalf("latest series count=%d, want 2", cards[0].Count)
	}
}

func TestGroupMediaSeriesCardsSortsByIngestTimeNotReleaseDate(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	olderIngest := model.Media{
		Base:        model.Base{CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now},
		LibraryID:   "lib-movie",
		Title:       "新上映但更早入库",
		Path:        `/media/movies/new-release.mkv`,
		ReleaseDate: "2026-12-31",
		Year:        2026,
		TMDbID:      1001,
	}
	newerIngest := model.Media{
		Base:        model.Base{CreatedAt: now, UpdatedAt: now},
		LibraryID:   "lib-movie",
		Title:       "旧作品但刚入库",
		Path:        `/media/movies/old-release.mkv`,
		ReleaseDate: "1999-01-01",
		Year:        1999,
		TMDbID:      1002,
	}

	cards := groupMediaSeriesCards([]model.Media{olderIngest, newerIngest})
	if len(cards) != 2 {
		t.Fatalf("cards=%#v, want two cards", cards)
	}
	if cards[0].Rep.Title != newerIngest.Title {
		t.Fatalf("first card=%q, want newest ingested item %q", cards[0].Rep.Title, newerIngest.Title)
	}
}

func TestMediaSeriesKeyIgnoresOpenListSingleFileWrapperDirectory(t *testing.T) {
	first := model.Media{
		LibraryID:  "lib-tv",
		Title:      "异形：地球",
		Path:       `cloud://openlist/115/剧集/Alien.Earth.S01E01.2160p/Alien.Earth.S01E01.2160p.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
		TMDbID:     157239,
	}
	second := model.Media{
		LibraryID:  "lib-tv",
		Title:      "异形：地球",
		Path:       `cloud://openlist/115/剧集/Alien.Earth.S01E02.2160p/Alien.Earth.S01E02.2160p.mkv`,
		SeasonNum:  1,
		EpisodeNum: 2,
		TMDbID:     157239,
	}

	if got := seriesTitleFromMediaPath(first.Path); got != "" {
		t.Fatalf("single-file wrapper produced path title %q, want empty", got)
	}
	if got, want := mediaSeriesKey(first), mediaSeriesKey(second); got != want {
		t.Fatalf("single-file wrapper split series key=%q, want %q", got, want)
	}
	cards := groupMediaSeriesCards([]model.Media{first, second})
	if len(cards) != 1 || cards[0].Count != 2 {
		t.Fatalf("cards=%#v, want one merged series with two episodes", cards)
	}
}

func TestMediaSeriesKeyIgnoresOpenListPerEpisodeReleaseDirectories(t *testing.T) {
	flatReleaseFiles := []string{
		"Alien - Earth (2025) - S01E01 - Neverland [DSNP WEBDL-1080p][EAC3 5.1][h264]-Kitsune.mkv",
		"Alien - Earth (2025) - S01E02 - Mr. October [DSNP WEBDL-1080p][EAC3 5.1][h264]-FLUX.mkv",
	}
	paths := make([]string, 0, 8)
	for _, file := range flatReleaseFiles {
		paths = append(paths, "cloud://openlist/115/剧集/"+file+"/"+file)
	}
	for index, host := range []string{"TTHDTT", "TTHDTT", "BTHDTV", "BBEGGE", "TTHDTT", "BBHDTV"} {
		episode := index + 3
		directory := fmt.Sprintf(
			"【高清剧集网发布 www.%s.com】异形：地球.第一季[第%02d集][简繁英字幕].Alien.Earth.S01.2025.2160p.DSNP.WEB-DL.DDP5.1.HDR.H.265-ColorTV",
			host,
			episode,
		)
		file := fmt.Sprintf("Alien.Earth.S01E%02d.2025.2160p.DSNP.WEB-DL.DDP5.1.HDR.H.265-ColorTV.mkv", episode)
		paths = append(paths, "cloud://openlist/115/剧集/"+directory+"/"+file)
	}

	items := make([]model.Media, 0, len(paths))
	for index, path := range paths {
		if got := seriesTitleFromMediaPath(path); got != "" {
			t.Fatalf("episode %d release directory produced path title %q, want empty", index+1, got)
		}
		items = append(items, model.Media{
			Base:       model.Base{ID: fmt.Sprintf("alien-earth-%d", index+1)},
			LibraryID:  "lib-tv",
			Title:      "异形：地球",
			Path:       path,
			SeasonNum:  1,
			EpisodeNum: index + 1,
			TMDbID:     157239,
		})
	}

	cards := groupMediaSeriesCards(items)
	if len(cards) != 1 || cards[0].Count != 8 {
		t.Fatalf("cards=%#v, want one merged series with eight episodes", cards)
	}
}

func TestMediaSeriesKeyCollapsesSpecialTitleSuffix(t *testing.T) {
	main := model.Media{
		LibraryID:  "lib-tv",
		Path:       `cloud://openlist/电视剧/欧美剧/Example Show/Season 01/Example.Show.S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	special := model.Media{
		LibraryID:  "lib-tv",
		Path:       `cloud://openlist/电视剧/欧美剧/Example Show Specials/Example.Show.Special.01.mkv`,
		SeasonNum:  0,
		EpisodeNum: 1,
	}
	chineseSpecial := model.Media{
		LibraryID:  "lib-tv",
		Path:       `cloud://openlist/动漫/国漫/示例剧 特别篇/示例剧.SP01.mkv`,
		SeasonNum:  0,
		EpisodeNum: 1,
	}
	chineseMain := model.Media{
		LibraryID:  "lib-tv",
		Path:       `cloud://openlist/动漫/国漫/示例剧/Season 01/示例剧.S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}

	if got, want := mediaSeriesKey(special), mediaSeriesKey(main); got != want {
		t.Fatalf("english special key=%q, want main key=%q", got, want)
	}
	if got, want := mediaSeriesKey(chineseSpecial), mediaSeriesKey(chineseMain); got != want {
		t.Fatalf("chinese special key=%q, want main key=%q", got, want)
	}
}

func TestMediaSeriesKeyCollapsesSeasonZeroAndSpecialAliases(t *testing.T) {
	main := model.Media{
		LibraryID:  "lib-anime",
		Path:       `cloud://openlist/动漫/日番/宝可梦 (1997) {tmdb-60572}/Season 1/宝可梦.S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	seasonZero := model.Media{
		LibraryID:  "lib-anime",
		Path:       `cloud://openlist/动漫/日番/宝可梦 (1997) {tmdb-60572}/Season 0/宝可梦.S00E34.mkv`,
		SeasonNum:  0,
		EpisodeNum: 34,
	}
	specialEpisode := model.Media{
		LibraryID:  "lib-anime",
		Path:       `cloud://openlist/动漫/日番/宝可梦 Special Episode/宝可梦.SP01.mkv`,
		SeasonNum:  0,
		EpisodeNum: 1,
	}
	extraEpisode := model.Media{
		LibraryID:  "lib-anime",
		Path:       `cloud://openlist/动漫/日番/宝可梦 番外篇/宝可梦.SP02.mkv`,
		SeasonNum:  0,
		EpisodeNum: 2,
	}

	want := mediaSeriesKey(main)
	for name, item := range map[string]model.Media{
		"season zero":     seasonZero,
		"special episode": specialEpisode,
		"番外篇":             extraEpisode,
	} {
		if got := mediaSeriesKey(item); got != want {
			t.Fatalf("%s key=%q, want main key=%q", name, got, want)
		}
	}
}

func TestMediaSeriesKeyCollapsesNumberedSpecialSuffixes(t *testing.T) {
	main := model.Media{
		LibraryID:  "lib-tv",
		Path:       `F:\media\电视剧\欧美剧\Example Show\Season 01\Example Show - S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	chineseMain := model.Media{
		LibraryID:  "lib-tv",
		Path:       `F:\media\电视剧\欧美剧\示例剧\Season 01\示例剧.S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	cases := map[string]struct {
		item model.Media
		want model.Media
	}{
		"sp number": {
			item: model.Media{
				LibraryID:  "lib-tv",
				Path:       `F:\media\电视剧\欧美剧\Example Show SP01\Example Show.SP01.mkv`,
				SeasonNum:  0,
				EpisodeNum: 1,
			},
			want: main,
		},
		"ova number": {
			item: model.Media{
				LibraryID:  "lib-tv",
				Path:       `F:\media\电视剧\欧美剧\Example Show OVA 1\Example Show.OVA.1.mkv`,
				SeasonNum:  0,
				EpisodeNum: 1,
			},
			want: main,
		},
		"season zero episode": {
			item: model.Media{
				LibraryID:  "lib-tv",
				Path:       `F:\media\电视剧\欧美剧\Example Show S00E01\Example Show.S00E01.mkv`,
				SeasonNum:  0,
				EpisodeNum: 1,
			},
			want: main,
		},
		"wrapped special": {
			item: model.Media{
				LibraryID:  "lib-tv",
				Path:       `F:\media\电视剧\欧美剧\Example Show [Special]\Example Show.Special.mkv`,
				SeasonNum:  0,
				EpisodeNum: 1,
			},
			want: main,
		},
		"chinese numbered special": {
			item: model.Media{
				LibraryID:  "lib-tv",
				Path:       `F:\media\电视剧\欧美剧\示例剧 特别篇 第1集\示例剧.SP01.mkv`,
				SeasonNum:  0,
				EpisodeNum: 1,
			},
			want: chineseMain,
		},
	}
	for name, tt := range cases {
		want := mediaSeriesKey(tt.want)
		if got := mediaSeriesKey(tt.item); got != want {
			t.Fatalf("%s key=%q, want main key=%q", name, got, want)
		}
	}
}

func TestMediaSeriesKeyCleansReleaseNoiseFolders(t *testing.T) {
	clean := model.Media{
		LibraryID:  "lib-variety",
		Path:       `F:\media\电视剧\综艺\Hntv Spring Festival Gala S01e (2026)\Season 1\Hntv Spring Festival Gala S01e - S01E202.ts`,
		SeasonNum:  1,
		EpisodeNum: 202,
	}
	dirty := model.Media{
		LibraryID:  "lib-variety",
		Path:       `F:\media\电视剧\综艺\Hntv Spring Festival Gala Fps Hlg Qhstudio S01e (2026)\Season 1\Hntv Spring Festival Gala Fps Hlg Qhstudio S01e - S01E202.ts`,
		SeasonNum:  1,
		EpisodeNum: 202,
	}
	if got, want := mediaSeriesKey(dirty), mediaSeriesKey(clean); got != want {
		t.Fatalf("dirty folder key=%q, want clean folder key=%q", got, want)
	}

	noisyRelease := model.Media{
		LibraryID:  "lib-tv",
		Path:       `F:\media\电视剧\欧美剧\Motherhood Of Taihang Aac2 Mweb\Season 1\Motherhood Of Taihang Aac2 Mweb - S01E01-Aac2.Mweb.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	cleanRelease := model.Media{
		LibraryID:  "lib-tv",
		Path:       `F:\media\电视剧\欧美剧\Motherhood Of Taihang\Season 1\Motherhood Of Taihang - S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	if got, want := mediaSeriesKey(noisyRelease), mediaSeriesKey(cleanRelease); got != want {
		t.Fatalf("release-noise folder key=%q, want clean key=%q", got, want)
	}
}

func TestMediaSeriesKeyTreatsDomesticTelevisionFolderAsSeries(t *testing.T) {
	main := model.Media{
		LibraryID:  "lib-domestic-tv",
		Path:       `/media/国产电视剧/人世间 (2022) [TMDBID-156568]/人世间.S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
		TMDbID:     156568,
	}
	weakEpisode := model.Media{
		LibraryID: "lib-domestic-tv",
		Path:      `/media/国产电视剧/人世间 (2022) [TMDBID-156568]/人世间.S01E02.mkv`,
		// Some local/cloud scans may miss S/E at first while local NFO or
		// scraper metadata already carries an episode-level TMDb id.
		TMDbID: 4375419,
	}
	folderRecord := model.Media{
		LibraryID: "lib-domestic-tv",
		Path:      `/media/国产电视剧/人世间 (2022) [TMDBID-156568]`,
		Title:     "人世间",
		TMDbID:    156568,
	}

	if got, want := mediaSeriesKey(weakEpisode), mediaSeriesKey(main); got != want {
		t.Fatalf("domestic television folder key=%q, want main key=%q", got, want)
	}
	if got, want := mediaSeriesKey(folderRecord), mediaSeriesKey(main); got != want {
		t.Fatalf("domestic television folder record key=%q, want main key=%q", got, want)
	}

	cards := groupMediaSeriesCards([]model.Media{main, weakEpisode, folderRecord})
	if len(cards) != 1 || cards[0].Count != 3 {
		t.Fatalf("cards=%#v, want one merged series card with three items", cards)
	}
}

func TestMediaSeriesKeyUsesSeriesDirectoryExternalID(t *testing.T) {
	episodeIDOnly := model.Media{
		LibraryID:  "lib-domestic-tv",
		Path:       `/media/电视剧/国产剧/人世间 (2022)/Season 01/人世间.S01E03.{tmdb-7129826}.mkv`,
		SeasonNum:  1,
		EpisodeNum: 3,
		TMDbID:     7129826,
	}
	cleanFolder := model.Media{
		LibraryID:  "lib-domestic-tv",
		Path:       `/media/电视剧/国产剧/人世间 (2022)/Season 01/人世间.S01E04.mkv`,
		SeasonNum:  1,
		EpisodeNum: 4,
		TMDbID:     156568,
	}
	if got, want := mediaSeriesKey(episodeIDOnly), mediaSeriesKey(cleanFolder); got != want {
		t.Fatalf("episode filename tmdb id should not split clean folder key=%q, want %q", got, want)
	}
}
