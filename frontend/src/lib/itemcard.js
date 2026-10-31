// Skladová karta produktu: hľadanie v pohyboch, parametre a poradie obrázkov.
// Stavy a ocenenie počíta backend, tu sa iba filtruje a preskladáva.
import { matches, searchTerms } from "./search.js";
import { docLabel } from "./stock.js";

/** Pohyby karty podľa hľadania (doklad, partner, druh) a skladu. */
export function filterMoves(moves, { search = "", warehouse = "" } = {}) {
  const terms = searchTerms(search);
  return (moves ?? []).filter(
    (mv) =>
      (!warehouse || mv.warehouse === warehouse) &&
      matches(terms, [
        mv.number,
        mv.partner,
        moveLabel(mv),
        mv.warehouse,
        mv.date,
      ]),
  );
}

/** Názov druhu pohybu; presun rozlišuje smer podľa znamienka množstva. */
export function moveLabel(mv) {
  if (mv.kind === "transfer")
    return Number(mv.quantity) < 0 ? "Presun – výdaj" : "Presun – príjem";
  return docLabel(mv);
}

export const blankParam = () => ({ name: "", value: "", unit: "" });

/** Parametre na uloženie: orezané, bez prázdnych riadkov. */
export const cleanParams = (params) =>
  (params ?? [])
    .map((p) => ({
      name: p.name.trim(),
      value: p.value.trim(),
      unit: p.unit.trim(),
    }))
    .filter((p) => p.name || p.value || p.unit);

/** Nové poradie id obrázkov: presun z indexu from na index to. */
export function moveImage(ids, from, to) {
  if (to < 0 || to >= ids.length || from === to) return ids;
  const out = [...ids];
  const [x] = out.splice(from, 1);
  out.splice(to, 0, x);
  return out;
}

/** Typy obrázkov, ktoré server prijme (overuje aj podľa obsahu). */
export const imageTypes = ["image/jpeg", "image/png", "image/webp"];
export const maxImageSize = 2 * 1024 * 1024;
export const maxImages = 10;

/** Chyba pred odoslaním obrázka (null = v poriadku). */
export function imageProblem(file, count) {
  if (count >= maxImages)
    return `Produkt môže mať najviac ${maxImages} obrázkov.`;
  if (!imageTypes.includes(file.type))
    return `${file.name}: iba JPEG, PNG alebo WebP.`;
  if (file.size > maxImageSize) return `${file.name}: najviac 2 MB.`;
  return null;
}

/** Kontrolná číslica EAN/GTIN (8, 12, 13, 14 číslic) — rovnaké pravidlo ako na serveri. */
export function validEAN(s) {
  if (!/^(\d{8}|\d{12,14})$/.test(s)) return false;
  let sum = 0;
  for (let i = s.length - 2, w = 3; i >= 0; i--, w = 4 - w)
    sum += Number(s[i]) * w;
  return (10 - (sum % 10)) % 10 === Number(s[s.length - 1]);
}
