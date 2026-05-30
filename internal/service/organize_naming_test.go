package service

import "testing"

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
