-- Verzia 16: cenník (opakované položky faktúr).
CREATE TABLE catalog_items (
    id          INTEGER PRIMARY KEY,
    company_id  INTEGER NOT NULL REFERENCES companies(id),
    code        TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL,
    unit        TEXT NOT NULL DEFAULT '',
    price       INTEGER NOT NULL DEFAULT 0, -- desaťtisíciny (money.Dec)
    price_mode  TEXT NOT NULL DEFAULT 'net',
    vat_rate    INTEGER NOT NULL DEFAULT 0,
    note        TEXT NOT NULL DEFAULT '',
    archived    INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX catalog_items_code ON catalog_items(company_id, code COLLATE NOCASE) WHERE code <> '';
CREATE INDEX catalog_items_company ON catalog_items(company_id);
