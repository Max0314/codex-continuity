param(
    [switch]$SkipInstall,
    [switch]$SkipBundle
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$desktopDir = Join-Path $projectRoot 'desktop'
$sidecarDir = Join-Path $desktopDir 'src-tauri\binaries'
$releaseDir = Join-Path $projectRoot 'release'
$goBin = Join-Path $projectRoot 'tools\go\bin'
$bundleRoot = Join-Path $desktopDir 'src-tauri\target\release\bundle'
$tauriConfigPath = Join-Path $desktopDir 'src-tauri\tauri.conf.json'
$desktopVersion = (Get-Content -LiteralPath $tauriConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json).version
$staleBundle = $null
$cargoCommand = Get-Command cargo -ErrorAction SilentlyContinue
$cargoExe = if ($cargoCommand) {
    $cargoCommand.Source
} else {
    Join-Path $env:USERPROFILE '.cargo\bin\cargo.exe'
}

if (Test-Path -LiteralPath $goBin) {
    $env:Path = "$goBin;$env:Path"
}
if (-not (Test-Path -LiteralPath $cargoExe)) {
    throw 'Rust Cargo was not found. Install the stable MSVC Rust toolchain first.'
}
New-Item -ItemType Directory -Force -Path $sidecarDir, $releaseDir | Out-Null

$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -trimpath -ldflags '-s -w' -o (Join-Path $sidecarDir 'continuity-core-x86_64-pc-windows-msvc.exe') ./cmd/continuity-client
Remove-Item Env:\GOOS, Env:\GOARCH, Env:\CGO_ENABLED -ErrorAction SilentlyContinue

$env:Path = "$(Split-Path -Parent $cargoExe);$env:Path"

Push-Location $desktopDir
try {
    if (-not $SkipInstall) {
        npm ci
    }
    npm run build
    if ($SkipBundle) {
        & $cargoExe build --release --manifest-path 'src-tauri\Cargo.toml'
        if ($LASTEXITCODE -ne 0) {
            throw "Cargo release build failed with exit code $LASTEXITCODE."
        }
    } else {
        if (Test-Path -LiteralPath $bundleRoot) {
            $staleBundle = "$bundleRoot-stale-$(Get-Date -Format 'yyyyMMddHHmmss')"
            Move-Item -LiteralPath $bundleRoot -Destination $staleBundle
        }
        npm run tauri build
        if ($LASTEXITCODE -ne 0) {
            throw "Tauri bundle build failed with exit code $LASTEXITCODE."
        }
    }
} finally {
    Pop-Location
}

if ($staleBundle -and (Test-Path -LiteralPath $bundleRoot) -and (Test-Path -LiteralPath $staleBundle)) {
    Remove-Item -LiteralPath $staleBundle -Recurse -Force
}

if (Test-Path -LiteralPath $bundleRoot) {
    $nsisInstaller = Get-ChildItem -LiteralPath (Join-Path $bundleRoot 'nsis') -Filter '*.exe' -File -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($nsisInstaller) {
        Copy-Item -LiteralPath $nsisInstaller.FullName -Destination (Join-Path $releaseDir "codex-continuity_${desktopVersion}_x64-setup.exe") -Force
    }
    $msiInstaller = Get-ChildItem -LiteralPath (Join-Path $bundleRoot 'msi') -Filter '*.msi' -File -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($msiInstaller) {
        Copy-Item -LiteralPath $msiInstaller.FullName -Destination (Join-Path $releaseDir "codex-continuity_${desktopVersion}_x64_zh-CN.msi") -Force
    }
}
$desktopExe = Join-Path $desktopDir 'src-tauri\target\release\codex-continuity-desktop.exe'
$sidecarExe = Join-Path $desktopDir 'src-tauri\target\release\continuity-core.exe'
if ((Test-Path -LiteralPath $desktopExe) -and (Test-Path -LiteralPath $sidecarExe)) {
    Compress-Archive -LiteralPath $desktopExe, $sidecarExe -DestinationPath (Join-Path $releaseDir "codex-continuity_${desktopVersion}_windows-x64-portable.zip") -CompressionLevel Optimal -Force
}

$checksumFiles = Get-ChildItem -LiteralPath $releaseDir -File |
    Where-Object { $_.Name -notin '.gitkeep', 'SHA256SUMS.txt' -and $_.Extension -ne '.stale' } |
    Sort-Object Name
$checksumLines = $checksumFiles | ForEach-Object {
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
    "$hash  $($_.Name)"
}
Set-Content -Encoding ascii -LiteralPath (Join-Path $releaseDir 'SHA256SUMS.txt') -Value $checksumLines

Write-Host "Desktop client build completed: $releaseDir"
