-- +migrate Up
CREATE TABLE IF NOT EXISTS orders (
    id          TEXT PRIMARY KEY,
    status      TEXT NOT NULL DEFAULT 'pending',
    total       DECIMAL(10, 2) NOT NULL DEFAULT 0,
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS order_items (
    id          TEXT PRIMARY KEY,
    order_id    TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    item_id     TEXT NOT NULL,
    item_name   TEXT NOT NULL,
    price       DECIMAL(10, 2) NOT NULL,
    quantity    INTEGER NOT NULL
);

CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_created_at ON orders(created_at);
CREATE INDEX idx_order_items_order_id ON order_items(order_id);
CREATE INDEX idx_order_items_item_id ON order_items(item_id);

-- +migrate Down
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
