param(
    [int]$Port = 8765,
    [switch]$Background
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptDir

# Check if port is already running
$existing = Get-NetTCPConnection -LocalPort $Port -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "[*] 服务已在端口 $Port 运行中，正在打开浏览器..." -ForegroundColor Green
    Start-Process "http://localhost:$Port"
    exit
}

$exePath = Join-Path $scriptDir "monitor.exe"
if (-not (Test-Path $exePath)) {
    Write-Host "[*] 正在编译 monitor.exe..." -ForegroundColor Cyan
    $env:Path = "C:\Users\81301\go_sdk\go\bin;$env:Path"
    go build -o monitor.exe .
}

if ($Background) {
    Write-Host "[*] 正在后台启动 AI Agent Token 监控服务 (端口: $Port)..." -ForegroundColor Cyan
    Start-Process -FilePath $exePath -ArgumentList "-port $Port" -WorkingDirectory $scriptDir -WindowStyle Hidden
    Start-Sleep -Seconds 1
    Start-Process "http://localhost:$Port"
    Write-Host "[+] 服务已在后台成功运行！访问地址: http://localhost:$Port" -ForegroundColor Green
} else {
    Write-Host "[*] 正在启动 AI Agent Token 监控服务 (Golang 版)..." -ForegroundColor Cyan
    Start-Process "http://localhost:$Port"
    & $exePath -port $Port
}
