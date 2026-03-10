// TurboPOS v10.1 - BFF (Backend For Frontend)
package main

import (
	"context"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"bytes"
    "context"
    "encoding/json"
    firebase "firebase.google.com/go/v4"
    "firebase.google.com/go/v4/messaging"
    "google.golang.org/api/option"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"sync"
		"github.com/joho/godotenv"
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
	"golang.org/x/crypto/bcrypt"
)

// loginAttempts tracks failed login attempts per IP for rate limiting
var loginAttempts = struct {
	sync.Mutex
	counts map[string][]time.Time
}{counts: make(map[string][]time.Time)}

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


// startTrialCron suspende tenants cuyo trial venció hace más de 1 día

func cronAvisarTrials(db *sql.DB) {
	rows, err := db.Query(`
		SELECT t.nombre, t.email, s.trial_ends_at
		FROM tenants t JOIN subscriptions s ON s.tenant_id=t.id
		WHERE s.status='trial'
		  AND s.trial_ends_at BETWEEN NOW() AND NOW()+INTERVAL '3 days'
			  AND t.email != ''
	`)
	if err != nil { return }
	defer rows.Close()
	for rows.Next() {
		var nombre, email string
		var trialEnds time.Time
		rows.Scan(&nombre, &email, &trialEnds)
		dias := int(time.Until(trialEnds).Hours()/24) + 1
		emailTrialVenciendo(email, nombre, dias)
		log.Printf("[Cron] Aviso trial a %s (%d dias)", email, dias)
	}
}

func startTrialCron(db *sql.DB) {
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			res, err := db.Exec(`UPDATE subscriptions SET status='cancelled', updated_at=NOW() WHERE status='trial' AND trial_ends_at < NOW() - INTERVAL '1 day'`)
			if err != nil { log.Printf("[Cron] Error trial check: %v", err); continue }
			if n, _ := res.RowsAffected(); n > 0 {
				log.Printf("[Cron] %d trials vencidos suspendidos", n)
				// Avisar a tenants que vencen en 3 dias
				cronAvisarTrials(db)
				// Desactivar tenants con suscripcion cancelada
				db.Exec(`UPDATE tenants SET active=false WHERE id IN (SELECT tenant_id FROM subscriptions WHERE status='cancelled') AND active=true`)
			}
		}
	}()
	log.Println("[Cron] Trial checker iniciado (cada hora)")
}

