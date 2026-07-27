param(
    [string]$ImageName = 'codex-continuity:local'
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$releaseDir = Join-Path $projectRoot 'release'
$archivePath = Join-Path $releaseDir 'codex-continuity-image-linux-amd64.tar'
$gzipPath = "$archivePath.gz"

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw '未找到 Docker。请在有 Docker 的构建机执行，或从 GitHub Release 下载离线镜像包。'
}

New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null

Push-Location $projectRoot
try {
    docker build --platform linux/amd64 -t $ImageName .
    if ($LASTEXITCODE -ne 0) {
        throw 'Docker 镜像构建失败。'
    }
    docker save --output $archivePath $ImageName
    if ($LASTEXITCODE -ne 0) {
        throw 'Docker 镜像导出失败。'
    }
    $inputStream = [System.IO.File]::OpenRead($archivePath)
    $outputStream = [System.IO.File]::Create($gzipPath)
    $gzipStream = New-Object System.IO.Compression.GZipStream(
        $outputStream,
        [System.IO.Compression.CompressionMode]::Compress
    )
    try {
        $inputStream.CopyTo($gzipStream)
    } finally {
        $gzipStream.Dispose()
        $outputStream.Dispose()
        $inputStream.Dispose()
    }
    Remove-Item -LiteralPath $archivePath -Force
} finally {
    Pop-Location
}

Write-Host "离线镜像包已生成：$gzipPath"
