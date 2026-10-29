-- Verzia 14: upomienky k faktúram po splatnosti. Obsah je v JSON.
CREATE TABLE reminders (
    id          INTEGER PRIMARY KEY,
    company_id  INTEGER NOT NULL REFERENCES companies(id),
    number      TEXT NOT NULL DEFAULT '',
    date        TEXT NOT NULL,
    data        TEXT NOT NULL
);
CREATE INDEX reminders_company_date ON reminders(company_id, date);
