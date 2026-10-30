-- Verzia 31: kniha jázd (vozidlá, jazdy a tankovania).
CREATE TABLE vehicles (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    data       TEXT NOT NULL
);
CREATE INDEX vehicles_company ON vehicles (company_id);
-- kind: 'trip' (jazda) alebo 'fuel' (tankovanie)
CREATE TABLE logbook (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    vehicle_id INTEGER NOT NULL,
    kind       TEXT NOT NULL,
    date       TEXT NOT NULL,
    data       TEXT NOT NULL
);
CREATE INDEX logbook_vehicle ON logbook (company_id, kind, vehicle_id, date);
