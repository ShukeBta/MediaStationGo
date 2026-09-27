package service

import (
	"context"
	"time"
)

const (
	fd2PPVSessionCheckInterval = 30 * time.Minute
	fd2PPVSessionCheckOverhead = 60 * time.Second
)

func (s *SchedulerService) jobCheckFD2PPVSession(ctx context.Context) error {
	if s == nil || s.adult == nil || !s.adult.FD2PPVEnabled() {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, fd2PPVSessionTimeout(s.adult.flareSolverrTimeout))
	defer cancel()
	return s.adult.CheckFD2PPVSession(checkCtx)
}

func fd2PPVSessionTimeout(flareSolverrTimeoutSeconds int) time.Duration {
	if flareSolverrTimeoutSeconds <= 0 {
		flareSolverrTimeoutSeconds = 60
	}
	flareAttemptTimeout := time.Duration(flareSolverrTimeoutSeconds+10) * time.Second
	return time.Duration(fd2PPVLoginPageAttempts)*flareAttemptTimeout + fd2PPVSessionCheckOverhead
}
