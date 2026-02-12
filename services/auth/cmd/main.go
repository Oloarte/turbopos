package main

import (
    "context"
    "database/sql"
    "fmt"
    "log"
    "net"
    "time"

    pb "github.com/turbopos/turbopos/gen/go/proto/auth/v1"

    _ "github.com/lib/pq"
    "google.golang.org/grpc"
    "google.golang.org/grpc/reflection"
)

const Port = ":50052"

type AuthServer struct {
    pb.UnimplementedAuthServiceServer
    DB *sql.DB
}

func (s *AuthServer) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
    log.Printf("[AuthService] Ping recibido. Mensaje: %s", req.Message)

    // Cambiamos a comillas dobles normales para evitar problemas con PowerShell
    query := "INSERT INTO ping_log (message, created_at) VALUES ($1, $2)"
    _, err := s.DB.ExecContext(ctx, query, req.Message, time.Now())
    if err != nil {
        log.Printf("Error al insertar en ping_log: %v", err)
        return nil, fmt.Errorf("error interno")
    }

    log.Println("Registro guardado en ping_log exitosamente")
    return &pb.PingResponse{Status: "ok"}, nil
}

func main() {
    log.Println("Iniciando TurboPOS Auth Service...")

    connStr := "host=127.0.0.1 port=5432 user=postgres password=turbopos dbname=turbopos sslmode=disable"
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        log.Fatalf("Fallo crítico al preparar conexión a DB: %v", err)
    }
    defer db.Close()

    if err := db.Ping(); err != nil {
        log.Fatalf("Fallo al conectar a PostgreSQL: %v", err)
    }
    log.Println("Conexión a PostgreSQL (Docker) establecida")

    lis, err := net.Listen("tcp", Port)
    if err != nil {
        log.Fatalf("Fallo al iniciar TCP listener: %v", err)
    }

    grpcServer := grpc.NewServer()
    authService := &AuthServer{DB: db}

    pb.RegisterAuthServiceServer(grpcServer, authService)
    reflection.Register(grpcServer)

    log.Printf("Auth Service escuchando peticiones gRPC en %s", Port)
    
    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("Fallo en gRPC Server: %v", err)
    }
}
