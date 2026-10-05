param(
    [switch]$RefreshTools
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$frontendDir = Join-Path $repoRoot "frontend"
$frontendDist = Join-Path $frontendDir "dist"
$runYarnScript = Join-Path $PSScriptRoot "run-yarn.ps1"
$minimumNodeVersion = [Version]"24.0.0"
$minimumGoVersion = [Version]"1.25.0"
$requiredWailsVersion = "2.15.0"

if ($env:OS -ne "Windows_NT") {
    throw "scripts/dev.ps1 currently supports Windows only."
}

function Refresh-ProcessPath {
    $segments = @()
    foreach ($value in @(
        [Environment]::GetEnvironmentVariable("Path", "Machine"),
        [Environment]::GetEnvironmentVariable("Path", "User"),
        $env:Path
    )) {
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            $segments += $value -split ";"
        }
    }

    $env:Path = (($segments |
        Where-Object { -not [string]::IsNullOrWhiteSpace($_) } |
        Select-Object -Unique) -join ";")
}

function Add-ProcessPath {
    param(
        [Parameter(Mandatory = $true)][string]$Path
    )

    if ([string]::IsNullOrWhiteSpace($Path)) {
        return
    }

    $current = @($env:Path -split ";")
    if ($current -notcontains $Path) {
        $env:Path = "$Path;$env:Path"
    }
}

function Get-NodeVersion {
    if ($null -eq (Get-Command node -ErrorAction SilentlyContinue)) {
        return $null
    }

    try {
        $raw = [string](& node --version 2>$null)
        if ($LASTEXITCODE -ne 0 -or $raw -notmatch '^v(?<major>\d+)\.(?<minor>\d+)\.(?<patch>\d+)') {
            return $null
        }

        return [Version]::new(
            [int]$Matches["major"],
            [int]$Matches["minor"],
            [int]$Matches["patch"]
        )
    }
    catch {
        return $null
    }
}

function Get-GoVersion {
    if ($null -eq (Get-Command go -ErrorAction SilentlyContinue)) {
        return $null
    }

    try {
        $raw = [string](& go version 2>$null)
        if ($LASTEXITCODE -ne 0 -or $raw -notmatch '\bgo(?<major>\d+)\.(?<minor>\d+)(?:\.(?<patch>\d+))?') {
            return $null
        }

        $patch = 0
        if (-not [string]::IsNullOrWhiteSpace($Matches["patch"])) {
            $patch = [int]$Matches["patch"]
        }

        return [Version]::new(
            [int]$Matches["major"],
            [int]$Matches["minor"],
            $patch
        )
    }
    catch {
        return $null
    }
}

function Install-WingetPackage {
    param(
        [Parameter(Mandatory = $true)][string]$Id,
        [Parameter(Mandatory = $true)][string]$Name
    )

    $winget = Get-Command winget -ErrorAction SilentlyContinue
    if ($null -eq $winget) {
        throw "$Name 자동 설치에 Windows Package Manager(winget)가 필요합니다. Microsoft App Installer를 설치한 뒤 다시 dev.ps1을 실행해 주세요."
    }

    Write-Host "$Name 설치 또는 업데이트 중..."
    $arguments = @(
        "install",
        "--id", $Id,
        "--exact",
        "--source", "winget",
        "--accept-source-agreements",
        "--accept-package-agreements",
        "--disable-interactivity",
        "--silent",
        "--force"
    )
    & $winget.Source @arguments

    $exitCode = $LASTEXITCODE
    Refresh-ProcessPath

    if ($exitCode -ne 0) {
        Write-Warning "$Name winget 설치 명령이 exit code $exitCode 로 종료되었습니다. 설치 상태를 다시 확인합니다."
    }
}

function Ensure-Node {
    $version = Get-NodeVersion
    if ($null -eq $version -or $version -lt $minimumNodeVersion) {
        Install-WingetPackage -Id "OpenJS.NodeJS.LTS" -Name "Node.js 24+"
        $version = Get-NodeVersion
    }

    if ($null -eq $version -or $version -lt $minimumNodeVersion) {
        $actual = if ($null -eq $version) { "not found" } else { $version.ToString() }
        throw "Node.js 24 이상을 준비하지 못했습니다. current=$actual"
    }

    Write-Host "Node.js ready: v$version"
}

function Ensure-Go {
    $version = Get-GoVersion
    if ($null -eq $version -or $version -lt $minimumGoVersion) {
        Install-WingetPackage -Id "GoLang.Go" -Name "Go 1.25+"
        $version = Get-GoVersion
    }

    if ($null -eq $version -or $version -lt $minimumGoVersion) {
        $actual = if ($null -eq $version) { "not found" } else { $version.ToString() }
        throw "Go 1.25 이상을 준비하지 못했습니다. current=$actual"
    }

    Write-Host "Go ready: $version"
}

function Get-GoBin {
    $goBin = [string](& go env GOBIN)
    if ($LASTEXITCODE -ne 0) {
        throw "go env GOBIN failed with exit code $LASTEXITCODE"
    }

    $goBin = $goBin.Trim()
    if (-not [string]::IsNullOrWhiteSpace($goBin)) {
        return $goBin
    }

    $goPath = [string](& go env GOPATH)
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($goPath)) {
        throw "Unable to resolve Go binary directory."
    }

    $firstGoPath = @($goPath.Trim() -split [IO.Path]::PathSeparator)[0]
    return (Join-Path $firstGoPath "bin")
}

