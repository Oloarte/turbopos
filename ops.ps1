param (
    [Parameter(Mandatory=$true)]
    [ValidateSet("up", "down", "proto", "migrate-up", "run-auth", "test")]
    [string]$cmd
)

switch ($cmd) {
    "up" {
        docker compose up -d
        Write-Host "[OK] Base de datos de TurboPOS levantada." -ForegroundColor Green
    }
    "down" {
        docker compose down
        Write-Host "[OK] Entorno detenido." -ForegroundColor Yellow
    }
    "proto" {
        buf generate
        go mod tidy
        Write-Host "[OK] Contratos gRPC regenerados y dependencias actualizadas." -ForegroundColor Cyan
    }
    "migrate-up" {
        migrate -path db/migrations -database "postgresql://postgres:turbopos@127.0.0.1:5432/turbopos?sslmode=disable" up
        Write-Host "[OK] Migraciones aplicadas en la base de datos." -ForegroundColor Green
    }
    "run-auth" {
        Write-Host "[>] Iniciando Auth Service..." -ForegroundColor Magenta
        go run services/auth/cmd/main.go
    }
    "test" {
        go test -v ./...
    }
}
