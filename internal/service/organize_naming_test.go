package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// TestOrganizeNaming locks in the rename pipeline used by OrganizeDirectory:
// CleanQuery (title/year) + ParseEpisode (season/episode) + titleCaseWords.
// These cases previously regressed (release tags such as BD/UHD leaking into
// the title, Roman-numeral sequels becoming "Ii", Chinese season markers like
// 第二季 polluting the title).
func TestOrganizeNaming(t *testing.T) {
	cases := []struct {
		file       string
		wantTitle  string // titleCaseWords(CleanQuery) output
		wantYear   int
		wantSeason int
		wantEp     int
	}{
		{"流浪地球2.2023.2160p.WEB-DL.H265.mkv", "流浪地球2", 2023, 0, 0},
		{"[阳光电影www.ygdy8.com].复仇者联盟4.2019.BD.1080p.mkv", "复仇者联盟4", 2019, 0, 0},
		{"狂飙.S01E05.2023.1080p.WEB-DL.mp4", "狂飙", 2023, 1, 5},
		{"The.Wandering.Earth.II.2023.2160p.mkv", "The Wandering Earth II", 2023, 0, 0},
		{"庆余年第二季.Joy.of.Life.S02E01.2024.mp4", "庆余年 Joy Of Life", 2024, 2, 1},
		{"三体.Three-Body.2023.S01E03.4K.mkv", "三体 Three Body", 2023, 1, 3},
		{"Friends.S03E12.1994.720p.mkv", "Friends", 1994, 3, 12},
		{"Oppenheimer.2023.2160p.UHD.BluRay.mkv", "Oppenheimer", 2023, 0, 0},
		{"Rocky.IV.1985.1080p.BluRay.mkv", "Rocky IV", 1985, 0, 0},
		{"Big.Buck.Bunny.2008.1080p.CodexVerify.mp4", "Big Buck Bunny", 2008, 0, 0},
		{"Pressure.2026.2155p.iT.WEB-DL.HEVC.mkv", "Pressure", 2026, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			title, year := CleanQuery(tc.file)
			gotTitle := sanitizeFilename(titleCaseWords(title))
			season, ep := ParseEpisode(tc.file)
			if gotTitle != tc.wantTitle {
				t.Errorf("title = %q, want %q", gotTitle, tc.wantTitle)
			}
			if year != tc.wantYear {
				t.Errorf("year = %d, want %d", year, tc.wantYear)
			}
			if season != tc.wantSeason || ep != tc.wantEp {
				t.Errorf("season/ep = %d/%d, want %d/%d", season, ep, tc.wantSeason, tc.wantEp)
			}
		})
	}
}

// TestTitleCaseWordsRomanNumerals verifies sequel numerals are upper-cased
// while ordinary words that merely resemble numerals are not.
func TestTitleCaseWordsRomanNumerals(t *testing.T) {
	cases := map[string]string{
		"wandering earth ii": "Wandering Earth II",
		"rocky iv":           "Rocky IV",
		"final fantasy vii":  "Final Fantasy VII",
		"the mix tape":       "The Mix Tape", // "mix" must NOT become "MIX"
		"sid and nancy":      "Sid And Nancy",
	}
	for in, want := range cases {
		if got := titleCaseWords(in); got != want {
			t.Errorf("titleCaseWords(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOrganizeDirectoryHonorsConfiguredNamingFormats(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "media")
	repos := newOrganizerTestRepo(t)
	for key, value := range map[string]string{
		"organize.movie_format": "Movies/{title} ({year})/{title} [{year}]",
		"organize.tv_format":    "Series/{title} ({year})/S{season:02}/{title}.S{season:02}E{episode:02}",
		"organize.anime_format": "Bangumi/{title}/Season {season:02}/{title} - {episode:03}",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)

	movieSrc := filepath.Join(root, "downloads", "Dune.2021.2160p.mkv")
	writeOrgFile(t, movieSrc, "movie")
	if _, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   movieSrc,
		DestPath:     dest,
		MediaType:    "movie",
		TransferMode: TransferCopy,
	}); err != nil {
		t.Fatalf("organize movie: %v", err)
	}
	movieWant := filepath.Join(dest, "电影", "Movies", "Dune (2021)", "Dune [2021].mkv")
	if _, err := os.Stat(movieWant); err != nil {
		t.Fatalf("movie naming format not honored, want %q: %v", movieWant, err)
	}

	tvSrc := filepath.Join(root, "downloads", "Some.Show.S02E03.2024.mkv")
	writeOrgFile(t, tvSrc, "tv")
	if _, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   tvSrc,
		DestPath:     dest,
		MediaType:    "tv",
		TransferMode: TransferCopy,
	}); err != nil {
		t.Fatalf("organize tv: %v", err)
	}
	tvWant := filepath.Join(dest, "电视剧", "Series", "Some Show (2024)", "S02", "Some Show.S02E03.mkv")
	if _, err := os.Stat(tvWant); err != nil {
		t.Fatalf("tv naming format not honored, want %q: %v", tvWant, err)
	}

	animeSrc := filepath.Join(root, "downloads", "Frieren.S01E01.2023.mkv")
	writeOrgFile(t, animeSrc, "anime")
	if _, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:    animeSrc,
		DestPath:      dest,
		MediaType:     "anime",
		MediaCategory: "日番",
		TransferMode:  TransferCopy,
	}); err != nil {
		t.Fatalf("organize anime: %v", err)
	}
	animeWant := filepath.Join(dest, "动漫", "日番", "Bangumi", "Frieren", "Season 01", "Frieren - 001.mkv")
	if _, err := os.Stat(animeWant); err != nil {
		t.Fatalf("anime naming format not honored, want %q: %v", animeWant, err)
	}
}

