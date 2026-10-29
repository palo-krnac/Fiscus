import { describe, expect, it } from 'vitest'
import { firstOfNextMonth, intervalLabel, monthlyAmount, recurringStatusOf } from './recurring.js'

describe('recurringStatusOf', () => {
  it.each([
    [{}, 'Pozastavená'],
    [{ active: true }, 'Aktívna'],
    [{ id: 1, active: true, nextDate: '2026-11-01' }, 'Aktívna'],
    [{ id: 1, active: false, nextDate: '2026-11-01' }, 'Pozastavená'],
    [{ id: 1, active: false, nextDate: null }, 'Skončená'],
  ])('%o → %s', (t, label) => {
    expect(recurringStatusOf(t).label).toBe(label)
  })
})

it('monthlyAmount', () => {
  expect(monthlyAmount({ interval: 'monthly', summary: { total: '120.00' } })).toBe(120)
  expect(monthlyAmount({ interval: 'quarterly', summary: { total: '390.00' } })).toBe(130)
  expect(monthlyAmount({ interval: 'yearly', summary: { total: '1200.00' } })).toBe(100)
  expect(monthlyAmount({ interval: 'x', summary: { total: '5' } })).toBe(0)
})

it('firstOfNextMonth', () => {
  expect(firstOfNextMonth(new Date(2026, 9, 29))).toBe('2026-11-01')
  expect(firstOfNextMonth(new Date(2026, 11, 31))).toBe('2027-01-01')
})

it('intervalLabel', () => {
  expect(intervalLabel('halfyear')).toBe('Polročne')
})
