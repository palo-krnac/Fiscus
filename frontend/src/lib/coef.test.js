import { expect, it } from 'vitest'
import { coefPeriodLabel, coefYear } from './coef.js'

it.each([
  ['2026-12', 'decembri 2026'],
  ['2026-Q4', '4. štvrťroku 2026'],
  ['', ''],
])('coefPeriodLabel(%s)', (p, want) => expect(coefPeriodLabel(p)).toBe(want))

it('coefYear', () => {
  expect(coefYear('2026-Q3')).toBe(2026)
  expect(coefYear('2027-01')).toBe(2027)
})
