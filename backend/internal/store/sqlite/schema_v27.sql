-- Verzia 27: peňažný denník — druh výdavku prijatej faktúry a ručné rozpisy pohybov.
ALTER TABLE purchases ADD COLUMN expense TEXT NOT NULL DEFAULT '';

CREATE TABLE journal_overrides (
    company_id INTEGER NOT NULL REFERENCES companies(id),
    source_key TEXT    NOT NULL, -- zdroj pohybu: bank:12, cash:5
    lines_json TEXT    NOT NULL,
    note       TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (company_id, source_key)
);
