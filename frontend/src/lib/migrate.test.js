import { describe, expect, it } from 'vitest'
import { filterWarnings, newTotal, otherText, sources, summaryRows } from './migrate.js'

describe('prenos dát', () => {
  const report = {
    contacts: { new: 2, existing: 1, skipped: 0 },
    invoices: { new: 2, existing: 0, skipped: 1 },
    purchases: { new: 1 },
    other: { order: 1, issuedProformaInvoice: 2 },
    warnings: ['Faktúra 260100099 je v USD – preskočená', 'Zálohová faktúra ZF1 sa neprenáša'],
  }
  it.each([
    ['contacts', 2, 1, 0],
    ['items', 0, 0, 0],
    ['invoices', 2, 0, 1],
    ['purchases', 1, 0, 0],
  ])('zhrnutie %s', (key, n, e, s) => {
    const r = summaryRows(report).find((x) => x.key === key)
    expect([r.new, r.existing, r.skipped]).toEqual([n, e, s])
  })
  it('spolu nových', () => {
    expect(newTotal(report)).toBe(5)
    expect(newTotal(null)).toBe(0)
  })
  it('neprenášané agendy', () => {
    expect(otherText(report.other)).toBe('zálohové faktúry 2, objednávky 1')
    expect(otherText(undefined)).toBe('')
  })
  it.each([
    ['', 2],
    ['usd', 1],
    ['zalohova faktura', 1],
    ['nič', 0],
  ])('hľadanie v upozorneniach „%s“', (q, n) => {
    expect(filterWarnings(report.warnings, q)).toHaveLength(n)
  })
  it.each(['pohoda', 'money'])('zdroj %s je hotový', (id) => {
    const s = sources.find((x) => x.id === id)
    expect(s.ready).toBe(true)
    expect(s.accept).toContain('.xml')
  })
  it('agendy Money S3', () => {
    expect(otherText({ ObjPrij: 3, PokDokl: 1 })).toBe('prijaté objednávky 3, pokladničné doklady 1')
  })
})
