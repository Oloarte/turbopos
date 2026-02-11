# scan-proto.ps1 — Verifica que cada .proto tenga el go_package esperado
$ErrorActionPreference = 'SilentlyContinue'
$goBase = "github.com/Oloarte/turbopos/gen"
$bad    = 0

Get-ChildItem -Recurse -Path .\proto -Filter *.proto | ForEach-Object {
    $file = $_.FullName
    $txt  = Get-Content -Raw -Encoding UTF8 $file
    if ($txt.StartsWith([char]0xFEFF)) { $txt = $txt.Substring(1) }

    if ($txt -match '^\s*package\s+([a-z0-9.]+);\s*') {
        $pkg   = $Matches[1]
        $alias = ($pkg -replace '\.', '')
        $want  = "option go_package = `"$goBase/$($pkg -replace '\.','/');$alias`";"

        if ($txt -notmatch [regex]::Escape($want)) {
            Write-Warning "[MISMATCH] $file`n ↳ Esperado: $want"
            $bad++
        }
    }
}

if ($bad -eq 0) {
    Write-Host "`nTodos los .proto pasan la validación ✔"
} else {
    Write-Host "`n$bad archivo(s) necesitan arreglo ❌"
    exit 1
}
