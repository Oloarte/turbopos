// TurboPOS v10.1 - BFF (Backend For Frontend)
package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
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
	mux.HandleFunc("/api/v1/cobrar",          gw.handleCobrar)
	mux.HandleFunc("/api/v1/timbrar",         gw.handleTimbrar)
	mux.HandleFunc("/api/v1/status",          gw.handleStatus)
	mux.HandleFunc("/api/v1/logs",            gw.handleLogs)
	mux.HandleFunc("/api/v1/corte",           gw.handleCorte)
	mux.HandleFunc("/api/v1/loyalty",         gw.handleLoyalty)
	mux.HandleFunc("/api/v1/loyalty/redeem",  gw.handleLoyaltyRedeem)
	mux.HandleFunc("/api/v1/migrate/preview", gw.handleMigratePreview)
	mux.HandleFunc("/api/v1/migrate",         gw.handleMigrate)

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
			"phone": acc.GetPhone(), "customer_name": acc.GetName(),
			"points": acc.GetPoints(), "tier": acc.GetTier(),
			"total_spent": acc.GetTotalSpent(),
		},
		"history": history,
	})
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
			 ON CONFLICT (sku) DO UPDATE SET name=EXCLUDED.name, price=EXCLUDED.price`,
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

func (gw *Gateway) handleTimbrar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
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
		VentaId: req.SaleID, Total: req.Total, Rfc: req.RFC,
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
	rows, err := gw.db.Query(`
		SELECT COUNT(*), COALESCE(SUM(total),0),
			COALESCE(SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status='completed' THEN total ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN cfdi_uuid IS NOT NULL THEN 1 ELSE 0 END),0),
			payment_method
		FROM sales
		WHERE DATE(created_at AT TIME ZONE 'UTC') = $1::date
		GROUP BY payment_method`, fecha)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	type M struct {
		Metodo string `json:"metodo"`
		Transacciones int64 `json:"transacciones"`
		TotalVendido float64 `json:"total_vendido"`
		Canceladas int64 `json:"canceladas"`
		Neto float64 `json:"neto"`
		Timbradas int64 `json:"timbradas"`
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"fecha": fecha, "total_transacciones": totalTx,
		"total_vendido": totalVendido, "total_canceladas": totalCanceladas,
		"total_neto": totalNeto, "total_timbradas": totalTimbradas,
		"por_metodo": metodos, "generado_at": time.Now().Format(time.RFC3339),
	})
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

func limitSlice(s []map[string]string, n int) []map[string]string {
	if len(s) <= n { return s }
	return s[:n]
}

func parseFloat(s string) float64 {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "$", ""), ",", "")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
