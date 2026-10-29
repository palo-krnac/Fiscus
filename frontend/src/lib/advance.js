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

/** Zálohu možno vyúčtovať, ak je uhradená a ešte nemá vyúčtovanie. */
export const canSettle = (inv) => inv.kind === 'proforma' && inv.status === 'paid' && !inv.advance?.finalId
