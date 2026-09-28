# Timefunnel Fork Maintenance

This fork keeps MediaStationGo close to upstream while carrying a small set of
deployment-specific fixes for the Timefunnel media stack.

## Repository Layout

- `origin`: `https://github.com/timefunnel/MediaStationGo.git`
- `upstream`: `https://github.com/ShukeBta/MediaStationGo.git`
- maintenance branch: `codex/timefunnel-msg-fork`
- production image target: `ghcr.io/timefunnel/mediastation-go:<tag>`

## Maintenance Rules

1. Keep custom changes small and documented.
2. Prefer feature flags or configuration over hard-coded environment behavior.
3. Do not rewrite unrelated upstream code while fixing a local issue.
4. Add or update tests for every behavior change.
5. Sync upstream by merging or cherry-picking intentionally; do not overwrite
   local patches.

## Initial Patch Scope

Priority 1 fixes:

- Make OpenList playback resolve more tolerant of stale object cache by listing
  the parent directory before retrying `fs/get` when OpenList reports a false
  negative missing-file error.
- Keep cloud playback as pure 302 whenever OpenList can provide a CDN URL.
- Improve Emby-compatible playback metadata for Filmly/Infuse/VidHub without
  forcing server-side transcoding.
- Align recycle-bin or deleted-media behavior with OpenList Meta Hide so removed
  entries do not reappear on the next scan.

Priority 2 fixes:

- Add precise root-level scan and single-item scrape APIs for bot integration.
- Add library migration without re-scraping when only library/path ownership
  changes.
- Improve adult title normalization, poster/thumb handling, and subtitle status
  reporting.

## Applied Local Patches

- `timefunnel-20260710-openlist-warmup`: when OpenList `fs/get` reports a
  false missing-file error for 115/OpenList paths, list the parent directory and
  retry `fs/get` once while keeping playback on pure 302.
- `timefunnel-20260710-cloud-tombstone`: cloud media deleted from MSG is kept as
  a soft-deleted tombstone, and later scans skip that path instead of importing
  it again. Explicit restore from the recycle bin remains supported.
- maintenance branch pending release: Emby library folder cover grids are
  generated inside MSG from up to four child media posters, removing one of the
  remaining subtitle-proxy responsibilities.
- maintenance branch pending release: external subtitle streams are exposed
  directly through Emby PlaybackInfo and `/emby/api/subtitles/...`, so Emby
  clients can discover MSG subtitles without the external subtitle proxy.
- maintenance branch pending release: adult media names in Emby payloads are
  normalized to display a canonical code prefix without relying on the external
  subtitle proxy JSON patch.
- maintenance branch pending release: Emby media payloads now include image
  owner fields (`PrimaryImageItemId`, `PrimaryImageTag`, and backdrop owner
  ids), removing another subtitle-proxy JSON patch.
- maintenance branch pending release: OpenList cold playback signing keeps
  same-file requests serialized across different client User-Agents and allows
  at most two different files to resolve concurrently. Additional requests wait
  for a signing slot; CDN video transfer remains client-direct and is not
  limited by this gate.

## OpenList Cold Signing Controls

The OpenList playback resolver uses the following fixed safety boundaries:

- Existing cache and singleflight behavior remains keyed by provider, file,
  and client User-Agent because 115 CDN links are User-Agent-bound.
- Different User-Agents resolving the same file are serialized to one active
  signing request.
- Different files may use at most two active OpenList signing requests. Later
  requests wait for a slot without an artificial delay between requests.
- Queue wait is included in the existing 8-second cold-resolve deadline. The
  stream entry point allows 9 seconds in total, so raising only one timeout is
  not a valid latency fix.
- A queued request checks the provider circuit after admission. A detected
  `40140117` rate limit therefore prevents later queued work from continuing to
  call OpenList during the circuit cooldown.
- The gate applies only to OpenList direct-link resolution. It does not limit
  non-OpenList providers or client-to-`115cdn.net` video traffic.

Keep application logging at `info` while validating this behavior. Relevant
fields and messages are:

- `cloud resolve signing queue wait`, with `queue_ms` for time spent waiting on
  the file and global signing gates.
- `cloud playback direct redirect`, with `resolve_ms` and `target_host` for a
  successful direct redirect.
- `cloud playback direct resolve failed`, with `resolve_ms` and the surfaced
  error for failed startup attempts.

Do not increase the global concurrency above two solely to hide queue latency.
First compare `queue_ms`, `resolve_ms`, and `40140117` frequency under real
playback traffic.

## Baseline Validation

Before deploying a custom image:

```bash
go vet ./...
go build ./...
go test ./...
cd web && npm ci && npm run build
docker compose -f docker-compose.yml config --quiet
```

The local Windows workstation currently does not have Go or Docker installed, so
baseline validation should run in GitHub Actions or on a controlled remote build
host.

## Remote Test Workflow

Use `scripts/timefunnel-remote-go-test.ps1` from this repository when local Go is
not available. The helper keeps a reusable remote worktree and Go caches under
`/opt/codex-build/mediastationgo`, reuses the existing `golang:1.25` Docker
image, and refuses to start when the remote host is already busy or low on
available memory. Do not place this test cache under `/data`; that disk is
reserved for media service data and is already capacity constrained.

The default resource limit is intentionally conservative (`1` CPU, `1280m`
memory, and load1 threshold `1.5`) to avoid visible service jitter on the small
production VPS. Raise these values only for an explicit release validation
window.

Fast targeted validation:

```powershell
.\scripts\timefunnel-remote-go-test.ps1
```

Full Go validation, only when the server is idle and the wider change requires
it:

```powershell
.\scripts\timefunnel-remote-go-test.ps1 -Full
```

Operational rules:

1. Do not remove the `golang:1.25` image after every test; repeated pulls create
   avoidable network, CPU, and disk I/O spikes.
2. Keep Go module/build caches between test runs unless disk pressure requires a
   manual cleanup.
3. Prefer targeted package tests while iterating, then run `-Full` before a
   release batch.
4. Do not use remote tests for playback/OpenList/115 verification unless that is
   explicitly required.

## Production Deployment Rule

Production compose should only switch from upstream to the fork after:

1. CI passes on the fork branch.
2. A tagged GHCR image is published.
3. `/data/MediaStationGo/data` and `/data/MediaStationGo/postgres` are backed up.
4. A rollback image tag is recorded.
