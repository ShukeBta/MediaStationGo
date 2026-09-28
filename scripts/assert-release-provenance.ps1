[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$Repository,

    [Parameter(Mandatory)]
    [string]$Commit,

    [Parameter(Mandatory)]
    [string]$RemoteBranch,

    [Parameter(Mandatory)]
    [string]$ReleaseTag,

    [string]$Remote = 'origin'
)

$ErrorActionPreference = 'Stop'
$repoPath = (Resolve-Path -LiteralPath $Repository).Path

function Invoke-Git([string[]]$Arguments) {
    $output = & git -C $repoPath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "git $($Arguments -join ' ') failed"
    }
    return @($output)
}

if ((Invoke-Git @('rev-parse', '--is-shallow-repository')).Trim() -eq 'true') {
    throw '发布来源仓库不能是 shallow clone'
}

$changes = Invoke-Git @('status', '--porcelain')
if ($changes.Count -gt 0) {
    throw '发布来源工作树不干净'
}

$fullCommit = (Invoke-Git @('rev-parse', "$Commit^{commit}")).Trim()
$branchRef = "refs/heads/$RemoteBranch"
$branchHead = Invoke-Git @('ls-remote', '--exit-code', $Remote, $branchRef)
if ($branchHead.Count -ne 1) {
    throw "远端分支不存在或不唯一: $Remote/$RemoteBranch"
}

Invoke-Git @('fetch', '--no-tags', $Remote, $branchRef) | Out-Null
& git -C $repoPath merge-base --is-ancestor $fullCommit FETCH_HEAD
if ($LASTEXITCODE -ne 0) {
    throw "候选 SHA 不在远端分支祖先链中: $fullCommit"
}

$tagRef = "refs/tags/$ReleaseTag"
$tagLines = Invoke-Git @('ls-remote', '--tags', $Remote, $tagRef, "$tagRef^{}")
$tagCommit = $null
foreach ($line in $tagLines) {
    $parts = $line -split "`t", 2
    if ($parts.Count -eq 2 -and ($parts[1] -eq "$tagRef^{}" -or $parts[1] -eq $tagRef)) {
        $tagCommit = $parts[0]
    }
}
if ($tagCommit -ne $fullCommit) {
    throw "远端 release tag 未精确指向候选 SHA: $ReleaseTag"
}

Write-Host "Release provenance verified: $fullCommit on $Remote/$RemoteBranch via $ReleaseTag"
