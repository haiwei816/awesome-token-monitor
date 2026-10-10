@echo off
chcp 65001 >nul
echo [*] 正在停止 Antigravity Token 监控服务 (端口 8765)...
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0stop_monitor.ps1"
pause