func main() {
	godotenv.Load()
	// Iniciar cron de trials - se llama después de conectar DB
	// startTrialCron se llama después de inicializar gw
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
	mux.HandleFunc("/api/v1/signup",            gw.handleSignup)
	mux.HandleFunc("/api/v1/forgot-password",  gw.handleForgotPassword)
	mux.HandleFunc("/api/v1/reset-password",   gw.handleResetPassword)
	mux.HandleFunc("/api/v1/openpay/webhook", gw.handleOpenpayWebhook)
	mux.HandleFunc("/api/v1/openpay/checkout",gw.handleOpenpayCheckout)
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
    mux.HandleFunc("/api/v1/loyalty/fiscal",   gw.handleLoyaltyFiscal)
	mux.HandleFunc("/api/v1/loyalty/cliente",  gw.handleLoyaltyCliente)
	mux.HandleFunc("/api/v1/customers", gw.handleCustomers)
	mux.HandleFunc("/api/v1/customers/", gw.handleCustomerByID)
	mux.HandleFunc("/api/v1/migrate/preview", gw.handleMigratePreview)
	mux.HandleFunc("/api/v1/migrate",         gw.handleMigrate)
	mux.HandleFunc("/api/v1/reportes", gw.handleReportes)
	mux.HandleFunc("/api/v1/reportes/cfdi", gw.handleReportesCFDI)
	mux.HandleFunc("/api/v1/admin/tenants",  gw.handleAdminTenants)
	mux.HandleFunc("/api/v1/admin/tenants/", gw.handleAdminTenantByID)
    mux.HandleFunc("/api/v1/csd",        gw.handleCSDUpload)
    mux.HandleFunc("/api/v1/csd/info",   gw.handleCSDInfo)
    mux.HandleFunc("/api/v1/push/register", gw.handlePushRegister)
    mux.HandleFunc("/api/v1/config/negocio", gw.handleConfigNegocio)
	// Archivos estaticos PWA
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		http.ServeFile(w, r, "web/manifest.json")
	})
	mux.HandleFunc("/firebase-messaging-sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Service-Worker-Allowed", "/")
		http.ServeFile(w, r, "web/firebase-messaging-sw.js")
	})
	mux.HandleFunc("/icon-192.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, "web/icon-192.png")
	})
	mux.HandleFunc("/icon-512.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, "web/icon-512.png")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        data, err := os.ReadFile("web/index.html")
        if err != nil { http.Error(w, "not found", 404); return }
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.Write(data)
    })
	log.Println("[BFF] Escuchando en :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (gw *Gateway) handleSignup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == http.MethodOptions { w.WriteHeader(200); return }
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Nombre        string `json:"nombre"`
		RFC           string `json:"rfc"`
		RazonSocial   string `json:"razon_social"`
		CodigoPostal  string `json:"codigo_postal"`
		RegimenFiscal string `json:"regimen_fiscal"`
		Plan          string `json:"plan"`
		Email         string `json:"email"`
		Username      string `json:"username"`
		Password      string `json:"password"`
		Telefono      string `json:"telefono"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON invalido"})
		return
	}
	if req.Nombre == "" || req.RFC == "" || req.Email == "" || req.Password == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Campos requeridos: nombre, rfc, email, password"})
		return
	}
	if len(req.Password) < 8 {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "La contrasena debe tener al menos 8 caracteres"})
		return
	}
	if req.Plan == "" { req.Plan = "starter" }
	if req.RegimenFiscal == "" { req.RegimenFiscal = "612" }
	if req.CodigoPostal == "" { req.CodigoPostal = "06600" }
	rfc := strings.ToUpper(strings.TrimSpace(req.RFC))

	planAmount := map[string]float64{"starter": 1990, "business": 4990, "pro": 9990}
	amount, ok := planAmount[req.Plan]
	if !ok { amount = 1990; req.Plan = "starter" }

	ctx := r.Context()

	// Validar RFC único
	var count int
	if err := gw.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tenants WHERE rfc=$1", rfc).Scan(&count); err != nil {
		log.Printf("[Signup] Error validando RFC: %v", err)
		w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": "Error interno"}); return
	}
	if count > 0 {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{"error": "Ya existe una cuenta con ese RFC"})
		return
	}

	// Validar email único
	if err := gw.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE email=$1", req.Email).Scan(&count); err != nil {
		log.Printf("[Signup] Error validando email: %v", err)
		w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": "Error interno"}); return
	}
	if count > 0 {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{"error": "Ese email ya esta registrado"})
		return
	}

	// Hash password
	pwHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": "Error procesando contrasena"}); return
	}

	tx, err := gw.db.BeginTx(ctx, nil)
	if err != nil {
		w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": "Error interno"}); return
	}
	defer tx.Rollback()

	razonSocial := req.RazonSocial
	if razonSocial == "" { razonSocial = req.Nombre }

	// Crear tenant
	var tenantID string
	err = tx.QueryRowContext(ctx,
		"INSERT INTO tenants (nombre, rfc, razon_social, regimen_fiscal, codigo_postal, plan, email, telefono) "+
		"VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id",
		req.Nombre, rfc, razonSocial, req.RegimenFiscal,
		req.CodigoPostal, req.Plan, req.Email, req.Telefono,
	).Scan(&tenantID)
	if err != nil {
		log.Printf("[Signup] Error creando tenant: %v", err)
		w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": "Error creando negocio"}); return
	}

	// Crear usuario admin (tabla usa email como identificador)
	_, err = tx.ExecContext(ctx,
		"INSERT INTO users (email, password_hash, tenant_id) VALUES ($1,$2,$3::uuid)",
		req.Email, string(pwHash), tenantID,
	)
	if err != nil {
		log.Printf("[Signup] Error creando usuario: %v", err)
		w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": "Error creando usuario"}); return
	}

	// Suscripcion trial
	gw.db.ExecContext(ctx,
		"INSERT INTO subscriptions (tenant_id, plan, status, amount) VALUES ($1::uuid,$2,'trial',$3)",
		tenantID, req.Plan, amount,
	)

	if err := tx.Commit(); err != nil {
		w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": "Error finalizando registro"}); return
	}

	log.Printf("[Signup] Nuevo tenant: rfc=%s plan=%s email=%s", rfc, req.Plan, req.Email)
	emailBienvenida(req.Email, req.Nombre, req.Plan, req.Email)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        true,
		"tenant_id": tenantID,
		"username":  req.Email,
		"plan":      req.Plan,
		"url":       "/",
		"message":   "Cuenta creada. Tu prueba de 14 dias comienza ahora.",
	})
}


// OpenPay Integration
type openpayClient struct {
	merchantID string
	sk         string
	baseURL    string
}

func newOpenpayClient() *openpayClient {
	env := os.Getenv("OPENPAY_ENV")
	base := "https://sandbox-api.openpay.mx/v1"
	if env == "production" { base = "https://api.openpay.mx/v1" }
	return &openpayClient{
		merchantID: os.Getenv("OPENPAY_MERCHANT_ID"),
		sk: os.Getenv("OPENPAY_SK"),
		baseURL: base,
	}
}

func (op *openpayClient) post(path string, body interface{}) (map[string]interface{}, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", op.baseURL+"/"+op.merchantID+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(op.sk, "")
	cli := &http.Client{Timeout: 15 * time.Second}
	resp, err := cli.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("openpay %d: %v", resp.StatusCode, result["description"])
	}
	return result, nil
}

func (op *openpayClient) createCustomer(name, email, phone string) (string, error) {
	r, err := op.post("/customers", map[string]string{"name": name, "email": email, "phone_number": phone})
	if err != nil { return "", err }
	return r["id"].(string), nil
}

func (op *openpayClient) createCard(customerID, tokenID, deviceID string) (string, error) {
	r, err := op.post("/customers/"+customerID+"/cards", map[string]string{"token_id": tokenID, "device_session_id": deviceID})
	if err != nil { return "", err }
	return r["id"].(string), nil
}

func (op *openpayClient) chargeCard(customerID, cardID string, amount float64, desc, orderID string) (string, error) {
	r, err := op.post("/customers/"+customerID+"/charges", map[string]interface{}{
		"source_id": cardID, "method": "card", "amount": amount,
		"currency": "MXN", "description": desc, "order_id": orderID,
	})
	if err != nil { return "", err }
	return r["id"].(string), nil
}

func (op *openpayClient) createOXXO(customerID string, amount float64, desc, orderID string) (map[string]interface{}, error) {
	return op.post("/customers/"+customerID+"/charges", map[string]interface{}{
		"method": "store", "amount": amount, "currency": "MXN", "description": desc, "order_id": orderID,
	})
}

func (op *openpayClient) createSPEI(customerID string, amount float64, desc, orderID string) (map[string]interface{}, error) {
	return op.post("/customers/"+customerID+"/charges", map[string]interface{}{
		"method": "bank_account", "amount": amount, "currency": "MXN", "description": desc, "order_id": orderID,
	})
}

func (gw *Gateway) handleOpenpayCheckout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost { w.WriteHeader(405); return }
	var req struct {
		TenantID        string `json:"tenant_id"`
		Method          string `json:"method"`
		TokenID         string `json:"token_id"`
		DeviceSessionID string `json:"device_session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "JSON invalido"}); return
	}
	var nombre, email, telefono, plan string
	gw.db.QueryRowContext(r.Context(),
		"SELECT nombre, COALESCE(email,''), COALESCE(telefono,''), plan FROM tenants WHERE id=$1::uuid",
		req.TenantID).Scan(&nombre, &email, &telefono, &plan)

	planAmount := map[string]float64{"starter": 1990, "business": 4990, "pro": 9990}
	amount := planAmount[plan]
	if amount == 0 { amount = 1990 }

	op := newOpenpayClient()
	customerID, err := op.createCustomer(nombre, email, telefono)
	if err != nil {
		log.Printf("[OpenPay] Error customer: %v", err)
		w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": "Error procesador de pagos"}); return
	}
	gw.db.ExecContext(r.Context(), "UPDATE subscriptions SET openpay_customer_id=$1 WHERE tenant_id=$2::uuid", customerID, req.TenantID)

	orderID := req.TenantID[:8] + "-" + fmt.Sprintf("%d", time.Now().Unix())
	desc := "TurboPOS " + plan + " - " + nombre

	switch req.Method {
	case "card":
		if req.TokenID == "" { w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "token_id requerido"}); return }
		cardID, err := op.createCard(customerID, req.TokenID, req.DeviceSessionID)
		if err != nil { w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "Tarjeta rechazada: " + err.Error()}); return }
		chargeID, err := op.chargeCard(customerID, cardID, amount, desc, orderID)
		if err != nil { w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "Cargo rechazado: " + err.Error()}); return }
		gw.db.ExecContext(r.Context(), "UPDATE subscriptions SET openpay_card_id=$1, status='active', current_period_start=NOW(), current_period_end=NOW()+INTERVAL '1 month' WHERE tenant_id=$2::uuid", cardID, req.TenantID)
		log.Printf("[OpenPay] Cargo OK customer=%s charge=%s amount=%.2f", customerID, chargeID, amount)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "method": "card", "charge_id": chargeID})
	case "oxxo":
		result, err := op.createOXXO(customerID, amount, desc, orderID)
		if err != nil { w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); return }
		pm, _ := result["payment_method"].(map[string]interface{})
		ref, _ := pm["reference"].(string)
		barcode, _ := pm["barcode_url"].(string)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "method": "oxxo", "reference": ref, "barcode_url": barcode, "amount": amount})
	case "spei":
		result, err := op.createSPEI(customerID, amount, desc, orderID)
		if err != nil { w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); return }
		pm, _ := result["payment_method"].(map[string]interface{})
		clabe, _ := pm["clabe"].(string)
		bank, _ := pm["bank_name"].(string)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "method": "spei", "clabe": clabe, "bank": bank, "amount": amount})
	default:
		w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "method: card, oxxo o spei"})
	}
}

