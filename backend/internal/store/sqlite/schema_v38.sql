-- Skladová karta produktu: identifikácia, zaradenie, nákup, popis,
-- parametre (JSON — nefiltrujú sa v SQL) a obrázky.
ALTER TABLE catalog_items ADD COLUMN ean TEXT NOT NULL DEFAULT '';
ALTER TABLE catalog_items ADD COLUMN category TEXT NOT NULL DEFAULT '';
ALTER TABLE catalog_items ADD COLUMN brand TEXT NOT NULL DEFAULT '';
ALTER TABLE catalog_items ADD COLUMN supplier TEXT NOT NULL DEFAULT '';
ALTER TABLE catalog_items ADD COLUMN supplier_code TEXT NOT NULL DEFAULT '';
ALTER TABLE catalog_items ADD COLUMN purchase_price INTEGER NOT NULL DEFAULT 0;
ALTER TABLE catalog_items ADD COLUMN weight INTEGER NOT NULL DEFAULT 0;
ALTER TABLE catalog_items ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE catalog_items ADD COLUMN params_json TEXT NOT NULL DEFAULT '';

CREATE TABLE item_images (
    id           INTEGER PRIMARY KEY,
    company_id   INTEGER NOT NULL REFERENCES companies(id),
    item_id      INTEGER NOT NULL REFERENCES catalog_items(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size         INTEGER NOT NULL,
    position     INTEGER NOT NULL,
    data         BLOB NOT NULL,
    created      TEXT NOT NULL
);
CREATE INDEX item_images_item ON item_images(company_id, item_id, position);
