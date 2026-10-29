-- Verzia 11: pokladňa (príjmové a výdavkové pokladničné doklady).
CREATE TABLE cash_documents (
    id          INTEGER PRIMARY KEY,
    company_id  INTEGER NOT NULL REFERENCES companies(id),
    number      TEXT NOT NULL,
    kind        TEXT NOT NULL,
    date        TEXT NOT NULL,
    amount      INTEGER NOT NULL,          -- desaťtisíciny (money.Dec)
    partner     TEXT NOT NULL DEFAULT '',
    purpose     TEXT NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    link_kind   TEXT NOT NULL DEFAULT '',
    link_id     INTEGER NOT NULL DEFAULT 0,
    link_number TEXT NOT NULL DEFAULT '',
    cancelled   INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX cash_documents_number ON cash_documents(company_id, number);
CREATE INDEX cash_documents_company_date ON cash_documents(company_id, date);
