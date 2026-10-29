import { describe, expect, it } from 'vitest'
import { EU, countries, previewNumber, ratesOn } from './codes.js'

const periods = [
  { from: '2023-01-01', to: '2024-12-31', rates: [20, 10, 5, 0] },
  { from: '2025-01-01', rates: [23, 19, 5, 0] },
]

describe('ratesOn (zrkadlo vat.RatesOn)', () => {
  it.each([
    ['2024-12-31', [20, 10, 5, 0]],
    ['2025-01-01', [23, 19, 5, 0]],
    ['2026-10-29', [23, 19, 5, 0]],
    ['2019-05-01', [20, 10, 5, 0]],
  ])('%s', (date, rates) => expect(ratesOn(date, periods)).toEqual(rates))
})

describe('previewNumber', () => {
  const d = new Date(2026, 2, 5)
  it.each([
    ['FA{RRRR}{NNNN}', 'FA20260001'],
    ['{RRRR}/{NNN}', '2026/001'],
    ['DB-{RR}-{NNNNN}', 'DB-26-00001'],
    ['{RR}{MM}{NN}', '260301'],
  ])('%s → %s', (f, out) => expect(previewNumber(f, d)).toBe(out))
})

it('krajiny majú jedinečné kódy a SK je prvá', () => {
  const codes = countries.map((c) => c.code)
  expect(new Set(codes).size).toBe(codes.length)
  expect(codes[0]).toBe('SK')
  expect(EU.size).toBe(27)
})
