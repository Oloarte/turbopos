# TurboPOS v10.1 - Script de arranque completo
# Ejecutar: PowerShell -ExecutionPolicy Bypass -File .\ARRANCAR_TURBOPOS.ps1
# Desde: C:\dev\turbopos

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  TurboPOS v10.1 - Iniciando stack..." -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

# 0. Matar procesos anteriores
Write-Host ""
Write-Host "[0] Limpiando puertos anteriores..." -ForegroundColor Yellow
$puertos = @("50051","50052","50053","50054","50055","8080")
foreach ($p in $puertos) {
    $pidVal = (netstat -ano | findstr ":$p" | findstr "LISTENING") -split '\s+' | Where-Object { $_ -match '^\d+$' } | Select-Object -Last 1
    if ($pidVal) {
        taskkill /PID $pidVal /F 2>$null | Out-Null
        Write-Host "  Puerto :$p liberado (PID $pidVal)" -ForegroundColor Gray
    }
}
Start-Sleep 1

# 1. Auth Service :50051
Write-Host ""
Write-Host "[1] Iniciando Auth Service (:50051)..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit","-Command","cd C:\dev\turbopos; `$env:DB_PASS='turbopos'; go run services/auth/cmd/server/main.go"
Start-Sleep 3

# 2. Sales Service :50052
Write-Host "[2] Iniciando Sales Service (:50052)..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit","-Command","cd C:\dev\turbopos; go run services/sales/cmd/server/main.go"
Start-Sleep 3

# 3. Loyalty Service :50054
Write-Host "[3] Iniciando Loyalty Service (:50054)..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit","-Command","cd C:\dev\turbopos; go run services/loyalty/cmd/main.go"
Start-Sleep 3

# 4. CFDI Service :50053 + :50055
Write-Host "[4] Iniciando CFDI Service (:50053 gRPC + :50055 HTTP)..." -ForegroundColor Yellow
$finkokUser = Read-Host "  Finkok usuario (Enter = danieloloarte@hotmail.com)"
$finkokPass = Read-Host "  Finkok password (Enter = Spaceboy1.)"
if ($finkokUser -eq "") { $finkokUser = "danieloloarte@hotmail.com" }
if ($finkokPass -eq "") { $finkokPass = "Spaceboy1." }

Start-Process powershell -ArgumentList "-NoExit","-Command","cd C:\dev\turbopos; `$env:FINKOK_USER='$finkokUser'; `$env:FINKOK_PASS='$finkokPass'; go run services/cfdi/cmd/main.go"

Write-Host "  Esperando CFDI (puede tardar 30-60s)..." -ForegroundColor Gray
$i = 0
$cfdiListo = $false
do {
    Start-Sleep 3
    $i++
    Write-Host "  [$($i*3)s] Compilando..." -ForegroundColor Gray
    $netResult = netstat -ano | findstr ":50053" | findstr "LISTENING"
    if ($netResult) {
        $cfdiListo = $true
    }
} while ((-not $cfdiListo) -and ($i -lt 25))

if ($cfdiListo) {
    Write-Host "  OK - CFDI listo en $($i*3)s" -ForegroundColor Green
} else {
    Write-Host "  ADVERTENCIA - CFDI tardo demasiado, revisa la ventana" -ForegroundColor Red
}

# 5. BFF Gateway :8080
Write-Host ""
Write-Host "[5] Iniciando BFF Gateway (:8080)..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit","-Command","cd C:\dev\turbopos; go run services/bff/cmd/main.go"
Start-Sleep 6

# 6. Verificacion final
Write-Host ""
Write-Host "[6] Verificando stack..." -ForegroundColor Yellow

$servicios = @(
    @{ Nombre = "Auth    :50051"; Puerto = "50051" },
    @{ Nombre = "Sales   :50052"; Puerto = "50052" },
    @{ Nombre = "CFDI    :50053"; Puerto = "50053" },
    @{ Nombre = "Loyalty :50054"; Puerto = "50054" },
    @{ Nombre = "CFDI-H  :50055"; Puerto = "50055" },
    @{ Nombre = "BFF     :8080 "; Puerto = "8080"  }
)

$todoOk = $true
foreach ($svc in $servicios) {
    $result = netstat -ano | findstr ":$($svc.Puerto)" | findstr "LISTENING"
    if ($result) {
        Write-Host "  [OK] $($svc.Nombre)" -ForegroundColor Green
    } else {
        Write-Host "  [FALLA] $($svc.Nombre) - NO esta escuchando" -ForegroundColor Red
        $todoOk = $false
    }
}

if ($todoOk) {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host "  Stack completo listo en localhost" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Write-Host ""
    try {
        $health = Invoke-RestMethod "http://localhost:8080/api/v1/status" -TimeoutSec 5
        Write-Host "API Status:" -ForegroundColor Green
        $health.services.PSObject.Properties | ForEach-Object {
            $color = if ($_.Value -eq "OK") { "Green" } else { "Red" }
            Write-Host "  $($_.Name): $($_.Value)" -ForegroundColor $color
        }
    } catch {
        Write-Host "Health check fallo - prueba /api/v1/status en unos segundos" -ForegroundColor Yellow
    }
    Write-Host ""
    Write-Host "--- ENDPOINTS DISPONIBLES ---" -ForegroundColor Cyan
    Write-Host "POST /api/v1/cobrar           - Crear venta"
    Write-Host "POST /api/v1/timbrar          - Generar CFDI 4.0"
    Write-Host "POST /api/v1/cancelar         - Cancelar CFDI"
    Write-Host "GET  /api/v1/loyalty?phone=X  - Ver puntos"
    Write-Host "POST /api/v1/loyalty/redeem   - Canjear puntos"
    Write-Host "POST /api/v1/migrate/preview  - Preview CSV"
    Write-Host "POST /api/v1/migrate          - Importar CSV"
    Write-Host "GET  /api/v1/corte            - Corte Z"
    Write-Host "GET  /api/v1/status           - Health check"
    Write-Host "-----------------------------" -ForegroundColor Cyan
} else {
    Write-Host ""
    Write-Host "ADVERTENCIA: Algunos servicios no iniciaron." -ForegroundColor Red
    Write-Host "Revisa las ventanas abiertas para ver errores." -ForegroundColor Red
}
