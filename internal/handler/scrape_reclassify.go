package handler

import (
	"context"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// 刮削元数据时不再自动跨库纠偏或自动创建新媒体库，保持在当前媒体库内仅更新元数据
func reclassifyMediaAfterScrape(_ context.Context, _ *service.Container, _ ...string) int {
	return 0
}

func reclassifyMediaAfterScrapeWithTypeHints(_ context.Context, _ *service.Container, _ map[string]string, _ ...string) int {
	return 0
}

func reclassifyLibraryAfterScrape(_ context.Context, _ *service.Container, _ ...string) int {
	return 0
}
