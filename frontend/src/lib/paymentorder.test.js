import { describe, expect, it } from 'vitest'
import { defaultSelection, selectedTotal } from './paymentorder.js'

const cands = [
  { id: 1, dueDate: '2026-10-20', amount: '100.00' },
  { id: 2, dueDate: '2026-11-05', amount: '50.50' },
  { id: 3, dueDate: '2026-11-06', amount: '10.00' },
  { id: 4, dueDate: '2026-10-01', amount: '7.00', problem: 'Chýba IBAN dodávateľa' },
  { id: 5, dueDate: '', amount: '1.00' },
]

describe('defaultSelection', () => {
  it.each([
    [7, [1, 2, 5]],
    [0, [1, 5]],
    [30, [1, 2, 3, 5]],
  ])('do %i dní', (days, want) => {
    expect(defaultSelection(cands, '2026-10-29', days)).toEqual(want)
  })
})

describe('selectedTotal', () => {
  it('sčíta iba vybrané', () => {
    expect(selectedTotal(cands, [1, 2])).toBeCloseTo(150.5)
    expect(selectedTotal(cands, [])).toBe(0)
  })
})
