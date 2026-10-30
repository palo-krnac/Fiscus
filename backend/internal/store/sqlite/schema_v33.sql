-- Verzia 33: ďalšie údaje prijatej faktúry v JSON (čiastočný odpočet pri
-- vozidle, vozidlo, číslo opravovanej faktúry pri dobropise, pomerné
-- odpočítanie podľa § 50 a koeficient).
ALTER TABLE purchases ADD COLUMN extra_json TEXT NOT NULL DEFAULT '';
