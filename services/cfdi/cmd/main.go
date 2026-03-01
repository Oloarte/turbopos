package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"context"

	finkok "github.com/turbopos/turbopos/services/cfdi/internal/finkok"
	xmlgen "github.com/turbopos/turbopos/services/cfdi/internal/xmlgen"
	pb "github.com/turbopos/turbopos/gen/go/proto/cfdi/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"encoding/base64"
)

const (
	GRPCPort        = ":50053"
	HTTPPort        = ":50055"
	MaxErrorRatio   = 0.05
	WindowDur       = 10 * time.Minute
	HealthCheckDur  = 5 * time.Minute
	CertPath        = "services/cfdi/certs/test/eku9003173c9.cer"
	KeyPath         = "services/cfdi/certs/test/eku9003173c9.key"
	KeyPassword     = "12345678a"
	NoCert          = "30001000000500003416"
	FinkokHealthURL = "https://demo-facturacion.finkok.com/servicios/soap/stamp.wsdl"
)

type CFDIServer struct {
	pb.UnimplementedCFDIServiceServer
	pacs           [2]*finkok.Client
	CurrentPAC     int
	TotalRequests  int64
	FailedRequests int64
	mu             sync.RWMutex
	certBase64     string
	certDER        []byte
	keyBytes       []byte
}

func NewCFDIServer() *CFDIServer {
	user := os.Getenv("FINKOK_USER")
	pass := os.Getenv("FINKOK_PASS")
	if user == "" { log.Fatal("[CFDI] FINKOK_USER no definido") }
	if pass == "" { log.Fatal("[CFDI] FINKOK_PASS no definido") }

	cert, _, err := finkok.LoadCertificate(CertPath)
	if err != nil { log.Fatalf("[CFDI] Error certificado: %v", err) }

	certDER, err := os.ReadFile(CertPath)
	if err != nil { log.Fatalf("[CFDI] Error leyendo .cer: %v", err) }

	key, err := os.ReadFile(KeyPath)
	if err != nil { log.Fatalf("[CFDI] Error llave: %v", err) }

	s := &CFDIServer{certBase64: cert, certDER: certDER, keyBytes: key}
	s.pacs[0] = finkok.NewDemoClient(user, pass)
	s.pacs[1] = finkok.NewDemoClient(user, pass)

	go s.auditLoop()
	go s.healthCheckLoop()
	return s
}

func (s *CFDIServer) auditLoop() {
	for range time.NewTicker(WindowDur).C {
		s.mu.Lock()
		log.Printf("[Audit] Total: %d Errores: %d PAC: %d", s.TotalRequests, s.FailedRequests, s.CurrentPAC)
		s.TotalRequests = 0; s.FailedRequests = 0
		s.mu.Unlock()
	}
}

func (s *CFDIServer) healthCheckLoop() {
	for range time.NewTicker(HealthCheckDur).C {
		s.mu.RLock(); pac := s.CurrentPAC; s.mu.RUnlock()
		if pac == 0 { continue }
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(FinkokHealthURL)
		if err != nil { log.Printf("[HealthCheck] PAC primario sin respuesta: %v", err); continue }
		resp.Body.Close()
		if resp.StatusCode < 500 {
			s.mu.Lock()
			s.CurrentPAC = 0; s.TotalRequests = 0; s.FailedRequests = 0
			s.mu.Unlock()
			log.Printf("[HealthCheck] PAC primario recuperado")
		}
	}
}

func (s *CFDIServer) checkFailover() {
	s.mu.Lock(); defer s.mu.Unlock()
	if s.TotalRequests < 5 { return }
	ratio := float64(s.FailedRequests) / float64(s.TotalRequests)
	if ratio > MaxErrorRatio && s.CurrentPAC == 0 {
		log.Printf("[Kill-Switch] %.2f%% errores — cambiando a PAC Secundario", ratio*100)
		s.CurrentPAC = 1; s.TotalRequests = 0; s.FailedRequests = 0
	}
}

