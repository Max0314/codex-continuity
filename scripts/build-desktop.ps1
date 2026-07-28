param(
    [switch]$SkipInstall,
    [switch]$SkipBundle,
    [ValidateSet('All', 'Standard', 'Offline')]
    [string]$PackageMode = 'All'
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$desktopDir = Join-Path $projectRoot 'desktop'
$sidecarDir = Join-Path $desktopDir 'src-tauri\binaries'
$releaseDir = Join-Path $projectRoot 'release'
$goBin = Join-Path $projectRoot 'tools\go\bin'
$bundleRoot = Join-Path $desktopDir 'src-tauri\target\release\bundle'
$tauriConfigPath = Join-Path $desktopDir 'src-tauri\tauri.conf.json'
$offlineConfigPath = 'src-tauri\tauri.offline.conf.json'
$desktopVersion = (Get-Content -LiteralPath $tauriConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json).version
$standardInstallerName = "codex-continuity_${desktopVersion}_windows-x64-setup.exe"
$offlineInstallerName = "codex-continuity_${desktopVersion}_windows-x64-offline-setup.exe"
$portableName = "codex-continuity_${desktopVersion}_windows-x64-portable.zip"
$manifestName = 'desktop-release.json'
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

function Copy-LatestNSISInstaller {
    param(
        [Parameter(Mandatory = $true)]
        [string]$DestinationName
    )

    $nsisDir = Join-Path $bundleRoot 'nsis'
    $installer = Get-ChildItem -LiteralPath $nsisDir -Filter '*.exe' -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1
    if (-not $installer) {
        throw "Tauri did not produce an NSIS installer in $nsisDir."
    }
    $destination = Join-Path $releaseDir $DestinationName
    Copy-Item -LiteralPath $installer.FullName -Destination $destination -Force
    return Get-Item -LiteralPath $destination
}

function New-ReleaseArtifact {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Id,
        [Parameter(Mandatory = $true)]
        [string]$Label,
        [Parameter(Mandatory = $true)]
        [string]$FileName,
        [Parameter(Mandatory = $true)]
        [string]$WebViewMode,
        [bool]$Recommended = $false,
        [bool]$RequiresInternetIfRuntimeMissing = $false
    )

    $path = Join-Path $releaseDir $FileName
    if (-not (Test-Path -LiteralPath $path)) {
        return $null
    }
    $file = Get-Item -LiteralPath $path
    return [ordered]@{
        id = $Id
        label = $Label
        fileName = $FileName
        sizeBytes = $file.Length
        sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
        webViewMode = $WebViewMode
        recommended = $Recommended
        requiresInternetIfRuntimeMissing = $RequiresInternetIfRuntimeMissing
    }
}

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
    if ($SkipBundle) {
        npm run build
        if ($LASTEXITCODE -ne 0) {
            throw "Desktop frontend build failed with exit code $LASTEXITCODE."
        }
        & $cargoExe build --release --manifest-path 'src-tauri\Cargo.toml'
        if ($LASTEXITCODE -ne 0) {
            throw "Cargo release build failed with exit code $LASTEXITCODE."
        }
    } else {
        if (Test-Path -LiteralPath $bundleRoot) {
            $staleBundle = "$bundleRoot-stale-$(Get-Date -Format 'yyyyMMddHHmmss')"
            Move-Item -LiteralPath $bundleRoot -Destination $staleBundle
        }

        if ($PackageMode -in 'All', 'Standard') {
            npm run tauri build
            if ($LASTEXITCODE -ne 0) {
                throw "Tauri standard installer build failed with exit code $LASTEXITCODE."
            }
            Copy-LatestNSISInstaller -DestinationName $standardInstallerName | Out-Null
        }

        if ($PackageMode -in 'All', 'Offline') {
            npm run tauri build -- --config $offlineConfigPath
            if ($LASTEXITCODE -ne 0) {
                throw "Tauri offline installer build failed with exit code $LASTEXITCODE."
            }
            Copy-LatestNSISInstaller -DestinationName $offlineInstallerName | Out-Null
        }
    }
} finally {
    Pop-Location
}

if ($staleBundle -and (Test-Path -LiteralPath $bundleRoot) -and (Test-Path -LiteralPath $staleBundle)) {
    Remove-Item -LiteralPath $staleBundle -Recurse -Force
}

$desktopExe = Join-Path $desktopDir 'src-tauri\target\release\codex-continuity-desktop.exe'
$sidecarExe = Join-Path $desktopDir 'src-tauri\target\release\continuity-core.exe'
if ((Test-Path -LiteralPath $desktopExe) -and (Test-Path -LiteralPath $sidecarExe)) {
    Compress-Archive -LiteralPath $desktopExe, $sidecarExe -DestinationPath (Join-Path $releaseDir $portableName) -CompressionLevel Optimal -Force
}

$releaseArtifacts = @()
$standardArtifact = New-ReleaseArtifact `
    -Id 'standard' `
    -Label 'Standard installer' `
    -FileName $standardInstallerName `
    -WebViewMode 'official-download-if-missing' `
    -Recommended $true `
    -RequiresInternetIfRuntimeMissing $true
if ($standardArtifact) {
    $releaseArtifacts += $standardArtifact
}
$offlineArtifact = New-ReleaseArtifact `
    -Id 'offline' `
    -Label 'Full offline installer' `
    -FileName $offlineInstallerName `
    -WebViewMode 'bundled-offline' `
    -Recommended $false `
    -RequiresInternetIfRuntimeMissing $false
if ($offlineArtifact) {
    $releaseArtifacts += $offlineArtifact
}
$portableArtifact = New-ReleaseArtifact `
    -Id 'portable' `
    -Label 'Portable package' `
    -FileName $portableName `
    -WebViewMode 'system-required' `
    -Recommended $false `
    -RequiresInternetIfRuntimeMissing $false
if ($portableArtifact) {
    $releaseArtifacts += $portableArtifact
}

$releaseManifest = [ordered]@{
    schemaVersion = 1
    product = 'Codex Continuity'
    version = $desktopVersion
    generatedAt = [DateTime]::UtcNow.ToString('o')
    platform = 'windows-x64'
    artifacts = $releaseArtifacts
}
$releaseManifest |
    ConvertTo-Json -Depth 5 |
    Set-Content -LiteralPath (Join-Path $releaseDir $manifestName) -Encoding UTF8

$checksumFiles = Get-ChildItem -LiteralPath $releaseDir -File |
    Where-Object { $_.Name -notin '.gitkeep', 'SHA256SUMS.txt' -and $_.Extension -ne '.stale' } |
    Sort-Object Name
$checksumLines = $checksumFiles | ForEach-Object {
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
    "$hash  $($_.Name)"
}
Set-Content -Encoding ascii -LiteralPath (Join-Path $releaseDir 'SHA256SUMS.txt') -Value $checksumLines

Write-Host "Desktop client build completed ($PackageMode): $releaseDir"
