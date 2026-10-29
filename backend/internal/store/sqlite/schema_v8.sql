-- Verzia 8: bankové pohyby z importovaných výpisov.
CREATE TABLE bank_transactions (
    id           INTEGER PRIMARY KEY,
    company_id   INTEGER NOT NULL REFERENCES companies(id),
    account      TEXT NOT NULL DEFAULT '',
    date         TEXT NOT NULL,
    amount       INTEGER NOT NULL,          -- desaťtisíciny (money.Dec)
    currency     TEXT NOT NULL DEFAULT 'EUR',
    direction    TEXT NOT NULL,
    vs           TEXT NOT NULL DEFAULT '',
    ss           TEXT NOT NULL DEFAULT '',
    ks           TEXT NOT NULL DEFAULT '',
    counter_name TEXT NOT NULL DEFAULT '',
    counter_iban TEXT NOT NULL DEFAULT '',
    message      TEXT NOT NULL DEFAULT '',
    bank_ref     TEXT NOT NULL DEFAULT '',
    fingerprint  TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'open',
    match_json   TEXT NOT NULL DEFAULT '',
    note         TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX bank_transactions_fingerprint ON bank_transactions(company_id, fingerprint);
CREATE INDEX bank_transactions_company_date ON bank_transactions(company_id, date);
