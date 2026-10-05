param(
    [Parameter(ValueFromRemainingArguments = $true, Position = 0)]
    [string[]]$YarnArgs
)

$ErrorActionPreference = "Stop"

$corepack = Get-Command corepack -ErrorAction SilentlyContinue
if ($null -ne $corepack) {
    & $corepack.Source yarn @YarnArgs
    if ($LASTEXITCODE -ne 0) {
        throw "Yarn command failed through Corepack with exit code $LASTEXITCODE"
    }
    return
}

$npx = Get-Command npx -ErrorAction SilentlyContinue
if ($null -eq $npx) {
    throw "Corepack and npx are unavailable. Node.js installation is required."
}

& $npx.Source --yes corepack yarn @YarnArgs
if ($LASTEXITCODE -ne 0) {
    throw "Yarn command failed through npx Corepack with exit code $LASTEXITCODE"
}
