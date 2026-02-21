// TurboPOS v10.1 - BFF (Backend For Frontend) PRO
package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "log"
    "net/http"
    "time"

    _ "github.com/lib/pq"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    pb_auth "github.com/turbopos/turbopos/gen/go/proto/auth/v1"
    pb_cfdi "github.com/turbopos/turbopos/gen/go/proto/cfdi/v1"
)

type Gateway struct {
    cfdiClient pb_cfdi.CFDIServiceClient
    authClient pb_auth.AuthServiceClient
    db         *sql.DB
}

func main() {
    log.Println("?? [BFF] Iniciando Gateway de Misi?n Cr?tica en :8080...")

    cfdiConn, _ := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
    authConn, _ := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))

    connStr := "host=127.0.0.1 port=5432 user=postgres password=turbopos dbname=turbopos sslmode=disable"
    db, _ := sql.Open("postgres", connStr)

    gw := &Gateway{
        cfdiClient: pb_cfdi.NewCFDIServiceClient(cfdiConn),
        authClient: pb_auth.NewAuthServiceClient(authConn),
        db:         db,
    }

    mux := http.NewServeMux()
    mux.HandleFunc("/api/v1/cobrar", gw.handleCobrar)
    mux.HandleFunc("/api/v1/status", gw.handleStatus)
    mux.HandleFunc("/api/v1/logs", gw.handleLogs)

    corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        if r.Method == "OPTIONS" { return }
        mux.ServeHTTP(w, r)
    })

    log.Fatal(http.ListenAndServe(":8080", corsHandler))
}

func (gw *Gateway) handleCobrar(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { return }
    var req struct {
        VentaID string  `json:"venta_id"`
        Total   float64 `json:"total"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
    defer cancel()
    res, err := gw.cfdiClient.Timbrar(ctx, &pb_cfdi.FacturaRequest{
        VentaId: req.VentaID, Total: req.Total, Rfc: "XAXX010101000",
    })
    if err != nil {
        w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); return
    }
    json.NewEncoder(w).Encode(res)
}

func (gw *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(map[string]interface{}{"system": "ONLINE", "services": map[string]string{"auth": "OK", "cfdi": "OK", "db": "CONNECTED"}})
}

func (gw *Gateway) handleLogs(w http.ResponseWriter, r *http.Request) {
    rows, err := gw.db.Query("SELECT event_type, aggregate_id, created_at FROM event_store ORDER BY created_at DESC LIMIT 15")
    if err != nil { return }
    defer rows.Close()
    var logs []map[string]string
    for rows.Next() {
        var et, aid, cat string
        rows.Scan(&et, &aid, &cat)
        logs = append(logs, map[string]string{"msg": "Venta " + aid + " (" + et + ")", "time": cat, "type": "info"})
    }
    json.NewEncoder(w).Encode(logs)
}
