-- Verzia 10: API kľúče pre AI asistentov (MCP). Ukladá sa iba hash kľúča.
CREATE TABLE api_keys (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL DEFAULT 0, -- 0 = lokálny používateľ desktopu
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL,
    hash         TEXT NOT NULL UNIQUE,
    read_only    INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL,
    last_used_at INTEGER NOT NULL DEFAULT 0,
    revoked      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX api_keys_user ON api_keys(user_id);
