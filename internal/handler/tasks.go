// Package handler — live tasks board.
//
// /api/tasks aggregates running ffmpeg transcodes, qBittorrent torrents
// and recent scrape progress into a single snapshot suitable for the
// React Tasks panel. The panel can layer this REST snapshot on top of
// live WS events for instant updates.
package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const tasksLiveTorrentSnapshotMaxAge = 30 * time.Second

func tasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var transcodes []service.ActiveJob
		if svc.Transcoder != nil {
			transcodes = svc.Transcoder.Active()
		}
		var torrents []service.QBitTorrent
		if svc.Downloads != nil {
			torrents = svc.Downloads.LiveTorrentSnapshot(tasksLiveTorrentSnapshotMaxAge)
		}
		background := service.TaskSnapshot{}
		if svc.Tasks != nil {
			background = svc.Tasks.Snapshot()
		}
		c.JSON(http.StatusOK, gin.H{
			"transcodes":       transcodes,
			"torrents":         torrents,
			"background_tasks": background,
		})
	}
}
