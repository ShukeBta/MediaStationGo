package repository

import (
	"reflect"
	"strings"
	"testing"
)

func TestPostgresSeriesRepresentativeIDsQueryUsesOneIndexedProbePerGroup(t *testing.T) {
	query, args := postgresSeriesRepresentativeIDsQuery(
		[]SeriesCardGroupKey{
			{LibraryID: "library-a", SeriesKey: "series-a"},
			{LibraryID: "library-b", SeriesKey: "series-b"},
		},
		MediaQueryFilter{
			IncludeNSFW:       false,
			HiddenLibraryIDs:  []string{"hidden"},
			AllowedLibraryIDs: []string{"library-a", "library-b"},
		},
	)

	for _, required := range []string{
		"FROM (VALUES (?, ?), (?, ?)) AS selected_groups(library_id, series_key)",
		"CROSS JOIN LATERAL",
		"SELECT representative.id",
		"representative.library_id = selected_groups.library_id",
		"representative.series_key = selected_groups.series_key",
		"representative.nsfw = ?",
		"representative.library_id NOT IN (?)",
		"representative.library_id IN (?, ?)",
		"ORDER BY " + persistedSeriesRepresentativeOrder,
		"LIMIT 1",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("query missing %q:\n%s", required, query)
		}
	}
	if strings.Contains(query, "DISTINCT ON") {
		t.Fatalf("query still uses full-scope DISTINCT ON:\n%s", query)
	}
	if strings.Contains(query, "representative.*") {
		t.Fatalf("indexed probe still loads wide representative rows:\n%s", query)
	}
	wantArgs := []any{"library-a", "series-a", "library-b", "series-b", mediaSeriesKeyVersion, false, "hidden", "library-a", "library-b"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}
