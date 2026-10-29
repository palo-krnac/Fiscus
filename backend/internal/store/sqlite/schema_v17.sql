-- Verzia 17: logo firmy na dokladoch.
CREATE TABLE company_logos (
    company_id    INTEGER PRIMARY KEY REFERENCES companies(id),
    content_type  TEXT NOT NULL,
    data          BLOB NOT NULL,
    updated       TEXT NOT NULL
);
