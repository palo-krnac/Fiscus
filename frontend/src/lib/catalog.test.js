import { describe, expect, it } from 'vitest'
import { filterItems, lineFromItem, importActions, importErrors, importLabel } from './catalog.js'

const items = [
  { id: 1, code: 'PLC', name: 'Programovanie PLC', unit: 'hod', priceNet: '48.00', priceGross: '59.04', vatRate: 23 },
  { id: 2, code: 'KM', name: 'Servisný výjazd', unit: 'km', priceNet: '0.45', priceGross: '0.55', vatRate: 23, note: 'auto' },
  { id: 3, name: 'Starý paušál', unit: 'mes', priceNet: '10', priceGross: '12.30', vatRate: 23, archived: true },
]

describe('cenník', () => {
  it.each([
    ['', [1, 2]],
    ['servisny', [2]],
    ['plc prog', [1]],
    ['auto', [2]],
    ['pausal', []],
  ])('hľadanie %s', (q, ids) => {
    expect(filterItems(items, q).map((i) => i.id)).toEqual(ids)
  })
  it('archivované na požiadanie', () => {
    expect(filterItems(items, 'pausal', { archived: true }).map((i) => i.id)).toEqual([3])
  })
  it.each([
    [{ priceMode: 'net' }, '48.00'],
    [{ priceMode: 'gross' }, '59.04'],
    [{ priceMode: 'gross', nonVat: true }, '48.00'],
  ])('riadok faktúry %o', (opt, price) => {
    expect(lineFromItem(items[0], opt)).toEqual({ description: 'Programovanie PLC', quantity: '1', unit: 'hod', unitPrice: price, vatRate: 23, amount: '0' })
  })
})

describe('import cenníka', () => {
  it('chyby riadku', () => {
    expect(importErrors({ errors: [{ message: 'Zadajte názov položky' }, { message: 'Chýba cena' }] })).toBe('Zadajte názov položky; Chýba cena')
    expect(importErrors({})).toBe('')
  })
  it('všetky akcie majú popis', () => {
    for (const a of ['create', 'update', 'skip']) expect(importActions[a].label).toBeTruthy()
  })
})

describe('importLabel', () => {
  it.each([
    [0, 'Importovať 0 položiek'],
    [1, 'Importovať 1 položku'],
    [3, 'Importovať 3 položky'],
    [5, 'Importovať 5 položiek'],
  ])('%d', (n, want) => expect(importLabel(n)).toBe(want))
})
