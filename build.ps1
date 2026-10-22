[CmdletBinding()]
param(
    [ValidateSet('win', 'linux', 'linux-arm64', 'both', 'all')]
    [string]$Target = '',
    [string]$Version = '',
    [switch]$Upx
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

function Write-Step([string]$Message) {
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Require-Command([string]$Name, [string]$Hint) {
    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $command) {
        throw "未找到 $Name。$Hint"
    }
    return $command
}

if ([string]::IsNullOrWhiteSpace($Version)) {
    $versionFile = Join-Path $Root 'VERSION'
    if (Test-Path $versionFile) {
        $Version = (Get-Content $versionFile -Raw).Trim()
    }
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = '1.0.0'
}
$Version = $Version.Trim().TrimStart('v')

Write-Host "SmartDoc Vision 单文件构建" -ForegroundColor Green
Write-Host "项目目录：$Root"
Write-Host "版本：$Version"

if ([string]::IsNullOrWhiteSpace($Target)) {
    Write-Host ""
    Write-Host "请选择编译目标："
    Write-Host "  1. Windows  amd64"
    Write-Host "  2. Linux    amd64"
    Write-Host "  3. Linux    arm64"
    Write-Host "  4. 全部：Windows amd64 + Linux amd64 + Linux arm64"
    do {
        $choice = Read-Host "输入 1 / 2 / 3 / 4"
        $choice = $choice.Trim()
        if ($choice -notin @('1', '2', '3', '4')) {
            Write-Host "请输入 1、2、3 或 4。" -ForegroundColor Yellow
        }
    } while ($choice -notin @('1', '2', '3', '4'))
    $Target = @{ '1' = 'win'; '2' = 'linux'; '3' = 'linux-arm64'; '4' = 'all' }[$choice]
}

$go = Require-Command 'go' '请先安装 Go 并加入 PATH。'
$node = Require-Command 'node' '请先安装 Node.js 并加入 PATH。'
$npmCommand = Get-Command 'npm.cmd' -ErrorAction SilentlyContinue
if (-not $npmCommand) {
    $npmCommand = Require-Command 'npm' '请先安装 npm 并加入 PATH。'
}

$targets = switch ($Target) {
    'win' { @(@{ Name = 'Windows amd64'; GOOS = 'windows'; GOARCH = 'amd64'; File = "smartdoc-vision-$Version-windows-amd64.exe" }) }
    'linux' { @(@{ Name = 'Linux amd64'; GOOS = 'linux'; GOARCH = 'amd64'; File = "smartdoc-vision-$Version-linux-amd64" }) }
    'linux-arm64' { @(@{ Name = 'Linux arm64'; GOOS = 'linux'; GOARCH = 'arm64'; File = "smartdoc-vision-$Version-linux-arm64" }) }
    'both' {
        @(
            @{ Name = 'Windows amd64'; GOOS = 'windows'; GOARCH = 'amd64'; File = "smartdoc-vision-$Version-windows-amd64.exe" },
            @{ Name = 'Linux amd64'; GOOS = 'linux'; GOARCH = 'amd64'; File = "smartdoc-vision-$Version-linux-amd64" }
        )
    }
    'all' {
        @(
            @{ Name = 'Windows amd64'; GOOS = 'windows'; GOARCH = 'amd64'; File = "smartdoc-vision-$Version-windows-amd64.exe" },
            @{ Name = 'Linux amd64'; GOOS = 'linux'; GOARCH = 'amd64'; File = "smartdoc-vision-$Version-linux-amd64" },
            @{ Name = 'Linux arm64'; GOOS = 'linux'; GOARCH = 'arm64'; File = "smartdoc-vision-$Version-linux-arm64" }
        )
    }
}

Write-Step "构建前端生产资源"
Push-Location (Join-Path $Root 'web')
try {
    if (-not (Test-Path 'node_modules')) {
        if (Test-Path 'package-lock.json') {
            & $npmCommand.Source ci
        } else {
            & $npmCommand.Source install
        }
        if ($LASTEXITCODE -ne 0) {
            throw "前端依赖安装失败，退出码 $LASTEXITCODE"
        }
    }
    & $npmCommand.Source run build
    if ($LASTEXITCODE -ne 0) {
        throw "前端构建失败，退出码 $LASTEXITCODE"
    }
    if (-not (Test-Path 'dist/index.html')) {
        throw "前端构建完成但没有找到 web/dist/index.html"
    }
} finally {
    Pop-Location
}

Write-Step "复制前端资源到 Go embed 目录"
$embedDir = Join-Path $Root 'backend/internal/webui/dist'
if (Test-Path $embedDir) {
    Get-ChildItem -Path $embedDir -Force | Remove-Item -Recurse -Force
} else {
    New-Item -ItemType Directory -Path $embedDir -Force | Out-Null
}
Copy-Item -Path (Join-Path $Root 'web/dist/*') -Destination $embedDir -Recurse -Force
New-Item -ItemType File -Path (Join-Path $embedDir '.gitkeep') -Force | Out-Null

$outputDir = Join-Path $Root 'dist'
New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
$outputs = @()

Write-Step "编译 Go 单文件程序"
Push-Location (Join-Path $Root 'backend')
$oldGOOS = $env:GOOS
$oldGOARCH = $env:GOARCH
$oldCGO = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    foreach ($item in $targets) {
        $env:GOOS = $item.GOOS
        $env:GOARCH = $item.GOARCH
        $output = Join-Path $outputDir $item.File
        Write-Host "  -> $($item.Name) : $output"
        & $go.Source build -trimpath -ldflags "-s -w -X smartdoc-vision/backend/internal/version.Version=$Version" -o $output ./cmd/server
        if ($LASTEXITCODE -ne 0) {
            throw "$($item.Name) 编译失败，退出码 $LASTEXITCODE"
        }
        if (-not (Test-Path $output)) {
            throw "$($item.Name) 编译完成但没有生成 $output"
        }
        $outputs += $output
    }
} finally {
    Pop-Location
    $env:GOOS = $oldGOOS
    $env:GOARCH = $oldGOARCH
    $env:CGO_ENABLED = $oldCGO
}

