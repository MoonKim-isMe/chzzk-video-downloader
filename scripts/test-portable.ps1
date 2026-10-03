# Contract checks for the packaging scripts. Uses fixtures, not a real Windows build.
[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$fixture = Join-Path ([IO.Path]::GetTempPath()) ("chzzk portable test " + [Guid]::NewGuid().ToString("N"))
$originalOS = $env:OS

function Assert-Throws {
    param([scriptblock]$Action, [string]$Expected)
    $caught = $false
    try { & $Action } catch {
        if ($_.Exception.Message -notlike "*$Expected*") { throw }
        $caught = $true
    }
    if (-not $caught) { throw "Expected rejection: $Expected" }
}

try {
    New-Item -ItemType Directory -Path (Join-Path $fixture "scripts") -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "build-windows.ps1") -Destination (Join-Path $fixture "scripts")
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "verify-portable.ps1") -Destination (Join-Path $fixture "scripts")
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "..\wails.json") -Destination $fixture
    Set-Content -LiteralPath (Join-Path $fixture "THIRD_PARTY_NOTICES.md") -Value "Test notices"
    Set-Content -LiteralPath (Join-Path $fixture "scripts\prepare-windows-tools.ps1") -Value 'param([switch]$Force)'
    $config = Get-Content -LiteralPath (Join-Path $fixture "wails.json") -Raw | ConvertFrom-Json
    $bundleDir = Join-Path $fixture "internal\downloader\bundled\windows-amd64"
    New-Item -ItemType Directory -Path $bundleDir -Force | Out-Null
    $tools = @(foreach ($name in @("yt-dlp.exe", "ffmpeg.exe", "ffprobe.exe")) {
        $toolPath = Join-Path $bundleDir $name
        [IO.File]::WriteAllBytes($toolPath, [byte[]](0x4d, 0x5a, 1, 2))
        @{ name = $name; sha256 = (Get-FileHash -LiteralPath $toolPath -Algorithm SHA256).Hash.ToLowerInvariant() }
    })
    $manifestPath = Join-Path $bundleDir "bundle-manifest.generated.json"
    @{ platform = "windows/amd64"; bundleId = "fixture"; tools = $tools } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $manifestPath

    # Mock only the external builder and Windows file properties. Hashes and
    # release validation use the real filesystem, including a path with spaces.
    function wails {
        if ($args -contains "-nsis" -or $args -contains "-upx" -or
            ($args -join " ") -notlike "*-webview2 embed*") {
            throw "Unexpected build strategy"
        }
        $output = $args[[Array]::IndexOf($args, "-o") + 1]
        New-Item -ItemType Directory -Path "build/bin" -Force | Out-Null
        $bytes = New-Object byte[] 256
        $bytes[0] = 0x4d; $bytes[1] = 0x5a; $bytes[60] = 128
        $bytes[128] = 0x50; $bytes[129] = 0x45
        $bytes[132] = 0x64; $bytes[133] = 0x86
        [IO.File]::WriteAllBytes((Join-Path (Get-Location) "build/bin/$output"), $bytes)
        $global:LASTEXITCODE = 0
    }
    function Get-Item {
        param([string]$LiteralPath)
        [pscustomobject]@{ VersionInfo = [pscustomobject]@{
            CompanyName = $config.info.companyName
            ProductName = $config.info.productName
            ProductVersion = $config.info.productVersion
            LegalCopyright = $config.info.copyright
        }}
    }
    $env:OS = "Windows_NT"
    $build = Join-Path $fixture "scripts/build-windows.ps1"
    & $build
    $releaseDir = Join-Path $fixture "build/portable/v$($config.info.productVersion)"
    $verify = Join-Path $fixture "scripts/verify-portable.ps1"
    $exe = Join-Path $releaseDir "CHZZK-Video-Downloader-v$($config.info.productVersion)-portable.exe"
    if (-not (Test-Path -LiteralPath $exe)) { throw "Versioned EXE was not packaged" }
    & $verify -ReleaseDir $releaseDir

    $bytes = [IO.File]::ReadAllBytes($exe)
    $bytes[255] = 9
    [IO.File]::WriteAllBytes($exe, $bytes)
    Assert-Throws { & $verify -ReleaseDir $releaseDir } "checksum"
    $bytes[133] = 0x14
    [IO.File]::WriteAllBytes($exe, $bytes)
    Assert-Throws { & $verify -ReleaseDir $releaseDir } "amd64 PE"
    & $build
    Copy-Item -LiteralPath $exe -Destination (Join-Path $releaseDir "Setup.exe")
    Assert-Throws { & $verify -ReleaseDir $releaseDir } "exactly one"
    Remove-Item -LiteralPath (Join-Path $releaseDir "Setup.exe")
    Remove-Item -LiteralPath (Join-Path $releaseDir "THIRD_PARTY_NOTICES.md")
    Assert-Throws { & $verify -ReleaseDir $releaseDir } "notices"
    Remove-Item -LiteralPath (Join-Path $bundleDir "ffprobe.exe")
    Assert-Throws { & $build } "Missing bundled tool"
    [IO.File]::WriteAllBytes((Join-Path $bundleDir "ffprobe.exe"), [byte[]](0x4d, 0x5a, 1, 2))
    [IO.File]::WriteAllBytes((Join-Path $bundleDir "yt-dlp.exe"), [byte[]](1, 2, 3))
    Assert-Throws { & $build } "checksum mismatch"
    Assert-Throws { & $build -Installer } "parameter"
    $env:OS = "NotWindows"
    Assert-Throws { & $build } "on Windows"
    Write-Host "Portable packaging contract checks passed."
}
finally {
    $env:OS = $originalOS
    Remove-Item -LiteralPath $fixture -Recurse -Force -ErrorAction SilentlyContinue
}
