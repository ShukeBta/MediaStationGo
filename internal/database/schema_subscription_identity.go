package database

import (
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const duplicateSubscriptionMigrationReason = "迁移合并重复订阅规则"

func ensureSubscriptionIdentityUniqueness(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&model.Subscription{}) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var rows []model.Subscription
		if err := tx.Unscoped().Order("created_at asc, id asc").Find(&rows).Error; err != nil {
			return err
		}
		seen := make(map[string]string)
		for i := range rows {
			row := &rows[i]
			storedKey := row.IdentityKey
			key := model.RefreshSubscriptionIdentity(row)
			updates := map[string]any{"identity_key": key}
			if !row.DeletedAt.Valid && row.ArchivedAt == nil {
				activeKey := row.UserID + "\x00" + key
				if _, duplicate := seen[activeKey]; duplicate {
					archivedAt := row.UpdatedAt
					if archivedAt.IsZero() {
						archivedAt = row.CreatedAt
					}
					if archivedAt.IsZero() {
						archivedAt = time.Now()
					}
					updates["enabled"] = false
					updates["archived_at"] = &archivedAt
					updates["archive_reason"] = duplicateSubscriptionMigrationReason
				} else {
					seen[activeKey] = row.ID
				}
			}
			if len(updates) == 1 {
				// 仅回填 identity:未变化则跳过,变化时不刷新 updated_at,
				// 避免每次启动重写全部订阅行。
				if storedKey == key {
					continue
				}
				if err := tx.Unscoped().Model(&model.Subscription{}).Where("id = ?", row.ID).UpdateColumns(updates).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Unscoped().Model(&model.Subscription{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_user_identity_active
ON subscriptions(user_id, identity_key)
WHERE deleted_at IS NULL AND archived_at IS NULL AND identity_key <> ''
`).Error
	})
}
