import { describe, expect, it } from 'vitest'
import { auditColumns, contactColumns, invoiceColumns, purchaseColumns } from './columns.js'
import { toCSV } from './export.js'

const inv = {
  number: 'DB20260001', kind: 'credit_note', status: 'issued', customer: { name: 'Odberateľ', ico: '31322832', country: 'SK' },
  issueDate: '2026-10-29', dueDate: '2026-11-12', paymentMethod: 'transfer',
  summary: { base: '-252.00', vat: '-57.96', total: '-309.96' },
}

describe('stĺpce exportov', () => {
  it('faktúra vrátane dobropisu', () => {
    const csv = toCSV(invoiceColumns, [inv]).slice(1).split('\r\n')
    const head = csv[0].split(';')
    const row = csv[1].split(';')
    const col = (h) => row[head.indexOf(h)]
    expect(col('Druh')).toBe('dobropis')
    expect(col('Spolu')).toBe('-309,96')
    expect(col('Vystavená')).toBe('29.10.2026')
    expect(col('Stav')).toBe('Vystavená')
  })
  it('každý stĺpec má hlavičku a nespadne na prázdnom riadku', () => {
    for (const cols of [invoiceColumns, purchaseColumns, contactColumns, auditColumns(() => '')]) {
      for (const c of cols) {
        expect(c.header).toBeTruthy()
        expect(() => c.value({ summary: {}, customer: {}, supplier: {}, user: {}, time: '2026-10-29T10:00:00Z' })).not.toThrow()
      }
    }
  })
})