$envTemplate = Join-Path $Root '.env.example'
if (Test-Path $envTemplate) {
    Copy-Item -Path $envTemplate -Destination (Join-Path $outputDir '.env.example') -Force
    $targetEnv = Join-Path $outputDir '.env'
    if (-not (Test-Path $targetEnv)) {
        Copy-Item -Path $envTemplate -Destination $targetEnv -Force
        Write-Host "  已生成 dist/.env 配置模板（可直接编辑）" -ForegroundColor Yellow
    }
}

# UPX variants are only produced for the two x64 targets; arm64 keeps the
# original binary only. The uncompressed builds always remain in dist/.
$upxTargets = @($targets | Where-Object { $_.GOARCH -eq 'amd64' -and $_.GOOS -in @('windows', 'linux') })
$upxCommand = Get-Command 'upx' -ErrorAction SilentlyContinue
$compress = $false
if ($Upx) {
    if (-not $upxCommand) {
        throw "未找到 upx。请安装 UPX 并加入 PATH，或去掉 -Upx。"
    }
    $compress = $true
} elseif ($upxCommand -and $upxTargets.Count -gt 0) {
    Write-Host ""
    $answer = Read-Host "检测到 UPX（$($upxCommand.Source)），是否额外生成 Windows/Linux amd64 的 UPX 压缩版本？[y/N]"
    if (-not [string]::IsNullOrWhiteSpace($answer) -and $answer.Trim().ToLowerInvariant() -in @('y', 'yes')) {
        $compress = $true
    }
}

if ($compress) {
    Write-Step "生成 UPX 压缩版本（Windows amd64 / Linux amd64）"
    foreach ($item in $upxTargets) {
        $source = Join-Path $outputDir $item.File
        if ($item.GOOS -eq 'windows') {
            $compressed = Join-Path $outputDir ($item.File -replace '\.exe$', '-upx.exe')
        } else {
            $compressed = Join-Path $outputDir "$($item.File)-upx"
        }
        Copy-Item -Path $source -Destination $compressed -Force
        Write-Host "  -> $compressed"
        & $upxCommand.Source --best --lzma $compressed
        if ($LASTEXITCODE -ne 0) {
            Remove-Item -Path $compressed -Force -ErrorAction SilentlyContinue
            throw "UPX 压缩失败：$compressed"
        }
        $outputs += $compressed
    }
} elseif ($Upx) {
    Write-Host "当前目标不包含 Windows/Linux amd64，跳过 UPX 压缩。" -ForegroundColor Yellow
} else {
    Write-Host ""
    Write-Host "未生成 UPX 版本。安装 UPX 后可用 -Upx 非交互生成，或不压缩直接分发原产物。" -ForegroundColor Yellow
}

Write-Step "产物清单"
foreach ($file in $outputs) {
    $size = [math]::Round((Get-Item $file).Length / 1MB, 2)
    Write-Host ("  {0}  ({1} MB)" -f $file, $size) -ForegroundColor Green
}

Write-Host ""
Write-Host "构建完成。单个可执行文件已包含前端页面；运行后访问 http://127.0.0.1:8080/admin" -ForegroundColor Green
Write-Host "注意：数据库、Redis、JWT/密钥等配置仍通过 .env 或环境变量提供。" -ForegroundColor Yellow
