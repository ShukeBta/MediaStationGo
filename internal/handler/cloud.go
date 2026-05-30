// Package handler — cloud-disk (网盘) endpoints: directory browsing, QR-code
// login, media import and 302 playback redirects.
package handler

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

// cloudListHandler browses a configured cloud disk directory.
func cloudListHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		typ := c.Param("type")
		dir := c.Query("dir")
		entries, err := svc.StorageCfg.CloudList(c.Request.Context(), typ, dir)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error(), "items": []any{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": entries})
	}
}

// cloudImportHandler turns a cloud file into a playable 302-backed media item.
func cloudImportHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		typ := c.Param("type")
		var in struct {
			Ref  string `json:"ref" binding:"required"`
			Name string `json:"name"`
			Size int64  `json:"size"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		m, err := svc.StorageCfg.CloudImport(c.Request.Context(), typ, in.Ref, in.Name, in.Size)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, m)
	}
}

// cloud115QRStartHandler begins a 115 QR-code login and returns the session +
// QR image URL for the frontend to render.
func cloud115QRStartHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, err := cloud.QRStart(c.Request.Context(), nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, sess)
	}
}

// cloud115QRPollHandler polls a 115 QR session; on confirmation it returns the
// session cookie so the frontend can save it as the storage credential.
func cloud115QRPollHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var sess cloud.QRSession
		if err := c.ShouldBindJSON(&sess); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		st, err := cloud.QRPoll(c.Request.Context(), nil, &sess)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, st)
	}
}

// cloudPlayHandler resolves a cloud file to its direct link and either issues a
// 302 redirect (true offload — host does not stream the bytes) or, when the
// provider requires authenticated headers, reverse-proxies the response.
func cloudPlayHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		typ := c.Param("type")
		ref := c.Query("ref")
		if ref == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ref required"})
			return
		}
		link, err := svc.StorageCfg.CloudResolve(c.Request.Context(), typ, ref)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		if !link.Proxy {
			// Pure offload: send the client straight to the cloud CDN.
			c.Redirect(http.StatusFound, link.URL)
			return
		}
		// Proxy mode: the direct link needs auth headers the browser cannot
		// carry. Stream through with Range forwarding.
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, link.URL, nil)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		for k, v := range link.Headers {
			req.Header.Set(k, v)
		}
		if rng := c.GetHeader("Range"); rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		defer resp.Body.Close()
		for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
			if v := resp.Header.Get(h); v != "" {
				c.Header(h, v)
			}
		}
		c.Status(resp.StatusCode)
		_, _ = io.Copy(c.Writer, resp.Body)
	}
}
