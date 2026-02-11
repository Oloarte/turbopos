# fix-proto.ps1 — Inserta option go_package correcto en cada .proto
$goBase = "github.com/Oloarte/turbopos/gen"

Get-ChildItem -Recurse -Path .\proto -Filter *.proto | ForEach-Object {
    $file   = $_.FullName
    $text   = Get-Content -Raw -Encoding UTF8 $file
    if ($text.StartsWith([char]0xFEFF)) { $text = $text.Substring(1) }

    if ($text -match '^\s*package\s+([a-z0-9.]+);\s*') {
        $pkg   = $Matches[1]          # ej. auth.v1
        $alias = ($pkg -replace '\.', '')
        $want  = "option go_package = `"$goBase/$($pkg -replace '\.','/');$alias`";"

        if ($text -notmatch 'option\s+go_package') {
            # lo inserta justo debajo de la línea package
            $text = $text -replace "(package\s+$pkg;\s*)", "`$1`r`n$want`r`n"
        } else {
            # lo reemplaza si ya existe (pero es incorrecto)
            $text = $text -replace 'option\s+go_package\s*=.*;', $want
        }
        Set-Content -Encoding UTF8 $file $text
        Write-Host "✔  $(Resolve-Path $file)"
    }
}
