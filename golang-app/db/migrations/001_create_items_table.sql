-- +migrate Up
CREATE TABLE IF NOT EXISTS items (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'active',
    tags        TEXT[] NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_items_name ON items(name);
CREATE INDEX idx_items_status ON items(status);
CREATE INDEX idx_items_created_at ON items(created_at);

-- +migrate Down
DROP TABLE IF EXISTS items;
