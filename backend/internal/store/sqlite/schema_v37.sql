-- Súbor prijatého dokladu (fotka bločku z mobilu, sken faktúry).
CREATE TABLE purchase_files (
    purchase_id  INTEGER PRIMARY KEY REFERENCES purchases(id) ON DELETE CASCADE,
    company_id   INTEGER NOT NULL REFERENCES companies(id),
    name         TEXT NOT NULL,
    content_type TEXT NOT NULL,
    data         BLOB NOT NULL,
    created      TEXT NOT NULL
);
CREATE INDEX purchase_files_company ON purchase_files(company_id);
