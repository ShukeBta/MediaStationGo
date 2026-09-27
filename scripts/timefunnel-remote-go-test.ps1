[CmdletBinding()]
param(
    [string]$Remote = "root@156.238.228.206",
    [int]$Port = 23612,
    [string]$Repo = "https://github.com/timefunnel/MediaStationGo.git",
    [string]$Branch = "codex/timefunnel-msg-fork",
    [string]$RemoteRoot = "/opt/codex-build/mediastationgo",
    [string]$Image = "golang:1.25",
    [string]$Packages = "./internal/service ./internal/handler",
    [string]$Cpus = "2",
    [string]$Memory = "2g",
    [ValidateRange(1, 2)][int]$Parallelism = 2,
    [double]$MaxLoad1 = 1.5,
    [int]$MinMemAvailableMb = 1200,
    [switch]$Full,
    [switch]$NoGofmt,
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($RemoteRoot -eq "/data" -or $RemoteRoot.StartsWith("/data/")) {
    throw "RemoteRoot must not be under /data; /data is reserved for media service data."
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath exited with code $LASTEXITCODE"
    }
}

function Get-CheckedOutput {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )

    $output = & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath exited with code $LASTEXITCODE"
    }
    return ($output -join "`n").Trim()
}

function ConvertTo-BashSingleQuoted {
    param([Parameter(Mandatory = $true)][string]$Value)
    return "'" + ($Value -replace "'", "'\''") + "'"
}

if ($Full) {
    $Packages = "./..."
}

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = Get-CheckedOutput "git" @("-C", $scriptDir, "rev-parse", "--show-toplevel")
Set-Location $repoRoot

& git diff --quiet -- .
if ($LASTEXITCODE -eq 0) {
    $hasDiff = $false
} elseif ($LASTEXITCODE -eq 1) {
    $hasDiff = $true
} else {
    throw "git diff --quiet exited with code $LASTEXITCODE"
}

$untracked = @(& git ls-files --others --exclude-standard)
if ($LASTEXITCODE -ne 0) {
    throw "git ls-files exited with code $LASTEXITCODE"
}

$remotePatch = "$RemoteRoot/patches/latest-input.patch"
$remoteFormattedPatch = "$RemoteRoot/patches/latest-formatted.patch"
$localFormattedPatch = Join-Path $repoRoot ".codex-remote-formatted.patch"

Write-Host "Remote: $Remote port $Port"
Write-Host "Remote root: $RemoteRoot"
Write-Host "Image: $Image"
Write-Host "Packages: $Packages"
Write-Host "Resource limit: cpus=$Cpus memory=$Memory parallelism=$Parallelism"
Write-Host "Local tracked diff: $hasDiff"
if ($untracked.Count -gt 0) {
    Write-Host "Untracked files will be included in the remote patch with git add -N:"
    $untracked | ForEach-Object { Write-Host "  $_" }
}

if ($DryRun) {
    Write-Host "Dry run only. No SSH, SCP, Docker, or remote test was executed."
    exit 0
}

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) "timefunnel-msg-remote-test"
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
$localPatch = Join-Path $tempDir "latest-input.patch"
if (Test-Path $localPatch) {
    Remove-Item -Force $localPatch
}

$intentToAddApplied = $false
try {
    if ($untracked.Count -gt 0) {
        Invoke-Checked "git" (@("add", "-N", "--") + $untracked)
        $intentToAddApplied = $true
    }
    Invoke-Checked "git" @("diff", "--binary", "HEAD", "--output=$localPatch", "--", ".")
} finally {
    if ($intentToAddApplied) {
        Invoke-Checked "git" (@("reset", "--") + $untracked)
    }
}
Invoke-Checked "ssh" @("-p", "$Port", $Remote, "mkdir -p $(ConvertTo-BashSingleQuoted "$RemoteRoot/patches")")
Invoke-Checked "scp" @("-P", "$Port", $localPatch, "$Remote`:$remotePatch")

$qRemoteRoot = ConvertTo-BashSingleQuoted $RemoteRoot
$qRepo = ConvertTo-BashSingleQuoted $Repo
$qBranch = ConvertTo-BashSingleQuoted $Branch
$qImage = ConvertTo-BashSingleQuoted $Image
$qRemotePatch = ConvertTo-BashSingleQuoted $remotePatch
$qRemoteFormattedPatch = ConvertTo-BashSingleQuoted $remoteFormattedPatch
$qPackages = ConvertTo-BashSingleQuoted $Packages
$qCpus = ConvertTo-BashSingleQuoted $Cpus
$qMemory = ConvertTo-BashSingleQuoted $Memory
$qParallelism = ConvertTo-BashSingleQuoted ([string]$Parallelism)
$qMaxLoad1 = ConvertTo-BashSingleQuoted ([string]$MaxLoad1)
$qMinMemAvailableMb = ConvertTo-BashSingleQuoted ([string]$MinMemAvailableMb)
$gofmtEnabled = if ($NoGofmt) { "0" } else { "1" }

