-- Úvodné nastavenie firmy: druh účtovníctva, dokončený sprievodca
-- (existujúce firmy ho už nepotrebujú) a jednorazový prenos z iného programu.
ALTER TABLE companies ADD COLUMN bookkeeping TEXT NOT NULL DEFAULT '';
ALTER TABLE companies ADD COLUMN setup_done INTEGER NOT NULL DEFAULT 1;
ALTER TABLE companies ADD COLUMN migrated_from TEXT NOT NULL DEFAULT '';
ALTER TABLE companies ADD COLUMN migrated_at TEXT NOT NULL DEFAULT '';
