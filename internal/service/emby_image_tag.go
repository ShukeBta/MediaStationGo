package service

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"
)

func embyImageTag(itemID, kind, raw string, updatedAt time.Time) string {
	itemID = strings.TrimSpace(itemID)
	raw = strings.TrimSpace(raw)
	if itemID == "" || raw == "" {
		return ""
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	stamp := ""
	if !updatedAt.IsZero() {
		stamp = updatedAt.UTC().Format(time.RFC3339Nano)
	}
	sum := sha1.Sum([]byte(kind + "\x00" + raw + "\x00" + stamp))
	encoded := hex.EncodeToString(sum[:])
	return itemID + "-" + encoded[:12]
}

func embyItemETag(itemID string, updatedAt time.Time, parts ...string) string {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return ""
	}
	stamp := ""
	if !updatedAt.IsZero() {
		stamp = updatedAt.UTC().Format(time.RFC3339Nano)
	}
	h := sha1.New()
	_, _ = h.Write([]byte(itemID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(stamp))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}