$remoteScript = @"
set -euo pipefail

REMOTE_ROOT=$qRemoteRoot
REPO=$qRepo
BRANCH=$qBranch
IMAGE=$qImage
PATCH=$qRemotePatch
FORMATTED_PATCH=$qRemoteFormattedPatch
TEST_PACKAGES=$qPackages
CPUS=$qCpus
MEMORY=$qMemory
PARALLELISM=$qParallelism
MAX_LOAD_1=$qMaxLoad1
MIN_MEM_AVAILABLE_MB=$qMinMemAvailableMb
GOFMT_ENABLED=$gofmtEnabled

WORKTREE="`$REMOTE_ROOT/worktree"
GOMODCACHE="`$REMOTE_ROOT/gomod"
GOCACHE="`$REMOTE_ROOT/gocache"

case "`$REMOTE_ROOT" in
  /data|/data/*)
    echo "REMOTE_ROOT must not be under /data; /data is reserved for media service data." >&2
    exit 22
    ;;
esac

load1="`$(awk '{print `$1}' /proc/loadavg)"
too_busy="`$(awk -v load_value="`$load1" -v max="`$MAX_LOAD_1" 'BEGIN {print (load_value > max) ? 1 : 0}')"
if [ "`$too_busy" = "1" ]; then
  echo "remote host is busy: load1=`$load1 max=`$MAX_LOAD_1" >&2
  exit 20
fi

available_mb="`$(awk '/MemAvailable/ {print int(`$2/1024)}' /proc/meminfo)"
if [ "`$available_mb" -lt "`$MIN_MEM_AVAILABLE_MB" ]; then
  echo "remote memory is low: available_mb=`$available_mb min=`$MIN_MEM_AVAILABLE_MB" >&2
  exit 21
fi

mkdir -p "`$REMOTE_ROOT/patches" "`$GOMODCACHE" "`$GOCACHE"

if [ ! -d "`$WORKTREE/.git" ]; then
  rm -rf "`$WORKTREE"
  git clone --depth 1 --branch "`$BRANCH" "`$REPO" "`$WORKTREE"
fi

cd "`$WORKTREE"
git fetch --depth 1 origin "`$BRANCH"
git reset --hard "origin/`$BRANCH"
git clean -fdx

if [ -s "`$PATCH" ]; then
  git apply --whitespace=nowarn "`$PATCH"
else
  echo "local patch is empty; testing origin/`$BRANCH"
fi

if docker image inspect "`$IMAGE" >/dev/null 2>&1; then
  echo "using cached docker image: `$IMAGE"
else
  echo "docker image missing; pulling once: `$IMAGE"
  docker pull "`$IMAGE"
fi

run_go() {
  docker run --rm --cpus="`$CPUS" -m "`$MEMORY" \
    -e GOMAXPROCS="`$PARALLELISM" \
    -e GOMODCACHE=/go/pkg/mod \
    -e GOCACHE=/go/build-cache \
    -v "`$WORKTREE:/src" \
    -v "`$GOMODCACHE:/go/pkg/mod" \
    -v "`$GOCACHE:/go/build-cache" \
    -w /src \
    "`$IMAGE" "`$@"
}

if [ "`$GOFMT_ENABLED" = "1" ]; then
  changed_go="`$(git diff --name-only -- '*.go' | tr '\n' ' ')"
  if [ -n "`$changed_go" ]; then
    # shellcheck disable=SC2086
    run_go gofmt -w `$changed_go
  fi
fi

git diff --binary --output="`$FORMATTED_PATCH" -- .

# shellcheck disable=SC2086
run_go go test -p "`$PARALLELISM" `$TEST_PACKAGES

echo "formatted patch: `$FORMATTED_PATCH"
du -sh "`$REMOTE_ROOT" 2>/dev/null || true
"@

$remoteScript = $remoteScript -replace "`r`n", "`n"
$remoteScript | & ssh -p $Port $Remote "bash -s"
if ($LASTEXITCODE -ne 0) {
    throw "remote test exited with code $LASTEXITCODE"
}

Invoke-Checked "scp" @("-P", "$Port", "$Remote`:$remoteFormattedPatch", $localFormattedPatch)
Write-Host "Remote test passed."
Write-Host "Formatted patch copied to $localFormattedPatch"
