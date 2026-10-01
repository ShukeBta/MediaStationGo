package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func embySystemInfoHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, embyWithRequestAddress(c, svc.Emby.SystemInfo()))
	}
}

func embySystemInfoPublicHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, embyPublicSystemInfoPayload(c, svc))
	}
}

func embyRequestBaseURL(c *gin.Context) string {
	proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if proto == "" {
		if c.Request != nil && c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	if comma := strings.Index(proto, ","); comma >= 0 {
		proto = strings.TrimSpace(proto[:comma])
	}

	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if comma := strings.Index(host, ","); comma >= 0 {
		host = strings.TrimSpace(host[:comma])
	}
	if host == "" && c.Request != nil {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return ""
	}
	return strings.TrimRight(proto+"://"+host, "/")
}

func embyWithRequestAddress(c *gin.Context, payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload)+2)
	for key, value := range payload {
		out[key] = value
	}
	if address := embyRequestBaseURL(c); address != "" {
		out["LocalAddress"] = address
		out["WanAddress"] = address
		out["PublishedServerUrl"] = address
		embyApplyRequestPorts(out, address)
	}
	return out
}

// Clients must reconnect to the published NAS/reverse-proxy port, rather than
// the container's private listener. Only advertise HTTPS when this request
// arrived through HTTPS (including the existing forwarded-protocol handling).
func embyApplyRequestPorts(payload map[string]any, address string) {
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return
	}
	port := 80
	if parsed.Scheme == "https" {
		port = 443
	}
	if explicitPort := parsed.Port(); explicitPort != "" {
		port, err = strconv.Atoi(explicitPort)
		if err != nil || port < 1 || port > 65535 {
			return
		}
	}
	payload["HttpServerPortNumber"], payload["HttpsPortNumber"] = 0, 0
	payload["SupportsHttps"] = parsed.Scheme == "https"
	if parsed.Scheme == "https" {
		payload["HttpsPortNumber"] = port
	} else {
		payload["HttpServerPortNumber"] = port
	}
	if _, ok := payload["WebSocketPortNumber"]; ok {
		payload["WebSocketPortNumber"] = port
	}
}

func embySystemEndpointHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"IsLocal":     true,
			"IsInNetwork": true,
		})
	}
}

func embyPingHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Emby/Jellyfin 期望 plain text "Emby Server"
		c.String(http.StatusOK, "Emby Server")
	}
}

func embyRootHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, embyPublicSystemInfoPayload(c, svc))
	}
}

func embyPublicSystemInfoPayload(c *gin.Context, svc *service.Container) map[string]any {
	identity := map[string]any{
		"Id":                     "mediastation-go-001",
		"ServerId":               "mediastation-go-001",
		"ServerName":             "MediaStationGo",
		"Version":                "4.8.10.0",
		"ServerVersion":          "4.8.10.0",
		"ProductName":            "Emby Server",
		"SupportsHttps":          false,
		"SupportsAutoDiscovery":  true,
		"StartupWizardCompleted": true,
	}
	if svc != nil && svc.Emby != nil {
		info := svc.Emby.SystemInfoPublic()
		for key := range identity {
			if value, ok := info[key]; ok {
				identity[key] = value
			}
		}
	}
	return embyWithRequestAddress(c, identity)
}
