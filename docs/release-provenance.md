# Release Provenance

Every production MediaStationGo image must be built from a commit that is
reachable from the remote `codex/local-dev` development branch and exactly
referenced by an annotated `release/mediastationgo/<YYYYMMDD>-<sha7>` tag. The
release record, OCI label, container startup version, and deployed commit must
agree.

Before an explicitly authorized production deployment, run:

```powershell
.\scripts\assert-release-provenance.ps1 `
  -Repository . `
  -Commit <full-sha> `
  -RemoteBranch codex/local-dev `
  -ReleaseTag release/mediastationgo/<YYYYMMDD>-<sha7>
```

The gate rejects shallow clones, dirty worktrees, commits outside the remote
branch ancestry, and tags that do not resolve to the requested commit.

`codex/local-dev` is the continuous development, candidate-build, server-validation,
and production-release source. A release does not require the candidate commit to
be merged into `main`. Do not rewrite published `codex/local-dev` history without
an explicit ancestry audit; before any necessary migration, first push an
`archive/...` tag for every legacy tip.

`main` is a stable aggregation branch. Merge a reviewed batch from
`codex/local-dev` only after it has accumulated a set of stable features and the
user explicitly requests a new stable baseline. Per-task development, candidate
validation, and production deployment must not implicitly merge into `main`.
