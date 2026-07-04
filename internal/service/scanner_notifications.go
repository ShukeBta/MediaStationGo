package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) notifyScanFinished(lib *model.Library, res *ScanResult, err error, cloud bool) {
	if s == nil || s.notify == nil || lib == nil || res == nil {
		return
	}
	if err != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			s.notify.Broadcast(ctx, "MediaStationGo 扫描异常", fmt.Sprintf("媒体库：%s\n错误：%s", lib.Name, err.Error()), EventSystemAlert)
		}()
		return
	}
	if res.Added+res.Updated <= 0 {
		return
	}
	source := "本地媒体库"
	if cloud {
		source = "网盘媒体库"
	}
	body := fmt.Sprintf("%s：%s\n新增：%d\n更新：%d\n跳过：%d\n移除：%d", source, lib.Name, res.Added, res.Updated, res.Skipped, res.Removed)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.notify.Broadcast(ctx, "MediaStationGo 入库完成", body, EventLibraryIngest)
	}()
}
