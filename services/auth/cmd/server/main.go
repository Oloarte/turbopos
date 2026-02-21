package main

import (
    "context"
    "database/sql"
    "fmt"
    "log"
    "net"
    "os"
    "time"

    _ "github.com/lib/pq"
    authv1 "github.com/turbopos/turbopos/gen/go/proto/auth/v1"
    "google.golang.org/grpc"
    "google.golang.org/grpc/reflection"
)

type server struct {
    authv1.UnimplementedAuthServiceServer
    db *sql.DB
}

// Ping — ciclo completo: recibe mensaje → guarda en ping_log → responde
func (s *server) Ping(ctx context.Context, req *authv1.PingRequest) (*authv1.PingResponse, error) {
    msg := req.GetMessage()
    if msg == "" {
        msg = "ping"
    }

    _, err := s.db.ExecContext(ctx,
        "INSERT INTO ping_log (message, created_at) VALUES ($1, $2)",
        msg, time.Now(),
    )
    if err != nil {
        log.Printf("ERROR ping_log insert: %v", err)
        return nil, fmt.Errorf("db insert failed: %w", err)
    }

    log.Printf("✓ Ping recibido: %q — guardado en ping_log", msg)

    return &authv1.PingResponse{
        Status: fmt.Sprintf("pong: %s", msg),
    }, nil
}

func main() {
    dbHost := getenv("DB_HOST", "localhost")
    dbPort := getenv("DB_PORT", "5432")
    dbUser := getenv("DB_USER", "postgres")
    dbPass := getenv("DB_PASS", "postgres")
    dbName := getenv("DB_NAME", "turbopos")
    grpcPort := getenv("GRPC_PORT", "50051")

    dsn := fmt.Sprintf(
        "host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
        dbHost, dbPort, dbUser, dbPass, dbName,
    )

    db, err := sql.Open("postgres", dsn)
    if err != nil {
        log.Fatalf("ERROR abriendo conexion DB: %v", err)
    }
    defer db.Close()

    for i := 1; i <= 5; i++ {
        if err := db.Ping(); err != nil {
            log.Printf("Intento %d/5 — DB no disponible: %v", i, err)
            time.Sleep(2 * time.Second)
            continue
        }
        log.Println("✓ Conexion a PostgreSQL establecida")
        break
    }

    lis, err := net.Listen("tcp", ":"+grpcPort)
    if err != nil {
        log.Fatalf("ERROR iniciando listener: %v", err)
    }

    grpcServer := grpc.NewServer()
    authv1.RegisterAuthServiceServer(grpcServer, &server{db: db})
    reflection.Register(grpcServer)

    log.Printf("✓ Auth gRPC server escuchando en :%s", grpcPort)
    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("ERROR sirviendo gRPC: %v", err)
    }
}

func getenv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
