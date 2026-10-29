import { expect, it } from 'vitest'
import { ecbErrors, formatRates, parseRates } from './legal.js'

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
