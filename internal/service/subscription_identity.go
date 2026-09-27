package service

import (
	"context"
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
