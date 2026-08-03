package service

import "testing"

func TestOrganizeMetadataRejectsEpisodeOnlyQuery(t *testing.T) {
	match := &Match{Title: "错误节目", Year: 2026, TMDbID: 284725}
	for _, query := range []string{"第 11 集", "第1期上：最狠开局！五哈团命悬一线好刺激"} {
		if organizeMetadataMatchTrusted(query, 2026, match) {
			t.Fatalf("episode-only query %q must not be trusted for automatic match", query)
		}
	}
}

func TestOrganizeMetadataTrustsExactOriginalTitle(t *testing.T) {
	match := &Match{Title: "镖人", OriginalName: "Blades of the Guardians", Year: 2023, TMDbID: 107463}
	if !organizeMetadataMatchTrusted("blades of the guardians", 2023, match) {
		t.Fatal("exact original title should be trusted")
	}
}

func TestOrganizeMetadataTrustsCleanedBroadcastReleaseQuery(t *testing.T) {
	match := &Match{Title: "湖南卫视春节联欢晚会", OriginalName: "HNTV Spring Festival Gala", Year: 2026, TMDbID: 123456}
	query := "hntv spring festival gala fps hlg qhstudio s01e qhstudio"
	if !organizeMetadataMatchTrusted(query, 2026, match) {
		t.Fatal("release noise should not block an otherwise matching title")
	}
}

func TestOrganizeMetadataRejectsChineseReleaseAliasWithoutOriginalName(t *testing.T) {
	match := &Match{
		Title:     "莫离",
		Languages: []string{"zh"},
		Countries: []string{"CN"},
		Year:      2026,
		TMDbID:    292696,
	}
	if organizeMetadataMatchTrusted("the first jasmine", 2026, match) {
		t.Fatal("multi-word English release alias must not be trusted without title or original-name evidence")
	}
}

func TestOrganizeMetadataRejectsLooseChineseOriginEnglishAlias(t *testing.T) {
	match := &Match{
		Title:        "镖人",
		OriginalName: "Biao Ren",
		Languages:    []string{"zh"},
		Countries:    []string{"CN"},
		Year:         2023,
		TMDbID:       107463,
	}
	if organizeMetadataMatchTrusted("blades of the guardians", 2023, match) {
		t.Fatal("unrelated English query must not trust Chinese-origin metadata just because it has multiple Latin tokens")
	}
}

func TestOrganizeMetadataRejectsUnverifiedLocalizedSearchKeyword(t *testing.T) {
	match := &Match{
		Title:         "Monarch: Legacy of Monsters",
		OriginalName:  "Monarch: Legacy of Monsters",
		Year:          2023,
		TMDbID:        202411,
		SearchKeyword: "帝王计划：怪兽遗产",
	}
	if organizeMetadataMatchTrusted("帝王计划：怪兽遗产", 2023, match) {
		t.Fatal("echoing the search keyword must not make an unrelated first result trustworthy")
	}
}

func TestOrganizeMetadataTrustsProviderReturnedAlias(t *testing.T) {
	match := &Match{
		Title:        "镖人",
		OriginalName: "镖人",
		Aliases:      []string{"Blades of the Guardians"},
		Year:         2023,
		TMDbID:       107463,
	}
	if !organizeMetadataMatchTrusted("blades of the guardians", 2023, match) {
		t.Fatal("a title returned by another TMDb language response should be trusted as an alias")
	}
}

func TestOrganizeMetadataRejectsShortLocalizedSearchKeyword(t *testing.T) {
	match := &Match{
		Title:         "Hello, Saturday",
		OriginalName:  "Hello, Saturday",
		TMDbID:        123456,
		SearchKeyword: "你好",
	}
	if organizeMetadataMatchTrusted("你好", 0, match) {
		t.Fatal("short generic localized search keyword must not be trusted")
	}
}

func TestOrganizeMetadataRejectsSingleTokenChineseAlias(t *testing.T) {
	match := &Match{
		Title:     "莫离",
		Languages: []string{"zh"},
		Countries: []string{"CN"},
		Year:      2026,
		TMDbID:    292696,
	}
	if organizeMetadataMatchTrusted("jasmine", 2026, match) {
		t.Fatal("single token alias should stay too weak for automatic matching")
	}
}

func TestOrganizeMetadataRejectsLooseSingleTokenQuery(t *testing.T) {
	match := &Match{Title: "步步惊心泰版", OriginalName: "Scarlet Heart Thailand", Year: 2025, TMDbID: 252886}
	if organizeMetadataMatchTrusted("thailand", 0, match) {
		t.Fatal("single generic token must not trust a broader title")
	}
}

func TestOrganizeMetadataRejectsDirtyUnrelatedQuery(t *testing.T) {
	match := &Match{Title: "春节联欢晚会", OriginalName: "Spring Festival Gala", Year: 2026, TMDbID: 284725}
	query := "hntv spring festival gala fps hlg qhstudio s01e qhstudio"
	if organizeMetadataMatchTrusted(query, 2026, match) {
		t.Fatal("dirty query with a missing broadcaster token must not trust a partial title")
	}
}
