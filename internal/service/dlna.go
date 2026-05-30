// Package service — DLNA / UPnP discovery.
//
// DLNAService scans the LAN for "MediaRenderer" UPnP devices via SSDP
// (multicast UDP 239.255.255.250:1900) and exposes a one-shot "cast"
// helper that POSTs a SOAP envelope to the renderer's AVTransport
// service to start playback of an HTTP URL.
//
// We do NOT mediate the renderer ↔ client traffic; the renderer pulls
// the bytes directly from MediaStationGo's /api/stream endpoint, so
// the cast call only ever transports a URL string.
package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DLNAService discovers UPnP MediaRenderer devices and casts media to them.
type DLNAService struct {
	log *zap.Logger

	mu      sync.Mutex
	cache   []DLNADevice
	cachedAt time.Time
}

// NewDLNAService is the constructor.
func NewDLNAService(log *zap.Logger) *DLNAService {
	return &DLNAService{log: log}
}

// DLNADevice is the public projection of a discovered renderer.
type DLNADevice struct {
	UDN          string `json:"udn"`
	FriendlyName string `json:"friendly_name"`
	Manufacturer string `json:"manufacturer"`
	ModelName    string `json:"model_name"`
	Location     string `json:"location"`     // device description URL
	ControlURL   string `json:"control_url"`  // AVTransport SOAP endpoint
	IPAddress    string `json:"ip_address"`
}

// ssdpDiscover sends an M-SEARCH and returns the LOCATION URLs of every
// device that replied within timeout.
func (d *DLNAService) ssdpDiscover(ctx context.Context, timeout time.Duration) ([]string, error) {
	addr, err := net.ResolveUDPAddr("udp4", "239.255.255.250:1900")
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	msg := strings.Join([]string{
		"M-SEARCH * HTTP/1.1",
		"HOST: 239.255.255.250:1900",
		`MAN: "ssdp:discover"`,
		"MX: 2",
		"ST: urn:schemas-upnp-org:device:MediaRenderer:1",
		"", "",
	}, "\r\n")
	if _, err := conn.WriteTo([]byte(msg), addr); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	_ = conn.SetReadDeadline(deadline)

	seen := map[string]struct{}{}
	var locations []string
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return locations, nil
		default:
		}
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break
		}
		body := string(buf[:n])
		for _, line := range strings.Split(body, "\r\n") {
			if strings.HasPrefix(strings.ToUpper(line), "LOCATION:") {
				loc := strings.TrimSpace(line[len("LOCATION:"):])
				if _, ok := seen[loc]; ok {
					continue
				}
				seen[loc] = struct{}{}
				locations = append(locations, loc)
			}
		}
	}
	return locations, nil
}

// Discover returns every reachable MediaRenderer on the LAN. Results are
// cached for 30 seconds so the React UI's polling does not spam the
// network.
func (d *DLNAService) Discover(ctx context.Context, force bool) ([]DLNADevice, error) {
	d.mu.Lock()
	if !force && time.Since(d.cachedAt) < 30*time.Second && d.cache != nil {
		// Copy into a non-nil slice so an empty cache serializes as [] not null.
		out := append(make([]DLNADevice, 0, len(d.cache)), d.cache...)
		d.mu.Unlock()
		return out, nil
	}
	d.mu.Unlock()

	locations, err := d.ssdpDiscover(ctx, 3*time.Second)
	if err != nil {
		// SSDP often fails on container networks; treat as "no devices"
		// rather than 500 the API. Return an empty (non-nil) slice so the
		// JSON response is [] not null.
		d.log.Debug("ssdp discover failed", zap.Error(err))
		return []DLNADevice{}, nil
	}
	devices := make([]DLNADevice, 0, len(locations))
	for _, loc := range locations {
		dev, err := d.fetchDescription(ctx, loc)
		if err != nil {
			d.log.Debug("desc fetch", zap.String("loc", loc), zap.Error(err))
			continue
		}
		devices = append(devices, *dev)
	}

	d.mu.Lock()
	d.cache = devices
	d.cachedAt = time.Now()
	d.mu.Unlock()
	return devices, nil
}

// fetchDescription parses the device's UPnP XML descriptor and pulls out
// the AVTransport control URL.
func (d *DLNAService) fetchDescription(ctx context.Context, location string) (*DLNADevice, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	type service struct {
		ServiceType string `xml:"serviceType"`
		ControlURL  string `xml:"controlURL"`
	}
	type device struct {
		FriendlyName string    `xml:"friendlyName"`
		Manufacturer string    `xml:"manufacturer"`
		ModelName    string    `xml:"modelName"`
		UDN          string    `xml:"UDN"`
		ServiceList  struct {
			Services []service `xml:"service"`
		} `xml:"serviceList"`
	}
	type root struct {
		Device device `xml:"device"`
	}
	var r root
	if err := xml.Unmarshal(body, &r); err != nil {
		return nil, err
	}

	out := &DLNADevice{
		UDN:          r.Device.UDN,
		FriendlyName: r.Device.FriendlyName,
		Manufacturer: r.Device.Manufacturer,
		ModelName:    r.Device.ModelName,
		Location:     location,
	}
	if u, err := url.Parse(location); err == nil {
		out.IPAddress = u.Hostname()
	}
	for _, svc := range r.Device.ServiceList.Services {
		if strings.Contains(svc.ServiceType, "AVTransport") {
			out.ControlURL = absoluteURL(location, svc.ControlURL)
			break
		}
	}
	return out, nil
}

func absoluteURL(base, ref string) string {
	bu, err := url.Parse(base)
	if err != nil {
		return ref
	}
	ru, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return bu.ResolveReference(ru).String()
}

// soapTemplate is the AVTransport SetAVTransportURI envelope.
const soapTemplate = `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
      <CurrentURI>%s</CurrentURI>
      <CurrentURIMetaData></CurrentURIMetaData>
    </u:SetAVTransportURI>
  </s:Body>
</s:Envelope>`

const playTemplate = `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:Play xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
      <Speed>1</Speed>
    </u:Play>
  </s:Body>
</s:Envelope>`

// Cast tells the device at controlURL to start playing mediaURL. Returns
// the renderer's HTTP status for diagnostic purposes.
func (d *DLNAService) Cast(ctx context.Context, controlURL, mediaURL string) error {
	if controlURL == "" {
		return errors.New("device has no AVTransport control URL")
	}
	if err := d.soap(ctx, controlURL, "SetAVTransportURI",
		fmt.Sprintf(soapTemplate, escapeXML(mediaURL))); err != nil {
		return err
	}
	return d.soap(ctx, controlURL, "Play", playTemplate)
}

// soap POSTs an envelope and returns the parsed faultstring (if any).
// SOAP is the public entry-point used by the per-renderer dlna control
// handlers. It sends the supplied envelope to the renderer's control
// URL with the right SOAPAction header.
func (d *DLNAService) SOAP(ctx context.Context, controlURL, action, envelope string) error {
	return d.soap(ctx, controlURL, action, envelope)
}

func (d *DLNAService) soap(ctx context.Context, controlURL, action, envelope string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL,
		bytes.NewReader([]byte(envelope)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction",
		fmt.Sprintf(`"urn:schemas-upnp-org:service:AVTransport:1#%s"`, action))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dlna %s: %d: %s", action, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;",
		`"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}
