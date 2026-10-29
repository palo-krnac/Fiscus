-- Verzia 26: dvojfázové overenie používateľa (kľúč TOTP, záložné kódy ako odtlačky).
ALTER TABLE users ADD COLUMN mfa_json TEXT NOT NULL DEFAULT '';
