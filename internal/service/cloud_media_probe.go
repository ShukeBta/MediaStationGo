package service

import (
	"context"
	"errors"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type cloudPlaybackResolver interface {
	CloudResolve(ctx context.Context, typ, fileRef, clientUA string) (*cloud.DirectLink, error)
}

type cloudPlaybackProber interface {
	ProbeHTTP(ctx context.Context, rawURL string, headers map[string]string) (*ProbeResult, error)
}

// probeCloudFileMetadataWith is the explicit/admin cloud probe path. It must
// resolve with the same internal User-Agent that ffprobe uses to read the
// resulting URL because 115 links can be User-Agent-bound.
func probeCloudFileMetadataWith(ctx context.Context, resolver cloudPlaybackResolver, prober cloudPlaybackProber, typ, ref string) (*ProbeResult, error) {
	link, err := resolver.CloudResolve(ctx, typ, ref, cloudMediaInternalUserAgent)
	if err != nil {
		return nil, err
	}
	if link == nil || strings.TrimSpace(link.URL) == "" {
		return nil, errors.New("cloud media resolved to an empty URL")
	}
	return prober.ProbeHTTP(ctx, link.URL, cloudMediaInternalHeaders(link.Headers))
}
