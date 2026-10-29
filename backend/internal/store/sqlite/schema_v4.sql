-- Verzia 4: dobropisy — odkaz na pôvodnú faktúru a dôvod opravy (JSON).
ALTER TABLE invoices ADD COLUMN correction_json TEXT NOT NULL DEFAULT '';
