-- Verzia 23: tuzemské prenesenie daňovej povinnosti pri položke (§ 69 ods. 12) a kód colného sadzobníka.
ALTER TABLE invoice_lines ADD COLUMN reverse TEXT NOT NULL DEFAULT '';
ALTER TABLE invoice_lines ADD COLUMN commodity TEXT NOT NULL DEFAULT '';
