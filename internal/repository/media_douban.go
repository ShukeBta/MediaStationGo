package repository

import (
	"gorm.io/gorm"
	"strings"
)

// Clear provider-only values atomically when a manual/scrape/batch operation
// changes an identifier, while preserving them for same-ID metadata edits.
func prepareDoubanBindingUpdate(updates map[string]any) {
	id, ok := updates["douban_id"].(string)
	if !ok {
		return
	}
	id = strings.TrimSpace(id)
	for column, empty := range map[string]any{"douban_rating": 0, "douban_fetched_at": nil, "douban_degraded": false} {
		if _, supplied := updates[column]; !supplied {
			updates[column] = gorm.Expr("CASE WHEN COALESCE(douban_id, '') = ? THEN "+column+" ELSE ? END", id, empty)
		}
	}
}
