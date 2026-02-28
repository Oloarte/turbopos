# TurboPOS - Script de inicio automatico
$ROOT = "C:\dev\turbopos"
$env:DB_PASS     = "turbopos"
$env:FINKOK_USER = "danieloloarte@hotmail.com"
$env:FINKOK_PASS = "Spaceboy1."
Set-Location $ROOT
Write-Host ""
Write-Host "  =================================" -ForegroundColor Yellow
Write-Host "       TurboPOS v10.1" -ForegroundColor Yellow
Write-Host "    Sistema de Punto de Venta" -ForegroundColor Yellow
Write-Host "  =================================" -ForegroundColor Yellow
Write-Host ""
Write-Host "  [1/4] Liberando puertos..." -ForegroundColor Cyan
@("50051","50052","50053","50054","8080") | ForEach-Object {
    netstat -ano | findstr ":$_" | findstr "LISTENING" | ForEach-Object {
        $p = ($_ -split '\s+') | Where-Object {$_ -match '^\d+$'} | Select-Object -Last 1
        if ($p -and $p -ne "0") { taskkill /PID $p /F 2>$null | Out-Null }
    }
}
Start-Sleep 2
Write-Host "         OK - Puertos liberados" -ForegroundColor Green
Write-Host "  [2/4] Compilando servicios..." -ForegroundColor Cyan
$builds = @(
    @{ name="auth";    src="./services/auth/cmd/";         out="auth.exe"    },
    @{ name="sales";   src="./services/sales/cmd/server/"; out="sales.exe"   },
    @{ name="cfdi";    src="./services/cfdi/cmd/";         out="cfdi.exe"    },
    @{ name="loyalty"; src="./services/loyalty/cmd/";      out="loyalty.exe" },
    @{ name="bff";     src="./services/bff/cmd/";          out="bff.exe"     }
)
$buildFailed = $false
foreach ($b in $builds) {
    Write-Host "         -> $($b.name)..." -ForegroundColor Gray -NoNewline
    $output = & go build -o $b.out $b.src 2>&1
    if ($LASTEXITCODE -eq 0) { Write-Host " OK" -ForegroundColor Green }
    else { Write-Host " ERROR" -ForegroundColor Red; Write-Host $output -ForegroundColor Red; $buildFailed = $true }
}
if ($buildFailed) {
    Write-Host "  ERROR en la compilacion." -ForegroundColor Red
    Read-Host "  Presiona Enter para salir"
    exit 1
}
Write-Host "  [3/4] Iniciando servicios..." -ForegroundColor Cyan
Start-Process powershell -WindowStyle Minimized -ArgumentList "-NoExit","-Command","Set-Location '$ROOT'; `$env:DB_PASS='turbopos'; .\auth.exe"
Start-Sleep 2
Start-Process powershell -WindowStyle Minimized -ArgumentList "-NoExit","-Command","Set-Location '$ROOT'; `$env:DB_PASS='turbopos'; .\sales.exe"
Start-Process powershell -WindowStyle Minimized -ArgumentList "-NoExit","-Command","Set-Location '$ROOT'; `$env:FINKOK_USER='danieloloarte@hotmail.com'; `$env:FINKOK_PASS='Spaceboy1.'; .\cfdi.exe"
Start-Process powershell -WindowStyle Minimized -ArgumentList "-NoExit","-Command","Set-Location '$ROOT'; .\loyalty.exe"
Start-Sleep 4
Start-Process powershell -WindowStyle Minimized -ArgumentList "-NoExit","-Command","Set-Location '$ROOT'; `$env:DB_PASS='turbopos'; .\bff.exe"
Start-Sleep 5
Write-Host "  [4/4] Verificando sistema..." -ForegroundColor Cyan
try {
    $status = Invoke-RestMethod "http://localhost:8080/api/v1/status" -TimeoutSec 10
    $sAuth    = if ($status.services.auth    -eq "OK") { "OK" } else { "ERROR" }
    $sSales   = if ($status.services.sales   -eq "OK") { "OK" } else { "ERROR" }
    $sCfdi    = if ($status.services.cfdi    -eq "OK") { "OK" } else { "ERROR" }
    $sLoyalty = if ($status.services.loyalty -eq "OK") { "OK" } else { "ERROR" }
    Write-Host ""
    Write-Host "  =================================" -ForegroundColor DarkGray
    Write-Host "   Auth      : $sAuth" -ForegroundColor White
    Write-Host "   Sales     : $sSales" -ForegroundColor White
    Write-Host "   CFDI/SAT  : $sCfdi" -ForegroundColor White
    Write-Host "   Lealtad   : $sLoyalty" -ForegroundColor White
    Write-Host "  ---------------------------------" -ForegroundColor DarkGray
    Write-Host "   URL: http://localhost:8080" -ForegroundColor Yellow
    Write-Host "  =================================" -ForegroundColor DarkGray
    Write-Host ""
    Start-Process "http://localhost:8080"
    Write-Host "  Sistema listo." -ForegroundColor Green
} catch {
    Write-Host "  AVISO: Abre manualmente: http://localhost:8080" -ForegroundColor Yellow
}
Write-Host ""
Read-Host "  Presiona Enter para cerrar esta ventana"
