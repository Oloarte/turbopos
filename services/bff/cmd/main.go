// TurboPOS v10.1 - BFF (Backend For Frontend)
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
	pb_auth  "github.com/turbopos/turbopos/gen/go/proto/auth/v1"
	pb_sales "github.com/turbopos/turbopos/gen/go/proto/sales/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Gateway struct {
	authClient  pb_auth.AuthServiceClient
	salesClient pb_sales.SalesServiceClient
	db          *sql.DB
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	log.Println("[BFF] Iniciando Gateway TurboPOS en :8080...")

	authConn, err := grpc.Dial(getenv("AUTH_ADDR", "localhost:50051"),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("ERROR conectando auth: %v", err)
	}

	salesConn, err := grpc.Dial(getenv("SALES_ADDR", "localhost:50052"),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("ERROR conectando sales: %v", err)
	}

	dsn := "host=" + getenv("DB_HOST", "127.0.0.1") +
		" port=" + getenv("DB_PORT", "5432") +
		" user=" + getenv("DB_USER", "postgres") +
		" password=" + getenv("DB_PASS", "turbopos") +
		" dbname=" + getenv("DB_NAME", "turbopos") +
		" sslmode=disable"
	db, _ := sql.Open("postgres", dsn)

	gw := &Gateway{
		authClient:  pb_auth.NewAuthServiceClient(authConn),
		salesClient: pb_sales.NewSalesServiceClient(salesConn),
		db:          db,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/cobrar", gw.handleCobrar)
	mux.HandleFunc("/api/v1/status", gw.handleStatus)
	mux.HandleFunc("/api/v1/logs",   gw.handleLogs)
	mux.HandleFunc("/api/v1/corte",  gw.handleCorte)

	cors := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			return
		}
		mux.ServeHTTP(w, r)
	})

	log.Println("[BFF] Escuchando en :8080")
	log.Fatal(http.ListenAndServe(":8080", cors))
}

func (gw *Gateway) handleCobrar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CashierID     string  `json:"cashier_id"`
		Total         float64 `json:"total"`
		PaymentMethod string  `json:"payment_method"`
		Items         []struct {
			ProductID string  `json:"product_id"`
			Name      string  `json:"name"`
			Quantity  int32   `json:"quantity"`
			UnitPrice float64 `json:"unit_price"`
			Subtotal  float64 `json:"subtotal"`
		} `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON invalido"})
		return
	}

	if req.PaymentMethod == "" {
		req.PaymentMethod = "cash"
	}
	if req.CashierID == "" {
		req.CashierID = "00000000-0000-0000-0000-000000000001"
	}

	var items []*pb_sales.SaleItem
	for _, it := range req.Items {
		items = append(items, &pb_sales.SaleItem{
			ProductId: it.ProductID,
			Name:      it.Name,
			Quantity:  it.Quantity,
			UnitPrice: it.UnitPrice,
			Subtotal:  it.Subtotal,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := gw.salesClient.CreateSale(ctx, &pb_sales.CreateSaleRequest{
		CashierId:     req.CashierID,
		Total:         req.Total,
		PaymentMethod: req.PaymentMethod,
		Items:         items,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sale_id":    res.GetSaleId(),
		"status":     res.GetStatus(),
		"total":      res.GetTotal(),
		"created_at": res.GetCreatedAt(),
	})
}

func (gw *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	authOK := "OK"
	_, err := gw.authClient.Ping(ctx, &pb_auth.PingRequest{Message: "health"})
	if err != nil {
		authOK = "ERROR: " + err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"system": "ONLINE",
		"services": map[string]string{
			"auth":  authOK,
			"sales": "OK",
		},
	})
}

func (gw *Gateway) handleLogs(w http.ResponseWriter, r *http.Request) {
	rows, err := gw.db.Query(
		"SELECT id, total, payment_method, status, created_at FROM sales ORDER BY created_at DESC LIMIT 15")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id, method, status, cat string
		var total float64
		rows.Scan(&id, &total, &method, &status, &cat)
		logs = append(logs, map[string]interface{}{
			"sale_id": id, "total": total,
			"method": method, "status": status, "time": cat,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (gw *Gateway) handleCorte(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	fecha := r.URL.Query().Get("fecha")
	if fecha == "" {
		fecha = time.Now().Format("2006-01-02")
	}

	query := `
		SELECT
			COUNT(*)                                                        AS total_transacciones,
			COALESCE(SUM(total), 0)                                         AS total_vendido,
			COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0) AS canceladas,
			COALESCE(SUM(CASE WHEN status = 'completed' THEN total ELSE 0 END), 0) AS neto,
			payment_method
		FROM sales
		WHERE DATE(created_at AT TIME ZONE 'UTC') = $1::date
		GROUP BY payment_method`

	rows, err := gw.db.Query(query, fecha)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type MetodoPago struct {
		Metodo            string  `json:"metodo"`
		Transacciones     int64   `json:"transacciones"`
		TotalVendido      float64 `json:"total_vendido"`
		Canceladas        int64   `json:"canceladas"`
		Neto              float64 `json:"neto"`
	}

	var metodos []MetodoPago
	var totalTx, totalCanceladas int64
	var totalVendido, totalNeto float64

	for rows.Next() {
		var m MetodoPago
		rows.Scan(&m.Transacciones, &m.TotalVendido, &m.Canceladas, &m.Neto, &m.Metodo)
		metodos = append(metodos, m)
		totalTx += m.Transacciones
		totalVendido += m.TotalVendido
		totalCanceladas += m.Canceladas
		totalNeto += m.Neto
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"fecha":               fecha,
		"total_transacciones": totalTx,
		"total_vendido":       totalVendido,
		"total_canceladas":    totalCanceladas,
		"total_neto":          totalNeto,
		"por_metodo":          metodos,
		"generado_at":         time.Now().Format(time.RFC3339),
	})
}
