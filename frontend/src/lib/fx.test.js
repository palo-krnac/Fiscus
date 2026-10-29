import { describe, expect, it } from 'vitest'
import { currencyOptions, eurBalance, eurSummary, filterRates, formatRate, isForeignCurrency, purchaseEUR, rateHint, rateText } from './fx.js'

describe('fx', () => {
  it.each([
    ['EUR', false],
    ['', false],
    [undefined, false],
    ['CZK', true],
  ])('cudzia mena %s → %s', (c, want) => expect(isForeignCurrency(c)).toBe(want))

  it('možnosti mien', () => {
    expect(currencyOptions([{ code: 'CZK', name: 'česká koruna' }])).toEqual([
      { code: 'EUR', label: 'EUR – euro' },
      { code: 'CZK', label: 'CZK – česká koruna' },
    ])
  })

  it.each([
    ['25.1230', '25,123'],
    ['1.0823', '1,0823'],
    ['17532.12', '17532,12'],
    ['0', ''],
    ['', ''],
  ])('kurz %s → %s', (r, want) => expect(formatRate(r)).toBe(want))

  it.each([
    [{ currency: 'CZK', fxRate: '25.123', fxDate: '2026-10-23' }, '1 EUR = 25,123 CZK (ECB 23. 10. 2026)', 'Kurz ECB z 23. 10. 2026 (deň pred dodaním)'],
    [{ currency: 'USD', fxRate: '1.08', fxManual: true }, '1 EUR = 1,08 USD (dohodnutý)', 'Zadaný ručne'],
    [{ currency: 'USD', fxRate: '0' }, '', 'Kurz ECB k dátumu dodania zatiaľ nie je'],
    [{ currency: 'EUR', fxRate: '0' }, '', ''],
  ])('%j', (inv, text, hint) => {
    expect(rateText(inv)).toBe(text)
    expect(rateHint(inv)).toBe(hint)
  })
})

describe('eurSummary', () => {
  const eur = { total: '10.00' }
  const czk = { total: '251.23' }
  it.each([
    [{ currency: 'EUR', summary: eur }, eur],
    [{ currency: 'CZK', summary: czk, local: eur }, eur],
    [{ currency: 'CZK', summary: czk }, czk],
    [{ summary: eur }, eur],
  ])('%j', (inv, want) => expect(eurSummary(inv)).toBe(want))
})

describe('purchaseEUR', () => {
  const eur = { total: '100.00' }
  const czk = { total: '2500.00' }
  it.each([
    [{ summary: eur }, eur],
    [{ currency: 'EUR', summary: eur }, eur],
    [{ currency: 'CZK', summary: czk, local: { rates: [], summary: eur } }, eur],
    [{ currency: 'CZK', summary: czk, local: null }, czk],
  ])('%j', (p, want) => expect(purchaseEUR(p)).toBe(want))
})

describe('filterRates', () => {
  const rows = [
    { currency: 'CZK', date: '2026-10-23', rate: '25.1230', source: 'ecb' },
    { currency: 'USD', date: '2026-10-23', rate: '1.0823', source: 'ecb' },
    { currency: 'CZK', date: '2026-10-24', rate: '25.05', source: 'manual' },
  ]
  it.each([
    ['', 3],
    ['czk', 2],
    ['czk rucne', 1],
    ['23. 10. 2026', 2],
    ['25,123', 1],
    ['usd 1,08', 1],
  ])('„%s“ → %d', (q, n) => expect(filterRates(rows, q)).toHaveLength(n))
})

describe('eurBalance', () => {
  it.each([
    [{ balanceEur: '13.96', summary: { total: '448.93' } }, '13.96'],
    [{ summary: { total: '448.93' } }, '448.93'],
  ])('%j', (inv, want) => expect(eurBalance(inv)).toBe(want))
})
