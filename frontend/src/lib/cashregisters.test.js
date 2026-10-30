import { describe, expect, it } from 'vitest'
import { blankRegister, registerLabel, registerLocked, registerOptions, registersPayload } from './cashregisters.js'

const list = [
  { id: 0, name: 'Pokladnica', currency: 'EUR', documents: 12, balance: '549.70' },
  { id: 1, name: 'Pokladňa CZK', currency: 'CZK', prefix: 'CZ', documents: 3 },
  { id: 2, name: 'Stará prevádzka', currency: 'EUR', prefix: 'P2', archived: true, documents: 0 },
  { id: 3, name: 'Výstava', currency: 'EUR', prefix: 'VY', archived: true, documents: 4 },
]

describe('pokladne', () => {
  it.each([
    [list[0], 'Pokladnica'],
    [list[1], 'Pokladňa CZK (CZK)'],
    [list[3], 'Výstava – archivovaná'],
  ])('popis %#', (r, want) => expect(registerLabel(r)).toBe(want))

  it('výber bez prázdnych archivovaných', () => {
    expect(registerOptions(list, 0).map((o) => o.value)).toEqual([0, 1, 3])
    expect(registerOptions(list, 2).map((o) => o.value)).toEqual([0, 1, 2, 3])
  })

  it('nová pokladňa má záporné ID', () => {
    const a = blankRegister(list)
    expect(a.id).toBe(-1)
    expect(blankRegister([...list, a]).id).toBe(-2)
  })

  it('zamknutá mena a predpona pri dokladoch', () => {
    expect(registerLocked(list[1])).toBe(true)
    expect(registerLocked(list[2])).toBe(false)
  })

  it('uložia sa iba nastavenia', () => {
    expect(registersPayload([list[1]])).toEqual([{ id: 1, name: 'Pokladňa CZK', currency: 'CZK', prefix: 'CZ', archived: undefined }])
  })
})
