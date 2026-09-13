package service

import (
	"context"

	"github.com/ShukeBta/MediaStationGo/internal/database"
)

// Stage the snapshot for the next startup rather than replacing a database
// still held open by SQLite connection pools or a WAL writer.
func (b *BackupService) restoreSQLite(ctx context.Context, src string) error {
	return database.StageSQLiteRestore(ctx, b.cfg, src)
}
