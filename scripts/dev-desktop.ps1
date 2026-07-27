param(
    [switch]$SkipInstall
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$desktopDir = Join-Path $projectRoot 'desktop'
$sidecarDir = Join-Path $desktopDir 'src-tauri\binaries'
$goBin = Join-Path $projectRoot 'tools\go\bin'
$rustupHome = Join-Path $env:USERPROFILE '.rustup'
$cargoHome = Join-Path $env:USERPROFILE '.cargo'
$rustTemp = Join-Path $projectRoot 'tools\tmp-rust'

if (Test-Path -LiteralPath $goBin) {
    $env:Path = "$goBin;$env:Path"
}
New-Item -ItemType Directory -Force -Path $sidecarDir, $rustTemp | Out-Null

$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -trimpath -ldflags '-s -w' -o (Join-Path $sidecarDir 'continuity-core-x86_64-pc-windows-msvc.exe') ./cmd/continuity-client
Remove-Item Env:\GOOS, Env:\GOARCH, Env:\CGO_ENABLED -ErrorAction SilentlyContinue

$vswhere = 'C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe'
if (-not (Test-Path -LiteralPath $vswhere)) {
    throw 'Visual Studio Build Tools were not found. Install Desktop development with C++.'
}
$vsInstall = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if (-not $vsInstall) {
    throw 'MSVC x64 tools were not found. Install Desktop development with C++.'
}
Import-Module (Join-Path $vsInstall 'Common7\Tools\Microsoft.VisualStudio.DevShell.dll')
Enter-VsDevShell -VsInstallPath $vsInstall -SkipAutomaticLocation -DevCmdArguments '-arch=x64'
$env:RUSTUP_HOME = $rustupHome
$env:CARGO_HOME = $cargoHome
$env:Path = "$(Join-Path $cargoHome 'bin');$env:Path"
$env:TEMP = $rustTemp
$env:TMP = $rustTemp

Push-Location $desktopDir
try {
    if (-not $SkipInstall) {
        npm install
    }
    npm run tauri dev
} finally {
    Pop-Location
}
