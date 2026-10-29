// Peňažný denník: popisy stĺpcov a pomôcky pre ručný rozpis pohybu.
// Rozpis overuje server (súčet = suma pohybu); tu je iba zobrazenie.

/** Krátke označenie stĺpca podľa opatrenia MF SR (§ 4 ods. 6). */
export function columnCode(code) {
  return { 'c.in': 'c', 'c.out': 'c', 'f.dph': 'f', 'g.dph': 'g', 'g.dm': 'g' }[code] ?? code
}

export function columnLabel(columns, code) {
  return columns?.find((c) => c.code === code)?.label ?? code
}

/** Stĺpce vhodné pre smer pohybu (príjem → príjmové, výdavok → výdavkové). */
export function columnsFor(columns, amount) {
  const income = Number(amount) >= 0
  return (columns ?? []).filter((c) => c.income === income)
}

/** Stav pohybu v denníku. */
export function entryState(e) {
  if (e.means === 'close') return { label: 'Uzávierka', tone: 'draft' }
  if (!e.lines?.length) return { label: 'Nerozpísaný', tone: 'overdue' }
  if (e.auto) return { label: 'Návrh Fiškusa', tone: 'issued' }
  return { label: 'Rozpísaný ručne', tone: 'paid' }
}

/** Rozpis ako text: „Služby 100,00 · Zaplatená DPH 23,00“. */
export function linesText(columns, lines, fmt) {
  return (lines ?? []).map((l) => `${columnLabel(columns, l.column)} ${fmt(l.amount)}`).join(' · ')
}

const cents = (v) => {
  const n = Number(String(v ?? '').replace(',', '.'))
  return Number.isFinite(n) ? Math.round(n * 100) : 0
}

/** Koľko z pohybu ešte nie je rozpísané (pri zadávaní, v centoch → reťazec). */
export function unallocated(amount, lines) {
  const total = Math.abs(cents(amount))
  const used = (lines ?? []).reduce((s, l) => s + cents(l.amount), 0)
  return ((total - used) / 100).toFixed(2)
}

/** Rýchle rozpisy celou sumou pre najčastejšie pohyby bez dokladu. */
export function presets(amount) {
  return Number(amount) >= 0
    ? [
        { column: 'd3', label: 'Ostatný príjem' },
        { column: 'f', label: 'Príjem mimo základu dane' },
        { column: 'c.in', label: 'Prevod (priebežná položka)' },
      ]
    : [
        { column: 'e2', label: 'Služby' },
        { column: 'e1', label: 'Zásoby' },
        { column: 'e4', label: 'Poistné a príspevky' },
        { column: 'e6', label: 'Ostatné výdavky' },
        { column: 'g.dph', label: 'Platba DPH' },
        { column: 'g', label: 'Výdavok mimo základu dane' },
        { column: 'c.out', label: 'Prevod (priebežná položka)' },
      ]
}

/** Absolútna suma pohybu pre riadok rozpisu (výdavok sa v stĺpci píše kladne). */
export const absAmount = (amount) => String(amount ?? '').replace(/^-/, '')
