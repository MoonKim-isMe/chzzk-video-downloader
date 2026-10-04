param(
    [switch]$RefreshTools
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

$frontendDir = Join-Path $repoRoot "frontend"
$frontendDist = Join-Path $frontendDir "dist"
$distFiles = @()
if (Test-Path -LiteralPath $frontendDist -PathType Container) {
    $distFiles = @(Get-ChildItem -LiteralPath $frontendDist -Recurse -File -Force -ErrorAction SilentlyContinue)
}
if ($distFiles.Count -eq 0) {
    Write-Host "frontend/dist is empty. Building frontend assets required by Go embed..."
    Push-Location $frontendDir
    try {
        & yarn build
        if ($LASTEXITCODE -ne 0) {
            throw "Frontend build failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }

    $distFiles = @()
    if (Test-Path -LiteralPath $frontendDist -PathType Container) {
        $distFiles = @(Get-ChildItem -LiteralPath $frontendDist -Recurse -File -Force -ErrorAction SilentlyContinue)
    }
    if ($distFiles.Count -eq 0) {
        throw "Frontend build completed without embeddable files in frontend/dist."
    }
}

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
