[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$ReleaseDir
)

$ErrorActionPreference = "Stop"
$ReleaseDir = (Resolve-Path -LiteralPath $ReleaseDir).Path
$metadata = Get-Content -LiteralPath (Join-Path $ReleaseDir "release-metadata.json") -Raw | ConvertFrom-Json
$expectedArtifact = "CHZZK-Video-Downloader-v$($metadata.version)-portable.exe"
if ($metadata.version -notmatch '^\d+\.\d+\.\d+$' -or
    $metadata.artifact -cne $expectedArtifact -or
    $metadata.platform -ne "windows/amd64" -or
    $metadata.distribution -ne "portable" -or
    $metadata.projectLicense -ne "MIT" -or
    $metadata.projectLicenseFile -ne "LICENSE" -or
    $metadata.thirdPartyNotices -ne "THIRD_PARTY_NOTICES.md") {
    throw "Invalid Portable release metadata."
}
$exePath = Join-Path $ReleaseDir $expectedArtifact
$executables = @(Get-ChildItem -LiteralPath $ReleaseDir -Recurse -File -Filter "*.exe")
if ($executables.Count -ne 1 -or $executables[0].FullName -ne $exePath) {
    throw "A Portable release must contain exactly one application EXE."
}
$stream = [IO.File]::OpenRead($exePath)
$reader = New-Object IO.BinaryReader($stream)
try {
    if ($stream.Length -lt 64 -or $reader.ReadUInt16() -ne 0x5a4d) {
        throw "The Portable artifact is not a Windows executable."
    }
    $stream.Position = 60
    $peOffset = $reader.ReadUInt32()
    if ($peOffset -lt 64 -or $peOffset -gt $stream.Length - 6) {
        throw "The Portable artifact has an invalid PE header."
    }
    $stream.Position = $peOffset
    if ($reader.ReadUInt32() -ne 0x00004550 -or $reader.ReadUInt16() -ne 0x8664) {
        throw "The Portable artifact must be a Windows amd64 PE executable."
    }
}
finally {
    $reader.Dispose()
}
$hash = (Get-FileHash -LiteralPath $exePath -Algorithm SHA256).Hash.ToLowerInvariant()
$checksum = (Get-Content -LiteralPath "$exePath.sha256" -Raw).Trim()
if ($metadata.sha256 -cne $hash -or $checksum -cne "$hash  $expectedArtifact") {
    throw "Portable EXE checksum verification failed."
}
if (-not (Test-Path -LiteralPath (Join-Path $ReleaseDir "LICENSE") -PathType Leaf)) {
    throw "Project license is missing."
}
if (-not (Test-Path -LiteralPath (Join-Path $ReleaseDir "THIRD_PARTY_NOTICES.md") -PathType Leaf)) {
    throw "Third-party notices are missing."
}
Write-Host "Portable release verified: $expectedArtifact"
