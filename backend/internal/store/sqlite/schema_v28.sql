-- Verzia 28: DPH na pokladničných dokladoch (blok = zjednodušená faktúra, tržba z e-kasy).
ALTER TABLE cash_documents ADD COLUMN tax_json TEXT NOT NULL DEFAULT '';