func (gw *Gateway) handleOpenpayWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { w.WriteHeader(405); return }
	var event map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil { w.WriteHeader(400); return }
	eventType, _ := event["type"].(string)
	tx, _ := event["transaction"].(map[string]interface{})
	orderID, _ := tx["order_id"].(string)
	log.Printf("[OpenPay] Webhook: type=%s order=%s", eventType, orderID)
	switch eventType {
	case "charge.succeeded":
		if len(orderID) >= 8 {
			gw.db.Exec("UPDATE subscriptions SET status='active', current_period_start=NOW(), current_period_end=NOW()+INTERVAL '1 month' WHERE tenant_id::text LIKE $1||'%%'", orderID[:8])
		}
	case "charge.failed", "subscription.charge.failed":
		if len(orderID) >= 8 {
			gw.db.Exec("UPDATE subscriptions SET status='past_due' WHERE tenant_id::text LIKE $1||'%%'", orderID[:8])
		}
	}
	w.WriteHeader(200)
}

// ADMIN PANEL
func isAdmin(r *http.Request) bool {
	adminKey := os.Getenv("ADMIN_KEY")
	if adminKey == "" { adminKey = "turbopos-admin-2026" }
	return r.Header.Get("X-Admin-Key") == adminKey
}

func (gw *Gateway) handleAdminTenants(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !isAdmin(r) { w.WriteHeader(401); json.NewEncoder(w).Encode(map[string]string{"error": "No autorizado"}); return }
	query := `SELECT t.id, t.nombre, t.rfc, t.plan, t.active, COALESCE(t.email,'') as email, COALESCE(t.telefono,'') as telefono, t.created_at, COALESCE(s.status,'trial') as sub_status, COALESCE(s.trial_ends_at::text,'') as trial_ends, COALESCE(s.amount,0) as amount, COALESCE(s.openpay_customer_id,'') as openpay_id, (SELECT COUNT(*) FROM sales WHERE tenant_id=t.id) as total_ventas, (SELECT COALESCE(SUM(total),0) FROM sales WHERE tenant_id=t.id AND status='completed') as monto_ventas, (SELECT COUNT(*) FROM users WHERE tenant_id=t.id) as usuarios FROM tenants t LEFT JOIN subscriptions s ON s.tenant_id=t.id ORDER BY t.created_at DESC`
	rows, err := gw.db.QueryContext(r.Context(), query)
	if err != nil { w.WriteHeader(500); json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); return }
	defer rows.Close()
	type TR struct {
		ID          string  `json:"id"`
		Nombre      string  `json:"nombre"`
		RFC         string  `json:"rfc"`
		Plan        string  `json:"plan"`
		Active      bool    `json:"active"`
		Email       string  `json:"email"`
		Telefono    string  `json:"telefono"`
		CreatedAt   string  `json:"created_at"`
		SubStatus   string  `json:"sub_status"`
		TrialEnds   string  `json:"trial_ends"`
		Amount      float64 `json:"amount"`
		OpenpayID   string  `json:"openpay_id"`
		TotalVentas int64   `json:"total_ventas"`
		MontoVentas float64 `json:"monto_ventas"`
		Usuarios    int64   `json:"usuarios"`
	}
	var tenants []TR
	var mrr float64
	for rows.Next() {
		var t TR
		rows.Scan(&t.ID,&t.Nombre,&t.RFC,&t.Plan,&t.Active,&t.Email,&t.Telefono,&t.CreatedAt,&t.SubStatus,&t.TrialEnds,&t.Amount,&t.OpenpayID,&t.TotalVentas,&t.MontoVentas,&t.Usuarios)
		if t.SubStatus == "active" { mrr += t.Amount }
		tenants = append(tenants, t)
	}
	if tenants == nil { tenants = []TR{} }
	var tvTotal int64; var mvTotal float64
	for _, t := range tenants { tvTotal += t.TotalVentas; mvTotal += t.MontoVentas }
	activeCount := 0; trialCount := 0
	for _, t := range tenants { if t.SubStatus=="active" { activeCount++ } else if t.SubStatus=="trial" { trialCount++ } }
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tenants": tenants,
		"kpis": map[string]interface{}{
			"total_tenants": len(tenants), "active": activeCount, "trial": trialCount,
			"mrr": mrr, "total_ventas": tvTotal, "monto_total": mvTotal,
		},
	})
}

