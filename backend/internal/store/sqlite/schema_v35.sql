-- Kontaktné osoby a ďalšie adresy v adresári, dodacia adresa a kontaktná
-- osoba na faktúre (JSON — zoznamy sa nikdy nefiltrujú v SQL).
ALTER TABLE contacts ADD COLUMN persons_json TEXT NOT NULL DEFAULT '';
ALTER TABLE contacts ADD COLUMN addresses_json TEXT NOT NULL DEFAULT '';
ALTER TABLE invoices ADD COLUMN delivery_json TEXT NOT NULL DEFAULT '';
