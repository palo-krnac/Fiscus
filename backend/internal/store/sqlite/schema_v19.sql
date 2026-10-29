-- Verzia 19: odoslané e-maily (doklady v prílohe, skúšobné správy).
CREATE TABLE emails (
    id          INTEGER PRIMARY KEY,
    company_id  INTEGER NOT NULL REFERENCES companies(id),
    doc_kind    TEXT NOT NULL,
    doc_id      INTEGER NOT NULL DEFAULT 0,
    doc_number  TEXT NOT NULL DEFAULT '',
    to_addr     TEXT NOT NULL,
    cc_addr     TEXT NOT NULL DEFAULT '',
    subject     TEXT NOT NULL,
    file        TEXT NOT NULL DEFAULT '',
    sent        TEXT NOT NULL,
    user_name   TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX emails_company_sent ON emails (company_id, sent);
CREATE INDEX emails_doc ON emails (company_id, doc_kind, doc_id);
