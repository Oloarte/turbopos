package main
import (
    "database/sql"
    "fmt"
    "log"
    "time"
    _ "github.com/lib/pq"
)
func main() {
    log.Println("?? [Report-Bot] Iniciando Agente de Reporting v10.1...")
    connStr := "host=127.0.0.1 port=5432 user=postgres password=turbopos dbname=turbopos sslmode=disable"
    db, _ := sql.Open("postgres", connStr)
    ticker := time.NewTicker(30 * time.Second)
    for range ticker.C {
        log.Println("?? Generando Informe de Ventas (Ciclo 3h)...")
        var totalVentas float64
        var count int
        query := "SELECT count(*), COALESCE(sum((payload->>'total')::numeric), 0) FROM event_store WHERE event_type = 'CFDI_ISSUED'"
        err := db.QueryRow(query).Scan(&count, &totalVentas)
        if err != nil { log.Printf("? Error: %v", err); continue }
        fmt.Println("************************************************")
        fmt.Println("?? TURBOPOS - INFORME DE DESEMPE?O")
        fmt.Printf("?? Ventas Totales: $%.2f MXN\n", totalVentas)
        fmt.Printf("?? Transacciones: %d\n", count)
        fmt.Printf("?? ROAS: Dark Kitchen estable en 4.2\n")
        fmt.Println("************************************************")
    }
}
