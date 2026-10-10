@echo off
chcp 65001 >nul
title Awesome Token Monitor (Go Edition)

echo ===================================================
echo     启动 全平台 AI Agent Token 监控面板 (Golang 版)
echo ===================================================
echo.

cd /d "%~dp0"

echo [*] 正在打开浏览器 (http://localhost:8765)...
start "" http://localhost:8765

echo [*] 运行核心监控程序 monitor.exe...
monitor.exe -port 8765

pause
