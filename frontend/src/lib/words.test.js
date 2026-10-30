import { describe, expect, it } from 'vitest'
import { amountInWords, amountInWordsIn, numberInWords } from './words.js'

describe('suma slovom', () => {
  it.each([
    [0, 'nula'],
    [1, 'jeden'],
    [2, 'dva'],
    [11, 'jedenásť'],
    [21, 'dvadsaťjeden'],
    [100, 'sto'],
    [253, 'dvestopäťdesiattri'],
    [1000, 'tisíc'],
    [2000, 'dvetisíc'],
    [1250, 'tisícdvestopäťdesiat'],
    [12345, 'dvanásťtisíctristoštyridsaťpäť'],
    [2000000, 'dvamilióny'],
    [5000001, 'päťmiliónovjeden'],
  ])('%i → %s', (n, w) => expect(numberInWords(n)).toBe(w))

  it.each([
    ['1.00', 'jedno euro'],
    ['2.00', 'dve eurá'],
    ['3.00', 'tri eurá'],
    ['22.00', 'dvadsaťdva eur'],
    ['5.00', 'päť eur'],
    ['253.50', 'dvestopäťdesiattri eur 50 centov'],
    ['12.01', 'dvanásť eur 1 cent'],
    ['0.03', 'nula eur 3 centy'],
  ])('%s → %s', (a, w) => expect(amountInWords(a)).toBe(w))
})

describe('suma slovom v cudzej mene', () => {
  it.each([
    ['2500.00', 'CZK', 'dvetisícpäťsto CZK'],
    ['12.50', 'USD', 'dvanásť USD 50/100'],
    ['3.00', 'EUR', 'tri eurá'],
  ])('%s %s', (a, c, want) => expect(amountInWordsIn(a, c)).toBe(want))
})
