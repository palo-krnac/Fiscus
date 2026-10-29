-- Verzia 2: nastavenia firmy, adresár, neplatiteľ DPH na faktúre.

ALTER TABLE companies ADD COLUMN email TEXT NOT NULL DEFAULT '';
ALTER TABLE companies ADD COLUMN phone TEXT NOT NULL DEFAULT '';
ALTER TABLE companies ADD COLUMN default_due_days INTEGER NOT NULL DEFAULT 14;

CREATE TABLE contacts (
    id          INTEGER PRIMARY KEY,
    company_id  INTEGER NOT NULL REFERENCES companies(id),
    name        TEXT NOT NULL,
    street      TEXT NOT NULL DEFAULT '',
    city        TEXT NOT NULL DEFAULT '',
    zip         TEXT NOT NULL DEFAULT '',
    country     TEXT NOT NULL DEFAULT 'SK',
    ico         TEXT NOT NULL DEFAULT '',
    dic         TEXT NOT NULL DEFAULT '',
    icdph       TEXT NOT NULL DEFAULT '',
    email       TEXT NOT NULL DEFAULT '',
    phone       TEXT NOT NULL DEFAULT '',
    iban        TEXT NOT NULL DEFAULT '',
    due_days    INTEGER NOT NULL DEFAULT 0,
    note        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX contacts_company ON contacts(company_id);

-- contact_id je iba odkaz; údaje odberateľa sú na faktúre skopírované,
-- takže zmazanie kontaktu faktúru nezmení (preto bez REFERENCES).
ALTER TABLE invoices ADD COLUMN contact_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE invoices ADD COLUMN non_vat_payer INTEGER NOT NULL DEFAULT 0;
