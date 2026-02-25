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
	pb_auth    "github.com/turbopos/turbopos/gen/go/proto/auth/v1"
	pb_cfdi    "github.com/turbopos/turbopos/gen/go/proto/cfdi/v1"
	pb_loyalty "github.com/turbopos/turbopos/gen/go/proto/loyalty/v1"
	pb_sales   "github.com/turbopos/turbopos/gen/go/proto/sales/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Gateway struct {
	authClient    pb_auth.AuthServiceClient
	salesClient   pb_sales.SalesServiceClient
	cfdiClient    pb_cfdi.CFDIServiceClient
	loyaltyClient pb_loyalty.LoyaltyServiceClient
	db            *sql.DB
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	log.Println("[BFF] Iniciando Gateway TurboPOS en :8080...")

	authConn, err := grpc.Dial(getenv("AUTH_ADDR", "localhost:50051"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil { log.Fatalf("ERROR conectando auth: %v", err) }

	salesConn, err := grpc.Dial(getenv("SALES_ADDR", "localhost:50052"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil { log.Fatalf("ERROR conectando sales: %v", err) }

	cfdiConn, err := grpc.Dial(getenv("CFDI_ADDR", "localhost:50053"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil { log.Fatalf("ERROR conectando cfdi: %v", err) }

	loyaltyConn, err := grpc.Dial(getenv("LOYALTY_ADDR", "localhost:50054"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil { log.Fatalf("ERROR conectando loyalty: %v", err) }

	dsn := "host=" + getenv("DB_HOST", "127.0.0.1") +
		" port=" + getenv("DB_PORT", "5432") +
		" user=" + getenv("DB_USER", "postgres") +
		" password=" + getenv("DB_PASS", "turbopos") +
		" dbname=" + getenv("DB_NAME", "turbopos") +
		" sslmode=disable"
	db, _ := sql.Open("postgres", dsn)

	gw := &Gateway{
		authClient:    pb_auth.NewAuthServiceClient(authConn),
		salesClient:   pb_sales.NewSalesServiceClient(salesConn),
		cfdiClient:    pb_cfdi.NewCFDIServiceClient(cfdiConn),
		loyaltyClient: pb_loyalty.NewLoyaltyServiceClient(loyaltyConn),
		db:            db,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/cobrar",  gw.handleCobrar)
	mux.HandleFunc("/api/v1/timbrar", gw.handleTimbrar)
	mux.HandleFunc("/api/v1/status",  gw.handleStatus)
	mux.HandleFunc("/api/v1/logs",    gw.handleLogs)
	mux.HandleFunc("/api/v1/corte",   gw.handleCorte)

	cors := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" { return }
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
		CustomerName  string  `json:"customer_name"`

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

	if req.PaymentMethod == "" { req.PaymentMethod = "cash" }
	if req.CashierID == "" { req.CashierID = "00000000-0000-0000-0000-000000000001" }

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

	// Sumar puntos si viene ?phone=
	if phone := r.URL.Query().Get("phone"); phone != "" {
		go func() {
			ctxL, cancelL := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancelL()
			acc, err := gw.loyaltyClient.EarnPoints(ctxL, &pb_loyalty.EarnPointsRequest{
				Phone:  phone,
				SaleId: res.GetSaleId(),
				Total:  req.Total,
				Name:   req.CustomerName,
			})
			if err != nil {
				log.Printf("[BFF] Loyalty error: %v", err)
			} else {
				log.Printf("[BFF] Loyalty +%d pts para %s Ã¢â‚¬â€ total: %d tier: %s",
					int(req.Total), phone, acc.GetPoints(), acc.GetTier())
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sale_id":    res.GetSaleId(),
		"status":     res.GetStatus(),
		"total":      res.GetTotal(),
		"created_at": res.GetCreatedAt(),
	})
}

func (gw *Gateway) handleTimbrar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SaleID string  `json:"sale_id"`
		RFC    string  `json:"rfc"`
		Total  float64 `json:"total"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON invalido"})
		return
	}

	if req.RFC == "" { req.RFC = "XAXX010101000" }

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := gw.cfdiClient.Timbrar(ctx, &pb_cfdi.FacturaRequest{
		VentaId: req.SaleID,
		Total:   req.Total,
		Rfc:     req.RFC,
	})
	if err != nil {
		log.Printf("[BFF] Error timbrando %s: %v", req.SaleID, err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_, dbErr := gw.db.ExecContext(ctx,
		`UPDATE sales SET cfdi_uuid = $1, cfdi_status = $2 WHERE id = $3::uuid`,
		res.GetUuid(), "timbrado", req.SaleID)
	if dbErr != nil {
		log.Printf("[BFF] WARN: no se pudo guardar cfdi_uuid: %v", dbErr)
	}

	log.Printf("[BFF] Timbrado exitoso: sale=%s uuid=%s", req.SaleID, res.GetUuid())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sale_id":   req.SaleID,
		"cfdi_uuid": res.GetUuid(),
		"sello_sat": res.GetSelloSat(),
		"status":    res.GetStatus(),
		"pac_usado": res.GetPacUsado(),
		"timestamp": res.GetTimestamp(),
	})
}

func (gw *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	authOK := "OK"
	_, err := gw.authClient.Ping(ctx, &pb_auth.PingRequest{Message: "health"})
	if err != nil { authOK = "ERROR: " + err.Error() }

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"system": "ONLINE",
		"services": map[string]string{
			"auth": authOK, "sales": "OK", "cfdi": "OK", "loyalty": "OK",
		},
	})
}

func (gw *Gateway) handleLogs(w http.ResponseWriter, r *http.Request) {
	rows, err := gw.db.Query(
		`SELECT id, total, payment_method, status, cfdi_uuid, created_at
		 FROM sales ORDER BY created_at DESC LIMIT 15`)
	if err != nil { w.WriteHeader(http.StatusInternalServerError); return }
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id, method, status, cat string
		var total float64
		var cfdiUUID sql.NullString
		rows.Scan(&id, &total, &method, &status, &cfdiUUID, &cat)
		entry := map[string]interface{}{
			"sale_id": id, "total": total, "method": method, "status": status, "time": cat,
		}
		if cfdiUUID.Valid { entry["cfdi_uuid"] = cfdiUUID.String }
		logs = append(logs, entry)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (gw *Gateway) handleCorte(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }

	fecha := r.URL.Query().Get("fecha")
	if fecha == "" { fecha = time.Now().Format("2006-01-02") }

	query := `
		SELECT COUNT(*), COALESCE(SUM(total),0),
			COALESCE(SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status='completed' THEN total ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN cfdi_uuid IS NOT NULL THEN 1 ELSE 0 END),0),
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
		Metodo        string  `json:"metodo"`
		Transacciones int64   `json:"transacciones"`
		TotalVendido  float64 `json:"total_vendido"`
		Canceladas    int64   `json:"canceladas"`
		Neto          float64 `json:"neto"`
		Timbradas     int64   `json:"timbradas"`
	}

	var metodos []MetodoPago
	var totalTx, totalCanceladas, totalTimbradas int64
	var totalVendido, totalNeto float64

	for rows.Next() {
		var m MetodoPago
		rows.Scan(&m.Transacciones, &m.TotalVendido, &m.Canceladas, &m.Neto, &m.Timbradas, &m.Metodo)
		metodos = append(metodos, m)
		totalTx += m.Transacciones
		totalVendido += m.TotalVendido
		totalCanceladas += m.Canceladas
		totalNeto += m.Neto
		totalTimbradas += m.Timbradas
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"fecha": fecha, "total_transacciones": totalTx,
		"total_vendido": totalVendido, "total_canceladas": totalCanceladas,
		"total_neto": totalNeto, "total_timbradas": totalTimbradas,
		"por_metodo": metodos, "generado_at": time.Now().Format(time.RFC3339),
	})
}
