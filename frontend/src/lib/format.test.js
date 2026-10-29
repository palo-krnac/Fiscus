import { describe, expect, it } from 'vitest'
import {
  formatAmount, formatMoney, formatDate, formatQuantity,
  parseNumber, toISODate, fromISODate, statusOf,
} from './format.js'

const nb = ' '

describe('formatAmount / formatMoney', () => {
  it.each([
    ['0', '0,00'],
    ['12.5', '12,50'],
    ['1234.5', `1${nb}234,50`],
    ['-123', '-123,00'],
    ['1000000', `1${nb}000${nb}000,00`],
  ])('%s → %s', (input, out) => {
    expect(formatAmount(input)).toBe(out)
  })

  it('pridá menu', () => {
    expect(formatMoney('559.65')).toBe(`559,65${nb}€`)
    expect(formatMoney('10', 'CZK')).toBe(`10,00${nb}CZK`)
  })

  it('prázdne hodnoty', () => {
    expect(formatAmount(null)).toBe('')
    expect(formatMoney('')).toBe('')
  })
})

describe('dátumy', () => {
  it('formátuje po slovensky', () => {
    expect(formatDate('2026-10-09')).toBe('9. 10. 2026')
    expect(formatDate(null)).toBe('')
  })

  it('ISO prevod bez posunu časového pásma', () => {
    const d = new Date(2026, 0, 31) // lokálna polnoc
    expect(toISODate(d)).toBe('2026-01-31')
    expect(toISODate(fromISODate('2026-12-01'))).toBe('2026-12-01')
  })
})

describe('parseNumber', () => {
  it.each([
    ['12,5', '12.5'],
    ['1 234,50', '1234.50'],
    [' 7 ', '7'],
    ['-0,25', '-0.25'],
    ['abc', null],
    ['', null],
    ['1,2,3', null],
  ])('%s → %s', (input, out) => {
    expect(parseNumber(input)).toBe(out)
  })
})

it('množstvo bez zbytočných núl', () => {
  expect(formatQuantity('16')).toBe('16')
  expect(formatQuantity('2.5')).toBe('2,5')
})

it('stav prijatej faktúry', async () => {
  const { purchaseStatusOf } = await import('./format.js')
  expect(purchaseStatusOf({ status: 'booked' }).label).toBe('Neuhradená')
  expect(purchaseStatusOf({ status: 'paid' }).tone).toBe('paid')
  expect(purchaseStatusOf({ status: 'booked', overdue: true, daysOverdue: 2 }).label).toBe('Po splatnosti 2 dni')
})

describe('statusOf', () => {
  it.each([
    [{ status: 'issued', paidAmount: '200.00' }, 'Čiastočne uhradená'],
    [{ status: 'issued', paidAmount: '0.00' }, 'Vystavená'],
    [{ status: 'issued', paidAmount: '50.00', overdue: true, daysOverdue: 3 }, 'Po splatnosti 3 dni, čiastočne uhradená'],
    [{ status: 'paid', paidAmount: '492.00' }, 'Uhradená'],
  ])('čiastočná úhrada %j', (row, label) => expect(statusOf(row).label).toBe(label))
  it('omeškanie má prednosť a skloňuje dni', () => {
    expect(statusOf({ status: 'issued', overdue: true, daysOverdue: 1 }).label).toBe('Po splatnosti 1 deň')
    expect(statusOf({ status: 'issued', overdue: true, daysOverdue: 3 }).label).toBe('Po splatnosti 3 dni')
    expect(statusOf({ status: 'issued', overdue: true, daysOverdue: 14 }).label).toBe('Po splatnosti 14 dní')
  })
  it('bežné stavy', () => {
    expect(statusOf({ status: 'paid' }).label).toBe('Uhradená')
    const credit = [{ source: 'credit_note', amount: '100' }]
    expect(statusOf({ status: 'paid', kind: 'invoice', payments: credit }).label).toBe('Vyrovnaná dobropisom')
    expect(statusOf({ status: 'paid', kind: 'credit_note', payments: credit }).label).toBe('Započítaný s faktúrou')
    expect(statusOf({ status: 'paid', kind: 'invoice', payments: [...credit, { source: 'bank', amount: '10' }] }).label).toBe('Uhradená')
    expect(statusOf({ status: 'draft' }).tone).toBe('draft')
  })
})
