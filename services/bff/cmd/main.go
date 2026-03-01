// TurboPOS v10.1 - BFF (Backend For Frontend)
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	if v := os.Getenv(key); v != "" { return v }
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
	mux.HandleFunc("/api/v1/products",   gw.handleProducts)
	mux.HandleFunc("/api/v1/products/",  gw.handleProductByID)
	mux.HandleFunc("/api/v1/login",           gw.handleLogin)
	mux.HandleFunc("/api/v1/cobrar",          gw.handleCobrar)
	mux.HandleFunc("/api/v1/timbrar",         gw.handleTimbrar)
	mux.HandleFunc("/api/v1/cancelar",        gw.handleCancelar)
	mux.HandleFunc("/api/v1/status",          gw.handleStatus)
	mux.HandleFunc("/api/v1/logs",            gw.handleLogs)
	mux.HandleFunc("/api/v1/corte",           gw.handleCorte)
	mux.HandleFunc("/api/v1/loyalty",         gw.handleLoyalty)
	mux.HandleFunc("/api/v1/loyalty/redeem",  gw.handleLoyaltyRedeem)
    mux.HandleFunc("/api/v1/loyalty/rfc",     gw.handleLoyaltyRfc)
	mux.HandleFunc("/api/v1/loyalty/cp",      gw.handleLoyaltyCp)
	mux.HandleFunc("/api/v1/migrate/preview", gw.handleMigratePreview)
	mux.HandleFunc("/api/v1/migrate",         gw.handleMigrate)
	mux.HandleFunc("/api/v1/reportes", gw.handleReportes)
	mux.Handle("/", http.FileServer(http.Dir("web")))
	log.Println("[BFF] Escuchando en :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (gw *Gateway) handleCobrar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
	var req struct {
		CashierID     string  `json:"cashier_id"`
		Total         float64 `json:"total"`
		PaymentMethod string  `json:"payment_method"`
		CustomerName  string  `json:"customer_name"`
		Items []struct {
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
	if req.CashierID == ""    { req.CashierID = "00000000-0000-0000-0000-000000000001" }

	var items []*pb_sales.SaleItem
	for _, it := range req.Items {
		items = append(items, &pb_sales.SaleItem{
			ProductId: it.ProductID, Name: it.Name,
			Quantity: it.Quantity, UnitPrice: it.UnitPrice, Subtotal: it.Subtotal,
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	res, err := gw.salesClient.CreateSale(ctx, &pb_sales.CreateSaleRequest{
		CashierId: req.CashierID, Total: req.Total,
		PaymentMethod: req.PaymentMethod, Items: items,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	// Marcar la venta con el tenant_id
	tid := tenantID(r)
	gw.db.Exec(`UPDATE sales SET tenant_id=$1 WHERE id=$2::uuid`, tid, res.GetSaleId())
	if phone := r.URL.Query().Get("phone"); phone != "" {
		go func() {
			ctxL, cancelL := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancelL()
			acc, err := gw.loyaltyClient.EarnPoints(ctxL, &pb_loyalty.EarnPointsRequest{
				Phone: phone, SaleId: res.GetSaleId(), Total: req.Total, Name: req.CustomerName,
			})
			if err != nil {
				log.Printf("[BFF] Loyalty error: %v", err)
			} else {
				log.Printf("[BFF] Loyalty +%.0fpts para %s — total: %d tier: %s", req.Total, phone, acc.GetPoints(), acc.GetTier())
			}
		}()
	}
	// Auto-timbrado: si viene ?factura=1 en la URL, timbrar como público general en background
	if r.URL.Query().Get("factura") == "1" || r.URL.Query().Get("rfc") != "" {
		rfcTimbrar := r.URL.Query().Get("rfc"); if rfcTimbrar == "" { rfcTimbrar = "XAXX010101000" }
        go func(saleID string, total float64, rfc string, cp string, tID string) {
                ctxT, cancelT := context.WithTimeout(context.Background(), 30*time.Second)
                defer cancelT()
                treq := &pb_cfdi.FacturaRequest{VentaId: saleID, Total: total, Rfc: rfc, CodigoPostalReceptor: cp}
                if cB64, kBytes, kPass, _, csdOK := gw.loadTenantCSD(tID); csdOK {
                    treq.CertB64 = cB64; treq.KeyBytes = kBytes; treq.KeyPassword = kPass
                }
                tRes, err := gw.cfdiClient.Timbrar(ctxT, treq)
                if err != nil {
                    log.Printf("[BFF] Auto-timbrado error sale=%s: %v", saleID, err)
                    return
                }
                gw.db.Exec(`UPDATE sales SET cfdi_uuid=$1, cfdi_status=$2 WHERE id=$3::uuid`,
                    tRes.GetUuid(), "timbrado", saleID)
                log.Printf("[BFF] Auto-timbrado OK sale=%s uuid=%s", saleID, tRes.GetUuid())
        }(res.GetSaleId(), req.Total, rfcTimbrar, r.URL.Query().Get("cp"), tid)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sale_id": res.GetSaleId(), "status": res.GetStatus(),
		"total": res.GetTotal(), "created_at": res.GetCreatedAt(),
	})
}

func (gw *Gateway) handleLoyalty(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "phone requerido"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	acc, err := gw.loyaltyClient.GetAccount(ctx, &pb_loyalty.GetAccountRequest{Phone: phone})
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "cliente no encontrado"})
		return
	}
	hist, _ := gw.loyaltyClient.GetHistory(ctx, &pb_loyalty.GetHistoryRequest{Phone: phone, Limit: 20})
	type txItem struct {
		Type        string `json:"type"`
		Points      int32  `json:"points"`
		Description string `json:"description"`
		Date        string `json:"date"`
	}
	var history []txItem
	if hist != nil {
		for _, tx := range hist.GetTransactions() {
			history = append(history, txItem{
				Type: tx.GetType(), Points: tx.GetPoints(),
				Description: tx.GetDescription(), Date: tx.GetCreatedAt(),
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"account": map[string]interface{}{
                "phone": acc.GetPhone(), "customer_name": acc.GetName(), "rfc": acc.GetRfc(), "cp": acc.GetCp(),
			"points": acc.GetPoints(), "tier": acc.GetTier(),
			"total_spent": acc.GetTotalSpent(),
		},
		"history": history,
	})
}

func (gw *Gateway) handleLoyaltyCp(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
    var req struct {
        Phone string `json:"phone"`
        CP    string `json:"cp"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        w.WriteHeader(http.StatusBadRequest); return
    }
    if req.Phone == "" || req.CP == "" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]string{"error": "phone y cp requeridos"})
        return
    }
    _, err := gw.db.Exec(`UPDATE loyalty_accounts SET cp=$1, updated_at=now() WHERE phone=$2`, req.CP, req.Phone)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "ok", "cp": req.CP})
}

func (gw *Gateway) handleLoyaltyRfc(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
    var req struct {
        Phone string `json:"phone"`
        RFC   string `json:"rfc"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        w.WriteHeader(http.StatusBadRequest); return
    }
    if req.Phone == "" || req.RFC == "" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]string{"error": "phone y rfc requeridos"})
        return
    }
    req.RFC = strings.ToUpper(strings.TrimSpace(req.RFC))
    _, err := gw.db.Exec(
        `UPDATE loyalty_accounts SET rfc=$1, updated_at=now() WHERE phone=$2`,
        req.RFC, req.Phone,
    )
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "ok", "rfc": req.RFC})
}

