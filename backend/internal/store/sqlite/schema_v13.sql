-- Verzia 13: pravidelná fakturácia (šablóny faktúr s rozvrhom). Obsah je v JSON.
CREATE TABLE recurring_invoices (
    id          INTEGER PRIMARY KEY,
    company_id  INTEGER NOT NULL REFERENCES companies(id),
    active      INTEGER NOT NULL DEFAULT 0,
    next_date   TEXT NOT NULL DEFAULT '',
    data        TEXT NOT NULL
);
CREATE INDEX recurring_invoices_company ON recurring_invoices(company_id);
