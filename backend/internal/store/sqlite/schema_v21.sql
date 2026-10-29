-- Verzia 21: úhrady faktúr (aj čiastočné) — zoznam v JSON.
ALTER TABLE invoices ADD COLUMN payments_json TEXT NOT NULL DEFAULT '';
