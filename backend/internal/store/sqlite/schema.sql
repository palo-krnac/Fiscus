-- Fiskus — schéma SQLite, verzia 1.
-- Peniaze sú INTEGER v desaťtisícinách (money.Dec.Raw), dátumy TEXT YYYY-MM-DD.
-- Každá tabuľka s dátami firmy má company_id.

CREATE TABLE companies (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    street     TEXT NOT NULL DEFAULT '',
    city       TEXT NOT NULL DEFAULT '',
    zip        TEXT NOT NULL DEFAULT '',
    ico        TEXT NOT NULL DEFAULT '',
    dic        TEXT NOT NULL DEFAULT '',
    icdph      TEXT NOT NULL DEFAULT '',
    iban       TEXT NOT NULL DEFAULT '',
    vat_payer  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE invoices (
    id               INTEGER PRIMARY KEY,
    company_id       INTEGER NOT NULL REFERENCES companies(id),
    kind             TEXT NOT NULL,
    status           TEXT NOT NULL,
    number           TEXT NOT NULL DEFAULT '',
    variable_symbol  TEXT NOT NULL DEFAULT '',
    customer_name    TEXT NOT NULL DEFAULT '',
    customer_street  TEXT NOT NULL DEFAULT '',
    customer_city    TEXT NOT NULL DEFAULT '',
    customer_zip     TEXT NOT NULL DEFAULT '',
    customer_country TEXT NOT NULL DEFAULT '',
    customer_ico     TEXT NOT NULL DEFAULT '',
    customer_dic     TEXT NOT NULL DEFAULT '',
    customer_icdph   TEXT NOT NULL DEFAULT '',
    customer_email   TEXT NOT NULL DEFAULT '',
    issue_date       TEXT NOT NULL DEFAULT '',
    delivery_date    TEXT NOT NULL DEFAULT '',
    due_date         TEXT NOT NULL DEFAULT '',
    paid_date        TEXT NOT NULL DEFAULT '',
    currency         TEXT NOT NULL DEFAULT 'EUR',
    price_mode       TEXT NOT NULL DEFAULT 'net',
    reverse_charge   INTEGER NOT NULL DEFAULT 0,
    note             TEXT NOT NULL DEFAULT '',
    summary_json     TEXT NOT NULL DEFAULT '{}'
);
-- číslo dokladu je v rámci firmy jedinečné (koncepty majú '')
CREATE UNIQUE INDEX invoices_number ON invoices(company_id, number) WHERE number <> '';
CREATE INDEX invoices_company_date ON invoices(company_id, issue_date);

CREATE TABLE invoice_lines (
    invoice_id   INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    position     INTEGER NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    quantity     INTEGER NOT NULL,
    unit         TEXT NOT NULL DEFAULT '',
    unit_price   INTEGER NOT NULL,
    vat_rate     INTEGER NOT NULL,
    amount       INTEGER NOT NULL,
    PRIMARY KEY (invoice_id, position)
);

CREATE TABLE sequences (
    company_id INTEGER NOT NULL REFERENCES companies(id),
    series     TEXT NOT NULL,
    year       INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    PRIMARY KEY (company_id, series, year)
);
