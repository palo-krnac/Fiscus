-- Verzia 12: cenové ponuky. Ponuka nie je daňový doklad; obsah (odberateľ,
-- položky, rekapitulácia) je v JSON, stĺpce slúžia na triedenie a kontrolu čísla.
CREATE TABLE quotes (
    id          INTEGER PRIMARY KEY,
    company_id  INTEGER NOT NULL REFERENCES companies(id),
    number      TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL,
    issue_date  TEXT NOT NULL DEFAULT '',
    data        TEXT NOT NULL
);
CREATE UNIQUE INDEX quotes_number ON quotes(company_id, number) WHERE number <> '';
CREATE INDEX quotes_company_date ON quotes(company_id, issue_date);
