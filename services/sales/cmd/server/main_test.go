package main

import (
    "context"
    "testing"

    salesv1 "github.com/turbopos/turbopos/gen/go/proto/sales/v1"
)

func TestCreateSale_Con_DB_Nil(t *testing.T) {
    s := &server{db: nil}
    req := &salesv1.CreateSaleRequest{
        CashierId:     "00000000-0000-0000-0000-000000000001",
        Total:         52.00,
        PaymentMethod: "cash",
        Items: []*salesv1.SaleItem{
            {ProductId: "5d4373c8-a762-43e9-845c-fcfabfe4794e", Name: "Coca-Cola", Quantity: 2, UnitPrice: 20, Subtotal: 40},
        },
    }
    _, err := s.CreateSale(context.Background(), req)
    if err == nil {
        t.Error("esperaba error por DB nil")
    }
    t.Logf("Error esperado: %v", err)
}

func TestGetSale_Con_DB_Nil(t *testing.T) {
    s := &server{db: nil}
    req := &salesv1.GetSaleRequest{SaleId: "71817a09-8616-4681-b300-b0ddc0995269"}
    _, err := s.GetSale(context.Background(), req)
    if err == nil {
        t.Error("esperaba error por DB nil")
    }
    t.Logf("Error esperado: %v", err)
}

func TestGetenv_Sales_Fallback(t *testing.T) {
    val := getenv("VARIABLE_INEXISTENTE_SALES_XYZ", "default_sales")
    if val != "default_sales" {
        t.Errorf("esperaba 'default_sales', got '%s'", val)
    }
}
