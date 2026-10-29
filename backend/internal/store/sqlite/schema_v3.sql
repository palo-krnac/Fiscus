-- Verzia 3: číselníky firmy (merné jednotky, formáty číselných radov)
-- a spôsob úhrady na faktúre. Číselníky firmy sú JSON (malé zoznamy).

ALTER TABLE companies ADD COLUMN units_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE companies ADD COLUMN number_formats_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE invoices ADD COLUMN payment_method TEXT NOT NULL DEFAULT 'transfer';
