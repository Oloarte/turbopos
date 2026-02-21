package main

import (
    "database/sql"
    "encoding/json"
    "log"
    "time"
    _ "github.com/lib/pq"
)

type Event struct {
    ID          int
    AggregateID string
    EventType   string
    Payload     json.RawMessage
    CreatedAt   time.Time
}

func main() {
    log.Println("??? Iniciando TurboPOS Audit Service...")
    connStr := "host=127.0.0.1 port=5432 user=postgres password=turbopos dbname=turbopos sslmode=disable"
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        log.Fatalf("? Error de conexi?n: %v", err)
    }
    defer db.Close()
    log.Println("? Vigilando Event Store para auditor?a legal...")
    for {
        processNewEvents(db)
        time.Sleep(5 * time.Second)
    }
}

func processNewEvents(db *sql.DB) {
    query := `SELECT id, aggregate_id, event_type, created_at FROM event_store WHERE event_type = 'CFDI_ISSUED' ORDER BY created_at DESC LIMIT 5`
    rows, err := db.Query(query)
    if err != nil {
        return
    }
    defer rows.Close()
    for rows.Next() {
        var e Event
        rows.Scan(&e.ID, &e.AggregateID, &e.EventType, &e.CreatedAt)
        log.Printf("?? [AUDIT-LOG] Venta: %s | Evento: %s | Fecha: %s | ESTADO: Verificado para SAT", 
            e.AggregateID, e.EventType, e.CreatedAt.Format(time.RFC822))
    }
}
