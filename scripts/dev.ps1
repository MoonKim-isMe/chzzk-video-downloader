param(
    [switch]$RefreshTools
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

& (Join-Path $PSScriptRoot "prepare-windows-tools.ps1") -Force:$RefreshTools
if ($LASTEXITCODE -ne 0) {
    throw "Failed to prepare Windows download tools."
}

Push-Location $repoRoot
try {
    & wails dev
    if ($LASTEXITCODE -ne 0) {
        throw "Wails dev failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
