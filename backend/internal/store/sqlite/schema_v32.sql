-- Verzia 32: množstvo tovaru pre KV A.2 na položke (doklad k platbe zálohy
-- s prenesením daňovej povinnosti pri tovare uvádza celé množstvo dodávky).
ALTER TABLE invoice_lines ADD COLUMN a2_quantity INTEGER;
ALTER TABLE invoice_lines ADD COLUMN a2_unit TEXT NOT NULL DEFAULT '';
