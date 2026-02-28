package main

import (
	"context"
	"database/sql"
	"log"
	"math"
	"net"
	"os"
	"time"

	_ "github.com/lib/pq"
	pb "github.com/turbopos/turbopos/gen/go/proto/loyalty/v1"
	"google.golang.org/grpc"
	"fmt"
    "google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const Port = ":50054"

// Reglas de puntos
const (
	PointsPerPeso   = 1.0  // 1 punto por cada peso gastado
	TierSilver      = 500  // puntos acumulados para tier silver
	TierGold        = 2000 // puntos acumulados para tier gold
	TierPlatinum    = 5000 // puntos acumulados para tier platinum
)

type LoyaltyServer struct {
	pb.UnimplementedLoyaltyServiceServer
	db *sql.DB
}

func NewLoyaltyServer(db *sql.DB) *LoyaltyServer {
	return &LoyaltyServer{db: db}
}

// GetAccount obtiene o crea una cuenta de lealtad por teléfono
func (s *LoyaltyServer) GetAccount(ctx context.Context, req *pb.GetAccountRequest) (*pb.AccountResponse, error) {
    if req.Phone == "" {
        return nil, status.Errorf(codes.InvalidArgument, "phone requerido")
    }

    var id, name, tier, rfc string
    var points int32
    var totalSpent float64

    err := s.db.QueryRowContext(ctx, `
        SELECT id, COALESCE(name,''), points, total_spent, tier, COALESCE(rfc,'')
        FROM loyalty_accounts WHERE phone = $1
    `, req.Phone).Scan(&id, &name, &points, &totalSpent, &tier, &rfc)

    if err == sql.ErrNoRows {
        return nil, status.Errorf(codes.NotFound, "cliente no encontrado: %s", req.Phone)
    }
    if err != nil {
        return nil, status.Errorf(codes.Internal, "buscar cuenta: %v", err)
    }

    return &pb.AccountResponse{
        AccountId:  id,
        Phone:      req.Phone,
        Name:       name,
        Points:     points,
        TotalSpent: totalSpent,
        Tier:       tier,
        Rfc:        rfc,
    }, nil
}

// EarnPoints agrega puntos por una venta
func (s *LoyaltyServer) EarnPoints(ctx context.Context, req *pb.EarnPointsRequest) (*pb.AccountResponse, error) {
	if req.Phone == "" {
		return nil, status.Errorf(codes.InvalidArgument, "phone requerido")
	}
	if req.Total <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "total debe ser mayor a 0")
	}

	// Calcular puntos: 1 punto por peso, redondeado hacia abajo
	points := int32(math.Floor(req.Total * PointsPerPeso))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "iniciar transacción: %v", err)
	}
	defer tx.Rollback()

	// Obtener o crear cuenta
	var accountID string
	var currentPoints int32
	var totalSpent float64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO loyalty_accounts (phone, name, points, total_spent)
		VALUES ($1, $2, 0, 0)
		ON CONFLICT (phone) DO UPDATE SET
			name = CASE WHEN $2 != '' THEN $2 ELSE loyalty_accounts.name END,
			updated_at = NOW()
		RETURNING id, points, total_spent
	`, req.Phone, req.Name).Scan(&accountID, &currentPoints, &totalSpent)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "upsert cuenta: %v", err)
	}

	// Sumar puntos y gasto
	newPoints := currentPoints + points
	newTotalSpent := totalSpent + req.Total
	newTier := calculateTier(newTotalSpent)

	_, err = tx.ExecContext(ctx, `
		UPDATE loyalty_accounts
		SET points = $1, total_spent = $2, tier = $3, updated_at = NOW()
		WHERE id = $4
	`, newPoints, newTotalSpent, newTier, accountID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "actualizar puntos: %v", err)
	}

	// Registrar transacción
	saleID := sql.NullString{String: req.SaleId, Valid: req.SaleId != ""}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO loyalty_transactions (account_id, sale_id, type, points, description)
		VALUES ($1, $2, 'earn', $3, $4)
	`, accountID, saleID, points, fmt.Sprintf("Compra $%.2f → +%d puntos", req.Total, points))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "registrar transacción: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "commit: %v", err)
	}

	log.Printf("[Loyalty] EarnPoints phone=%s sale=%s total=%.2f puntos=+%d total_pts=%d tier=%s",
		req.Phone, req.SaleId, req.Total, points, newPoints, newTier)

	return &pb.AccountResponse{
		AccountId:  accountID,
		Phone:      req.Phone,
		Name:       req.Name,
		Points:     newPoints,
		TotalSpent: newTotalSpent,
		Tier:       newTier,
	}, nil
}

