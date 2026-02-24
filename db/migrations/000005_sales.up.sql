CREATE TABLE IF NOT EXISTS products (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    price       NUMERIC(10,2) NOT NULL,
    stock       INT NOT NULL DEFAULT 0,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sales (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cashier_id      UUID NOT NULL,
    total           NUMERIC(10,2) NOT NULL,
    payment_method  VARCHAR(50) NOT NULL DEFAULT 'cash',
    status          VARCHAR(50) NOT NULL DEFAULT 'completed',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sale_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id     UUID NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
    product_id  UUID NOT NULL,
    name        VARCHAR(255) NOT NULL,
    quantity    INT NOT NULL,
    unit_price  NUMERIC(10,2) NOT NULL,
    subtotal    NUMERIC(10,2) NOT NULL
);

CREATE INDEX idx_sales_cashier   ON sales(cashier_id);
CREATE INDEX idx_sale_items_sale ON sale_items(sale_id);

INSERT INTO products (name, price, stock) VALUES
    ('Coca-Cola 600ml', 20.00, 100),
    ('Agua 1L',         12.00, 150),
    ('Papas Sabritas',  18.00,  80);