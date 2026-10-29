-- Verzia 22: prijaté faktúry v cudzej mene — mena, kurz a sumy v eurách v JSON.
ALTER TABLE purchases ADD COLUMN fx_json TEXT NOT NULL DEFAULT '';
