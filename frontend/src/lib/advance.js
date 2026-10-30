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

/**
 * Čo sa stane s platbou zálohy (advanceMode určuje server):
 * receipt – daňový doklad k platbe, within15 – faktúra k dodaniu do 15 dní,
 * untaxed – platba nezakladá daňovú povinnosť, nonvat – neplatiteľ, manual.
 */
export function advanceModeText(inv) {
  switch (inv?.advanceMode) {
    case 'receipt':
      return inv.reverseCharge || inv.lines?.some((l) => l.reverse)
        ? 'Zálohová faktúra nie je daňový doklad. Ku každej platbe vznikne doklad k prijatej platbe bez DPH s prenesením daňovej povinnosti a zálohu potom vyúčtujete.'
        : 'Zálohová faktúra nie je daňový doklad. Ku každej prijatej platbe (aj čiastočnej) vznikne daňový doklad k prijatej platbe a zálohu potom vyúčtujete.'
    case 'within15':
      return 'Dodanie do 15 dní od platby: k platbe sa daňový doklad nevystaví. Zálohu vyúčtujte faktúrou s dodaním najneskôr 15. deň po platbe a v tom istom období DPH.'
    case 'untaxed':
      return 'Pri dodaní tovaru do EÚ, vývoze a službe mimo EÚ platba vopred nezakladá daňovú povinnosť — doklad k platbe sa nevystaví, DPH a súhrnný výkaz sú až z vyúčtovania.'
    case 'manual':
      return `${inv.advanceNote}.`.replace(/\.\.$/, '.')
    default:
      return 'Zálohová faktúra nie je daňový doklad. Po úhrade zálohu vyúčtujete.'
  }
}

/** Voľbu „dodanie do 15 dní“ má zmysel ponúknuť iba tam, kde by inak vznikol doklad k platbe. */
export const canChooseWithin15 = (inv) => inv?.kind === 'proforma' && ['receipt', 'within15'].includes(inv.advanceMode)

/** Posledný deň dodania pri platbe bez dokladu (deň platby + 15 dní), ISO dátum. */
export function within15Deadline(paidISO) {
  const d = new Date(`${paidISO}T12:00:00Z`)
  d.setUTCDate(d.getUTCDate() + 15)
  return d.toISOString().slice(0, 10)
}
