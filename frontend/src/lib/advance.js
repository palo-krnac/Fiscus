// Zálohy: názvy druhov dokladov a odpočet zálohy vo vyúčtovaní (sumy z API).

/** Je to vyúčtovacia faktúra s odpočtom zálohy? */
export const hasDeductions = (inv) => !!inv?.advance?.deductions?.length

/** Krátky štítok druhu dokladu do zoznamu ('' pre bežnú faktúru). */
export const kindBadge = (kind) => ({ credit_note: 'dobropis', proforma: 'záloha', advance_receipt: 'k platbe' })[kind] ?? ''

/** Názov druhu dokladu do exportu. */
export const kindLabel = (kind) =>
  ({ credit_note: 'dobropis', proforma: 'zálohová faktúra', advance_receipt: 'daňový doklad k prijatej platbe' })[kind] ?? 'faktúra'

/** Nadpis v editore. */
export function editorTitle(inv) {
  const named = { credit_note: 'Dobropis', proforma: 'Zálohová faktúra', advance_receipt: 'Doklad k prijatej platbe' }[inv.kind] ?? 'Faktúra'
  if (hasDeductions(inv)) return inv.number ? `Vyúčtovacia faktúra ${inv.number}` : 'Koncept vyúčtovacej faktúry'
  if (inv.number) return `${named} ${inv.number}`
  if (inv.kind === 'credit_note') return 'Koncept dobropisu'
  if (inv.kind === 'proforma') return inv.id ? 'Koncept zálohovej faktúry' : 'Nová zálohová faktúra'
  return inv.id ? 'Koncept faktúry' : 'Nová faktúra'
}


/** Do nového dokladu sa smie dobropis iba k faktúre a dokladu k platbe — nie k zálohe. */
export const canCredit = (inv) => inv.kind === 'invoice' || inv.kind === 'advance_receipt'

/** Zálohu možno vyúčtovať, ak je uhradená aspoň časť a ešte nemá vyúčtovanie. */
export const canSettle = (inv) =>
  inv.kind === 'proforma' && !inv.advance?.finalId && (inv.status === 'paid' || (inv.status === 'issued' && Number(inv.paidAmount) > 0))

/** Doklady k prijatým platbám zálohy (aj zo starších údajov s jedným dokladom). */
export function receiptsOf(inv) {
  const a = inv?.advance
  if (!a) return []
  if (a.receipts?.length) return a.receipts
  return a.receiptId ? [{ id: a.receiptId, number: a.receiptNumber, date: inv.paidDate, amount: inv.summary?.total }] : []
}

/** Zálohové faktúry odpočítané vo vyúčtovaní: [{ id, number, total }] (sumy zo servera). */
export function settledAdvances(inv) {
  const a = inv?.advance
  if (!a?.deductions?.length) return []
  const out = new Map()
  for (const d of a.deductions) {
    const id = d.proformaId || a.proformaId
    const cur = out.get(id) ?? { id, number: d.proformaNumber || (id === a.proformaId ? a.proformaNumber : ''), total: 0, docs: [] }
    cur.total += Number(d.total)
    cur.docs.push(d.number)
    out.set(id, cur)
  }
  return [...out.values()]
}
