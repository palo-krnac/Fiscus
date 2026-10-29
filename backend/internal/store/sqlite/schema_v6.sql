-- Verzia 6: používatelia, roly vo firmách, relácie a audit.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    email         TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL DEFAULT '',
    active        INTEGER NOT NULL DEFAULT 1,
    super_admin   INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX users_email ON users(lower(email));

CREATE TABLE memberships (
    user_id    INTEGER NOT NULL REFERENCES users(id),
    company_id INTEGER NOT NULL REFERENCES companies(id),
    role       TEXT NOT NULL,
    PRIMARY KEY (user_id, company_id)
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    expires_at INTEGER NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);

-- Audit je iba na pridávanie. Záznamy sú zreťazené hashom (prev_hash → hash).
CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY,
    time        TEXT NOT NULL,
    company_id  INTEGER NOT NULL DEFAULT 0,
    user_id     INTEGER NOT NULL DEFAULT 0,
    user_name   TEXT NOT NULL DEFAULT '',
    channel     TEXT NOT NULL,
    ip          TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL,
    action      TEXT NOT NULL,
    entity_type TEXT NOT NULL DEFAULT '',
    entity_id   INTEGER NOT NULL DEFAULT 0,
    summary     TEXT NOT NULL DEFAULT '',
    before_json TEXT NOT NULL DEFAULT '',
    after_json  TEXT NOT NULL DEFAULT '',
    prev_hash   TEXT NOT NULL DEFAULT '',
    hash        TEXT NOT NULL
);
CREATE INDEX audit_company_time ON audit_log(company_id, time);
-- zákaz úprav a mazania auditu aj priamo v databáze
CREATE TRIGGER audit_no_update BEFORE UPDATE ON audit_log BEGIN SELECT RAISE(ABORT, 'audit sa nedá meniť'); END;
CREATE TRIGGER audit_no_delete BEFORE DELETE ON audit_log BEGIN SELECT RAISE(ABORT, 'audit sa nedá mazať'); END;
