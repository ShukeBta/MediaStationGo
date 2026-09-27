package service

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var ErrSubscriptionAlreadyExists = errors.New("subscription already exists")

type SubscriptionAlreadyExistsError struct {
	ExistingID string
}

func (e *SubscriptionAlreadyExistsError) Error() string {
	if e == nil || e.ExistingID == "" {
		return ErrSubscriptionAlreadyExists.Error()
	}
	return fmt.Sprintf("%s: %s", ErrSubscriptionAlreadyExists, e.ExistingID)
}

func (e *SubscriptionAlreadyExistsError) Unwrap() error {
	return ErrSubscriptionAlreadyExists
}

func newSubscriptionAlreadyExistsError(existingID string) error {
	return &SubscriptionAlreadyExistsError{ExistingID: existingID}
}

func SubscriptionAlreadyExistsID(err error) string {
	var conflict *SubscriptionAlreadyExistsError
	if errors.As(err, &conflict) && conflict != nil {
		return conflict.ExistingID
	}
	return ""
}

func (s *SubscriptionService) subscriptionDuplicate(ctx context.Context, sub *model.Subscription, excludeID string) (*model.Subscription, error) {
	if s == nil || s.repo == nil || s.repo.Subscription == nil || sub == nil {
		return nil, nil
	}
	return s.repo.Subscription.FindActiveByIdentity(ctx, sub.UserID, sub.IdentityKey, excludeID)
}

// activeSubscriptionByIdentityTx 在同一事务连接内查找功能相同的活动订阅,
// 避免事务中另取连接(SQLite 单连接池会死锁,内存库会看不到表)。
func activeSubscriptionByIdentityTx(ctx context.Context, tx *gorm.DB, sub *model.Subscription) (*model.Subscription, error) {
	if tx == nil || sub == nil || sub.IdentityKey == "" {
		return nil, nil
	}
	var dup model.Subscription
	err := tx.WithContext(ctx).
		Where("user_id = ? AND identity_key = ? AND archived_at IS NULL AND id <> ?", sub.UserID, sub.IdentityKey, sub.ID).
		First(&dup).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &dup, nil
}
