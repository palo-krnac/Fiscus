import { describe, expect, it } from 'vitest'
import { emptyTax, taxErrors, taxLabel, taxPayload } from './cashvat.js'

describe('cashvat', () => {
  it('nový rozpis prevezme sumu dokladu', () => {
    expect(emptyTax('61,50').rows).toEqual([{ vatRate: 23, gross: '61,50' }])
  })
  it('payload prevedie čiarky a vynechá prázdne riadky', () => {
    const t = { ...emptyTax('1 061,50'), rows: [{ vatRate: '23', gross: '1 061,50' }, { vatRate: 5, gross: '' }] }
    expect(taxPayload(t).rows).toEqual([{ vatRate: 23, gross: '1061.50' }])
    expect(taxPayload(null)).toBeUndefined()
  })
  it('chyby rozpisu', () => {
    expect(taxErrors([{ field: 'tax.rows', message: 'x' }, { field: 'amount', message: 'y' }, { field: 'tax', message: 'z' }])).toEqual({ 'tax.rows': 'x', tax: 'z' })
  })
  it('popis do zoznamu', () => {
    const f = (v) => v.toFixed(2).replace('.', ',')
    expect(taxLabel({ tax: { rows: [{ vat: '11.50' }, { vat: '0.50' }] } }, f)).toBe('DPH 12,00 €')
    expect(taxLabel({}, f)).toBe('')
  })
})
