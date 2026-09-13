-- Canonical player + node state. Inventory lives here, not in Redis.
CREATE TABLE IF NOT EXISTS players (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    x          INTEGER NOT NULL,
    y          INTEGER NOT NULL,
    inventory  JSONB NOT NULL DEFAULT '[]',
    skills     JSONB NOT NULL DEFAULT '{}',
    hp         INTEGER,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS nodes (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    x               INTEGER NOT NULL,
    y               INTEGER NOT NULL,
    remaining       INTEGER NOT NULL,
    cooldown_ticks  INTEGER NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS players_updated_at_idx ON players (updated_at DESC);
