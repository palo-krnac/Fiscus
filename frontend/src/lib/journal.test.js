import { describe, expect, it } from 'vitest'
import { absAmount, columnCode, columnsFor, entryState, linesText, presets, unallocated } from './journal.js'

const cols = [
  { code: 'd2', label: 'Predaj výrobkov a služieb', income: true },
  { code: 'f.dph', label: 'Prijatá DPH', income: true },
  { code: 'e2', label: 'Služby', income: false },
  { code: 'g.dph', label: 'Zaplatená DPH', income: false },
]

describe('peňažný denník', () => {
  it.each([
    ['d2', 'd2'],
    ['f.dph', 'f'],
    ['g.dm', 'g'],
    ['c.out', 'c'],
  ])('kód %s', (c, want) => expect(columnCode(c)).toBe(want))
  it('stĺpce podľa smeru', () => {
    expect(columnsFor(cols, '492').map((c) => c.code)).toEqual(['d2', 'f.dph'])
    expect(columnsFor(cols, '-123').map((c) => c.code)).toEqual(['e2', 'g.dph'])
  })
  it.each([
    [{ lines: [] }, 'Nerozpísaný'],
    [{ lines: [{}], auto: true }, 'Návrh Fiškusa'],
    [{ lines: [{}], auto: false }, 'Rozpísaný ručne'],
    [{ lines: [{}], means: 'close' }, 'Uzávierka'],
  ])('stav %j', (e, label) => expect(entryState(e).label).toBe(label))
  it('rozpis ako text', () => {
    expect(linesText(cols, [{ column: 'e2', amount: '100' }, { column: 'g.dph', amount: '23' }], (v) => v)).toBe('Služby 100 · Zaplatená DPH 23')
  })
  it.each([
    ['-123', [{ amount: '100' }], '23.00'],
    ['492', [{ amount: '400' }, { amount: '92' }], '0.00'],
    ['0.3', [{ amount: '0.1' }, { amount: '0.2' }], '0.00'],
    ['50', [], '50.00'],
  ])('nerozpísané z %s', (a, ls, want) => expect(unallocated(a, ls)).toBe(want))
  it('rýchle rozpisy', () => {
    expect(presets('10').every((p) => !p.column.startsWith('e'))).toBe(true)
    expect(presets('-10').some((p) => p.column === 'e4')).toBe(true)
    expect(absAmount('-216.12')).toBe('216.12')
  })
})
