package service

import "testing"

func TestNormalizeExternalIDs(t *testing.T) {
	if got := NormalizeIMDBID("https://www.imdb.com/title/tt1234567/?ref_=x"); got != "tt1234567" {
		t.Fatalf("NormalizeIMDBID url = %q", got)
	}
	if got := NormalizeIMDBID("1234567"); got != "tt1234567" {
		t.Fatalf("NormalizeIMDBID digits = %q", got)
	}
	if got := NormalizeDoubanID("https://movie.douban.com/subject/3622222/"); got != "3622222" {
		t.Fatalf("NormalizeDoubanID url = %q", got)
	}
	if got := NormalizeDoubanID("douban: 3622222"); got != "3622222" {
		t.Fatalf("NormalizeDoubanID text = %q", got)
	}
	if got := NormalizeTMDbID("https://www.themoviedb.org/movie/12345"); got != 12345 {
		t.Fatalf("NormalizeTMDbID url = %d", got)
	}
	if got := NormalizeTMDbID("tmdb_id=67890"); got != 67890 {
		t.Fatalf("NormalizeTMDbID text = %d", got)
	}
}
