-- Verzia 30: viac pokladní a pokladňa v cudzej mene.
ALTER TABLE cash_documents ADD COLUMN register_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cash_documents ADD COLUMN fx_json TEXT NOT NULL DEFAULT '';