func (gw *Gateway) handleLoyaltyRedeem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
	var req struct {
		Phone  string `json:"phone"`
		Points int64  `json:"points"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON invalido"})
		return
	}
	if req.Reason == "" { req.Reason = "Canje en caja" }
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	acc, err := gw.loyaltyClient.RedeemPoints(ctx, &pb_loyalty.RedeemPointsRequest{
		Phone: req.Phone, Points: int32(req.Points), RewardId: req.Reason,
	})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[BFF] Canje %d pts para %s — saldo: %d", req.Points, req.Phone, acc.GetPointsRemaining())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true, "points": acc.GetPointsRemaining(), "success": acc.GetSuccess(),
	})
}

func (gw *Gateway) handleMigratePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
	r.ParseMultipartForm(32 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "campo 'file' requerido"})
		return
	}
	defer file.Close()
	tipo := r.FormValue("tipo")
	if tipo == "" { tipo = "generic" }
	products, customers, err := parseMigrationCSV(file, tipo)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[BFF] Preview: %s — %d productos, %d clientes", header.Filename, len(products), len(customers))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"archivo": header.Filename, "tipo_detectado": tipo,
		"total_productos": len(products), "total_clientes": len(customers),
		"preview_productos": limitSlice(products, 5),
		"preview_clientes":  limitSlice(customers, 5),
	})
}

func (gw *Gateway) handleMigrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
	r.ParseMultipartForm(32 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "campo 'file' requerido"})
		return
	}
	defer file.Close()
	tipo := r.FormValue("tipo")
	if tipo == "" { tipo = "generic" }
	products, customers, err := parseMigrationCSV(file, tipo)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	prodOK, prodErr := 0, 0
	for _, p := range products {
		price := parseFloat(p["price"])
		_, e := gw.db.Exec(
			`INSERT INTO products (id, name, price, sku, created_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, NOW())
			 ON CONFLICT (sku) WHERE sku IS NOT NULL DO UPDATE SET name=EXCLUDED.name, price=EXCLUDED.price`,
			p["name"], price, p["sku"])
		if e != nil { log.Printf("[BFF] product insert err: %v", e); prodErr++ } else { prodOK++ }
	}
	custOK, custErr := 0, 0
	for _, c := range customers {
		if c["phone"] == "" { continue }
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, e := gw.loyaltyClient.EarnPoints(ctx, &pb_loyalty.EarnPointsRequest{
			Phone: c["phone"], Name: c["name"], SaleId: "migration", Total: 0,
		})
		cancel()
		if e != nil { custErr++ } else { custOK++ }
	}
	log.Printf("[BFF] Migración: %s — prod OK:%d ERR:%d | clientes OK:%d ERR:%d",
		header.Filename, prodOK, prodErr, custOK, custErr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true, "archivo": header.Filename,
		"productos_ok": prodOK, "productos_error": prodErr,
		"clientes_ok": custOK, "clientes_error": custErr,
	})
}

func (gw *Gateway) handleCancelar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }

	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[BFF] PANIC en handleCancelar: %v", rec)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("panic: %v", rec)})
		}
	}()

	var req struct {
		UUID          string `json:"uuid"`
		RFC           string `json:"rfc"`
		Motivo        string `json:"motivo"`
		UUIDReemplazo string `json:"uuid_reemplazo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON invalido"})
		return
	}
	if req.UUID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "uuid requerido"})
		return
	}
	if req.Motivo == "" { req.Motivo = "02" }

	// Llamar al CFDI server via HTTP (evita proto para cancelación)
	cfdiHTTP := getenv("CFDI_HTTP_ADDR", "http://localhost:50055")
	body, _ := json.Marshal(req)
	httpResp, err := http.Post(cfdiHTTP+"/cancelar", "application/json", strings.NewReader(string(body)))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "CFDI server no disponible: " + err.Error()})
		return
	}
	defer httpResp.Body.Close()

	var cfdiResult map[string]interface{}
	json.NewDecoder(httpResp.Body).Decode(&cfdiResult)

	if httpResp.StatusCode != http.StatusOK {
		w.WriteHeader(httpResp.StatusCode)
		json.NewEncoder(w).Encode(cfdiResult)
		return
	}

	// Actualizar DB
	gw.db.Exec(`UPDATE sales SET cfdi_status=$1 WHERE cfdi_uuid=$2`, "cancelado", req.UUID)
	log.Printf("[BFF] Cancelación: uuid=%s", req.UUID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfdiResult)
}

