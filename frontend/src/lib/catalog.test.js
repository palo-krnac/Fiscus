import { describe, expect, it } from 'vitest'
import { filterItems, itemFromLine, lineFromItem, importActions, importErrors, importLabel, sameNameItem } from './catalog.js'

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
    expect(lineFromItem(items[0], opt)).toEqual({ description: 'Programovanie PLC', quantity: '1', unit: 'hod', unitPrice: price, vatRate: 23, amount: '0', itemId: items[0].id })
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

describe('uloženie riadku faktúry do cenníka', () => {
  it.each([
    ['Programovanie PLC', 'programovanie  plc', true],
    [' Servisný výjazd ', 'Servisný výjazd', true],
    ['Kábel', 'Kábel 5x2,5', false],
    ['', '', false],
  ])('%s / %s', (a, b, same) => {
    expect(!!sameNameItem([{ name: a }], b)).toBe(same)
  })
  it('archivovaná položka sa nepočíta', () => {
    expect(sameNameItem([{ name: 'X', archived: true }], 'x')).toBeNull()
  })
  it.each([
    [{ description: ' Práca ', unit: 'hod', unitPrice: '45', vatRate: 23 }, {}, { name: 'Práca', unit: 'hod', price: '45', priceMode: 'net', vatRate: 23 }],
    [{ description: 'Kniha', unit: '', unitPrice: '20', vatRate: 5 }, { priceMode: 'gross' }, { name: 'Kniha', unit: 'ks', price: '20', priceMode: 'gross', vatRate: 5 }],
    [{ description: 'Služba', unit: 'ks', unitPrice: '10', vatRate: 23 }, { priceMode: 'gross', nonVat: true }, { name: 'Služba', unit: 'ks', price: '10', priceMode: 'net', vatRate: 0 }],
  ])('položka z riadku %j', (line, opt, want) => {
    expect(itemFromLine(line, opt)).toEqual(want)
  })
})
