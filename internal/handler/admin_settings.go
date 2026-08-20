package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type settingReq struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}

func listSettingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := svc.Repo.Setting.All(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, settings)
	}
}

func updateSettingHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req settingReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		oldValue := ""
		if req.Key == service.AdultLibraryIDsSettingKey {
			oldValue, _ = svc.Repo.Setting.Get(c.Request.Context(), req.Key)
		}
		if err := svc.Repo.Setting.Set(c.Request.Context(), req.Key, req.Value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		oldAdultLibraryIDs := service.DecodeAllowedLibraryIDs(oldValue)
		newAdultLibraryIDs := service.DecodeAllowedLibraryIDs(req.Value)
		if req.Key == service.AdultLibraryIDsSettingKey && len(oldAdultLibraryIDs) == 0 && len(newAdultLibraryIDs) > 0 {
			_ = svc.Repo.DB.WithContext(c.Request.Context()).Model(&model.User{}).Where("hide_adult = ?", false).Update("hide_adult", true).Error
		}
		service.ApplyRuntimeSetting(svc.Cfg, req.Key, req.Value)
		if svc.FFprobe != nil && (req.Key == "ffprobe.max_concurrent" || req.Key == "app.ffprobe_max_concurrent") {
			svc.FFprobe.SetMaxConcurrent(svc.Cfg.App.FFprobeMaxConcurrent)
		}
		if req.Key == "transcode.enabled" && !svc.Cfg.Transcoder.Enabled {
			svc.Transcoder.StopAll()
		}
		if req.Key == "transcode.hw_enabled" || req.Key == "transcode.hw_accel" || req.Key == "transcoder.hardware_accel" || req.Key == "transcoder.encoder" {
			svc.Transcoder.StopAll()
		}
		if req.Key == "cloud.auto_sync_enabled" && !service.ParseBoolSetting(req.Value, false) && svc.Scan != nil {
			_ = svc.Scan.CancelAllCloudScans()
		}
		c.Status(http.StatusNoContent)
	}
}

type testAdultScraperReq struct {
	Engine    string `json:"engine"`
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
	JavDBURL  string `json:"javdb_url"`
	JavBusURL string `json:"javbus_url"`
	Cookie    string `json:"cookie"`
}

func testAdultScraperHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req testAdultScraperReq
		_ = c.ShouldBindJSON(&req)

		engine := strings.ToLower(strings.TrimSpace(req.Engine))
		if engine == "" {
			engine = "metatube"
		}

		if engine == "metatube" {
			serverURL := strings.TrimSpace(req.ServerURL)
			if serverURL == "" {
				serverURL, _ = svc.Repo.Setting.Get(c.Request.Context(), "adult.scraper.metatube_server")
			}
			if serverURL == "" {
				serverURL = "http://127.0.0.1:7700"
			}
			token := strings.TrimSpace(req.Token)
			if token == "" {
				token, _ = svc.Repo.Setting.Get(c.Request.Context(), "adult.scraper.metatube_token")
			}

			client := service.NewMetaTubeProvider(svc.Log)
			res, err := client.TestConnection(c.Request.Context(), serverURL, token)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success":    false,
					"latency_ms": res.LatencyMs,
					"error":      err.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"success":    res.Success,
				"latency_ms": res.LatencyMs,
				"providers":  res.Providers,
				"error":      res.Error,
			})
			return
		}

		// Builtin scraper test
		bases := []string{}
		if req.JavDBURL != "" {
			bases = append(bases, strings.TrimSpace(req.JavDBURL))
		}
		if req.JavBusURL != "" {
			bases = append(bases, strings.Split(req.JavBusURL, ",")...)
		}
		if len(bases) == 0 {
			if s, _ := svc.Repo.Setting.Get(c.Request.Context(), "adult.scraper.builtin_javdb_url"); s != "" {
				bases = append(bases, s)
			}
			if s, _ := svc.Repo.Setting.Get(c.Request.Context(), "adult.scraper.builtin_javbus_url"); s != "" {
				bases = append(bases, strings.Split(s, ",")...)
			}
		}
		if len(bases) == 0 {
			bases = []string{"https://javdb.com", "https://javbus.sbs", "https://www.javbus.com"}
		}

		cookie := strings.TrimSpace(req.Cookie)
		if cookie == "" {
			cookie, _ = svc.Repo.Setting.Get(c.Request.Context(), "adult.scraper.builtin_cookie")
		}
		if !strings.Contains(cookie, "age=") {
			cookie = cookie + "; age=verified"
		}

		httpClient := service.NewExternalHTTPClient(8 * time.Second)
		start := time.Now()
		var lastErr error
		success := false
		for _, b := range bases {
			b = strings.TrimRight(strings.TrimSpace(b), "/")
			if b == "" {
				continue
			}
			if !strings.HasPrefix(b, "http://") && !strings.HasPrefix(b, "https://") {
				b = "https://" + b
			}
			httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, b, nil)
			if err != nil {
				lastErr = err
				continue
			}
			httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			httpReq.Header.Set("Cookie", cookie)
			resp, err := httpClient.Do(httpReq)
			if err != nil {
				lastErr = err
				continue
			}
			bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
			_ = resp.Body.Close()
			if resp.StatusCode >= 400 {
				lastErr = fmt.Errorf("%s returned HTTP %d", b, resp.StatusCode)
				continue
			}
			text := string(bodyBytes)
			if strings.Contains(text, "driver-verify") || strings.Contains(text, "Age Verification") {
				lastErr = fmt.Errorf("%s intercepted by age verification", b)
				continue
			}
			success = true
			break
		}

		latency := time.Since(start).Milliseconds()
		if success {
			c.JSON(http.StatusOK, gin.H{
				"success":    true,
				"latency_ms": latency,
				"message":    "内置刮削源连接正常",
			})
			return
		}
		errMsg := "内置源连接失败"
		if lastErr != nil {
			errMsg = lastErr.Error()
		}
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"latency_ms": latency,
			"error":      errMsg,
		})
	}
}

func recentLogsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := svc.Repo.Log.Recent(c.Request.Context(), 200)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, rows)
	}
}