function Test-WailsVersion {
    param(
        [Parameter(Mandatory = $true)][string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $false
    }

    try {
        $output = ((& $Path version 2>$null) | Out-String).Trim()
        return $LASTEXITCODE -eq 0 -and $output -match ("(?<!\d)" + [Regex]::Escape($requiredWailsVersion) + "(?!\d)")
    }
    catch {
        return $false
    }
}

function Ensure-Wails {
    $goBin = Get-GoBin
    New-Item -ItemType Directory -Path $goBin -Force | Out-Null
    Add-ProcessPath -Path $goBin

    $candidate = Join-Path $goBin "wails.exe"
    $wails = Get-Command wails -ErrorAction SilentlyContinue
    $wailsPath = if ($null -ne $wails) { $wails.Source } else { $candidate }

    if (-not (Test-WailsVersion -Path $wailsPath)) {
        Write-Host "Wails v$requiredWailsVersion 설치 중..."
        & go install "github.com/wailsapp/wails/v2/cmd/wails@v$requiredWailsVersion"
        if ($LASTEXITCODE -ne 0) {
            throw "Wails installation failed with exit code $LASTEXITCODE"
        }
        $wailsPath = $candidate
    }

    if (-not (Test-WailsVersion -Path $wailsPath)) {
        throw "Wails v$requiredWailsVersion 를 준비하지 못했습니다."
    }

    Write-Host "Wails ready: v$requiredWailsVersion"
    return $wailsPath
}

function Get-WebView2Version {
    $clientId = "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
    $paths = @(
        "HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients\$clientId",
        "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\$clientId",
        "HKCU:\SOFTWARE\Microsoft\EdgeUpdate\Clients\$clientId"
    )

    foreach ($path in $paths) {
        $item = Get-ItemProperty -LiteralPath $path -ErrorAction SilentlyContinue
        if ($null -ne $item -and -not [string]::IsNullOrWhiteSpace([string]$item.pv)) {
            return [string]$item.pv
        }
    }

    return $null
}

function Ensure-WebView2 {
    $version = Get-WebView2Version
    if ([string]::IsNullOrWhiteSpace($version)) {
        Install-WingetPackage -Id "Microsoft.EdgeWebView2Runtime" -Name "Microsoft Edge WebView2 Runtime"
        $version = Get-WebView2Version
    }

    if ([string]::IsNullOrWhiteSpace($version)) {
        Write-Warning "WebView2 Runtime 설치를 확인하지 못했습니다. Wails 실행 시 Runtime 설치 안내가 추가로 표시될 수 있습니다."
        return
    }

    Write-Host "WebView2 Runtime ready: $version"
}

Ensure-Node
Ensure-Go
$wailsPath = Ensure-Wails
Ensure-WebView2

if (-not (Test-Path -LiteralPath $runYarnScript -PathType Leaf)) {
    throw "Missing Yarn runner: $runYarnScript"
}

Push-Location $frontendDir
try {
    $yarnVersion = [string]((& $runYarnScript --version | Select-Object -Last 1))
    $yarnVersion = $yarnVersion.Trim()
    if ($yarnVersion -ne "4.9.2") {
        throw "Expected Yarn 4.9.2 from frontend/package.json, got '$yarnVersion'."
    }
    Write-Host "Yarn ready: $yarnVersion"

    Write-Host "Installing frontend dependencies..."
    & $runYarnScript install --immutable
}
finally {
    Pop-Location
}

$distFiles = @()
if (Test-Path -LiteralPath $frontendDist -PathType Container) {
    $distFiles = @(Get-ChildItem -LiteralPath $frontendDist -Recurse -File -Force -ErrorAction SilentlyContinue)
}
if ($distFiles.Count -eq 0) {
    Write-Host "frontend/dist is empty. Building frontend assets required by Go embed..."
    Push-Location $frontendDir
    try {
        & $runYarnScript build
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

$appIconSource = Join-Path $repoRoot "assets\appicon.png"
if (-not (Test-Path -LiteralPath $appIconSource -PathType Leaf)) {
    throw "Missing application icon source: $appIconSource"
}
$buildDir = Join-Path $repoRoot "build"
$windowsAssetsDir = Join-Path $buildDir "windows"
New-Item -ItemType Directory -Path $windowsAssetsDir -Force | Out-Null
Copy-Item -LiteralPath $appIconSource -Destination (Join-Path $buildDir "appicon.png") -Force
$generatedWindowsIcon = Join-Path $windowsAssetsDir "icon.ico"
if (Test-Path -LiteralPath $generatedWindowsIcon -PathType Leaf) {
    Remove-Item -LiteralPath $generatedWindowsIcon -Force
}

& (Join-Path $PSScriptRoot "prepare-windows-tools.ps1") -Force:$RefreshTools

Push-Location $repoRoot
try {
    & $wailsPath dev
    if ($LASTEXITCODE -ne 0) {
        throw "Wails dev failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
