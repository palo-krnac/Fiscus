import { describe, expect, it } from 'vitest'
import { canCredit, canSettle, editorTitle, hasDeductions, kindBadge, kindLabel, receiptsOf, settledAdvances } from './advance.js'

describe('zálohy', () => {
  it.each([
    [{ kind: 'invoice' }, 'Nová faktúra'],
    [{ kind: 'proforma' }, 'Nová zálohová faktúra'],
    [{ kind: 'proforma', id: 3 }, 'Koncept zálohovej faktúry'],
    [{ kind: 'proforma', id: 3, number: 'ZF20260001' }, 'Zálohová faktúra ZF20260001'],
    [{ kind: 'advance_receipt', id: 4, number: 'ZD20260001' }, 'Doklad k prijatej platbe ZD20260001'],
    [{ kind: 'credit_note', id: 5 }, 'Koncept dobropisu'],
    [{ kind: 'invoice', id: 6, advance: { deductions: [{}] } }, 'Koncept vyúčtovacej faktúry'],
    [{ kind: 'invoice', id: 6, number: 'FA20260050', advance: { deductions: [{}] } }, 'Vyúčtovacia faktúra FA20260050'],
  ])('nadpis %o', (inv, title) => {
    expect(editorTitle(inv)).toBe(title)
  })
  it('štítky', () => {
    expect(kindBadge('invoice')).toBe('')
    expect(kindBadge('proforma')).toBe('záloha')
    expect(kindLabel('advance_receipt')).toBe('daňový doklad k prijatej platbe')
  })
  it.each([
    [{ kind: 'proforma', status: 'paid' }, true],
    [{ kind: 'proforma', status: 'issued' }, false],
    [{ kind: 'proforma', status: 'issued', paidAmount: '100.00' }, true],
    [{ kind: 'proforma', status: 'issued', paidAmount: '0.00' }, false],
    [{ kind: 'proforma', status: 'paid', advance: { finalId: 9 } }, false],
    [{ kind: 'invoice', status: 'paid' }, false],
  ])('vyúčtovanie %o', (inv, ok) => {
    expect(canSettle(inv)).toBe(ok)
  })
  it('dobropis a odpočet', () => {
    expect(canCredit({ kind: 'proforma' })).toBe(false)
    expect(canCredit({ kind: 'advance_receipt' })).toBe(true)
    expect(hasDeductions({ advance: { deductions: [{}] } })).toBe(true)
    expect(hasDeductions({})).toBe(false)
  })
  it('doklady k platbám', () => {
    expect(receiptsOf({})).toEqual([])
    expect(receiptsOf({ paidDate: '2026-10-10', summary: { total: '492.00' }, advance: { receiptId: 4, receiptNumber: 'ZD1' } })).toEqual([
      { id: 4, number: 'ZD1', date: '2026-10-10', amount: '492.00' },
    ])
    expect(receiptsOf({ advance: { receiptId: 4, receipts: [{ id: 4 }, { id: 5 }] } })).toHaveLength(2)
  })
  it('odpočítané zálohy vo vyúčtovaní', () => {
    const inv = {
      advance: {
        proformaId: 7,
        proformaNumber: 'ZF1',
        deductions: [
          { proformaId: 7, receiptId: 11, number: 'ZD1', total: '100.00' },
          { proformaId: 7, receiptId: 12, number: 'ZD2', total: '50.00' },
          { proformaId: 8, proformaNumber: 'ZF2', receiptId: 13, number: 'ZD3', total: '20.00' },
        ],
      },
    }
    const got = settledAdvances(inv)
    expect(got.map((a) => [a.id, a.number, a.total, a.docs.length])).toEqual([[7, 'ZF1', 150, 2], [8, 'ZF2', 20, 1]])
    expect(settledAdvances({})).toEqual([])
  })
})
