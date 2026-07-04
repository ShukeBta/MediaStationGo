package service

import "testing"

func TestParseEpisode(t *testing.T) {
	cases := []struct {
		in           string
		wantS, wantE int
	}{
		{"Breaking.Bad.S01E02.1080p.mkv", 1, 2},
		{"breaking.bad.s5e14.mkv", 5, 14},
		{"Friends 1x02.mp4", 1, 2},
		{"Friends 10x24 - The One Where.mkv", 10, 24},
		{"Some Anime - EP05 [1080p].mkv", 1, 5},
		{"Some Anime - E12.mkv", 1, 12},
		{"[MagicStar] 凡人修仙传 年番 - 146 [1080p].mkv", 1, 146},
		{`Some Show/Season 02/Some Show - EP03.mkv`, 2, 3},
		{`Some Show/S02/Some Show - E04.mkv`, 2, 4},
		{`剧集/第2季/剧集 第05集.mkv`, 2, 5},
		{"日剧 第03集.mkv", 1, 3},
		{"日剧 第十集.mkv", 1, 10},
		{"日剧 第二十五话.mkv", 1, 25},
		{"日剧 第12话.mkv", 1, 12},
		{"综艺 第4期下.mkv", 1, 4},
		{`综艺/Season 06/综艺 第17期.mkv`, 6, 17},
		{`动漫/第二季/04.mkv`, 2, 4},
		{`动漫/第十季/第十一集.mkv`, 10, 11},
		{`剧集/S00/剧集 - E01.mkv`, 0, 1},
		{`剧集/Specials/剧集 - 02.mkv`, 0, 2},
		{`剧集/特别篇/03.mkv`, 0, 3},
		{`剧集/剧集 - S00E04.mkv`, 0, 4},
		{"Movie.2020.1080p.mkv", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			s, e := ParseEpisode(tc.in)
			if s != tc.wantS || e != tc.wantE {
				t.Errorf("ParseEpisode(%q) = (%d, %d), want (%d, %d)",
					tc.in, s, e, tc.wantS, tc.wantE)
			}
		})
	}
}

func TestEpisodeRefsFromTitleParsesRanges(t *testing.T) {
	tests := []struct {
		name string
		want []episodeRef
	}{
		{
			name: "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p",
			want: []episodeRef{{Season: 1, Episode: 7}, {Season: 1, Episode: 8}},
		},
		{
			name: "The Heir 2026 S01E39-E42 WEB-DL",
			want: []episodeRef{{Season: 1, Episode: 39}, {Season: 1, Episode: 40}, {Season: 1, Episode: 41}, {Season: 1, Episode: 42}},
		},
	}
	for _, tt := range tests {
		got := episodeRefsFromTitle(tt.name)
		if len(got) != len(tt.want) {
			t.Fatalf("episodeRefsFromTitle(%q) len = %d, want %d: %#v", tt.name, len(got), len(tt.want), got)
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Fatalf("episodeRefsFromTitle(%q)[%d] = %#v, want %#v", tt.name, i, got[i], tt.want[i])
			}
		}
	}
}
