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
    salesv1 "github.com/turbopos/turbopos/gen/go/proto/sales/v1"
    "google.golang.org/grpc"
    "google.golang.org/grpc/reflection"
)

type server struct {
    salesv1.UnimplementedSalesServiceServer
    db *sql.DB
}

func (s *server) CreateSale(ctx context.Context, req *salesv1.CreateSaleRequest) (*salesv1.CreateSaleResponse, error) {
    if s.db == nil {
        return nil, fmt.Errorf("db no disponible")
    }

    // Insertar la venta principal
    var saleID string
    err := s.db.QueryRowContext(ctx,
        `INSERT INTO sales (cashier_id, total, payment_method)
         VALUES ($1::uuid, $2, $3)
         RETURNING id`,
        req.GetCashierId(), req.GetTotal(), req.GetPaymentMethod(),
    ).Scan(&saleID)
    if err != nil {
        return nil, fmt.Errorf("crear venta: %w", err)
    }

    // Insertar cada item
    for _, item := range req.GetItems() {
        _, err := s.db.ExecContext(ctx,
            `INSERT INTO sale_items (sale_id, product_id, name, quantity, unit_price, subtotal)
             VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)`,
            saleID, item.GetProductId(), item.GetName(),
            item.GetQuantity(), item.GetUnitPrice(), item.GetSubtotal(),
        )
        if err != nil {
            return nil, fmt.Errorf("insertar item: %w", err)
        }

        // Descontar stock
        _, err = s.db.ExecContext(ctx,
            `UPDATE products SET stock = stock - $1 WHERE id = $2::uuid`,
            item.GetQuantity(), item.GetProductId(),
        )
        if err != nil {
            log.Printf("WARN: no se pudo descontar stock de %s: %v", item.GetProductId(), err)
        }
    }

    log.Printf("✓ Venta creada: %s · Total: $%.2f · Items: %d", saleID, req.GetTotal(), len(req.GetItems()))

    return &salesv1.CreateSaleResponse{
        SaleId:    saleID,
        Status:    "completed",
        Total:     req.GetTotal(),
        CreatedAt: time.Now().Format(time.RFC3339),
    }, nil
}

func (s *server) GetSale(ctx context.Context, req *salesv1.GetSaleRequest) (*salesv1.GetSaleResponse, error) {
    if s.db == nil {
        return nil, fmt.Errorf("db no disponible")
    }

    var resp salesv1.GetSaleResponse
    err := s.db.QueryRowContext(ctx,
        `SELECT id, cashier_id, total, payment_method, created_at
         FROM sales WHERE id = $1::uuid`,
        req.GetSaleId(),
    ).Scan(&resp.SaleId, &resp.CashierId, &resp.Total, &resp.PaymentMethod, &resp.CreatedAt)
    if err != nil {
        return nil, fmt.Errorf("venta no encontrada: %w", err)
    }

    rows, err := s.db.QueryContext(ctx,
        `SELECT product_id, name, quantity, unit_price, subtotal
         FROM sale_items WHERE sale_id = $1::uuid`,
        req.GetSaleId(),
    )
    if err != nil {
        return nil, fmt.Errorf("items: %w", err)
    }
    defer rows.Close()

    for rows.Next() {
        item := &salesv1.SaleItem{}
        rows.Scan(&item.ProductId, &item.Name, &item.Quantity, &item.UnitPrice, &item.Subtotal)
        resp.Items = append(resp.Items, item)
    }

    return &resp, nil
}

func main() {
    dsn := fmt.Sprintf(
        "host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
        getenv("DB_HOST","localhost"), getenv("DB_PORT","5432"),
        getenv("DB_USER","postgres"), getenv("DB_PASS","postgres"),
        getenv("DB_NAME","turbopos"),
    )

    db, err := sql.Open("postgres", dsn)
    if err != nil {
        log.Fatalf("ERROR DB: %v", err)
    }
    defer db.Close()

    for i := 1; i <= 5; i++ {
        if err := db.Ping(); err != nil {
            log.Printf("Intento %d/5: %v", i, err)
            time.Sleep(2 * time.Second)
            continue
        }
        log.Println("✓ PostgreSQL conectado")
        break
    }

    grpcPort := getenv("GRPC_PORT","50052")
    lis, err := net.Listen("tcp", ":"+grpcPort)
    if err != nil {
        log.Fatalf("ERROR listener: %v", err)
    }

    srv := grpc.NewServer()
    salesv1.RegisterSalesServiceServer(srv, &server{db: db})
    reflection.Register(srv)

    log.Printf("✓ Sales gRPC server en :%s", grpcPort)
    srv.Serve(lis)
}

func getenv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