func (s *CFDIServer) Timbrar(ctx context.Context, req *pb.FacturaRequest) (*pb.FacturaResponse, error) {
	s.mu.Lock(); s.TotalRequests++; pac := s.CurrentPAC; s.mu.Unlock()
	pacName := "FINKOK_SANDBOX"
	if pac == 1 { pacName = "PAC_SECUNDARIO" }
	log.Printf("[CFDI] Timbrando venta=%s rfc=%s pac=%s", req.VentaId, req.Rfc, pacName)

	items := []xmlgen.SaleItem{{
		Nombre: "Venta POS", Cantidad: 1,
		PrecioUnitario: float64(req.Total), Subtotal: float64(req.Total),
	}}
	xmlStr, err := xmlgen.GenerarXML(xmlgen.SaleData{
		SaleID: req.VentaId, Fecha: time.Now(), RFC: req.Rfc,
		Items: items, Total: float64(req.Total),
        FormaPago: "01", LugarExpedicion: "64000", CodigoPostalReceptor: req.GetCodigoPostalReceptor(),
	}, s.certBase64, NoCert)
	if err != nil {
		s.mu.Lock(); s.FailedRequests++; s.mu.Unlock()
		return nil, status.Errorf(codes.Internal, "generar XML: %v", err)
	}
	xmlFirmado, err := xmlgen.FirmarXML(xmlStr, s.keyBytes, KeyPassword)
	if err != nil {
		s.mu.Lock(); s.FailedRequests++; s.mu.Unlock()
		return nil, status.Errorf(codes.Internal, "firmar XML: %v", err)
	}
	result, err := s.pacs[pac].Timbrar(xmlFirmado)
	if err != nil {
		s.mu.Lock(); s.FailedRequests++; s.mu.Unlock()
		go s.checkFailover()
		return nil, status.Errorf(codes.Unavailable, "PAC error: %v", err)
	}
	if result.Error != "" {
		s.mu.Lock(); s.FailedRequests++; s.mu.Unlock()
		go s.checkFailover()
		return nil, status.Errorf(codes.InvalidArgument, "Finkok: %s", result.Error)
	}
	log.Printf("[CFDI] Timbrado OK UUID=%s PAC=%s", result.UUID, pacName)
	return &pb.FacturaResponse{
		Status: "timbrado", Uuid: result.UUID,
		SelloSat: result.SelloSAT, PacUsado: int32(pac),
		Timestamp: time.Now().UnixMilli(),
	}, nil
}

// ─── HTTP handler para cancelación ──────────────────────────────────────────
// GET/POST :50055/cancelar — llamado directamente por el BFF via HTTP

func (s *CFDIServer) serveCancelar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed); return
	}
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
	if req.RFC == ""    { req.RFC = "EKU9003173C9" }
	if req.Motivo == "" { req.Motivo = "02" }
	// UUID en mayúsculas (formato SAT)
	req.UUID = strings.ToUpper(req.UUID)

	log.Printf("[CFDI] Cancelando UUID=%s RFC=%s Motivo=%s", req.UUID, req.RFC, req.Motivo)

	s.mu.RLock(); pac := s.CurrentPAC; s.mu.RUnlock()
	certB64 := base64.StdEncoding.EncodeToString(s.certDER)

	result, err := s.pacs[pac].Cancelar(req.UUID, req.RFC, req.Motivo, req.UUIDReemplazo, certB64, s.keyBytes, KeyPassword)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if result.Error != "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": result.Error})
		return
	}

	log.Printf("[CFDI] Cancelación OK UUID=%s Status=%s", req.UUID, result.Status)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"uuid":    req.UUID,
		"status":  result.Status,
		"mensaje": result.Mensaje,
		"acuse":   result.Acuse,
	})
}

func main() {
	srv := NewCFDIServer()

	// gRPC en :50053
	lis, err := net.Listen("tcp", GRPCPort)
	if err != nil { log.Fatalf("listener gRPC: %v", err) }
	grpcSrv := grpc.NewServer()
	pb.RegisterCFDIServiceServer(grpcSrv, srv)
	reflection.Register(grpcSrv)

	// HTTP en :50055 para cancelación
	mux := http.NewServeMux()
	mux.HandleFunc("/cancelar", srv.serveCancelar)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	go func() {
		log.Printf("[CFDI] HTTP cancelación en %s", HTTPPort)
		if err := http.ListenAndServe(HTTPPort, mux); err != nil {
			log.Fatalf("HTTP server: %v", err)
		}
	}()

	log.Printf("[CFDI] gRPC en %s | HTTP en %s", GRPCPort, HTTPPort)
	if err := grpcSrv.Serve(lis); err != nil { log.Fatalf("serve: %v", err) }
}

