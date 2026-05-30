package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestDLNADiscoverCachedEmptyReturnsNonNil guards against the bug where an
// empty device cache was copied via append([]DLNADevice(nil), cache...),
// which yields a nil slice and serializes to JSON null. The web UI reads
// res.devices.length, so a null devices field crashed the DLNA page.
func TestDLNADiscoverCachedEmptyReturnsNonNil(t *testing.T) {
	d := NewDLNAService(zap.NewNop())
	// Prime the cache with an empty (but non-nil) result, fresh enough to be served.
	d.cache = []DLNADevice{}
	d.cachedAt = time.Now()

	out, err := d.Discover(context.Background(), false)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if out == nil {
		t.Fatal("Discover returned nil slice for empty cache; want non-nil so JSON is [] not null")
	}

	b, err := json.Marshal(map[string]any{"devices": out})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(b); got != `{"devices":[]}` {
		t.Fatalf("devices serialized as %s; want {\"devices\":[]}", got)
	}
}
