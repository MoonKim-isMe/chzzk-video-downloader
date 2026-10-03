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

function Assert-Version {
    param([string]$Expected)
    foreach ($entry in @(@("wails.json", "productVersion"), @("frontend/package.json", "version"))) {
        $json = Get-Content -LiteralPath (Join-Path $fixture $entry[0]) -Encoding UTF8 -Raw | ConvertFrom-Json
        $actual = if ($entry[0] -eq "wails.json") { $json.info.productVersion } else { $json.version }
        if ($actual -cne $Expected) { throw "Unexpected version: $actual, expected $Expected" }
    }
    $folder = Join-Path $fixture "build/portable/v$Expected"
    $metadata = Get-Content -LiteralPath (Join-Path $folder "release-metadata.json") -Encoding UTF8 -Raw | ConvertFrom-Json
    if ($metadata.version -cne $Expected -or $metadata.artifact -cne "CHZZK-Video-Downloader-v$Expected-portable.exe") {
        throw "Release version and EXE filename must match source versions"
    }
}

function Assert-UnchangedVersions {
    param([string]$WailsBefore, [string]$PackageBefore)
    if ([Convert]::ToBase64String([IO.File]::ReadAllBytes((Join-Path $fixture "wails.json"))) -cne $WailsBefore -or
        [Convert]::ToBase64String([IO.File]::ReadAllBytes((Join-Path $fixture "frontend/package.json"))) -cne $PackageBefore) {
        throw "Source version files must remain byte-identical"
    }
}

try {
    New-Item -ItemType Directory -Path (Join-Path $fixture "scripts") -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "build-windows.ps1") -Destination (Join-Path $fixture "scripts")
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "verify-portable.ps1") -Destination (Join-Path $fixture "scripts")
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "..\wails.json") -Destination $fixture
    New-Item -ItemType Directory -Path (Join-Path $fixture "frontend") -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "../frontend/package.json") -Destination (Join-Path $fixture "frontend")
    # Check successful version changes preserve UTF-8 BOM, CRLF and other fields.
    foreach ($path in @("wails.json", "frontend/package.json")) {
        $fullPath = Join-Path $fixture $path
        $text = [IO.File]::ReadAllText($fullPath).Replace("`r`n", "`n").Replace("`n", "`r`n")
        [IO.File]::WriteAllText($fullPath, $text, (New-Object Text.UTF8Encoding($true)))
    }
    $initialWails = [IO.File]::ReadAllText((Join-Path $fixture "wails.json"))
    $initialPackage = [IO.File]::ReadAllText((Join-Path $fixture "frontend/package.json"))
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
        if ($mockBuildFailure) {
            $global:LASTEXITCODE = 1
            return
        }
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
        $config = Get-Content -LiteralPath (Join-Path $fixture "wails.json") -Encoding UTF8 -Raw | ConvertFrom-Json
        [pscustomobject]@{ VersionInfo = [pscustomobject]@{
            CompanyName = $config.info.companyName
            ProductName = $config.info.productName
            ProductVersion = $config.info.productVersion
            LegalCopyright = $config.info.copyright
        }}
    }
    $env:OS = "Windows_NT"
    $build = Join-Path $fixture "scripts/build-windows.ps1"
    $startVersion = [string]$config.info.productVersion
    & $build
    $config = Get-Content -LiteralPath (Join-Path $fixture "wails.json") -Encoding UTF8 -Raw | ConvertFrom-Json
    $startParts = $startVersion.Split('.')
    $expectedPatch = "$($startParts[0]).$($startParts[1]).$([int]$startParts[2] + 1)"
    if ($config.info.productVersion -cne $expectedPatch) { throw "Default patch version was not increased" }
    $front = Get-Content -LiteralPath (Join-Path $fixture "frontend/package.json") -Encoding UTF8 -Raw | ConvertFrom-Json
    if ($front.version -cne $expectedPatch) { throw "Frontend version was not synchronized" }
    foreach ($item in @(@("wails.json", $initialWails, "productVersion"), @("frontend/package.json", $initialPackage, "version"))) {
        $fullPath = Join-Path $fixture $item[0]
        $expected = $item[1].Replace('"' + $item[2] + '": "' + $startVersion + '"', '"' + $item[2] + '": "' + $expectedPatch + '"')
        $bytes = [IO.File]::ReadAllBytes($fullPath)
        if ([IO.File]::ReadAllText($fullPath) -cne $expected -or $bytes[0] -ne 239 -or $bytes[1] -ne 187 -or $bytes[2] -ne 191) {
            throw "Version changes must preserve JSON formatting and BOM"
        }
    }
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
    & $build -NoVersionBump
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
    $env:OS = "Windows_NT"
    [IO.File]::WriteAllBytes((Join-Path $bundleDir "yt-dlp.exe"), [byte[]](0x4d, 0x5a, 1, 2))
    & $build -Minor -RefreshTools
    $expectedMinor = "$($startParts[0]).$([int]$startParts[1] + 1).0"
    Assert-Version $expectedMinor
    & $build -Major
    $expectedMajor = "$([int]$startParts[0] + 1).0.0"
    Assert-Version $expectedMajor
    $wailsPath = Join-Path $fixture "wails.json"
    $packagePath = Join-Path $fixture "frontend/package.json"
    $wailsBefore = [Convert]::ToBase64String([IO.File]::ReadAllBytes($wailsPath))
    $packageBefore = [Convert]::ToBase64String([IO.File]::ReadAllBytes($packagePath))
    & $build -NoVersionBump
    Assert-UnchangedVersions $wailsBefore $packageBefore
    $mockBuildFailure = $true
    Assert-Throws { & $build } "Wails build failed"
    Assert-UnchangedVersions $wailsBefore $packageBefore
    Assert-Throws { & $build -Minor } "Wails build failed"
    Assert-UnchangedVersions $wailsBefore $packageBefore
    Assert-Throws { & $build -Major } "Wails build failed"
    Assert-UnchangedVersions $wailsBefore $packageBefore
    $mockBuildFailure = $false
    # A failure after compilation must also restore the exact source bytes.
    $majorParts = $expectedMajor.Split('.')
    $failedRelease = Join-Path $fixture "build/portable/v$($majorParts[0]).0.1"
    New-Item -ItemType Directory -Path $failedRelease -Force | Out-Null
    [IO.File]::WriteAllBytes((Join-Path $failedRelease "Setup.exe"), [byte[]](1, 2))
    Assert-Throws { & $build } "exactly one"
    Assert-UnchangedVersions $wailsBefore $packageBefore
    Remove-Item -LiteralPath $failedRelease -Recurse -Force
    Assert-Throws { & $build -Major -Minor } "parameter set"
    Assert-Throws { & $build -Major -NoVersionBump } "parameter set"
    Assert-Throws { & $build -Minor -NoVersionBump } "parameter set"
    Assert-UnchangedVersions $wailsBefore $packageBefore
    $package = [IO.File]::ReadAllText($packagePath).Replace('"version": "' + $expectedMajor + '"', '"version": "9.9.9"')
    [IO.File]::WriteAllText($packagePath, $package)
    Assert-Throws { & $build } "versions must match"
    Write-Host "Portable packaging and version contract checks passed."
}
finally {
    $env:OS = $originalOS
    Remove-Item -LiteralPath $fixture -Recurse -Force -ErrorAction SilentlyContinue
}