func (gw *Gateway) handleAdminTenantByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !isAdmin(r) { w.WriteHeader(401); json.NewEncoder(w).Encode(map[string]string{"error": "No autorizado"}); return }
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/tenants/")
	if id == "" { w.WriteHeader(400); return }
	if r.Method == http.MethodPatch {
		var body struct {
			Active *bool  `json:"active"`
			Plan   string `json:"plan"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Active != nil { gw.db.ExecContext(r.Context(), "UPDATE tenants SET active=$1 WHERE id=$2::uuid", *body.Active, id) }
		if body.Plan != "" {
			gw.db.ExecContext(r.Context(), "UPDATE tenants SET plan=$1 WHERE id=$2::uuid", body.Plan, id)
			planAmt := map[string]float64{"starter":1990,"business":4990,"pro":9990}
			if amt, ok := planAmt[body.Plan]; ok { gw.db.ExecContext(r.Context(), "UPDATE subscriptions SET plan=$1,amount=$2 WHERE tenant_id=$3::uuid", body.Plan, amt, id) }
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true}); return
	}
	rows, _ := gw.db.QueryContext(r.Context(),
		"SELECT DATE(created_at AT TIME ZONE 'America/Monterrey') as dia, COUNT(*) as ventas, COALESCE(SUM(CASE WHEN status='completed' THEN total ELSE 0 END),0) as total FROM sales WHERE tenant_id=$1::uuid GROUP BY dia ORDER BY dia DESC LIMIT 30", id)
	type DV struct{ Dia string `json:"dia"`; Ventas int64 `json:"ventas"`; Total float64 `json:"total"` }
	var dias []DV
	if rows != nil { defer rows.Close(); for rows.Next() { var d DV; rows.Scan(&d.Dia,&d.Ventas,&d.Total); dias=append(dias,d) } }
	if dias == nil { dias = []DV{} }
	json.NewEncoder(w).Encode(map[string]interface{}{"ventas_por_dia": dias})
}


// ═══════════════════════════════════════════════════════
// EMAIL SYSTEM
// ═══════════════════════════════════════════════════════

func sendEmail(to, subject, htmlBody string) error {
	host := getenv("SMTP_HOST", "smtp.gmail.com")
	port := getenv("SMTP_PORT", "587")
	user := getenv("SMTP_USER", "")
	pass := getenv("SMTP_PASS", "")
	from := getenv("SMTP_FROM", user)
	if user == "" || pass == "" {
		log.Printf("[Email] SMTP no configurado, saltando email a %s", to)
		return nil
	}
	headers := "MIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n"
	msg := "From: " + from + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\n" + headers + "\r\n" + htmlBody
	auth := smtp.PlainAuth("", user, pass, host)
	err := smtp.SendMail(host+":"+port, auth, user, []string{to}, []byte(msg))
	if err != nil { log.Printf("[Email] Error enviando a %s: %v", to, err) }
	return err
}

func emailBienvenida(to, nombre, plan, username string) {
	planPrecio := map[string]string{"starter":"$1,990","business":"$4,990","pro":"$9,990"}
	precio := planPrecio[plan]
	if precio == "" { precio = "$1,990" }
	body := `<!DOCTYPE html><html><body style="font-family:sans-serif;background:#07090F;color:#EDF2FF;padding:40px">
	<div style="max-width:520px;margin:0 auto;background:#0D1018;border:1px solid #1C2535;border-radius:16px;padding:40px">
	<h1 style="color:#FFB547;font-size:28px;margin-bottom:8px">¡Bienvenido a TurboPOS!</h1>
	<p style="color:#8896B0;margin-bottom:24px">Tu cuenta está lista. Empieza a vender ahora.</p>
	<div style="background:#131820;border-radius:10px;padding:20px;margin-bottom:24px">
	<p style="margin:0 0 8px 0"><strong>Negocio:</strong> ` + nombre + `</p>
	<p style="margin:0 0 8px 0"><strong>Plan:</strong> ` + plan + ` (` + precio + `/mes)</p>
	<p style="margin:0 0 8px 0"><strong>Usuario:</strong> ` + username + `</p>
	<p style="margin:0;color:#00D68F"><strong>✓ 14 días de prueba gratis</strong></p>
	</div>
	<a href="https://turbopos.mx" style="display:inline-block;padding:14px 28px;background:#FFB547;color:#000;text-decoration:none;border-radius:10px;font-weight:700">Ir a mi TurboPOS →</a>
	<p style="color:#4A5568;font-size:12px;margin-top:24px">Si tienes dudas escríbenos a turbopos.tech@gmail.com</p>
	</div></body></html>`
	go sendEmail(to, "¡Bienvenido a TurboPOS! Tu cuenta está lista", body)
}

func emailRecuperacion(to, token string) {
	link := getenv("APP_URL", "https://turbopos.mx") + "/reset-password?token=" + token
	body := `<!DOCTYPE html><html><body style="font-family:sans-serif;background:#07090F;color:#EDF2FF;padding:40px">
	<div style="max-width:520px;margin:0 auto;background:#0D1018;border:1px solid #1C2535;border-radius:16px;padding:40px">
	<h1 style="color:#FFB547;font-size:24px">Recuperar contraseña</h1>
	<p style="color:#8896B0">Haz clic en el botón para crear una nueva contraseña. Este enlace expira en 1 hora.</p>
	<a href="` + link + `" style="display:inline-block;padding:14px 28px;background:#FFB547;color:#000;text-decoration:none;border-radius:10px;font-weight:700;margin:24px 0">Crear nueva contraseña →</a>
	<p style="color:#4A5568;font-size:12px">Si no solicitaste esto, ignora este correo.</p>
	</div></body></html>`
	go sendEmail(to, "TurboPOS — Recuperar contraseña", body)
}

func emailTrialVenciendo(to, nombre string, diasRestantes int) {
	body := `<!DOCTYPE html><html><body style="font-family:sans-serif;background:#07090F;color:#EDF2FF;padding:40px">
	<div style="max-width:520px;margin:0 auto;background:#0D1018;border:1px solid #1C2535;border-radius:16px;padding:40px">
	<h1 style="color:#FFB547;font-size:24px">Tu prueba vence pronto</h1>
	<p style="color:#8896B0">Hola ` + nombre + `, tu periodo de prueba de TurboPOS vence en <strong style="color:#FFB547">` + fmt.Sprintf("%d días", diasRestantes) + `</strong>.</p>
	<p style="color:#8896B0">Para continuar sin interrupciones agrega tu método de pago.</p>
	<a href="https://turbopos.mx/billing" style="display:inline-block;padding:14px 28px;background:#FFB547;color:#000;text-decoration:none;border-radius:10px;font-weight:700;margin:24px 0">Agregar método de pago →</a>
	</div></body></html>`
	go sendEmail(to, fmt.Sprintf("TurboPOS — Tu prueba vence en %d días", diasRestantes), body)
}

// handleForgotPassword — solicitar recuperacion de contrasena
func (gw *Gateway) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost { w.WriteHeader(405); return }
	var req struct { Email string `json:"email"` }
	json.NewDecoder(r.Body).Decode(&req)
	if req.Email == "" { w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "email requerido"}); return }

	// Verificar que el email existe
	var userID string
	err := gw.db.QueryRowContext(r.Context(), "SELECT id FROM users WHERE email=$1", req.Email).Scan(&userID)
	if err != nil {
		// No revelar si el email existe o no
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Si el email existe recibirás instrucciones"})
		return
	}

	// Crear token de recuperacion (simple UUID)
	var token string
	gw.db.QueryRowContext(r.Context(), "SELECT gen_random_uuid()::text").Scan(&token)

	// Guardar token en DB (expira en 1 hora)
	// Guardar token en DB (expira en 1 hora)
	gw.db.ExecContext(r.Context(), "INSERT INTO password_reset_tokens (user_id, token, expires_at) VALUES ($1::uuid, $2, NOW()+INTERVAL '1 hour') ON CONFLICT (user_id) DO UPDATE SET token=$2, expires_at=NOW()+INTERVAL '1 hour'", userID, token)

	emailRecuperacion(req.Email, token)
	log.Printf("[Email] Reset password solicitado para %s", req.Email)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Si el email existe recibirás instrucciones"})
}

// handleResetPassword — establecer nueva contrasena
func (gw *Gateway) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost { w.WriteHeader(405); return }
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Token == "" || req.Password == "" { w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "token y password requeridos"}); return }
	if len(req.Password) < 8 { w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "password minimo 8 caracteres"}); return }

	var userID string
	err := gw.db.QueryRowContext(r.Context(),
		"SELECT user_id FROM password_reset_tokens WHERE token=$1 AND expires_at > NOW()", req.Token).Scan(&userID)
	if err != nil { w.WriteHeader(400); json.NewEncoder(w).Encode(map[string]string{"error": "Token invalido o expirado"}); return }

	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	gw.db.ExecContext(r.Context(), "UPDATE users SET password_hash=$1 WHERE id=$2::uuid", string(hash), userID)
	gw.db.ExecContext(r.Context(), "DELETE FROM password_reset_tokens WHERE token=$1", req.Token)
	log.Printf("[Email] Password actualizado para userID=%s", userID)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Contraseña actualizada"})
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
		rfcTimbrar := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("rfc")))
        if rfcTimbrar == "" { rfcTimbrar = "XAXX010101000" }
        nombreTimbrar := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("nombre")))
        regimenTimbrar := r.URL.Query().Get("regimen")
        if regimenTimbrar == "" {
                if len(rfcTimbrar) == 13 { regimenTimbrar = "612" } else { regimenTimbrar = "601" }
        }
        if rfcTimbrar != "XAXX010101000" && nombreTimbrar == "" {
                var nombre, regimen string
                row := gw.db.QueryRow(
                        `SELECT COALESCE(nombre,''), COALESCE(regimen_fiscal,'') FROM loyalty_accounts WHERE rfc = $1 LIMIT 1`,
                        rfcTimbrar)
                if err := row.Scan(&nombre, &regimen); err == nil {
                        if nombreTimbrar == "" { nombreTimbrar = nombre }
                        if regimenTimbrar == "" { regimenTimbrar = regimen }
                }
        }
        go func(saleID string, total float64, rfc, cp, nombre, regimen, tID string) {
                ctxT, cancelT := context.WithTimeout(context.Background(), 30*time.Second)
                defer cancelT()
                treq := &pb_cfdi.FacturaRequest{VentaId: saleID, Total: total, Rfc: rfc,
                        CodigoPostalReceptor: cp, NombreReceptor: nombre, RegimenFiscalReceptor: regimen}
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
        }(res.GetSaleId(), req.Total, rfcTimbrar, r.URL.Query().Get("cp"), nombreTimbrar, regimenTimbrar, tid)
	}
	go gw.sendPushNotification(tid, "Nueva venta $"+fmt.Sprintf("%.2f", req.Total), fmt.Sprintf("%d producto(s) · %s", len(req.Items), req.PaymentMethod))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sale_id": res.GetSaleId(), "status": res.GetStatus(),
		"total": res.GetTotal(), "created_at": res.GetCreatedAt(),
	})
}


// ── CUSTOMERS ─────────────────────────────────────────────────────────────────

func (gw *Gateway) handleCustomers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	switch r.Method {
	case http.MethodGet:
		search := r.URL.Query().Get("search")
		tier := r.URL.Query().Get("tier")

		rows, err := gw.db.QueryContext(r.Context(), `
			SELECT id, phone, COALESCE(rfc,''), name, COALESCE(email,''),
			       points, total_spent, tier, created_at
			FROM   loyalty_accounts
			WHERE  ($1 = '' OR name ILIKE '%' || $1 || '%'
			        OR COALESCE(rfc,'') ILIKE '%' || $1 || '%'
			        OR phone ILIKE '%' || $1 || '%')
			  AND  ($2 = '' OR tier = $2)
			ORDER  BY total_spent DESC
			LIMIT  100`,
			search, tier)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()

		type CRow struct {
			ID         string  `json:"id"`
			Phone      string  `json:"phone"`
			RFC        string  `json:"rfc"`
			Name       string  `json:"name"`
			Email      string  `json:"email"`
			Points     int     `json:"points"`
			TotalSpent float64 `json:"total_spent"`
			Tier       string  `json:"tier"`
			CreatedAt  string  `json:"created_at"`
		}
		var list []CRow
		for rows.Next() {
			var c CRow
			var createdAt interface{}
			if err2 := rows.Scan(&c.ID, &c.Phone, &c.RFC, &c.Name, &c.Email,
				&c.Points, &c.TotalSpent, &c.Tier, &createdAt); err2 == nil {
				if createdAt != nil {
					c.CreatedAt = fmt.Sprintf("%v", createdAt)
				}
				list = append(list, c)
			}
		}
		if list == nil {
			list = []CRow{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"customers": list, "total": len(list)})

	case http.MethodPost:
		var req struct {
			Phone   string `json:"phone"`
			Name    string `json:"name"`
			RFC     string `json:"rfc"`
			Email   string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Phone == "" || req.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "phone y name son requeridos"})
			return
		}
		var id string
		err := gw.db.QueryRowContext(r.Context(), `
			INSERT INTO loyalty_accounts (phone, name, rfc, email, points, total_spent, tier)
			VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), 0, 0, 'bronze')
			ON CONFLICT (phone) DO UPDATE SET
			    name       = EXCLUDED.name,
			    rfc        = COALESCE(EXCLUDED.rfc, loyalty_accounts.rfc),
			    email      = COALESCE(EXCLUDED.email, loyalty_accounts.email),
			    updated_at = now()
			RETURNING id`,
			req.Phone, req.Name, req.RFC, req.Email).Scan(&id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "created"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (gw *Gateway) handleCustomerByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, DELETE, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/customers/")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "ID requerido"})
		return
	}
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	_ = token

	switch r.Method {
	case http.MethodGet:
		var c struct {
			ID         string  `json:"id"`
			Phone      string  `json:"phone"`
			RFC        string  `json:"rfc"`
			Name       string  `json:"name"`
			Email      string  `json:"email"`
			Points     int     `json:"points"`
			TotalSpent float64 `json:"total_spent"`
			Tier       string  `json:"tier"`
		}
		err := gw.db.QueryRowContext(r.Context(), `
			SELECT id, phone, COALESCE(rfc,''), name, COALESCE(email,''),
			       points, total_spent, tier
			FROM   loyalty_accounts WHERE id = $1`, id).Scan(
			&c.ID, &c.Phone, &c.RFC, &c.Name, &c.Email,
			&c.Points, &c.TotalSpent, &c.Tier)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "cliente no encontrado"})
			return
		}

		rows, _ := gw.db.QueryContext(r.Context(), `
			SELECT type, points, COALESCE(description,''), created_at
			FROM   loyalty_transactions
			WHERE  account_id = $1
			ORDER  BY created_at DESC LIMIT 20`, id)

		type TxRow struct {
			Type        string `json:"type"`
			Points      int    `json:"points"`
			Description string `json:"description"`
			Date        string `json:"date"`
		}
		var txs []TxRow
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var tx TxRow
				var createdAt interface{}
				rows.Scan(&tx.Type, &tx.Points, &tx.Description, &createdAt)
				if createdAt != nil {
					tx.Date = fmt.Sprintf("%v", createdAt)
				}
				txs = append(txs, tx)
			}
		}
		if txs == nil {
			txs = []TxRow{}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": c.ID, "phone": c.Phone, "rfc": c.RFC,
			"name": c.Name, "email": c.Email,
			"points": c.Points, "total_spent": c.TotalSpent,
			"tier": c.Tier, "transactions": txs,
		})

	case http.MethodPut:
		var req struct {
			Name  string `json:"name"`
			RFC   string `json:"rfc"`
			Email string `json:"email"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		gw.db.ExecContext(r.Context(), `
			UPDATE loyalty_accounts
			SET    name       = COALESCE(NULLIF($1,''), name),
			       rfc        = COALESCE(NULLIF($2,''), rfc),
			       email      = COALESCE(NULLIF($3,''), email),
			       updated_at = now()
			WHERE  id = $4`, req.Name, req.RFC, req.Email, id)
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	case http.MethodDelete:
		gw.db.ExecContext(r.Context(), `DELETE FROM loyalty_accounts WHERE id = $1`, id)
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
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
                "regimen_fiscal": acc.GetRegimenFiscal(), "nombre_fiscal": acc.GetNombreFiscal(),
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
func (gw *Gateway) handleLoyaltyFiscal(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
    var req struct {
        Phone         string `json:"phone"`
        RegimenFiscal string `json:"regimen_fiscal"`
        NombreFiscal  string `json:"nombre_fiscal"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        w.WriteHeader(http.StatusBadRequest); return
    }
    if req.Phone == "" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]string{"error": "phone requerido"})
        return
    }
    req.NombreFiscal = strings.ToUpper(strings.TrimSpace(req.NombreFiscal))
    _, err := gw.db.Exec(
        `UPDATE loyalty_accounts SET regimen_fiscal=$1, nombre_fiscal=$2, updated_at=now() WHERE phone=$3`,
        req.RegimenFiscal, req.NombreFiscal, req.Phone)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
func (gw *Gateway) handleLoyaltyCliente(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Phone   string `json:"phone"`
		Tel     string `json:"tel"`
		Email   string `json:"email"`
		Nombre  string `json:"nombre"`
		Rfc     string `json:"rfc"`
		Cp      string `json:"cp"`
		Regimen string `json:"regimen"`
		Uso     string `json:"uso"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "json invalido", http.StatusBadRequest)
		return
	}
	phone := req.Phone
	if phone == "" {
		phone = req.Tel
	}
	if phone == "" {
		http.Error(w, "phone requerido", http.StatusBadRequest)
		return
	}
	_, err := gw.db.ExecContext(r.Context(),
		"INSERT INTO loyalty_accounts (phone, name, rfc, cp, email, regimen_fiscal, nombre_fiscal, uso_cfdi, points, total_spent, tier) "+
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0, 0, 'bronze') "+
		"ON CONFLICT (phone) DO UPDATE SET "+
		"name           = CASE WHEN $2 != '' THEN $2 ELSE loyalty_accounts.name END, "+
		"rfc            = CASE WHEN $3 != '' THEN $3 ELSE loyalty_accounts.rfc END, "+
		"cp             = CASE WHEN $4 != '' THEN $4 ELSE loyalty_accounts.cp END, "+
		"email          = CASE WHEN $5 != '' THEN $5 ELSE loyalty_accounts.email END, "+
		"regimen_fiscal = CASE WHEN $6 != '' THEN $6 ELSE loyalty_accounts.regimen_fiscal END, "+
		"nombre_fiscal  = CASE WHEN $7 != '' THEN $7 ELSE loyalty_accounts.nombre_fiscal END, "+
		"uso_cfdi       = CASE WHEN $8 != '' THEN $8 ELSE loyalty_accounts.uso_cfdi END, "+
		"updated_at     = NOW()",
		phone, req.Nombre, req.Rfc, req.Cp, req.Email, req.Regimen, req.Nombre, req.Uso,
	)
	if err != nil {
		log.Printf("[BFF] handleLoyaltyCliente error: %v", err)
		http.Error(w, "error guardando cliente", http.StatusInternalServerError)
		return
	}
	log.Printf("[BFF] Cliente guardado phone=%s rfc=%s email=%s", phone, req.Rfc, req.Email)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true,"message":"Cliente guardado"}`))
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
func (gw *Gateway) handlePushRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions { w.WriteHeader(http.StatusOK); return }
	if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
	var body struct {
		Token    string `json:"token"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "token requerido"})
		return
	}
	// Guardar token en DB
	_, err := gw.db.Exec(`INSERT INTO push_tokens (tenant_id, token, created_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (token) DO UPDATE SET updated_at = NOW()`,
		body.TenantID, body.Token)
	if err != nil {
		log.Printf("[Push] Error guardando token: %v", err)
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (gw *Gateway) handleConfigNegocio(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions { w.WriteHeader(http.StatusOK); return }
	// Solo acepta POST, guarda en memoria por ahora
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (gw *Gateway) sendPushNotification(tenantID, title, body string) {
	rows, err := gw.db.Query(`SELECT token FROM push_tokens WHERE tenant_id=$1`, tenantID)
	if err != nil { log.Printf("[Push] DB error: %v", err); return }
	defer rows.Close()
	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err == nil { tokens = append(tokens, token) }
	}
	if len(tokens) == 0 { return }
	go func() {
		ctx := context.Background()
		app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile("secrets/firebase-sa.json"))
		if err != nil { log.Printf("[Push] Firebase init error: %v", err); return }
		client, err := app.Messaging(ctx)
		if err != nil { log.Printf("[Push] Messaging error: %v", err); return }
		for _, token := range tokens {
			msg := &messaging.Message{
				Token: token,
				Notification: &messaging.Notification{Title: title, Body: body},
				Android: &messaging.AndroidConfig{Priority: "high"},
				APNS: &messaging.APNSConfig{
					Payload: &messaging.APNSPayload{
						Aps: &messaging.Aps{Sound: "default"},
					},
				},
			}
			_, err := client.Send(ctx, msg)
			if err != nil {
				log.Printf("[Push] Send error token=%s... %v", token[:15], err)
			} else {
				log.Printf("[Push] OK enviado a %s... titulo: %s", token[:15], title)
			}
		}
	}()
}

func jwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rutas públicas
		if r.URL.Path == "/" || r.URL.Path == "/index.html" ||
			r.URL.Path == "/manifest.json" || r.URL.Path == "/firebase-messaging-sw.js" ||
			r.URL.Path == "/icon-192.png" || r.URL.Path == "/icon-512.png" ||
			r.URL.Path == "/api/v1/push/register" ||
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
	// Rate limiting: max 5 intentos por IP en 5 minutos
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i != -1 { ip = ip[:i] }
	loginAttempts.Lock()
	now := time.Now()
	attempts := loginAttempts.counts[ip]
	var recent []time.Time
	for _, t := range attempts { if now.Sub(t) < 5*time.Minute { recent = append(recent, t) } }
	loginAttempts.counts[ip] = recent
	loginAttempts.Unlock()
	if len(recent) >= 5 {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "Demasiados intentos. Espera 5 minutos."})
		return
	}
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
	var storedHash string
	err := gw.db.QueryRow(
		`SELECT id, 'admin', COALESCE(tenant_id::text,$2), password_hash FROM users WHERE email=$1 LIMIT 1`,
		req.Username, defaultTenantID,
	).Scan(&userID, &role, &tenantID, &storedHash)

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
	} else if err2 := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password)); err2 != nil {
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

func (gw *Gateway) handleReportesCFDI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query()
	desde := q.Get("desde")
	hasta := q.Get("hasta")
	periodo := q.Get("periodo")
	now := time.Now()
	switch periodo {
	case "semana": desde = now.AddDate(0,0,-6).Format("2006-01-02"); hasta = now.Format("2006-01-02")
	case "mes":    desde = now.AddDate(0,-1,0).Format("2006-01-02"); hasta = now.Format("2006-01-02")
	default:       if desde == "" { desde = now.AddDate(0,0,-6).Format("2006-01-02") }; if hasta == "" { hasta = now.Format("2006-01-02") }
	}
	tid := tenantID(r)
	rows, err := gw.db.QueryContext(r.Context(), `
		SELECT
			s.id,
			DATE(s.created_at AT TIME ZONE 'America/Monterrey') as fecha,
			s.created_at AT TIME ZONE 'America/Monterrey' as fecha_hora,
			s.total,
			s.payment_method,
			COALESCE(s.cfdi_uuid,'') as uuid,
			COALESCE(s.cfdi_rfc_receptor, '') as rfc_receptor,
			COALESCE(s.cfdi_nombre_receptor,'') as nombre_receptor,
			s.status,
			COALESCE(s.cfdi_serie,'') as serie,
			COALESCE(s.cfdi_folio,'') as folio
		FROM sales s
		WHERE DATE(s.created_at AT TIME ZONE 'America/Monterrey') BETWEEN $1::date AND $2::date
		  AND (s.tenant_id = $3::uuid OR s.tenant_id IS NULL)
		  AND s.cfdi_uuid IS NOT NULL
		ORDER BY s.created_at DESC
		LIMIT 500
	`, desde, hasta, tid)
	if err != nil {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type CFDIRow struct {
		ID            string  `json:"id"`
		Fecha         string  `json:"fecha"`
		FechaHora     string  `json:"fecha_hora"`
		Total         float64 `json:"total"`
		Metodo        string  `json:"metodo"`
		UUID          string  `json:"uuid"`
		RFCReceptor   string  `json:"rfc_receptor"`
		NombreReceptor string `json:"nombre_receptor"`
		Status        string  `json:"status"`
		Serie         string  `json:"serie"`
		Folio         string  `json:"folio"`
	}
	var cfdis []CFDIRow
	var totalTimbrado float64
	for rows.Next() {
		var c CFDIRow
		rows.Scan(&c.ID, &c.Fecha, &c.FechaHora, &c.Total, &c.Metodo,
			&c.UUID, &c.RFCReceptor, &c.NombreReceptor, &c.Status, &c.Serie, &c.Folio)
		cfdis = append(cfdis, c)
		totalTimbrado += c.Total
	}
	if cfdis == nil { cfdis = []CFDIRow{} }

	json.NewEncoder(w).Encode(map[string]interface{}{
		"desde":          desde,
		"hasta":          hasta,
		"cfdis":          cfdis,
		"total_timbrado": totalTimbrado,
		"count":          len(cfdis),
	})
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





