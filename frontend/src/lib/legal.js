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
