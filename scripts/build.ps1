param(
    [switch]$SkipWeb,
    [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$releaseDir = Join-Path $projectRoot 'release'

New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null

if (-not $SkipWeb) {
    Push-Location (Join-Path $projectRoot 'web')
    try {
        if (Test-Path 'package-lock.json') {
            npm ci
        } else {
            npm install
        }
        npm run build
    } finally {
        Pop-Location
    }
}

if (-not $SkipTests) {
    Push-Location $projectRoot
    try {
        go test ./cmd/... ./internal/...
    } finally {
        Pop-Location
    }
}

Push-Location $projectRoot
try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    go build -trimpath -ldflags '-s -w' -o (Join-Path $releaseDir 'continuity-windows-amd64.exe') ./cmd/continuity-client

    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    go build -trimpath -ldflags '-s -w' -o (Join-Path $releaseDir 'continuity-server-linux-amd64') ./cmd/continuity-server
} finally {
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
    Pop-Location
}

$artifacts = Get-ChildItem -LiteralPath $releaseDir -File | Where-Object {
    $_.Name -ne '.gitkeep' -and $_.Name -ne 'SHA256SUMS.txt'
}
$checksums = $artifacts | ForEach-Object {
    $hash = Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256
    "$($hash.Hash.ToLower())  $($_.Name)"
}
Set-Content -LiteralPath (Join-Path $releaseDir 'SHA256SUMS.txt') -Value $checksums -Encoding Ascii

Write-Host "构建完成：$releaseDir"
