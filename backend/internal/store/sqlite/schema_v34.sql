-- Verzia 34: sklad — skladové doklady (príjemka, výdajka, inventúrny
-- rozdiel; riadky a väzby v JSON) a položka cenníka pri riadku faktúry.
CREATE TABLE stock_docs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    number     TEXT    NOT NULL,
    kind       TEXT    NOT NULL,
    date       TEXT    NOT NULL,
    data_json  TEXT    NOT NULL
);
CREATE INDEX stock_docs_company ON stock_docs (company_id, date);

ALTER TABLE invoice_lines ADD COLUMN item_id INTEGER NOT NULL DEFAULT 0;
