package service

import (
	"testing"
	"time"
)

func TestJavDBSessionTimeoutIncludesDirectAndFlareSolverrBudgets(t *testing.T) {
	if got := javDBSessionTimeout(60); got != 90*time.Second {
		t.Fatalf("timeout = %s, want 90s", got)
	}
	if got := javDBSessionTimeout(0); got != 90*time.Second {
		t.Fatalf("default timeout = %s, want 90s", got)
	}
}
