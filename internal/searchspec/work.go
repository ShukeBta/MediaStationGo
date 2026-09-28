package searchspec

import "fmt"

// WorkTitleSQL returns the canonical work title projection. Episodic rows
// deliberately require the persisted series name: a release/episode title is
// never promoted to a searchable work title.
func WorkTitleSQL(alias string) string {
	prefix := columnPrefix(alias)
	episodic := episodicSQL(alias)
	return fmt.Sprintf(`CASE
  WHEN %s THEN COALESCE(NULLIF(TRIM(%semby_series_name), ''), '')
  ELSE COALESCE(NULLIF(TRIM(%semby_series_name), ''), NULLIF(TRIM(%stitle), ''), '')
END`, episodic, prefix, prefix, prefix)
}

// WorkDocumentSQL contains only work-level metadata. Episode title, path,
// relative path and overview are intentionally excluded so a single episode
// cannot make its parent work appear in search results.
func WorkDocumentSQL(alias string) string {
	prefix := columnPrefix(alias)
	episodic := episodicSQL(alias)
	title := WorkTitleSQL(alias)
	return fmt.Sprintf(`LOWER(
  (%s) || ' ' ||
  (CASE
    WHEN NOT (%s) OR LOWER(TRIM(COALESCE(%sscrape_status, ''))) = 'matched' THEN
      COALESCE(%stitle, '') || ' ' ||
      COALESCE(%soriginal_name, '') || ' ' ||
      COALESCE(%sgenres, '') || ' ' ||
      COALESCE(%sactors, '') || ' ' ||
      COALESCE(%ssearch_pinyin, '') || ' ' ||
      COALESCE(%ssearch_initials, '')
    ELSE ''
  END)
)`, title, episodic, prefix, prefix, prefix, prefix, prefix, prefix, prefix)
}

func episodicSQL(alias string) string {
	prefix := columnPrefix(alias)
	return fmt.Sprintf(`(COALESCE(%sseason_num, 0) > 0 OR COALESCE(%sepisode_num, 0) > 0 OR COALESCE(%sseries_id, '') <> '')`, prefix, prefix, prefix)
}

func columnPrefix(alias string) string {
	if alias == "" {
		return ""
	}
	return alias + "."
}
