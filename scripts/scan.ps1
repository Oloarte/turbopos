param([string]$OutFile = "scan-report.md")
Remove-Item -Path $OutFile -ErrorAction SilentlyContinue

function Append-Section {
    param([string]$Title, [scriptblock]$Command)
    Add-Content -Path $OutFile -Value "`n### $Title`n"
    Add-Content -Path $OutFile -Value "```text"
    try {
        $output = & $Command 2>&1 | Out-String
        if (-not [string]::IsNullOrWhiteSpace($output)) {
            Add-Content -Path $OutFile -Value $output.TrimEnd()
        } else {
            Add-Content -Path $OutFile -Value "Sin hallazgos / Todo en orden."
        }
    } catch {
        Add-Content -Path $OutFile -Value "Error al ejecutar: $_"
    }
    Add-Content -Path $OutFile -Value "```"
}

Write-Host "Iniciando analisis de seguridad..." -ForegroundColor Cyan

Append-Section "go vet" { go vet ./... }
Append-Section "staticcheck" { staticcheck ./... }
Append-Section "gosec" { gosec ./... }
Append-Section "govulncheck" { govulncheck ./... }

if (Test-Path -Path "*.tf" -PathType Leaf) {
    Append-Section "terraform fmt -check" { terraform fmt -check -recursive }
    Append-Section "tflint" { tflint }
    Append-Section "tfsec" { tfsec --no-color . }
}

Append-Section "go list (updates)" { go list -m -u all }
Append-Section "Syft SBOM" { syft dir:. | Select-Object -First 20 }
Append-Section "Trivy FS scan" { trivy fs --severity HIGH,CRITICAL --scanners vuln --no-progress . }

Write-Host "Analisis completado! Tu reporte: $OutFile" -ForegroundColor Green