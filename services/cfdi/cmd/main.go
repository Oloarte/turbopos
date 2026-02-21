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
    Port           = ":50051"
    MaxErrorRatio  = 0.05 // failover_trigger: "error_ratio>0.05/10m"
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
    go s.AuditWindowLoop() // Iniciar agente
    return s
}

// Resetea las métricas cada 10 mins según tu regla
func (s *CFDIServer) AuditWindowLoop() {
    ticker := time.NewTicker(WindowDuration)
    for range ticker.C {
        s.mu.Lock()
        log.Printf("[Audit-Loop] Ventana 10m cerrada. Total: %d, Errores: %d", s.TotalRequests, s.FailedRequests)
        s.TotalRequests = 0
        s.FailedRequests = 0
        s.mu.Unlock()
    }
}

func (s *CFDIServer) evaluateFailover() {
    s.mu.Lock()
    defer s.mu.Unlock()

    if s.TotalRequests < 5 { return } // Sample mínimo

    errorRatio := float64(s.FailedRequests) / float64(s.TotalRequests)
    if errorRatio > MaxErrorRatio && s.CurrentPAC == 0 {
        log.Printf("🚨 [PRC-SAT-Kill] ERROR RATIO %.2f%% EXCEDE 5%%!", errorRatio*100)
        log.Println("⚡ ACTIVANDO KILL-SWITCH: Cambiando a PAC Secundario (1)")
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

    pacName := "PAC_PRIMARIO_0"
    if pacActivo == 1 { pacName = "PAC_SECUNDARIO_1" }

    log.Printf("Timbrando Venta %s vía %s para RFC: %s", req.VentaId, pacName, req.Rfc)

    // Simular fallo aleatorio para probar el Kill-Switch (15% de fallo en PAC 0)
    success := true
    if pacActivo == 0 && rand.Float32() < 0.15 { success = false }

    if !success {
        s.mu.Lock()
        s.FailedRequests++
        s.mu.Unlock()
        go s.evaluateFailover()
        return nil, status.Errorf(codes.Unavailable, "Fallo en conexión con %s", pacName)
    }

    return &pb.FacturaResponse{
        Uuid:      fmt.Sprintf("mock-uuid-%d", time.Now().UnixNano()),
        SelloSat:  "SAT_SELLO_X29384729384_MOCK",
        PacUsado:  pacID,
        Timestamp: time.Now().UnixMilli(),
    }, nil
}

func main() {
    lis, err := net.Listen("tcp", Port)
    if err != nil { log.Fatalf("Fallo en TCP listener: %v", err) }

    grpcServer := grpc.NewServer()
    cfdiService := NewCFDIServer()

    pb.RegisterCFDIServiceServer(grpcServer, cfdiService)
    reflection.Register(grpcServer)

    log.Printf("🚀 CFDI Service iniciado en puerto %s", Port)
    log.Printf("🛡️  Agente PRC-SAT-Kill activo. PAC Primario por defecto.")
    
    if err := grpcServer.Serve(lis);\n\tpacID := int32(0) // ← 0=primario,1=secundario err != nil { log.Fatalf("Fallo en gRPC Server: %v", err) }
}


