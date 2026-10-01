param(
  [string]$WorkDir = "",
  [string]$PromptFile = "",
  [string]$Agent = "",
  [switch]$List
)

$ErrorActionPreference = "Stop"
$here = $PSScriptRoot
$arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { $arch = "arm64" }
$binDir = Join-Path $here "bin"
$bin = Join-Path $binDir "astack-windows-$arch.exe"

function Find-Go {
  $g = Get-Command go -ErrorAction SilentlyContinue
  if ($g) { return $g.Source }
  $cands = @(
    (Join-Path $env:USERPROFILE "go\bin\go.exe"),
    (Join-Path $env:ProgramFiles "Go\bin\go.exe"),
    (Join-Path $env:LOCALAPPDATA "Programs\Go\bin\go.exe")
  )
  foreach ($p in $cands) {
    if (Test-Path $p) { return $p }
  }
  return $null
}

function Needs-Build([string]$target) {
  if (-not (Test-Path $target)) { return $true }
  $binTime = (Get-Item $target).LastWriteTimeUtc
  $srcs = @(Get-ChildItem -Path $here -Filter *.go -File)
  $srcs += Get-Item (Join-Path $here "agents.json")
  $srcs += Get-Item (Join-Path $here "go.mod")
  foreach ($src in $srcs) {
    if ($src.LastWriteTimeUtc -gt $binTime) { return $true }
  }
  return $false
}

$dispatchArgs = @()
if ($List) {
  $dispatchArgs = @("--list")
} elseif ($WorkDir -and $PromptFile) {
  $dispatchArgs = @("--workdir", $WorkDir, "--prompt-file", $PromptFile)
  if ($Agent) { $dispatchArgs += @("--agent", $Agent) }
} else {
  $dispatchArgs = @($args)
}

if (Needs-Build $bin) {
  $goBin = Find-Go
  if ($goBin) {
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
    $old = Get-Location
    Set-Location $here
    try {
      & $goBin build -o $bin .
      if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } finally {
      Set-Location $old
    }
  }
}

if (Test-Path $bin) {
  & $bin @dispatchArgs
  exit $LASTEXITCODE
}

$goBin = Find-Go
if ($goBin) {
  $old = Get-Location
  Set-Location $here
  try {
    & $goBin run . @dispatchArgs
    exit $LASTEXITCODE
  } finally {
    Set-Location $old
  }
}

Write-Error "astack: no dispatcher binary; conductor should edit in this chat. not rerouting."
exit 3
