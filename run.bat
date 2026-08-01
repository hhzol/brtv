@echo off
setlocal enabledelayedexpansion

echo 🔨 Building brtv image...
docker build -t brtv:latest .

if errorlevel 1 (
    echo ❌ Build failed
    exit /b 1
)

echo ✅ Build complete
echo.
echo 🧹 Cleaning up old container if exists...
docker rm -f brtv >nul 2>&1

echo 🚀 Starting brtv container...
docker run -d --name brtv -p 0.0.0.0:8080:8080 brtv:latest

echo ⏳ Waiting for server to start...
timeout /t 2 /nobreak

echo 📋 Container logs:
docker logs brtv

echo.
echo ✨ Server is running on http://localhost:8080
echo.
echo 📝 Test commands:
echo   curl "http://localhost:8080/?id=bjws"
echo   curl "http://localhost:8080/?id=bjjs"
echo.
echo 🛑 Stop container: docker stop brtv
echo ▶️  Start container: docker start brtv
echo 🗑️  Remove container: docker rm -f brtv
