// Formátovanie čísel a dátumov podľa slovenských zvyklostí.
// Sumy prichádzajú z API ako reťazce ("1234.50"), aby sa nestratila presnosť.

const moneyFmt = new Intl.NumberFormat('sk-SK', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})

/** "1234.5" → "1 234,50" (bez meny — do tabuliek) */
export function formatAmount(value) {
  if (value === null || value === undefined || value === '') return ''
  const n = Number(value)
  if (Number.isNaN(n)) return String(value)
  // Intl používa úzku nedeliteľnú medzeru; zjednotíme na obyčajnú nedeliteľnú
  return moneyFmt.format(n).replace(/ /g, ' ')
}

/** "1234.5" → "1 234,50 €" */
export function formatMoney(value, currency = 'EUR') {
  const a = formatAmount(value)
  if (a === '') return ''
  return currency === 'EUR' ? `${a} €` : `${a} ${currency}`
}

/** "2026-10-29" → "29. 10. 2026" */
export function formatDate(iso) {
  if (!iso) return ''
  const [y, m, d] = iso.split('-')
  if (!d) return iso
  return `${Number(d)}. ${Number(m)}. ${y}`
}

/** Date (lokálny) → "2026-10-29" bez posunu časového pásma */
export function toISODate(date) {
  if (!date) return null
  const p = (n) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${p(date.getMonth() + 1)}-${p(date.getDate())}`
}

/** "2026-10-29" → Date (lokálna polnoc) */
export function fromISODate(iso) {
  if (!iso) return null
  const [y, m, d] = iso.split('-').map(Number)
  return new Date(y, m - 1, d)
}

/** Množstvo: "2.5" → "2,5", "16" → "16" */
export function formatQuantity(value) {
  if (value === null || value === undefined || value === '') return ''
  const n = Number(value)
  if (Number.isNaN(n)) return String(value)
  return new Intl.NumberFormat('sk-SK', { maximumFractionDigits: 4 }).format(n)
}

/** Zápis používateľa "1 234,5" → "1234.5" pre API; neplatný vstup → null */
export function parseNumber(input) {
  if (input === null || input === undefined) return null
  const s = String(input).replace(/[\s  ]/g, '').replace(',', '.')
  if (s === '' || !/^-?\d+(\.\d+)?$/.test(s)) return null
  return s
}

export const STATUS = {
  draft: { label: 'Koncept', tone: 'draft' },
  issued: { label: 'Vystavená', tone: 'issued' },
  paid: { label: 'Uhradená', tone: 'paid' },
  cancelled: { label: 'Stornovaná', tone: 'cancelled' },
}

/** Stav prijatej faktúry vrátane omeškania. */
export function purchaseStatusOf(row) {
  if (row.overdue) return statusOf(row)
  return row.status === 'paid' ? { label: 'Uhradená', tone: 'paid' } : { label: 'Neuhradená', tone: 'issued' }
}

/** Stav riadku zoznamu vrátane omeškania. */
export function statusOf(row) {
  const partly = row.status === 'issued' && Number(row.paidAmount) !== 0 && !Number.isNaN(Number(row.paidAmount))
  if (row.overdue) {
    const d = row.daysOverdue
    return { label: `Po splatnosti ${d} ${d === 1 ? 'deň' : d < 5 ? 'dni' : 'dní'}${partly ? ', čiastočne uhradená' : ''}`, tone: 'overdue' }
  }
  if (partly) return { label: 'Čiastočne uhradená', tone: 'issued' }
  // vyrovnaná iba zápočtom dobropisu (žiadne peniaze neprišli ani neodišli)
  if (row.status === 'paid' && row.payments?.length && row.payments.every((p) => p.source === 'credit_note')) {
    return row.kind === 'credit_note'
      ? { label: 'Započítaný s faktúrou', tone: 'paid' }
      : { label: 'Vyrovnaná dobropisom', tone: 'paid' }
  }
  return STATUS[row.status] ?? { label: row.status, tone: 'draft' }
}

/** Odkiaľ je úhrada faktúry. */
export const paymentSource = (s) => ({ bank: 'banka', cash: 'pokladňa', manual: 'ručne', credit_note: 'zápočet dobropisu' })[s] ?? s