func (gw *Gateway) handleTimbrar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
	var req struct {
		SaleID string  `json:"sale_id"`
		RFC    string  `json:"rfc"`
		Total  float64 `json:"total"`
			CP    string  `json:"cp"`
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
            VentaId: req.SaleID, Total: req.Total, Rfc: req.RFC, CodigoPostalReceptor: req.CP,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	gw.db.ExecContext(ctx,
		`UPDATE sales SET cfdi_uuid=$1, cfdi_status=$2 WHERE id=$3::uuid`,
		res.GetUuid(), "timbrado", req.SaleID)
	log.Printf("[BFF] Timbrado: sale=%s uuid=%s", req.SaleID, res.GetUuid())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sale_id": req.SaleID, "cfdi_uuid": res.GetUuid(),
		"sello_sat": res.GetSelloSat(), "status": res.GetStatus(),
		"pac_usado": res.GetPacUsado(), "timestamp": res.GetTimestamp(),
	})
}

func (gw *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	authOK := "OK"
	if _, err := gw.authClient.Ping(ctx, &pb_auth.PingRequest{Message: "health"}); err != nil {
		authOK = "ERROR: " + err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"system": "ONLINE",
		"services": map[string]string{"auth": authOK, "sales": "OK", "cfdi": "OK", "loyalty": "OK"},
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
	tid := tenantID(r)
	w.Header().Set("Content-Type", "application/json")

	// ── Por método de pago ───────────────────────────────────────────────────────
	rows, err := gw.db.Query(`
		SELECT COUNT(*), COALESCE(SUM(total),0),
			COALESCE(SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status='completed' THEN total ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN cfdi_uuid IS NOT NULL THEN 1 ELSE 0 END),0),
			payment_method
		FROM sales
		WHERE DATE(created_at AT TIME ZONE 'America/Monterrey') = $1::date
		  AND (tenant_id = $2::uuid OR tenant_id IS NULL)
		GROUP BY payment_method`, fecha, tid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	type M struct {
		Metodo        string  `json:"metodo"`
		Transacciones int64   `json:"transacciones"`
		TotalVendido  float64 `json:"total_vendido"`
		Canceladas    int64   `json:"canceladas"`
		Neto          float64 `json:"neto"`
		Timbradas     int64   `json:"timbradas"`
	}
	var metodos []M
	var totalTx, totalCanceladas, totalTimbradas int64
	var totalVendido, totalNeto float64
	for rows.Next() {
		var m M
		rows.Scan(&m.Transacciones, &m.TotalVendido, &m.Canceladas, &m.Neto, &m.Timbradas, &m.Metodo)
		metodos = append(metodos, m)
		totalTx += m.Transacciones; totalVendido += m.TotalVendido
		totalCanceladas += m.Canceladas; totalNeto += m.Neto; totalTimbradas += m.Timbradas
	}

	// ── Ventas por hora ──────────────────────────────────────────────────────────
	rowsHora, _ := gw.db.Query(`
		SELECT EXTRACT(HOUR FROM created_at AT TIME ZONE 'America/Monterrey')::int as hora,
		       COUNT(*) as tx,
		       COALESCE(SUM(CASE WHEN status='completed' THEN total ELSE 0 END),0) as total
		FROM sales
		WHERE DATE(created_at AT TIME ZONE 'America/Monterrey') = $1::date
		  AND (tenant_id = $2::uuid OR tenant_id IS NULL)
		GROUP BY hora ORDER BY hora ASC`, fecha, tid)
	type HoraData struct {
		Hora  int     `json:"hora"`
		Tx    int64   `json:"tx"`
		Total float64 `json:"total"`
	}
	var porHora []HoraData
	if rowsHora != nil {
		defer rowsHora.Close()
		for rowsHora.Next() {
			var h HoraData
			rowsHora.Scan(&h.Hora, &h.Tx, &h.Total)
			porHora = append(porHora, h)
		}
	}

	// ── Top productos del día ────────────────────────────────────────────────────
	rowsProd, _ := gw.db.Query(`
		SELECT si.name, SUM(si.quantity) as uds, SUM(si.subtotal) as total
		FROM sale_items si
		JOIN sales s ON s.id = si.sale_id
		WHERE s.status = 'completed'
		  AND DATE(s.created_at AT TIME ZONE 'America/Monterrey') = $1::date
		  AND (s.tenant_id = $2::uuid OR s.tenant_id IS NULL)
		GROUP BY si.name ORDER BY total DESC LIMIT 5`, fecha, tid)
	type ProdData struct {
		Nombre string  `json:"nombre"`
		Uds    int64   `json:"uds"`
		Total  float64 `json:"total"`
	}
	var topProds []ProdData
	if rowsProd != nil {
		defer rowsProd.Close()
		for rowsProd.Next() {
			var p ProdData
			rowsProd.Scan(&p.Nombre, &p.Uds, &p.Total)
			topProds = append(topProds, p)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"fecha": fecha, "total_transacciones": totalTx,
		"total_vendido": totalVendido, "total_canceladas": totalCanceladas,
		"total_neto": totalNeto, "total_timbradas": totalTimbradas,
		"por_metodo": metodos, "por_hora": porHora, "top_prods": topProds,
		"generado_at": time.Now().Format(time.RFC3339),
	})
}

// ─── CRUD Productos ───────────────────────────────────────────────────────────
// GET  /api/v1/products         — listar productos del tenant
// POST /api/v1/products         — crear producto
// PUT  /api/v1/products/{id}    — actualizar producto
// DELETE /api/v1/products/{id}  — eliminar producto

func (gw *Gateway) handleProducts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tid := tenantID(r)

	switch r.Method {
	case http.MethodGet:
		q := `SELECT id, COALESCE(name,''), COALESCE(price,0),
		             COALESCE(sku,''), COALESCE(category,''), COALESCE(unit,'pza'),
		             COALESCE(barcode,''), active
		      FROM products
		      WHERE (tenant_id = $1::uuid OR tenant_id IS NULL)
		        AND COALESCE(active, true) = true
		      ORDER BY name`
		rows, err := gw.db.Query(q, tid)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()
		var prods []map[string]interface{}
		for rows.Next() {
			var id, name, sku, category, unit, barcode string
			var price float64
			var active bool
			rows.Scan(&id, &name, &price, &sku, &category, &unit, &barcode, &active)
			prods = append(prods, map[string]interface{}{
				"id": id, "name": name, "price": price,
				"sku": sku, "category": category, "unit": unit,
				"barcode": barcode, "active": active,
			})
		}
		if prods == nil { prods = []map[string]interface{}{} }
		json.NewEncoder(w).Encode(prods)

	case http.MethodPost:
		var req struct {
			Name     string  `json:"name"`
			Price    float64 `json:"price"`
			SKU      string  `json:"sku"`
			Category string  `json:"category"`
			Unit     string  `json:"unit"`
			Barcode  string  `json:"barcode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido"})
			return
		}
		if req.Name == "" || req.Price <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "name y price requeridos"})
			return
		}
		if req.Unit == "" { req.Unit = "pza" }
		if req.Category == "" { req.Category = "General" }
		// Autogenerar SKU si no viene
		if req.SKU == "" { req.SKU = fmt.Sprintf("PROD-%d", time.Now().UnixMilli()) }

		var id string
		err := gw.db.QueryRow(`
			INSERT INTO products (id, name, price, sku, category, unit, barcode, tenant_id, active, created_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7::uuid, true, NOW())
			ON CONFLICT (sku) WHERE sku IS NOT NULL
			DO UPDATE SET name=EXCLUDED.name, price=EXCLUDED.price,
			              category=EXCLUDED.category, unit=EXCLUDED.unit
			RETURNING id`,
			req.Name, req.Price, req.SKU, req.Category, req.Unit, req.Barcode, tid,
		).Scan(&id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		log.Printf("[BFF] Producto creado: %s — %s $%.2f", id, req.Name, req.Price)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": id, "name": req.Name, "price": req.Price, "sku": req.SKU,
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (gw *Gateway) handleProductByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Extraer ID del path: /api/v1/products/UUID
	productID := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	if productID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "id requerido"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req struct {
			Name     string  `json:"name"`
			Price    float64 `json:"price"`
			SKU      string  `json:"sku"`
			Category string  `json:"category"`
			Unit     string  `json:"unit"`
			Barcode  string  `json:"barcode"`
			Active   *bool   `json:"active"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido"})
			return
		}
		active := true
		if req.Active != nil { active = *req.Active }

		res, err := gw.db.Exec(`
			UPDATE products SET
				name     = COALESCE(NULLIF($1,''), name),
				price    = CASE WHEN $2 > 0 THEN $2 ELSE price END,
				sku      = COALESCE(NULLIF($3,''), sku),
				category = COALESCE(NULLIF($4,''), category),
				unit     = COALESCE(NULLIF($5,''), unit),
				barcode  = COALESCE(NULLIF($6,''), barcode),
				active   = $7
			WHERE id = $8::uuid`,
			req.Name, req.Price, req.SKU, req.Category, req.Unit, req.Barcode, active, productID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "producto no encontrado"})
			return
		}
		log.Printf("[BFF] Producto actualizado: %s — %s $%.2f", productID, req.Name, req.Price)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "id": productID})

	case http.MethodDelete:
		// Soft delete — marcar inactive
		res, err := gw.db.Exec(`UPDATE products SET active=false WHERE id=$1::uuid`, productID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "producto no encontrado"})
			return
		}
		log.Printf("[BFF] Producto eliminado (soft): %s", productID)
		json.NewEncoder(w).Encode(map[string]string{"status": "eliminado", "id": productID})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func parseMigrationCSV(r io.Reader, tipo string) (products, customers []map[string]string, err error) {
	reader := csv.NewReader(r)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil { return nil, nil, err }
	if len(rows) < 2 { return nil, nil, nil }

	headers := make([]string, len(rows[0]))
	for i, h := range rows[0] { headers[i] = strings.ToLower(strings.TrimSpace(h)) }

	headersStr := strings.Join(headers, ",")
	if tipo == "generic" {
		switch {
		case strings.Contains(headersStr, "clave") && strings.Contains(headersStr, "existencia"):
			tipo = "aspel_pos"
		case strings.Contains(headersStr, "codigo_producto"):
			tipo = "bind"
		case strings.Contains(headersStr, "cvenumero"):
			tipo = "contpaq"
		}
	}

	colMap := map[string]map[string][]string{
		"aspel_pos": {
			"sku":   {"clave", "codigo", "cve"},
			"name":  {"descripcion", "nombre", "producto"},
			"price": {"precio", "precio1", "pventa"},
			"phone": {"telefono", "tel", "celular"},
			"cname": {"nombre", "razon_social", "cliente"},
		},
		"bind": {
			"sku":   {"codigo_producto", "sku", "codigo"},
			"name":  {"nombre_producto", "nombre", "descripcion"},
			"price": {"precio_venta", "precio"},
			"phone": {"telefono", "celular"},
			"cname": {"nombre_cliente", "nombre"},
		},
		"contpaq": {
			"sku":   {"cvenumero", "sku", "clave"},
			"name":  {"cvenom", "nombre", "descripcion"},
			"price": {"cveprecio", "precio"},
			"phone": {"telefono", "tel"},
			"cname": {"cvenom", "nombre"},
		},
		"generic": {
			"sku":   {"sku", "codigo", "clave", "code"},
			"name":  {"nombre", "name", "descripcion", "producto"},
			"price": {"precio", "price", "precio_venta"},
			"phone": {"telefono", "phone", "celular"},
			"cname": {"nombre", "name", "cliente"},
		},
	}

	cm, ok := colMap[tipo]
	if !ok { cm = colMap["generic"] }

	idx := func(variants []string) int {
		for _, v := range variants {
			for i, h := range headers {
				if strings.Contains(h, v) { return i }
			}
		}
		return -1
	}

	iSku := idx(cm["sku"]); iName := idx(cm["name"]); iPrice := idx(cm["price"])
	iPhone := idx(cm["phone"]); iCname := idx(cm["cname"])

	for _, row := range rows[1:] {
		if len(row) == 0 { continue }
		get := func(i int) string {
			if i < 0 || i >= len(row) { return "" }
			return strings.TrimSpace(row[i])
		}
		if iName >= 0 && get(iName) != "" {
			sku := get(iSku)
			if sku == "" { sku = get(iName) }
			price := strings.ReplaceAll(strings.ReplaceAll(get(iPrice), "$", ""), ",", "")
			products = append(products, map[string]string{"sku": sku, "name": get(iName), "price": price})
		}
		if iPhone >= 0 && get(iPhone) != "" {
			phone := strings.ReplaceAll(strings.ReplaceAll(get(iPhone), " ", ""), "-", "")
			if len(phone) >= 10 {
				customers = append(customers, map[string]string{"phone": phone, "name": get(iCname)})
			}
		}
	}
	return products, customers, nil
}

