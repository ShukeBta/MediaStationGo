package service

import (
	"context"
	"sync"
	"sync/atomic"
)

type localScanContextKey struct{}
type localScanReservation struct {
	scanner *ScannerService
	active  atomic.Bool
}

// TryReserveLocalScan admits manual work before creating its task or goroutine.
// The returned context carries ownership across a batch of library/root scans.
func (s *ScannerService) TryReserveLocalScan(ctx context.Context) (context.Context, func(), bool) {
	reserved, release, err := s.reserveLocalScan(ctx, false)
	return reserved, release, err == nil
}

// WaitReserveLocalScan queues automatic work without dropping newly added roots.
func (s *ScannerService) WaitReserveLocalScan(ctx context.Context) (context.Context, func(), error) {
	return s.reserveLocalScan(ctx, true)
}

func (s *ScannerService) reserveLocalScan(ctx context.Context, wait bool) (context.Context, func(), error) {
	if err := ctx.Err(); err != nil {
		return ctx, nil, err
	}
	if owner, _ := ctx.Value(localScanContextKey{}).(*localScanReservation); owner != nil && owner.scanner == s && owner.active.Load() {
		return ctx, func() {}, nil
	}
	s.localScanOnce.Do(func() { s.localScanSlot = make(chan struct{}, 1) })
	if wait {
		select {
		case s.localScanSlot <- struct{}{}:
		case <-ctx.Done():
			return ctx, nil, ctx.Err()
		}
	} else {
		select {
		case s.localScanSlot <- struct{}{}:
		default:
			return ctx, nil, ErrLocalScanAlreadyRunning
		}
	}
	owner := &localScanReservation{scanner: s}
	owner.active.Store(true)
	var once sync.Once
	release := func() { once.Do(func() { owner.active.Store(false); <-s.localScanSlot }) }
	if err := ctx.Err(); err != nil {
		release()
		return ctx, nil, err
	}
	return context.WithValue(ctx, localScanContextKey{}, owner), release, nil
}
