param(
    [int]$Port = 8765
)

$connections = Get-NetTCPConnection -LocalPort $Port -ErrorAction SilentlyContinue
if ($connections) {
    foreach ($conn in $connections) {
        $procId = $conn.OwningProcess
        if ($procId -gt 0) {
            $proc = Get-Process -Id $procId -ErrorAction SilentlyContinue
            if ($proc) {
                Write-Host "[*] 正在终止进程: $($proc.ProcessName) (PID: $procId)..." -ForegroundColor Yellow
                Stop-Process -Id $procId -Force
            }
        }
    }
    Write-Host "[+] 监控服务已停止。" -ForegroundColor Green
} else {
    Write-Host "[i] 未发现端口 $Port 正在运行的监控服务。" -ForegroundColor Gray
}
