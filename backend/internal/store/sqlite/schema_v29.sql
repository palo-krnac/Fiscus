-- Verzia 29: register dlhodobého majetku (daňové odpisy).
CREATE TABLE assets (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    acquired   TEXT NOT NULL DEFAULT '',
    data       TEXT NOT NULL
);
CREATE INDEX assets_company ON assets (company_id, acquired);
