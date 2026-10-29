-- Verzia 25: predmet dokladu (krátky popis do zoznamov) a text nad položkami.
ALTER TABLE invoices ADD COLUMN subject TEXT NOT NULL DEFAULT '';
ALTER TABLE invoices ADD COLUMN intro TEXT NOT NULL DEFAULT '';
