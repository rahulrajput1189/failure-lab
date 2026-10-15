CREATE TABLE IF NOT EXISTS inventory_stock_operations (
    operation_key TEXT PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id),
    action TEXT NOT NULL CHECK (action IN ('release')),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
