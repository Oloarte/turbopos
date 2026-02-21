param($OutFile = "scan-report.md")
Remove-Item $OutFile -ErrorAction SilentlyContinue

function Run-Scan($titulo, $cmd) {
    "`n### $titulo`n```text" | Out-File $OutFile -Append -Encoding UTF8
    Invoke-Expression "$cmd 2>&1" | Out-String | Out-File $OutFile -Append -Encoding UTF8
    "```" | Out-File $OutFile -Append -Encoding UTF8
}

Write-Host "Iniciando analisis de seguridad..." -ForegroundColor Cyan

Run-Scan "go vet" "go vet ./..."
Run-Scan "staticcheck" "staticcheck ./..."
Run-Scan "gosec" "gosec ./..."
Run-Scan "govulncheck" "govulncheck ./..."
Run-Scan "go list updates" "go list -m -u all"
Run-Scan "Syft SBOM" "syft dir:. | Select-Object -First 20"
Run-Scan "Trivy FS" "trivy fs --severity HIGH,CRITICAL --scanners vuln --no-progress ."

Write-Host "Reporte completado exitosamente: $OutFile" -ForegroundColor Green