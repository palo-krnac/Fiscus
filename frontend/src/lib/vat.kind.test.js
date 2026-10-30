import { describe, expect, it } from 'vitest'
import { kindParams, lastKnownOf, suggestKind } from './vat.js'

describe('druh priznania', () => {
  it.each([
    [[], '2026-10-25', '2026-10-29', 'riadne'],
    [[{ result: '230.00' }], '2026-10-25', '2026-10-20', 'opravne'],
    [[{ result: '230.00' }], '2026-10-25', '2026-10-29', 'dodatocne'],
  ])('%j', (f, deadline, today, want) => expect(suggestKind(f, deadline, today)).toBe(want))

  it('posledná známa povinnosť', () => {
    expect(lastKnownOf([])).toBe('')
    expect(lastKnownOf([{ result: '230.00' }, { result: '-60.00' }])).toBe('-60.00')
  })

  it('parametre dotazu', () => {
    expect(kindParams({ druh: 'riadne' })).toEqual({})
    expect(kindParams({ druh: 'opravne', zistene: '2026-11-01' })).toEqual({ druh: 'opravne' })
    expect(kindParams({ druh: 'dodatocne', zistene: '2026-11-03', posledne: '-60,00', r37: '' })).toEqual({
      druh: 'dodatocne', zistene: '2026-11-03', posledne: '-60.00', r37: '',
    })
  })
})
