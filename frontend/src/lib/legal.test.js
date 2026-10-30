import { expect, it } from 'vitest'
import { ecbErrors, formatRates, holidayDayLabel, parseRates, parseYears } from './legal.js'

it('parseRates', () => {
  expect(parseRates('23, 19; 5 0')).toEqual([23, 19, 5, 0])
  expect(parseRates('21 %, x, 10')).toEqual([21, 10])
  expect(parseRates('')).toEqual([])
})

it('formatRates', () => {
  expect(formatRates([23, 19, 5, 0])).toBe('23, 19, 5, 0')
})

it('ecbErrors', () => {
  expect(ecbErrors({ 'ecb[2].rate': 'zlá', 'ecb[2].from': 'chýba', flatFee: 'x' })).toEqual({ 2: { rate: 'zlá', from: 'chýba' } })
})

it.each([
  ['05-01', '1. 5.'],
  ['12-24', '24. 12.'],
  ['easter-2', 'Veľká noc − 2 dni'],
  ['easter+1', 'Veľká noc + 1 deň'],
  ['easter+0', 'Veľkonočná nedeľa'],
  ['easter+49', 'Veľká noc + 49 dní'],
  ['zle', 'zle'],
])('holidayDayLabel(%s)', (day, want) => expect(holidayDayLabel(day)).toBe(want))

it('parseYears', () => {
  expect(parseYears('2026, 2028;2026 x 15')).toEqual([2026, 2028])
  expect(parseYears('')).toEqual([])
})
