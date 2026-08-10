package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

// Update applies API patch fields while recomputing the functional identity.
// The database partial unique index remains the final concurrency guard.
func (s *SubscriptionService) Update(ctx context.Context, id string, updates map[string]any) error {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return errors.New("subscription service unavailable")
	}
	var sub model.Subscription
	if err := s.repo.DB.WithContext(ctx).Where("id = ?", id).First(&sub).Error; err != nil {
		return err
	}
	raw, err := json.Marshal(updates)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &sub); err != nil {
		return err
	}
	if sub.Name == "" || sub.FeedURL == "" {
		return errors.New("name and feed_url required")
	}
	normalizeSubscriptionDefaults(&sub)
	model.RefreshSubscriptionIdentity(&sub)
	if duplicate, err := s.subscriptionDuplicate(ctx, &sub, sub.ID); err != nil {
		return err
	} else if duplicate != nil {
		return newSubscriptionAlreadyExistsError(duplicate.ID)
	}

	updates["search_mode"] = sub.SearchMode
	updates["resolution"] = sub.Resolution
	updates["wash_priority"] = sub.WashPriority
	updates["priority"] = sub.Priority
	updates["identity_key"] = sub.IdentityKey
	if err := s.repo.DB.WithContext(ctx).Model(&model.Subscription{}).
		Where("id = ?", sub.ID).Updates(updates).Error; err != nil {
		if duplicate, lookupErr := s.subscriptionDuplicate(ctx, &sub, sub.ID); lookupErr == nil && duplicate != nil {
			return newSubscriptionAlreadyExistsError(duplicate.ID)
		}
		return err
	}
	return nil
}
