# Release Provenance

Every production MediaStationGo image must be built from a commit that is
reachable from a protected GitHub branch and exactly referenced by an annotated
`release/mediastationgo/<YYYYMMDD>-<sha7>` tag. The release record, OCI label,
container startup version, and deployed commit must agree.

Before server verification or production deployment, run:

```powershell
.\scripts\assert-release-provenance.ps1 `
  -Repository . `
  -Commit <full-sha> `
  -RemoteBranch main `
  -ReleaseTag release/mediastationgo/<YYYYMMDD>-<sha7>
```

The gate rejects shallow clones, dirty worktrees, commits outside the remote
branch ancestry, and tags that do not resolve to the requested commit.

Protected release branches must reject force pushes and deletion. Direct
updates to `main` are allowed only after the user explicitly authorizes a
release and the exact release tag is created. Before any history migration, first push an
`archive/...` tag for every legacy tip. A `forced-update` or shallow fetch is a
release blocker until the ancestry and semantic differences are audited.
