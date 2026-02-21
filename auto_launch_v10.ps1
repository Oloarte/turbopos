Write-Host "🚀 INICIANDO SECUENCIA DE DESPEGUE TURBOPOS v10.1..." -ForegroundColor Cyan

function Start-TurboService {
    param([string]$name, [string]$path)
    if (Test-Path $path) {
        Write-Host ">> Activando $name..." -ForegroundColor Yellow
        Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd $path; Write-Host '--- $name ---'; go run main.go"
    } else {
        Write-Host "❌ ERROR: No se encontro la ruta $path" -ForegroundColor Red
    }
}

Start-TurboService -name "CFDI (50051)" -path "C:\dev\turbopos\services\cfdi\cmd"
Start-TurboService -name "AUTH (50052)" -path "C:\dev\turbopos\services\auth\cmd"
Start-TurboService -name "BFF (8080)" -path "C:\dev\turbopos\services\bff\cmd"
Start-TurboService -name "AUDIT-VAULT" -path "C:\dev\turbopos\services\audit\cmd"

Write-Host "`n✅ LANZAMIENTO COMPLETADO. Mira tu monitor status_check.go" -ForegroundColor Green