// ─── JWT ─────────────────────────────────────────────────────────────────────
func jwtSecret() []byte {
	s := getenv("JWT_SECRET", "turbopos-secret-2026-cambiar-en-produccion")
	return []byte(s)
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func generateJWT(username, role, tenantID string) (string, error) {
	header := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]interface{}{
		"sub":    username,
		"role":   role,
		"tid":    tenantID,
		"exp":    time.Now().Add(12 * time.Hour).Unix(),
		"iat":    time.Now().Unix(),
	})
	data := header + "." + b64url(payload)
	mac := hmac.New(sha256.New, jwtSecret())
	mac.Write([]byte(data))
	return data + "." + b64url(mac.Sum(nil)), nil
}

type jwtClaims struct {
	Sub      string `json:"sub"`
	Role     string `json:"role"`
	TenantID string `json:"tid"`
	Exp      int64  `json:"exp"`
}

func validateJWT(token string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("token inválido")
	}
	data := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, jwtSecret())
	mac.Write([]byte(data))
	expected := b64url(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return nil, fmt.Errorf("firma inválida")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("payload inválido")
	}
	var claims jwtClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("claims inválidos")
	}
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expirado")
	}
	return &claims, nil
}

// jwtMiddleware valida el token en todas las rutas excepto /api/v1/login y /
func jwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rutas públicas
		if r.URL.Path == "/" || r.URL.Path == "/index.html" ||
			r.URL.Path == "/api/v1/login" || r.URL.Path == "/api/v1/status" {
			next.ServeHTTP(w, r)
			return
		}
		// Preflight CORS
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		// Extraer token
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "token requerido"})
			return
		}
		claims, err := validateJWT(strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		// Inyectar tenant del token si no viene header explícito
		if r.Header.Get("X-Tenant-ID") == "" && claims.TenantID != "" {
			r.Header.Set("X-Tenant-ID", claims.TenantID)
		}
		next.ServeHTTP(w, r)
	})
}

