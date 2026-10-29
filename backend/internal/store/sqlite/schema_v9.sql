-- Verzia 9: druh plnenia (tovar / služba) pre obchod so zahraničím.
ALTER TABLE invoices ADD COLUMN supply_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE purchases ADD COLUMN supply_kind TEXT NOT NULL DEFAULT '';
