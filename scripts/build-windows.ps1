[CmdletBinding(DefaultParameterSetName = "Patch")]
param(
    [Parameter(ParameterSetName = "Minor")][switch]$Minor,
    [Parameter(ParameterSetName = "Major")][switch]$Major,
    [Parameter(ParameterSetName = "NoVersionBump")][switch]$NoVersionBump,
    [switch]$RefreshTools
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if ($env:OS -ne "Windows_NT") {
    throw "Build the Windows Portable EXE on Windows."
}

$wailsPath = Join-Path $repoRoot "wails.json"
$packagePath = Join-Path $repoRoot "frontend\package.json"
$originalWails = [IO.File]::ReadAllBytes($wailsPath)
$originalPackage = [IO.File]::ReadAllBytes($packagePath)
$wailsText = [IO.File]::ReadAllText($wailsPath)
$packageText = [IO.File]::ReadAllText($packagePath)
$config = $wailsText | ConvertFrom-Json
$package = $packageText | ConvertFrom-Json
$currentVersion = [string]$config.info.productVersion
if ($currentVersion -notmatch '^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$') {
    throw "wails.json info.productVersion must use major.minor.patch."
}
if ($package.version -cne $currentVersion) {
    throw "wails.json and frontend/package.json versions must match before building."
}
$parts = @($currentVersion.Split('.') | ForEach-Object { [int]$_ })
if ($Major) { $parts = @(($parts[0] + 1), 0, 0) }
elseif ($Minor) { $parts = @($parts[0], ($parts[1] + 1), 0) }
elseif (-not $NoVersionBump) { $parts[2]++ }
if (@($parts | Where-Object { $_ -gt 65535 }).Count -gt 0) {
    throw "Windows product version components must not exceed 65535."
}
$version = $parts -join '.'
$config.info.productVersion = $version

function Set-VersionText {
    param([string]$Text, [string]$Property, [string]$Value)
    $pattern = '("' + [Regex]::Escape($Property) + '"\s*:\s*")[^"]+(")'
    # Preserve formatting and every other JSON field.
    if ([Regex]::Matches($Text, $pattern).Count -ne 1) {
        throw "Expected exactly one $Property version field."
    }
    return [Regex]::Replace($Text, $pattern, '${1}' + $Value + '${2}')
}
$nextWails = Set-VersionText -Text $wailsText -Property "productVersion" -Value $version
$nextPackage = Set-VersionText -Text $packageText -Property "version" -Value $version
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
$versionsWritten = $false
try {
    # Wails reuses local build assets. Always refresh our managed version resource
    # so old/default info.json files cannot omit the string FileVersion required
    # by Windows PowerShell's .NET Framework metadata reader.
    $windowsAssetsDir = Join-Path $repoRoot "build\windows"
    New-Item -ItemType Directory -Path $windowsAssetsDir -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "windows-info.json") -Destination (Join-Path $windowsAssetsDir "info.json") -Force
    # Keep Wails development/build icon resources in sync with repository-managed sources.
    $appIconSource = Join-Path $repoRoot "assets\appicon.png"
    $windowsIconSource = Join-Path $repoRoot "assets\appicon.ico"
    foreach ($iconSource in @($appIconSource, $windowsIconSource)) {
        if (-not (Test-Path -LiteralPath $iconSource -PathType Leaf)) {
            throw "Missing application icon source: $iconSource"
        }
    }
    Copy-Item -LiteralPath $appIconSource -Destination (Join-Path $repoRoot "build\appicon.png") -Force
    Copy-Item -LiteralPath $windowsIconSource -Destination (Join-Path $windowsAssetsDir "icon.ico") -Force
    if ($version -cne $currentVersion) {
        $versionsWritten = $true
        $wailsBom = $originalWails.Length -ge 3 -and $originalWails[0] -eq 239 -and $originalWails[1] -eq 187 -and $originalWails[2] -eq 191
        $packageBom = $originalPackage.Length -ge 3 -and $originalPackage[0] -eq 239 -and $originalPackage[1] -eq 187 -and $originalPackage[2] -eq 191
        [IO.File]::WriteAllText($wailsPath, $nextWails, (New-Object Text.UTF8Encoding($wailsBom)))
        [IO.File]::WriteAllText($packagePath, $nextPackage, (New-Object Text.UTF8Encoding($packageBom)))
    }
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
    $expectedMetadata = [ordered]@{
        FileVersion = $version
        CompanyName = $config.info.companyName
        ProductName = $config.info.productName
        ProductVersion = $version
        LegalCopyright = $config.info.copyright
    }
    $mismatches = @(foreach ($field in $expectedMetadata.Keys) {
        $actual = [string]$fileInfo.$field
        $expected = [string]$expectedMetadata[$field]
        if ($actual -cne $expected) {
            $actualDisplay = if ([string]::IsNullOrEmpty($actual)) { "<empty>" } else { $actual }
            "${field}: expected='$expected', actual='$actualDisplay'"
        }
    })
    if ($mismatches.Count -gt 0) {
        throw ("Portable EXE product metadata does not match wails.json.`nEXE: $builtExe`n" + ($mismatches -join "`n"))
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
        projectLicense = "MIT"
        projectLicenseFile = "LICENSE"
        thirdPartyNotices = "THIRD_PARTY_NOTICES.md"
        webview2 = "embedded-bootstrapper"
        bundle = $bundle
    }
    [IO.File]::WriteAllText((Join-Path $releaseDir "release-metadata.json"), ($metadata | ConvertTo-Json -Depth 8), $utf8)
    Copy-Item -LiteralPath (Join-Path $repoRoot "LICENSE") -Destination $releaseDir -Force
    Copy-Item -LiteralPath (Join-Path $repoRoot "THIRD_PARTY_NOTICES.md") -Destination $releaseDir -Force
    & (Join-Path $PSScriptRoot "verify-portable.ps1") -ReleaseDir $releaseDir
    Write-Host "Portable release ready: $releaseExe"
    Write-Host "Product version: $currentVersion -> $version"
}
catch {
    if ($versionsWritten) {
        [IO.File]::WriteAllBytes($wailsPath, $originalWails)
        [IO.File]::WriteAllBytes($packagePath, $originalPackage)
    }
    throw
}
finally {
    Pop-Location
}
