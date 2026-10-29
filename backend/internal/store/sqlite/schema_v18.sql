-- Verzia 18: zálohy — väzba zálohovej faktúry, dokladu k prijatej platbe
-- a vyúčtovacej faktúry vrátane odpočtu zálohy (JSON).
ALTER TABLE invoices ADD COLUMN advance_json TEXT NOT NULL DEFAULT '';
