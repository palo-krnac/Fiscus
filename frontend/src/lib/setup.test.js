import { describe, expect, it } from 'vitest'
import { afterSetup, bookAllows, bookkeepingLabel, migrationNote, stepErrors } from './setup.js'

describe('úvodné nastavenie', () => {
  it.each([
    [{ book: 'simple' }, 'simple', true],
    [{ book: 'simple' }, 'double', false],
    [{ book: 'double' }, 'double', true],
    [{ book: 'double' }, '', true],
    [{}, 'double', true],
  ])('menu %j pri %s → %s', (item, book, want) => expect(bookAllows(item, book)).toBe(want))

  it.each([
    [0, { name: ' ' }, ['name']],
    [0, { name: 'Pavol Krnáč' }, []],
    [1, { vatPayer: true, icdph: 'SK 2120000003' }, []],
    [1, { vatPayer: true, icdph: '2120000003' }, ['icdph']],
    [1, { vatPayer: false }, []],
    [2, { bookkeeping: '' }, ['bookkeeping']],
    [2, { bookkeeping: 'simple' }, []],
  ])('krok %i %j', (step, f, keys) => expect(Object.keys(stepErrors(step, f))).toEqual(keys))

  it('popisy a smerovanie', () => {
    expect(bookkeepingLabel('double')).toBe('Podvojné účtovníctvo')
    expect(bookkeepingLabel('')).toContain('Nezvolené')
    expect(afterSetup('migrate')).toBe('/prenos')
    expect(afterSetup('new')).toBe('/faktury')
  })

  it('poznámka o prenose', () => {
    expect(migrationNote({})).toBe(null)
    expect(migrationNote({ migratedFrom: 'POHODA', migratedAt: '2026-10-31T12:00:00Z' })).toContain('z programu POHODA (31. 10. 2026)')
  })
})
