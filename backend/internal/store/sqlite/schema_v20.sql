-- Verzia 20: cudzie meny — kurz faktúry a sumy v eurách (JSON) a kurzy ECB.
ALTER TABLE invoices ADD COLUMN fx_json TEXT NOT NULL DEFAULT '';
CREATE TABLE fx_rates (
    currency  TEXT NOT NULL,
    date      TEXT NOT NULL,
    rate      INTEGER NOT NULL,
    source    TEXT NOT NULL,
    PRIMARY KEY (currency, date)
);
