param(
    [string]$AdminEmail = 'admin@example.com',
    [string]$AdminPassword = 'change-me-now',
    [int]$Port = 8080
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$env:CONTINUITY_ADDR = ":$Port"
$env:CONTINUITY_DATA_DIR = Join-Path $projectRoot 'data'
$env:CONTINUITY_WEB_DIR = Join-Path $projectRoot 'web\dist'
$env:CONTINUITY_DOWNLOAD_DIR = Join-Path $projectRoot 'release'
$env:CONTINUITY_ADMIN_EMAIL = $AdminEmail
$env:CONTINUITY_ADMIN_PASSWORD = $AdminPassword

Push-Location $projectRoot
try {
    go run ./cmd/continuity-server
} finally {
    Pop-Location
}
