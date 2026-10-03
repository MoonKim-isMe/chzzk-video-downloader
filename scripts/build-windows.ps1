[CmdletBinding()]
param(
    [switch]$RefreshTools
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if ($env:OS -ne "Windows_NT") {
    throw "Build the Windows Portable EXE on Windows."
}

$config = Get-Content -LiteralPath (Join-Path $repoRoot "wails.json") -Raw | ConvertFrom-Json
$version = [string]$config.info.productVersion
if ($version -notmatch '^\d+\.\d+\.\d+$') {
    throw "wails.json info.productVersion must use major.minor.patch."
}
$artifactName = "CHZZK-Video-Downloader-v$version-portable.exe"
$releaseDir = Join-Path $repoRoot "build\portable\v$version"

# This script throws on failure; do not rely on a stale native LASTEXITCODE.
& (Join-Path $PSScriptRoot "prepare-windows-tools.ps1") -Force:$RefreshTools
$bundleDir = Join-Path $repoRoot "internal\downloader\bundled\windows-amd64"
$bundle = Get-Content -LiteralPath (Join-Path $bundleDir "bundle-manifest.generated.json") -Raw | ConvertFrom-Json
if ($bundle.platform -ne "windows/amd64" -or [string]::IsNullOrWhiteSpace($bundle.bundleId) -or $bundle.bundleId -eq "unprepared") {
    throw "A prepared Windows amd64 download tool bundle is required."
}
$expectedTools = @("yt-dlp.exe", "ffmpeg.exe", "ffprobe.exe")
if (@($bundle.tools).Count -ne $expectedTools.Count) {
    throw "The download tool bundle must contain exactly three tools."
}
foreach ($name in $expectedTools) {
    $entries = @($bundle.tools | Where-Object { $_.name -ceq $name })
    if ($entries.Count -ne 1) {
        throw "Missing or duplicate bundled tool: $name"
    }
    $toolPath = Join-Path $bundleDir $name
    if (-not (Test-Path -LiteralPath $toolPath -PathType Leaf)) {
        throw "Missing bundled tool: $name. Retry with -RefreshTools."
    }
    $hash = (Get-FileHash -LiteralPath $toolPath -Algorithm SHA256).Hash
    if ($hash -ne $entries[0].sha256) {
        throw "Bundled tool checksum mismatch: $name. Retry with -RefreshTools."
    }
}

Push-Location $repoRoot
try {
    # No NSIS, app installer, UPX, or code signing. Embed the official WebView2
    # bootstrapper so machines without the runtime receive an installation prompt.
    $arguments = @("build", "-platform", "windows/amd64", "-webview2", "embed", "-o", $artifactName)
    $builtExe = Join-Path $repoRoot "build\bin\$artifactName"
    if (Test-Path -LiteralPath $builtExe) {
        Remove-Item -LiteralPath $builtExe -Force
    }
    & wails @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Wails build failed with exit code $LASTEXITCODE"
    }
    if (-not (Test-Path -LiteralPath $builtExe -PathType Leaf)) {
        throw "Wails did not produce $artifactName"
    }
    $fileInfo = (Get-Item -LiteralPath $builtExe).VersionInfo
    if ($fileInfo.CompanyName -ne $config.info.companyName -or
        $fileInfo.ProductName -ne $config.info.productName -or
        $fileInfo.ProductVersion -ne $version -or
        $fileInfo.LegalCopyright -ne $config.info.copyright) {
        throw "Portable EXE product metadata does not match wails.json."
    }

    New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null
    $releaseExe = Join-Path $releaseDir $artifactName
    Copy-Item -LiteralPath $builtExe -Destination $releaseExe -Force
    $hash = (Get-FileHash -LiteralPath $releaseExe -Algorithm SHA256).Hash.ToLowerInvariant()
    $utf8 = New-Object System.Text.UTF8Encoding($false)
    [IO.File]::WriteAllText("$releaseExe.sha256", "$hash  $artifactName`n", $utf8)
    $metadata = [ordered]@{
        product = $config.info.productName
        publisher = $config.info.companyName
        version = $version
        platform = "windows/amd64"
        distribution = "portable"
        artifact = $artifactName
        sha256 = $hash
        webview2 = "embedded-bootstrapper"
        bundle = $bundle
    }
    [IO.File]::WriteAllText((Join-Path $releaseDir "release-metadata.json"), ($metadata | ConvertTo-Json -Depth 8), $utf8)
    Copy-Item -LiteralPath (Join-Path $repoRoot "THIRD_PARTY_NOTICES.md") -Destination $releaseDir -Force
    & (Join-Path $PSScriptRoot "verify-portable.ps1") -ReleaseDir $releaseDir
    Write-Host "Portable release ready: $releaseExe"
}
finally {
    Pop-Location
}
