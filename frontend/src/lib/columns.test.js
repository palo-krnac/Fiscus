import { describe, expect, it } from 'vitest'
import { auditColumns, contactColumns, invoiceColumns, purchaseColumns, quoteColumns, recurringColumns, reminderColumns, overdueColumns, ecbColumns, vatPeriodColumns, itemColumns, rateColumns } from './columns.js'
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
    expect(col('Spolu €')).toBe('-309,96')
    expect(col('Kurz')).toBe('')
  })
  it('faktúra v cudzej mene: sumy v mene aj v eurách', () => {
    const czk = { ...inv, kind: 'invoice', currency: 'CZK', fxRate: '25.123', summary: { base: '1000.00', vat: '230.00', total: '1230.00' },
      local: { base: '39.80', vat: '9.15', total: '48.95' } }
    const csv = toCSV(invoiceColumns, [czk]).slice(1).split('\r\n')
    const head = csv[0].split(';')
    const col = (h) => csv[1].split(';')[head.indexOf(h)]
    expect([col('Spolu'), col('Mena'), col('Kurz'), col('Spolu €'), col('DPH €')]).toEqual(['1230,00', 'CZK', '25,123', '48,95', '9,15'])
  })
  it('ponuka: stav a faktúra', () => {
    const q = { number: 'CP20260003', status: 'invoiced', invoiceId: 7, invoiceNumber: '', customer: { name: 'X' },
      issueDate: '2026-10-01', validUntil: '2026-10-31', summary: { total: '123.00' } }
    const csv = toCSV(quoteColumns, [q, { ...q, status: 'issued', expired: true, invoiceId: 0 }]).slice(1).split('\r\n')
    const head = csv[0].split(';')
    const col = (i, h) => csv[i].split(';')[head.indexOf(h)]
    expect(col(1, 'Stav')).toBe('Vyfakturovaná')
    expect(col(1, 'Faktúra')).toBe('koncept')
    expect(col(1, 'Platná do')).toBe('31.10.2026')
    expect(col(2, 'Stav')).toBe('Po platnosti')
    expect(col(2, 'Faktúra')).toBe('')
  })
  it('každý stĺpec má hlavičku a nespadne na prázdnom riadku', () => {
    for (const cols of [rateColumns, invoiceColumns, purchaseColumns, contactColumns, quoteColumns, recurringColumns, reminderColumns, overdueColumns, ecbColumns, vatPeriodColumns, itemColumns, auditColumns(() => '')]) {
      for (const c of cols) {
        expect(c.header).toBeTruthy()
        expect(() => c.value({ summary: {}, customer: {}, supplier: {}, user: {}, time: '2026-10-29T10:00:00Z' })).not.toThrow()
      }
    }
  })
})
