-- Verzia 24: e-faktúry odoslané cez digitálneho poštára a prijaté z jeho schránky.
CREATE TABLE einvoices (
    id          INTEGER PRIMARY KEY,
    company_id  INTEGER NOT NULL REFERENCES companies(id),
    direction   TEXT NOT NULL,
    doc_id      INTEGER NOT NULL DEFAULT 0,
    doc_number  TEXT NOT NULL DEFAULT '',
    partner     TEXT NOT NULL DEFAULT '',
    peppol_id   TEXT NOT NULL DEFAULT '',
    file        TEXT NOT NULL DEFAULT '',
    message_id  TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    created     TEXT NOT NULL,
    user_name   TEXT NOT NULL DEFAULT '',
    data        BLOB
);
CREATE INDEX einvoices_company ON einvoices (company_id, created);
CREATE INDEX einvoices_doc ON einvoices (company_id, direction, doc_id);