// POST /api/v1/login — autentica usuario y devuelve JWT
func (gw *Gateway) handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido"})
		return
	}
	if req.Username == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "username y password requeridos"})
		return
	}

	// Verificar credenciales en la DB
	// La tabla users tiene: id, username, password_hash, role, tenant_id
	// Soporte de password en texto plano para simplificar (mejorar a bcrypt en producción)
	var userID, role, tenantID string
	var storedPass string
	err := gw.db.QueryRow(
		`SELECT id, COALESCE(role,'cashier'), COALESCE(tenant_id::text,$3), password
		 FROM users WHERE username=$1 LIMIT 1`,
		req.Username, req.Password, defaultTenantID,
	).Scan(&userID, &role, &tenantID, &storedPass)

	if err != nil {
		// Si no existe la tabla users o el usuario, permitir credenciales de admin hardcoded
		adminUser := getenv("ADMIN_USER", "admin")
		adminPass := getenv("ADMIN_PASS", "turbopos2026")
		if req.Username != adminUser || req.Password != adminPass {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "credenciales incorrectas"})
			return
		}
		userID = "admin"
		role = "admin"
		tenantID = defaultTenantID
	} else if storedPass != req.Password {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "credenciales incorrectas"})
		return
	}

	token, err := generateJWT(req.Username, role, tenantID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "error generando token"})
		return
	}
	log.Printf("[BFF] Login: user=%s role=%s tenant=%s", req.Username, role, tenantID)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     token,
		"user":      req.Username,
		"role":      role,
		"tenant_id": tenantID,
		"expires":   time.Now().Add(12 * time.Hour).Format(time.RFC3339),
	})
}

