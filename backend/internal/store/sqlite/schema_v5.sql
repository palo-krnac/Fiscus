-- Verzia 5: prijaté faktúry. Dodávateľ, rekapitulácia DPH a súčty sú JSON.
CREATE TABLE purchases (
    id              INTEGER PRIMARY KEY,
    company_id      INTEGER NOT NULL REFERENCES companies(id),
    number          TEXT NOT NULL,
    supplier_number TEXT NOT NULL DEFAULT '',
    variable_symbol TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL,
    contact_id      INTEGER NOT NULL DEFAULT 0,
    supplier_json   TEXT NOT NULL DEFAULT '{}',
    supplier_iban   TEXT NOT NULL DEFAULT '',
    issue_date      TEXT NOT NULL DEFAULT '',
    delivery_date   TEXT NOT NULL DEFAULT '',
    received_date   TEXT NOT NULL DEFAULT '',
    due_date        TEXT NOT NULL DEFAULT '',
    paid_date       TEXT NOT NULL DEFAULT '',
    payment_method  TEXT NOT NULL DEFAULT 'transfer',
    reverse_charge  INTEGER NOT NULL DEFAULT 0,
    no_deduction    INTEGER NOT NULL DEFAULT 0,
    description     TEXT NOT NULL DEFAULT '',
    note            TEXT NOT NULL DEFAULT '',
    rates_json      TEXT NOT NULL DEFAULT '[]',
    summary_json    TEXT NOT NULL DEFAULT '{}'
);
CREATE UNIQUE INDEX purchases_number ON purchases(company_id, number);
CREATE INDEX purchases_company_received ON purchases(company_id, received_date);
