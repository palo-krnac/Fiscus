// Legislatíva: prevod sadzieb DPH medzi textom vo formulári a API.

/** "23, 19; 5 0" → [23, 19, 5, 0]; neplatné časti vynechá. */
export function parseRates(text) {
  return String(text ?? '')
    .split(/[,;\s]+/)
    .map((x) => x.replace('%', '').trim())
    .filter((x) => x !== '' && /^\d+$/.test(x))
    .map(Number)
}

export const formatRates = (rates) => (rates ?? []).join(', ')

/** Chyby polí sadzieb ECB: "ecb[3].rate" → { 3: { rate: '…' } }. */
export function ecbErrors(fieldMap) {
  const out = {}
  for (const [k, msg] of Object.entries(fieldMap ?? {})) {
    const m = k.match(/^ecb\[(\d+)\]\.(\w+)$/)
    if (m) (out[m[1]] ??= {})[m[2]] = msg
  }
  return out
}

/** Deň sviatku na zobrazenie: "05-01" → "1. 5.", "easter-2" → "Veľká noc − 2 dni". */
export function holidayDayLabel(day) {
  const e = /^easter([+-]\d+)$/.exec(day ?? '')
  if (e) {
    const n = Number(e[1])
    const abs = Math.abs(n)
    const unit = abs === 1 ? 'deň' : abs < 5 ? 'dni' : 'dní'
    return n === 0 ? 'Veľkonočná nedeľa' : `Veľká noc ${n < 0 ? '−' : '+'} ${abs} ${unit}`
  }
  const m = /^(\d{2})-(\d{2})$/.exec(day ?? '')
  return m ? `${Number(m[2])}. ${Number(m[1])}.` : day ?? ''
}

/** Zoznam rokov "2026, 2028" → [2026, 2028]; neplatné časti vynechá. */
export function parseYears(text) {
  return [...new Set(String(text ?? '').split(/[\s,;]+/).map(Number).filter((y) => Number.isInteger(y) && y >= 1990 && y <= 2200))].sort()
}
