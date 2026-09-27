package handler

import (
	"context"
	"errors"
	"testing"
)

func TestDiscoverSearchSourceErrorMessageDistinguishesTimeout(t *testing.T) {
	if got := discoverSearchSourceErrorMessage("tmdb_movie", context.DeadlineExceeded); got != "TMDb 电影搜索超时" {
		t.Fatalf("timeout message = %q", got)
	}
	if got := discoverSearchSourceErrorMessage("bangumi", errors.New("upstream 503")); got != "Bangumi 动漫搜索暂不可用" {
		t.Fatalf("failure message = %q", got)
	}
}
