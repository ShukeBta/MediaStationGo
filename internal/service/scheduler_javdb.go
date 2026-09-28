package service

import (
	"context"
	"time"
)

const javDBSessionCheckInterval = 30 * time.Minute

func (s *SchedulerService) jobCheckJavDBSession(ctx context.Context) error {
	if s == nil || s.adult == nil {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, javDBSessionTimeout(s.adult.flareSolverrTimeout))
	defer cancel()
	return s.adult.CheckJavDBSession(checkCtx)
}

func javDBSessionTimeout(flareSolverrTimeoutSeconds int) time.Duration {
	if flareSolverrTimeoutSeconds <= 0 {
		flareSolverrTimeoutSeconds = 60
	}
	return time.Duration(flareSolverrTimeoutSeconds+30) * time.Second
}
