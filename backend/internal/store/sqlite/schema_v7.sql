-- Verzia 7: údaje firmy pre priznanie k DPH.
ALTER TABLE companies ADD COLUMN tax_office TEXT NOT NULL DEFAULT '';
ALTER TABLE companies ADD COLUMN vat_period TEXT NOT NULL DEFAULT 'month';