func TestOrganizeDirectoryHonorsTemplateNamingFormat(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "media")
	repos := newOrganizerTestRepo(t)
	template := "{{title}}{% if year %} ({{year}}){% endif %}/Season {{season}}/{{title}} - {{season_episode}}{% if episode %} - 第 {{episode}} 集{% endif %}{{fileExt}}"
	if err := repos.Setting.Set(t.Context(), "organize.tv_format", template); err != nil {
		t.Fatal(err)
	}
	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)

	src := filepath.Join(root, "downloads", "Verify.Show.S01E02.2026.mkv")
	writeOrgFile(t, src, "episode")
	if _, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		MediaType:    "tv",
		TransferMode: TransferCopy,
	}); err != nil {
		t.Fatalf("organize tv: %v", err)
	}

	want := filepath.Join(dest, "电视剧", "Verify Show (2026)", "Season 1", "Verify Show - S01E02 - 第 2 集.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("template naming format not honored, want %q: %v", want, err)
	}
}

func TestOrganizeDirectoryUsesSeriesFolderWhenFileTitleIsOnlyReleaseTags(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "media")
	src := filepath.Join(root, "downloads", "链锯人 总集篇 (2025)", "Season 1", "2025.2160p.120fps.WEB-DL.H265.10bit.DTS5.1.S01E01.mkv")
	writeOrgFile(t, src, "episode")

	repos := newOrganizerTestRepo(t)
	template := "{{title}}{% if year %} ({{year}}){% endif %}/Season {{season}}/{{title}} - {{season_episode}}{% if video_format %} - {{video_format}}{% endif %}{{fileExt}}"
	if err := repos.Setting.Set(t.Context(), "organize.tv_format", template); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "organize.anime_format", template); err != nil {
		t.Fatal(err)
	}
	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	if _, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:    src,
		DestPath:      dest,
		MediaType:     "anime",
		MediaCategory: "日番",
		TransferMode:  TransferCopy,
	}); err != nil {
		t.Fatalf("organize anime: %v", err)
	}

	want := filepath.Join(dest, "动漫", "日番", "链锯人 总集篇 (2025)", "Season 1", "链锯人 总集篇 - S01E01 - 2160p.120fps.WEB-DL.H265.10bit.DTS5.1.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organize should use series folder title and keep video_format, want %q: %v", want, err)
	}
}

func TestOrganizeNamingTemplateKeepsVideoFormatToken(t *testing.T) {
	rel := renderOrganizeNamingTemplate("{title} - {episode_tag} - {video_format}{fileExt}", organizeNamingData{
		Title:       "链锯人 总集篇",
		EpisodeTag:  "S01E01",
		VideoFormat: extractOrganizeReleaseTag("2025.2160p.120fps.WEB-DL.H265.10bit.DTS5.1.mkv"),
		FileExt:     ".mkv",
	})
	if !strings.Contains(rel, "2160p.120fps.WEB-DL.H265.10bit.DTS5.1") {
		t.Fatalf("video_format was lost from rendered name: %q", rel)
	}
}

func TestOrganizePipelineUsesSameNamingFormat(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "Pipeline.Show.S01E02.2026.mkv")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "episode")

	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), "organize.tv_format", "{title}/S{season:02}/{title} - EP{episode:02}"); err != nil {
		t.Fatal(err)
	}
	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	pipeline := NewOrganizePipelineService(zap.NewNop(), repos, organizer, nil, nil)

	res, err := pipeline.Run(t.Context(), OrganizePipelineRequest{
		Scope:        OrganizeScopeDirectory,
		Trigger:      OrganizeTriggerManual,
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: string(TransferCopy),
		MediaType:    "tv",
	})
	if err != nil {
		t.Fatalf("pipeline organize: %v", err)
	}
	if res.Result == nil || res.Result.Organized != 1 {
		t.Fatalf("pipeline result = %#v, want organized=1", res)
	}
	want := filepath.Join(dest, "电视剧", "Pipeline Show", "S01", "Pipeline Show - EP02.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("pipeline did not use configured naming format, want %q: %v", want, err)
	}
}

func TestOrganizeMediaHonorsConfiguredNamingFormat(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "incoming")
	source := filepath.Join(sourceDir, "Some Show S01E02.mkv")
	writeOrgFile(t, source, "episode")

	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), "organize.tv_format", "{title}/Season {season:02}/{title} - 第{episode:02}集"); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "TV", Path: root, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:  lib.ID,
		Title:      "Some Show",
		Path:       source,
		Container:  "mkv",
		SeasonNum:  1,
		EpisodeNum: 2,
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	dst, err := organizer.OrganizeMedia(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Some Show", "Season 01", "Some Show - 第02集.mkv")
	if dst != want {
		t.Fatalf("dst = %q, want %q", dst, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file missing: %v", err)
	}
}
