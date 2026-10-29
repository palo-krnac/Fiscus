-- Verzia 15: nastavenia programu spoločné pre všetky firmy (napr. legislatíva:
-- sadzby DPH, sadzby ECB, parametre úroku z omeškania). Hodnota je JSON.
CREATE TABLE settings (
    key    TEXT PRIMARY KEY,
    value  TEXT NOT NULL
);
