import { describe, expect, it } from 'vitest'
import { badgeRange, byDate, dayLabel, feedURLs, localOnly, monthGrid, monthRange, presets, relative, shiftMonth, soonCount, stateOf } from './calendar.js'

describe('monthGrid', () => {
  it('október 2026 začína štvrtkom, celé týždne od pondelka', () => {
    const w = monthGrid(2026, 10)
    expect(w).toHaveLength(5)
    expect(w[0][0]).toEqual({ date: '2026-09-28', day: 28, inMonth: false, weekend: false })
    expect(w[0][3]).toMatchObject({ date: '2026-10-01', inMonth: true })
    expect(w[0][5].weekend).toBe(true)
    expect(w.at(-1).at(-1).date).toBe('2026-11-01')
  })
  it.each([
    [2026, 2, 4],
    [2027, 2, 4],
    [2026, 3, 6],
  ])('%i/%i', (y, m) => {
    const w = monthGrid(y, m)
    const days = w.flat().filter((d) => d.inMonth)
    expect(days).toHaveLength(new Date(Date.UTC(y, m, 0)).getUTCDate())
    expect(w.flat().length % 7).toBe(0)
  })
})

describe('mesiace', () => {
  it.each([
    [2026, 12, 1, { y: 2027, m: 1 }],
    [2026, 1, -1, { y: 2025, m: 12 }],
    [2026, 10, 14, { y: 2027, m: 12 }],
  ])('shiftMonth(%i, %i, %i)', (y, m, d, want) => expect(shiftMonth(y, m, d)).toEqual(want))
  it('monthRange', () => {
    expect(monthRange(2028, 2)).toEqual({ from: '2028-02-01', to: '2028-02-29' })
  })
})

describe('zobrazenie termínov', () => {
  it('stav, deň, relatívny čas', () => {
    expect(stateOf({ kind: 'tax', done: true }).text).toBe('podané')
    expect(stateOf({ kind: 'own', done: true }).text).toBe('splnené')
    expect(stateOf({ overdue: true }).tone).toBe('overdue')
    expect(dayLabel('2026-10-26')).toBe('Po 26. 10.')
    expect(relative('2026-10-29', '2026-10-29')).toBe('dnes')
    expect(relative('2026-10-29', '2026-10-30')).toBe('zajtra')
    expect(relative('2026-10-29', '2026-11-01')).toBe('o 3 dni')
    expect(relative('2026-10-29', '2026-11-09')).toBe('o 11 dní')
    expect(relative('2026-10-29', '2026-10-26')).toBe('pred 3 dňami')
  })
  it('byDate a odznak', () => {
    const events = [
      { date: '2026-10-30', kind: 'tax' },
      { date: '2026-10-30', kind: 'own', done: true },
      { date: '2026-11-04', kind: 'pay' },
      { date: '2026-11-20', kind: 'pay' },
      { date: '2026-10-26', kind: 'own', overdue: true },
      { date: '2026-10-31', kind: 'receive' },
      { date: '2026-10-20', kind: 'receive', overdue: true },
    ]
    expect(byDate(events)['2026-10-30']).toHaveLength(2)
    // dnes 29. 10.: daň 30. 10., úhrada 4. 11., zmeškaný vlastný 26. 10. a starší zmeškaný; pohľadávky nie
    expect(soonCount({ today: '2026-10-29', events, overdue: [{ date: '2026-09-15', kind: 'pay', overdue: true }, { date: '2026-09-01', kind: 'receive', overdue: true }] })).toBe(4)
    expect(soonCount(null)).toBe(0)
  })
})

describe('odber', () => {
  it('adresy', () => {
    const u = feedURLs('https://fiskus.firma.sk/', 3, 'fkc_abc')
    expect(u.https).toBe('https://fiskus.firma.sk/api/public/calendar/3/fkc_abc/fiskus.ics')
    expect(u.webcal).toBe('webcal://fiskus.firma.sk/api/public/calendar/3/fkc_abc/fiskus.ics')
    expect(localOnly('http://127.0.0.1:48080')).toBe(true)
    expect(localOnly('https://fiskus.firma.sk')).toBe(false)
  })
  it('predvolené termíny majú názov a opakovanie', () => {
    for (const p of presets(2026)) {
      expect(p.title).not.toBe('')
      expect(['', 'monthly', 'quarterly', 'yearly']).toContain(p.repeat)
    }
    expect(presets(2026)[0].date).toBe('2026-01-08')
  })
})

describe('badgeRange', () => {
  it('od začiatku mesiaca po 8 dní dopredu', () => {
    expect(badgeRange(new Date(2026, 9, 29))).toEqual({ from: '2026-10-01', to: '2026-11-06' })
    expect(badgeRange(new Date(2026, 11, 28))).toEqual({ from: '2026-12-01', to: '2027-01-05' })
  })
})
