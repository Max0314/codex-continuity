param(
    [switch]$SkipWeb,
    [switch]$SkipTests,
    [switch]$SkipInstall
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$releaseDir = Join-Path $projectRoot 'release'
$goBin = Join-Path $projectRoot 'tools\go\bin'

if (Test-Path -LiteralPath $goBin) {
    $env:Path = "$goBin;$env:Path"
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'Go was not found. Install the version declared by go.mod or place it in tools/go.'
}
if (-not (Get-Command npm -ErrorAction SilentlyContinue)) {
    throw 'Node.js/npm was not found.'
}

New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null

if (-not $SkipWeb) {
    Push-Location (Join-Path $projectRoot 'web')
    try {
        if (-not $SkipInstall) {
            if (Test-Path 'package-lock.json') {
                npm ci
            } else {
                npm install
            }
            if ($LASTEXITCODE -ne 0) {
                throw "Dependency installation failed with exit code $LASTEXITCODE."
            }
        }
        npm run build
        if ($LASTEXITCODE -ne 0) {
            throw "Web build failed with exit code $LASTEXITCODE."
        }
    } finally {
        Pop-Location
    }
}

if (-not $SkipTests) {
    Push-Location $projectRoot
    try {
        go test ./cmd/... ./internal/...
        if ($LASTEXITCODE -ne 0) {
            throw "Go tests failed with exit code $LASTEXITCODE."
        }
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
    if ($LASTEXITCODE -ne 0) {
        throw "Windows client build failed with exit code $LASTEXITCODE."
    }

    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    go build -trimpath -ldflags '-s -w' -o (Join-Path $releaseDir 'continuity-server-linux-amd64') ./cmd/continuity-server
    if ($LASTEXITCODE -ne 0) {
        throw "Linux server build failed with exit code $LASTEXITCODE."
    }
} finally {
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
    Pop-Location
}

$artifacts = Get-ChildItem -LiteralPath $releaseDir -File | Where-Object {
    $_.Name -ne '.gitkeep' -and $_.Name -ne 'SHA256SUMS.txt' -and $_.Extension -ne '.stale'
} | Sort-Object Name
$checksums = $artifacts | ForEach-Object {
    $hash = Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256
    "$($hash.Hash.ToLower())  $($_.Name)"
}
Set-Content -LiteralPath (Join-Path $releaseDir 'SHA256SUMS.txt') -Value $checksums -Encoding Ascii

Write-Host "构建完成：$releaseDir"