// RedeemPoints canjea puntos por una recompensa
func (s *LoyaltyServer) RedeemPoints(ctx context.Context, req *pb.RedeemPointsRequest) (*pb.RedeemResponse, error) {
	if req.Phone == "" {
		return nil, status.Errorf(codes.InvalidArgument, "phone requerido")
	}
	if req.Points <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "points debe ser mayor a 0")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "iniciar transacción: %v", err)
	}
	defer tx.Rollback()

	// Verificar puntos disponibles
	var accountID string
	var currentPoints int32
	err = tx.QueryRowContext(ctx, `
		SELECT id, points FROM loyalty_accounts WHERE phone = $1 FOR UPDATE
	`, req.Phone).Scan(&accountID, &currentPoints)
	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "cuenta no encontrada para %s", req.Phone)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "buscar cuenta: %v", err)
	}

	if currentPoints < req.Points {
		return &pb.RedeemResponse{
			Success:          false,
			PointsRemaining:  currentPoints,
			Message:          fmt.Sprintf("Puntos insuficientes: tienes %d, necesitas %d", currentPoints, req.Points),
		}, nil
	}

	// Descontar puntos
	newPoints := currentPoints - req.Points
	_, err = tx.ExecContext(ctx, `
		UPDATE loyalty_accounts SET points = $1, updated_at = NOW() WHERE id = $2
	`, newPoints, accountID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "descontar puntos: %v", err)
	}

	// Registrar transacción
	_, err = tx.ExecContext(ctx, `
		INSERT INTO loyalty_transactions (account_id, type, points, description)
		VALUES ($1, 'redeem', $2, $3)
	`, accountID, -req.Points, fmt.Sprintf("Canje de %d puntos", req.Points))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "registrar canje: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "commit: %v", err)
	}

	log.Printf("[Loyalty] RedeemPoints phone=%s puntos=-%d restantes=%d", req.Phone, req.Points, newPoints)

	return &pb.RedeemResponse{
		Success:         true,
		PointsUsed:      req.Points,
		PointsRemaining: newPoints,
		Message:         fmt.Sprintf("Canjeaste %d puntos exitosamente", req.Points),
	}, nil
}

// GetHistory obtiene el historial de transacciones
func (s *LoyaltyServer) GetHistory(ctx context.Context, req *pb.GetHistoryRequest) (*pb.HistoryResponse, error) {
	if req.Phone == "" {
		return nil, status.Errorf(codes.InvalidArgument, "phone requerido")
	}

	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var accountID string
	var points int32
	err := s.db.QueryRowContext(ctx, `
		SELECT id, points FROM loyalty_accounts WHERE phone = $1
	`, req.Phone).Scan(&accountID, &points)
	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "cuenta no encontrada")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "buscar cuenta: %v", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, points, description, created_at
		FROM loyalty_transactions
		WHERE account_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, accountID, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "historial: %v", err)
	}
	defer rows.Close()

	var transactions []*pb.Transaction
	for rows.Next() {
		var t pb.Transaction
		var createdAt time.Time
		if err := rows.Scan(&t.Id, &t.Type, &t.Points, &t.Description, &createdAt); err != nil {
			continue
		}
		t.CreatedAt = createdAt.Format("2006-01-02T15:04:05")
		transactions = append(transactions, &t)
	}

	return &pb.HistoryResponse{
		AccountId:    accountID,
		Points:       points,
		Transactions: transactions,
	}, nil
}

func (s *LoyaltyServer) findOrCreateAccount(ctx context.Context, phone, name, rfc string) (*pb.AccountResponse, error) {
    var id, accName, tier, accRfc string
	var points int32
	var totalSpent float64

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO loyalty_accounts (phone, name, rfc)
		VALUES ($1, $2, $3)
		ON CONFLICT (phone) DO UPDATE SET updated_at = NOW()
        RETURNING id, COALESCE(name,''), points, total_spent, tier, COALESCE(rfc,'')
    `, phone, name, rfc).Scan(&id, &accName, &points, &totalSpent, &tier, &accRfc)
	if err != nil {
		return nil, err
	}

	return &pb.AccountResponse{
		AccountId:  id,
		Phone:      phone,
		Name:       accName,
		Points:     points,
		TotalSpent: totalSpent,
        Tier:       tier,
        Rfc:        accRfc,
	}, nil
}

func calculateTier(totalSpent float64) string {
	switch {
	case totalSpent >= TierPlatinum:
		return "platinum"
	case totalSpent >= TierGold:
		return "gold"
	case totalSpent >= TierSilver:
		return "silver"
	default:
		return "bronze"
	}
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:turbopos@localhost:5432/turbopos?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Error abriendo DB: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Error conectando DB: %v", err)
	}

	lis, err := net.Listen("tcp", Port)
	if err != nil {
		log.Fatalf("listener: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterLoyaltyServiceServer(srv, NewLoyaltyServer(db))
	reflection.Register(srv)

	log.Printf("[Loyalty] Servidor en %s — 1 punto por peso gastado", Port)
	log.Printf("[Loyalty] Tiers: Bronze→Silver(%d pts)→Gold(%d pts)→Platinum(%d pts)",
		TierSilver, TierGold, TierPlatinum)

	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}


