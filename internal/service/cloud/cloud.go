// Package cloud implements pluggable cloud-disk (网盘) providers used by the
// external-storage subsystem to expose remote files as playable media via
// HTTP 302 redirects.
//
// The design offloads playback to the cloud provider: instead of the
// host downloading and re-streaming bytes, a provider resolves a file to a
// short-lived direct download URL and the player is 302-redirected straight to
// the cloud CDN. The host only performs a tiny redirect, freeing its CPU and
// bandwidth.
//
// Each provider authenticates with a cookie (obtained via the web UI, an API
// cookie, or a QR-code login flow). Providers are intentionally side-effect
// free and take an *http.Client so they can be exercised against httptest
// mock servers in unit tests.
package cloud

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// timeNow is a seam so tests can pin timestamps.
var timeNow = time.Now

// Provider types recognised by the registry.
const (
	Type115         = "cloud115"    // 115 网盘
	TypeCloudDrive2 = "clouddrive2" // CloudDrive2 桥接网盘
	TypeOpenList    = "openlist"    // OpenList / AList-compatible bridge
)

// ErrUnsupported is returned for an unknown provider type.
var ErrUnsupported = errors.New("unsupported cloud provider")

// FileEntry is one item in a cloud directory listing.
type FileEntry struct {
	ID    string `json:"id"` // provider-native file id
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
	// PickCode is 115-specific; other providers use ID directly.
	PickCode string `json:"pick_code,omitempty"`
}

// DirectLink is a resolved playback target.
type DirectLink struct {
	URL string `json:"url"`
	// Headers that must accompany a request to URL (e.g. User-Agent, Cookie).
	Headers map[string]string `json:"-"`
	// Proxy reports whether URL requires the host to reverse-proxy the bytes
	// (because the headers cannot be carried by a plain browser 302). When
	// false the play handler issues a pure 302 redirect (true offload).
	Proxy bool `json:"-"`
}

// Provider is the common cloud-disk interface.
type Provider interface {
	// Type returns the provider key.
	Type() string
	// Ping validates the stored credentials (cookie). Cheap, used by the
	// storage-config Test() probe.
	Ping(ctx context.Context) error
	// List returns the entries under dirID. An empty dirID means the root.
	List(ctx context.Context, dirID string) ([]FileEntry, error)
	// Resolve turns a provider-native file reference (id or pickcode) into a
	// short-lived direct download link suitable for 302 playback.
	Resolve(ctx context.Context, fileRef string) (*DirectLink, error)
}

// MutableProvider is implemented by cloud bridges that support safe folder
// management through their official API or standard WebDAV methods.
type MutableProvider interface {
	Provider
	Mkdir(ctx context.Context, parentDir, name string) (*FileEntry, error)
	Rename(ctx context.Context, ref, name string) (*FileEntry, error)
}

// MovableProvider is implemented by writable cloud bridges that can move an
// entry across directories, optionally renaming it in the same operation.
type MovableProvider interface {
	MutableProvider
	Move(ctx context.Context, ref, targetDir, name string) (*FileEntry, error)
}

// New constructs a provider of the given type from a free-form config map
// (as persisted by StorageConfigService). The client is shared so callers can
// inject timeouts / test transports.
func New(typ string, cfg map[string]any, client *http.Client) (Provider, error) {
	if client == nil {
		client = http.DefaultClient
	}
	switch typ {
	case Type115:
		return new115(cfg, client), nil
	case TypeCloudDrive2:
		return newCloudDrive2(cfg, client), nil
	case TypeOpenList:
		return newOpenList(cfg, client), nil
	default:
		return nil, ErrUnsupported
	}
}

// IsCloudType reports whether typ is a cloud-disk provider.
func IsCloudType(typ string) bool {
	return typ == Type115 || typ == TypeCloudDrive2 || typ == TypeOpenList
}

// str coerces a config value to a trimmed string.
func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// boolish coerces a config value to bool ("true"/"1"/true → true).
func boolish(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	default:
		return false
	}
}

// defaultUA is a desktop browser UA accepted by upstream cloud providers.
const defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