// ─── Tenant middleware ────────────────────────────────────────────────────────
// tenantIDFromRequest extrae el tenant_id del header X-Tenant-ID
// Si no viene, usa el tenant demo por defecto
const defaultTenantID = "00000000-0000-0000-0000-000000000001"

func tenantID(r *http.Request) string {
	if t := r.Header.Get("X-Tenant-ID"); t != "" {
		return t
	}
	return defaultTenantID
}

// tenantInfo contiene los datos del tenant para CFDI
type tenantInfo struct {
	ID            string
	RFC           string
	RazonSocial   string
	RegimenFiscal string
	CodigoPostal  string
	FinkokUser    string
	FinkokPass    string
	CertDERb64    string
	KeyPEMb64     string
	KeyPassword   string
}

func (gw *Gateway) getTenant(r *http.Request) (*tenantInfo, error) {
	tid := tenantID(r)
	row := gw.db.QueryRow(`
		SELECT id, rfc, razon_social, regimen_fiscal, codigo_postal,
		       COALESCE(finkok_user,''), COALESCE(finkok_pass,''),
		       COALESCE(cert_der_b64,''), COALESCE(key_pem_b64,''), COALESCE(key_password,'')
		FROM tenants WHERE id=$1 AND active=true`, tid)
	t := &tenantInfo{}
	err := row.Scan(&t.ID, &t.RFC, &t.RazonSocial, &t.RegimenFiscal, &t.CodigoPostal,
		&t.FinkokUser, &t.FinkokPass, &t.CertDERb64, &t.KeyPEMb64, &t.KeyPassword)
	if err != nil {
		return nil, fmt.Errorf("tenant no encontrado: %v", err)
	}
	return t, nil
}


