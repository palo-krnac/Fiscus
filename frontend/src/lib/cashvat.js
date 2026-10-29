// DPH na pokladničnom doklade (blok, tržba z e-kasy). Základ a daň počíta
// server (POST …/cash/preview) — tu sa iba pripraví a prenesie zadanie.

const num = (s) => String(s ?? '').replace(/\s/g, '').replace(',', '.')

/** Nový rozpis DPH: jeden riadok v základnej sadzbe so sumou dokladu. */
export function emptyTax(amount, rate = 23) {
  return { rows: [{ vatRate: rate, gross: String(amount ?? '') }], supplierIcdph: '', noDeduction: false, expense: 'services', goods: false }
}

/** Rozpis DPH na odoslanie (desatinná čiarka → bodka, prázdne riadky preč). */
export function taxPayload(tax) {
  if (!tax) return undefined
  return {
    ...tax,
    rows: tax.rows.filter((r) => num(r.gross) !== '').map((r) => ({ vatRate: Number(r.vatRate), gross: num(r.gross) })),
  }
}

/** Chyby polí rozpisu DPH z odpovede servera (tax.*) pre formulár. */
export function taxErrors(fields = []) {
  const out = {}
  for (const f of fields) if (f.field === 'tax' || f.field.startsWith('tax.')) out[f.field] = f.message
  return out
}

/** Krátky popis DPH dokladu do zoznamu: „DPH 11,50 €“ (sumy zo servera). */
export function taxLabel(doc, formatAmount) {
  if (!doc?.tax?.rows?.length) return ''
  const sum = doc.tax.rows.reduce((a, r) => a + Number(r.vat), 0)
  return `DPH ${formatAmount(sum)} €`
}
