package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	finkok "github.com/turbopos/turbopos/services/cfdi/internal/finkok"
	xmlgen "github.com/turbopos/turbopos/services/cfdi/internal/xmlgen"
	pb "github.com/turbopos/turbopos/gen/go/proto/cfdi/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const (
	Port            = ":50053"
	MaxErrorRatio   = 0.05
	WindowDur       = 10 * time.Minute
	HealthCheckDur  = 5 * time.Minute
	CertPath        = "services/cfdi/certs/test/eku9003173c9.cer"
	KeyPath         = "services/cfdi/certs/test/eku9003173c9.pem"
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
	keyBytes       []byte
}

func NewCFDIServer() *CFDIServer {
	user := os.Getenv("FINKOK_USER")
	pass := os.Getenv("FINKOK_PASS")
	if user == "" { log.Fatal("[CFDI] FINKOK_USER no definido") }
	if pass == "" { log.Fatal("[CFDI] FINKOK_PASS no definido") }

	cert, _, err := finkok.LoadCertificate(CertPath)
	if err != nil { log.Fatalf("[CFDI] Error certificado: %v", err) }
	key, err := os.ReadFile(KeyPath)
	if err != nil { log.Fatalf("[CFDI] Error llave: %v", err) }

	s := &CFDIServer{certBase64: cert, keyBytes: key}
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
		s.TotalRequests = 0
		s.FailedRequests = 0
		s.mu.Unlock()
	}
}

// healthCheckLoop intenta volver al PAC primario cada 5 minutos
func (s *CFDIServer) healthCheckLoop() {
	for range time.NewTicker(HealthCheckDur).C {
		s.mu.RLock()
		pac := s.CurrentPAC
		s.mu.RUnlock()

		if pac == 0 {
			continue // ya estamos en primario
		}

		// Ping a Finkok primario
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(FinkokHealthURL)
		if err != nil {
			log.Printf("[HealthCheck] PAC primario sin respuesta: %v", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode < 500 {
			s.mu.Lock()
			s.CurrentPAC = 0
			s.TotalRequests = 0
			s.FailedRequests = 0
			s.mu.Unlock()
			log.Printf("[HealthCheck] PAC primario recuperado — volviendo a FINKOK_SANDBOX")
		}
	}
}

func (s *CFDIServer) checkFailover() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.TotalRequests < 5 { return }
	ratio := float64(s.FailedRequests) / float64(s.TotalRequests)
	if ratio > MaxErrorRatio && s.CurrentPAC == 0 {
		log.Printf("[Kill-Switch] %.2f%% errores — cambiando a PAC Secundario", ratio*100)
		s.CurrentPAC = 1
		s.TotalRequests = 0
		s.FailedRequests = 0
	}
}

func (s *CFDIServer) Timbrar(ctx context.Context, req *pb.FacturaRequest) (*pb.FacturaResponse, error) {
	s.mu.Lock()
	s.TotalRequests++
	pac := s.CurrentPAC
	s.mu.Unlock()

	pacName := "FINKOK_SANDBOX"
	if pac == 1 { pacName = "PAC_SECUNDARIO" }
	log.Printf("[CFDI] Timbrando venta=%s rfc=%s pac=%s", req.VentaId, req.Rfc, pacName)

	items := []xmlgen.SaleItem{{
		Nombre:         "Venta POS",
		Cantidad:       1,
		PrecioUnitario: float64(req.Total),
		Subtotal:       float64(req.Total),
	}}

	xmlStr, err := xmlgen.GenerarXML(xmlgen.SaleData{
		SaleID:          req.VentaId,
		Fecha:           time.Now(),
		RFC:             req.Rfc,
		Items:           items,
		Total:           float64(req.Total),
		FormaPago:       "01",
		LugarExpedicion: "64000",
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
		Status:    "timbrado",
		Uuid:      result.UUID,
		SelloSat:  result.SelloSAT,
		PacUsado:  int32(pac),
		Timestamp: time.Now().UnixMilli(),
	}, nil
}

func main() {
	lis, err := net.Listen("tcp", Port)
	if err != nil { log.Fatalf("listener: %v", err) }
	srv := grpc.NewServer()
	pb.RegisterCFDIServiceServer(srv, NewCFDIServer())
	reflection.Register(srv)
	log.Printf("[CFDI] Servidor en %s — Finkok REAL + HealthCheck cada 5min", Port)
	log.Printf("[CFDI] Kill-Switch: failover si error > 5%% | Retorno: automático si PAC primario se recupera")
	if err := srv.Serve(lis); err != nil { log.Fatalf("serve: %v", err) }
}
