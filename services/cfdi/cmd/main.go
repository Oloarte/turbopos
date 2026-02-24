package main

import (
    "context"
    "fmt"
    "log"
    "math/rand"
    "net"
    "sync"
    "time"

    pb "github.com/turbopos/turbopos/gen/go/proto/cfdi/v1"
    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/reflection"
    "google.golang.org/grpc/status"
)

const (
    Port           = ":50053"
    MaxErrorRatio  = 0.05
    WindowDuration = 10 * time.Minute
)

type CFDIServer struct {
    pb.UnimplementedCFDIServiceServer
    CurrentPAC     int
    TotalRequests  int64
    FailedRequests int64
    mu             sync.RWMutex
}

func NewCFDIServer() *CFDIServer {
    s := &CFDIServer{CurrentPAC: 0}
    go s.AuditWindowLoop()
    return s
}

func (s *CFDIServer) AuditWindowLoop() {
    ticker := time.NewTicker(WindowDuration)
    for range ticker.C {
        s.mu.Lock()
        log.Printf("[Audit-Loop] Ventana 10m. Total: %d, Errores: %d", s.TotalRequests, s.FailedRequests)
        s.TotalRequests = 0
        s.FailedRequests = 0
        s.mu.Unlock()
    }
}

func (s *CFDIServer) evaluateFailover() {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.TotalRequests < 5 {
        return
    }
    errorRatio := float64(s.FailedRequests) / float64(s.TotalRequests)
    if errorRatio > MaxErrorRatio && s.CurrentPAC == 0 {
        log.Printf("[Kill-Switch] ERROR RATIO %.2f%% > 5%% — cambiando a PAC Secundario", errorRatio*100)
        s.CurrentPAC = 1
        s.TotalRequests = 0
        s.FailedRequests = 0
    }
}

func (s *CFDIServer) Timbrar(ctx context.Context, req *pb.FacturaRequest) (*pb.FacturaResponse, error) {
    s.mu.Lock()
    s.TotalRequests++
    pacActivo := s.CurrentPAC
    s.mu.Unlock()

    pacName := "FINKOK_SANDBOX"
    if pacActivo == 1 {
        pacName = "PAC_SECUNDARIO"
    }

    log.Printf("[CFDI] Timbrando venta %s via %s para RFC: %s", req.VentaId, pacName, req.Rfc)

    // Mock: 15% fallo en PAC primario para probar kill-switch
    if pacActivo == 0 && rand.Float32() < 0.15 {
        s.mu.Lock()
        s.FailedRequests++
        s.mu.Unlock()
        go s.evaluateFailover()
        return nil, status.Errorf(codes.Unavailable, "Fallo en %s — activando evaluacion de failover", pacName)
    }

    uuid := fmt.Sprintf("MOCK-%d", time.Now().UnixNano())
    log.Printf("[CFDI] Timbrado exitoso: UUID=%s PAC=%s", uuid, pacName)

    return &pb.FacturaResponse{
        Status:   "timbrado",
        Uuid:     uuid,
        SelloSat: "SAT_SELLO_MOCK_" + uuid,
        PacUsado: int32(pacActivo),
        Timestamp: time.Now().UnixMilli(),
    }, nil
}

func main() {
    lis, err := net.Listen("tcp", Port)
    if err != nil {
        log.Fatalf("ERROR listener: %v", err)
    }

    grpcServer := grpc.NewServer()
    cfdiService := NewCFDIServer()
    pb.RegisterCFDIServiceServer(grpcServer, cfdiService)
    reflection.Register(grpcServer)

    log.Printf("[CFDI] Servidor iniciado en %s — PAC Primario: FINKOK_SANDBOX", Port)
    log.Printf("[CFDI] Kill-Switch activo: failover automatico si error > 5%% en 10 minutos")

    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("ERROR sirviendo: %v", err)
    }
}