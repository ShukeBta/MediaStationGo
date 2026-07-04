package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSystemUpdateCommand(t *testing.T) {
	status := SystemUpdateStatus{
		Image:           "ghcr.io/shukebta/mediastation-go:latest",
		WatchtowerImage: "containrrr/watchtower:latest",
		ContainerID:     "abc123def456",
		ContainerName:   "mediastation-go",
	}

	got := renderSystemUpdateCommand(
		"docker run {{watchtower_image}} --run-once {{container}} --image {{image}} --id {{container_id}}",
		status,
	)
	for _, marker := range []string{"{{watchtower_image}}", "{{container}}", "{{image}}", "{{container_id}}"} {
		if strings.Contains(got, marker) {
			t.Fatalf("command still contains marker %s: %q", marker, got)
		}
	}
	for _, want := range []string{
		"containrrr/watchtower:latest",
		"mediastation-go",
		"ghcr.io/shukebta/mediastation-go:latest",
		"abc123def456",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("command %q does not contain %q", got, want)
		}
	}
}

func TestDefaultSystemUpdateCommandUsesCompose(t *testing.T) {
	status := SystemUpdateStatus{
		ComposeDir:     "/opt/mediastation-go",
		ComposeCommand: "docker compose",
		ContainerName:  "mediastation-go",
	}
	got := renderSystemUpdateCommand(defaultSystemUpdateCommand(), status)
	for _, want := range []string{
		"cd " + shellQuote("/opt/mediastation-go"),
		"docker compose pull",
		"docker compose up -d",
		"docker image prune -f",
		"docker restart " + shellQuote("mediastation-go"),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("default command %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "watchtower") {
		t.Fatalf("default command should not use watchtower: %q", got)
	}
}

func TestComposeTargetInDirMatchesMediaStationCompose(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`
services:
  mediastation-go:
    image: ghcr.io/shukebta/mediastation-go:latest
`), 0o600); err != nil {
		t.Fatal(err)
	}
	target := composeTargetInDir(dir)
	if target.Dir != dir || !strings.HasSuffix(target.File, "docker-compose.yml") {
		t.Fatalf("compose target = %#v", target)
	}
}

func TestComposeTargetInDirIgnoresUnrelatedCompose(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`
services:
  redis:
    image: redis:7
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if target := composeTargetInDir(dir); target.Dir != "" {
		t.Fatalf("unrelated compose should be ignored: %#v", target)
	}
}

func TestParseContainerInspectLine(t *testing.T) {
	name, imageID := parseContainerInspectLine("/mediastation-go|sha256:abc")
	if name != "mediastation-go" || imageID != "sha256:abc" {
		t.Fatalf("parse inspect line = %q %q", name, imageID)
	}

	name, imageID = parseContainerInspectLine("/fallback")
	if name != "fallback" || imageID != "" {
		t.Fatalf("parse fallback line = %q %q", name, imageID)
	}
}

func TestDockerDigestHelpers(t *testing.T) {
	const local = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const remote = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	raw := `{"Descriptor":{"digest":"` + remote + `"}}`
	if got := firstDockerDigest(raw); got != remote {
		t.Fatalf("firstDockerDigest = %q, want %q", got, remote)
	}
	if got := compareDockerDigests("", remote); got != nil {
		t.Fatalf("missing local digest should be unknown, got %#v", *got)
	}
	if got := compareDockerDigests(local, remote); got == nil || !*got {
		t.Fatalf("different digests should mean update available, got %#v", got)
	}
	if got := compareDockerDigests(local, local); got == nil || *got {
		t.Fatalf("same digests should mean no update, got %#v", got)
	}
}

func TestSystemUpdateCustomFallback(t *testing.T) {
	status := systemUpdateCustomFallback(SystemUpdateStatus{}, systemUpdateFallback{
		command:        "echo update",
		customMessage:  "custom",
		customDetails:  "custom details",
		defaultMessage: "default",
		defaultDetails: "default details",
	})
	if !status.CanApply || status.Message != "custom" || status.Details != "custom details" {
		t.Fatalf("custom fallback = %#v", status)
	}

	status = systemUpdateCustomFallback(SystemUpdateStatus{}, systemUpdateFallback{
		customMessage:  "custom",
		customDetails:  "custom details",
		defaultMessage: "default",
		defaultDetails: "default details",
	})
	if status.CanApply || status.Message != "default" || status.Details != "default details" {
		t.Fatalf("default fallback = %#v", status)
	}
}

func TestSystemUpdateOutputDetailsKeepsTail(t *testing.T) {
	lines := make([]string, 0, 14)
	for i := 0; i < 14; i++ {
		lines = append(lines, "line")
	}
	got := systemUpdateOutputDetails(strings.Join(lines, "\n"))
	if len(got) != 12 {
		t.Fatalf("details length = %d, want 12", len(got))
	}
}

func TestSystemUpdateStatusIncludesCurrentVersion(t *testing.T) {
	svc := NewSystemUpdateService(nil, nil, nil, nil, "MediaStationGo-v0.0.99")

	status := svc.Status(context.Background())
	if status.CurrentVersion != "MediaStationGo-v0.0.99" {
		t.Fatalf("current_version = %q, want MediaStationGo-v0.0.99", status.CurrentVersion)
	}
}

func TestSystemUpdateStatusDefaultsCurrentVersion(t *testing.T) {
	svc := NewSystemUpdateService(nil, nil, nil, nil, "")

	status := svc.Status(context.Background())
	if status.CurrentVersion != "dev" {
		t.Fatalf("current_version = %q, want dev", status.CurrentVersion)
	}
}