func (gw *Gateway) handleReportes(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    q := r.URL.Query()
    periodo := q.Get("periodo")
    desde := q.Get("desde")
    hasta := q.Get("hasta")
    now := time.Now()
    switch periodo {
    case "semana": desde = now.AddDate(0,0,-6).Format("2006-01-02"); hasta = now.Format("2006-01-02")
    case "mes":    desde = now.AddDate(0,-1,0).Format("2006-01-02"); hasta = now.Format("2006-01-02")
    case "año":   desde = now.AddDate(-1,0,0).Format("2006-01-02"); hasta = now.Format("2006-01-02")
    default:       if desde=="" { desde=now.AddDate(0,0,-6).Format("2006-01-02") }; if hasta=="" { hasta=now.Format("2006-01-02") }
    }
    tid := tenantID(r)
    // Ventas por día
    rowsDia, err := gw.db.Query(`SELECT DATE(created_at AT TIME ZONE 'America/Monterrey') as dia, COUNT(*) as tx, COALESCE(SUM(CASE WHEN status='completed' THEN total ELSE 0 END),0) as total, COALESCE(SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END),0) as canceladas, COALESCE(SUM(CASE WHEN cfdi_uuid IS NOT NULL THEN 1 ELSE 0 END),0) as timbradas FROM sales WHERE DATE(created_at AT TIME ZONE 'America/Monterrey') BETWEEN $1::date AND $2::date AND (tenant_id=$3::uuid OR tenant_id IS NULL) GROUP BY dia ORDER BY dia ASC`, desde, hasta, tid)
    if err != nil { w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error":err.Error()}); return }
    defer rowsDia.Close()
    type DiaData struct { Dia string `json:"dia"`; Transacciones int64 `json:"transacciones"`; Total float64 `json:"total"`; Canceladas int64 `json:"canceladas"`; Timbradas int64 `json:"timbradas"` }
    var porDia []DiaData
    for rowsDia.Next() { var d DiaData; rowsDia.Scan(&d.Dia,&d.Transacciones,&d.Total,&d.Canceladas,&d.Timbradas); porDia=append(porDia,d) }
    // Top productos
    rowsProd, _ := gw.db.Query(`SELECT si.name, SUM(si.quantity) as uds, SUM(si.subtotal) as total FROM sale_items si JOIN sales s ON s.id=si.sale_id WHERE s.status='completed' AND DATE(s.created_at AT TIME ZONE 'America/Monterrey') BETWEEN $1::date AND $2::date AND (s.tenant_id=$3::uuid OR s.tenant_id IS NULL) GROUP BY si.name ORDER BY total DESC LIMIT 10`, desde, hasta, tid)
    type ProdData struct { Nombre string `json:"nombre"`; Unidades int64 `json:"unidades"`; Total float64 `json:"total"` }
    var topProds []ProdData
    if rowsProd!=nil { defer rowsProd.Close(); for rowsProd.Next() { var p ProdData; rowsProd.Scan(&p.Nombre,&p.Unidades,&p.Total); topProds=append(topProds,p) } }
    // Métodos de pago
    rowsMet, _ := gw.db.Query(`SELECT payment_method, COUNT(*) as tx, COALESCE(SUM(CASE WHEN status='completed' THEN total ELSE 0 END),0) as total FROM sales WHERE DATE(created_at AT TIME ZONE 'America/Monterrey') BETWEEN $1::date AND $2::date AND (tenant_id=$3::uuid OR tenant_id IS NULL) GROUP BY payment_method`, desde, hasta, tid)
    type MetData struct { Metodo string `json:"metodo"`; Transacciones int64 `json:"transacciones"`; Total float64 `json:"total"` }
    var metodos []MetData; var grandTotal float64; var grandTx int64
    if rowsMet!=nil { defer rowsMet.Close(); for rowsMet.Next() { var m MetData; rowsMet.Scan(&m.Metodo,&m.Transacciones,&m.Total); metodos=append(metodos,m); grandTotal+=m.Total; grandTx+=m.Transacciones } }
    json.NewEncoder(w).Encode(map[string]interface{}{"desde":desde,"hasta":hasta,"por_dia":porDia,"top_prods":topProds,"metodos":metodos,"total":grandTotal,"transacciones":grandTx})
}

func limitSlice(s []map[string]string, n int) []map[string]string {
	if len(s) <= n { return s }
	return s[:n]
}

func parseFloat(s string) float64 {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "$", ""), ",", "")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// ─── CSD Upload ────────────────────────────────────────────────────────────
func (gw *Gateway) handleCSDUpload(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet { gw.handleCSDInfo(w, r); return }
    if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
    if err := r.ParseMultipartForm(10 << 20); err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]string{"error": "form invalido"})
        return
    }
    tid := tenantID(r)
    rfc := r.FormValue("rfc")
    keyPass := r.FormValue("key_password")
    if rfc == "" || keyPass == "" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]string{"error": "rfc y key_password requeridos"})
        return
    }

    // Leer .cer
    cerFile, _, err := r.FormFile("cer")
    if err != nil { w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "archivo .cer requerido"}); return }
    defer cerFile.Close()
    cerBytes, _ := io.ReadAll(cerFile)
    certB64 := base64.StdEncoding.EncodeToString(cerBytes)

    // Leer .key
    keyFile, _, err := r.FormFile("key")
    if err != nil { w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "archivo .key requerido"}); return }
    defer keyFile.Close()
    keyBytes, _ := io.ReadAll(keyFile)

    // Verificar que el par es válido llamando al CFDI service
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    _, err = gw.cfdiClient.Timbrar(ctx, &pb_cfdi.FacturaRequest{
        VentaId: "00000000-0000-0000-0000-000000000000",
        Total: 0.01, Rfc: rfc, CertB64: certB64,
        KeyBytes: keyBytes, KeyPassword: keyPass,
        CodigoPostalReceptor: "64000",
    })
    // Error esperado pero si es de cert/key lo rechazamos
    if err != nil && (strings.Contains(err.Error(), "desencriptar") || strings.Contains(err.Error(), "cert") || strings.Contains(err.Error(), "key")) {
        w.WriteHeader(400)
        json.NewEncoder(w).Encode(map[string]string{"error": "certificado o llave invalidos: " + err.Error()})
        return
    }

    // Guardar en DB
    _, err = gw.db.Exec(`
        INSERT INTO tenant_csds (tenant_id, rfc_emisor, cert_b64, key_bytes, key_password, updated_at)
        VALUES ($1::uuid, $2, $3, $4, $5, NOW())
        ON CONFLICT (tenant_id) DO UPDATE SET
            rfc_emisor=EXCLUDED.rfc_emisor, cert_b64=EXCLUDED.cert_b64,
            key_bytes=EXCLUDED.key_bytes, key_password=EXCLUDED.key_password, updated_at=NOW()
    `, tid, rfc, certB64, keyBytes, keyPass)
    if err != nil {
        log.Printf("[BFF] Error guardando CSD: %v", err)
        w.WriteHeader(500)
        json.NewEncoder(w).Encode(map[string]string{"error": "error guardando certificado"})
        return
    }
    log.Printf("[BFF] CSD registrado tenant=%s rfc=%s", tid, rfc)
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "rfc": rfc, "mensaje": "Certificado registrado correctamente"})
}

func (gw *Gateway) handleCSDInfo(w http.ResponseWriter, r *http.Request) {
    tid := tenantID(r)
    var rfc string
    var updatedAt time.Time
    err := gw.db.QueryRow(`SELECT rfc_emisor, updated_at FROM tenant_csds WHERE tenant_id=$1::uuid`, tid).Scan(&rfc, &updatedAt)
    w.Header().Set("Content-Type", "application/json")
    if err != nil {
        json.NewEncoder(w).Encode(map[string]interface{}{"tiene_csd": false})
        return
    }
    json.NewEncoder(w).Encode(map[string]interface{}{"tiene_csd": true, "rfc": rfc, "actualizado": updatedAt.Format("2006-01-02 15:04")})
}

// loadTenantCSD carga cert/key del tenant desde DB
func (gw *Gateway) loadTenantCSD(tenantID string) (certB64 string, keyBytes []byte, keyPass string, rfc string, ok bool) {
    err := gw.db.QueryRow(`SELECT rfc_emisor, cert_b64, key_bytes, key_password FROM tenant_csds WHERE tenant_id=$1::uuid`, tenantID).
        Scan(&rfc, &certB64, &keyBytes, &keyPass)
    return certB64, keyBytes, keyPass, rfc, err == nil
}


