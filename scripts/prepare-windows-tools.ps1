param(
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$targetDir = Join-Path $repoRoot "internal\downloader\bundled\windows-amd64"
$manifestPath = Join-Path $targetDir "bundle-manifest.generated.json"
$ytDlpPath = Join-Path $targetDir "yt-dlp.exe"
$ffmpegPath = Join-Path $targetDir "ffmpeg.exe"
$ffprobePath = Join-Path $targetDir "ffprobe.exe"

$ytDlpBase = "https://github.com/yt-dlp/yt-dlp/releases/latest/download"
$ytDlpUrl = "$ytDlpBase/yt-dlp.exe"
$ytDlpChecksumsUrl = "$ytDlpBase/SHA2-256SUMS"

$ffmpegFileName = "ffmpeg-master-latest-win64-lgpl.zip"
$ffmpegBase = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest"
$ffmpegUrl = "$ffmpegBase/$ffmpegFileName"
$ffmpegChecksumsUrl = "$ffmpegBase/checksums.sha256"

function Test-Executable {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $false
    }

    try {
        & $Path @Arguments *> $null
        return $LASTEXITCODE -eq 0
    }
    catch {
        return $false
    }
}

function Get-ExpectedHash {
    param(
        [Parameter(Mandatory = $true)][string]$ChecksumFile,
        [Parameter(Mandatory = $true)][string]$FileName
    )

    $escapedName = [Regex]::Escape($FileName)
    foreach ($line in Get-Content -LiteralPath $ChecksumFile) {
        if ($line -match "^([0-9A-Fa-f]{64})\s+\*?$escapedName$") {
            return $Matches[1].ToLowerInvariant()
        }
    }

    throw "Checksum entry not found for $FileName"
}

function Assert-FileHash {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$ExpectedHash
    )

    $actual = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $ExpectedHash.ToLowerInvariant()) {
        throw "SHA-256 mismatch for $Path. expected=$ExpectedHash actual=$actual"
    }
    return $actual
}

function Write-Utf8NoBom {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Content
    )

    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $Content, $encoding)
}

New-Item -ItemType Directory -Path $targetDir -Force | Out-Null

if (-not $Force -and
    (Test-Path -LiteralPath $manifestPath -PathType Leaf) -and
    (Test-Executable -Path $ytDlpPath -Arguments @("--version")) -and
    (Test-Executable -Path $ffmpegPath -Arguments @("-version")) -and
    (Test-Executable -Path $ffprobePath -Arguments @("-version"))) {
    Write-Host "Windows download tools are already prepared."
    return
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("chzzk-video-downloader-tools-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null

try {
    Write-Host "Downloading yt-dlp..."
    $ytTemp = Join-Path $tempRoot "yt-dlp.exe"
    $ytChecksums = Join-Path $tempRoot "SHA2-256SUMS"
    Invoke-WebRequest -UseBasicParsing -Uri $ytDlpUrl -OutFile $ytTemp
    Invoke-WebRequest -UseBasicParsing -Uri $ytDlpChecksumsUrl -OutFile $ytChecksums
    $ytExpected = Get-ExpectedHash -ChecksumFile $ytChecksums -FileName "yt-dlp.exe"
    $ytHash = Assert-FileHash -Path $ytTemp -ExpectedHash $ytExpected

    Write-Host "Downloading FFmpeg..."
    $ffmpegArchive = Join-Path $tempRoot $ffmpegFileName
    $ffmpegChecksums = Join-Path $tempRoot "checksums.sha256"
    Invoke-WebRequest -UseBasicParsing -Uri $ffmpegUrl -OutFile $ffmpegArchive
    Invoke-WebRequest -UseBasicParsing -Uri $ffmpegChecksumsUrl -OutFile $ffmpegChecksums
    $ffmpegArchiveExpected = Get-ExpectedHash -ChecksumFile $ffmpegChecksums -FileName $ffmpegFileName
    [void](Assert-FileHash -Path $ffmpegArchive -ExpectedHash $ffmpegArchiveExpected)

    $extractDir = Join-Path $tempRoot "ffmpeg"
    Expand-Archive -LiteralPath $ffmpegArchive -DestinationPath $extractDir -Force

    $ffmpegSource = Get-ChildItem -Path $extractDir -Recurse -Filter "ffmpeg.exe" -File | Select-Object -First 1
    $ffprobeSource = Get-ChildItem -Path $extractDir -Recurse -Filter "ffprobe.exe" -File | Select-Object -First 1
    if ($null -eq $ffmpegSource -or $null -eq $ffprobeSource) {
        throw "FFmpeg archive does not contain ffmpeg.exe and ffprobe.exe"
    }

    Copy-Item -LiteralPath $ytTemp -Destination $ytDlpPath -Force
    Copy-Item -LiteralPath $ffmpegSource.FullName -Destination $ffmpegPath -Force
    Copy-Item -LiteralPath $ffprobeSource.FullName -Destination $ffprobePath -Force

    if (-not (Test-Executable -Path $ytDlpPath -Arguments @("--version"))) {
        throw "Prepared yt-dlp.exe is not executable"
    }
    if (-not (Test-Executable -Path $ffmpegPath -Arguments @("-version"))) {
        throw "Prepared ffmpeg.exe is not executable"
    }
    if (-not (Test-Executable -Path $ffprobePath -Arguments @("-version"))) {
        throw "Prepared ffprobe.exe is not executable"
    }

    $ffmpegHash = (Get-FileHash -LiteralPath $ffmpegPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $ffprobeHash = (Get-FileHash -LiteralPath $ffprobePath -Algorithm SHA256).Hash.ToLowerInvariant()

    $bundleId = "windows-amd64-$($ytHash.Substring(0, 12))-$($ffmpegHash.Substring(0, 12))-$($ffprobeHash.Substring(0, 12))"
    $manifest = [ordered]@{
        bundleId = $bundleId
        platform = "windows/amd64"
        tools = @(
            [ordered]@{
                name = "yt-dlp.exe"
                sha256 = $ytHash
                source = $ytDlpUrl
            },
            [ordered]@{
                name = "ffmpeg.exe"
                sha256 = $ffmpegHash
                source = $ffmpegUrl
            },
            [ordered]@{
                name = "ffprobe.exe"
                sha256 = $ffprobeHash
                source = $ffmpegUrl
            }
        )
    }

    Write-Utf8NoBom -Path $manifestPath -Content ($manifest | ConvertTo-Json -Depth 5)
    Write-Host "Windows download tools prepared: $bundleId"
}
finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
