import { describe, expect, it } from 'vitest'
import { fillFromRegistry, fillFromVIES, normalizeICO, validICO } from './registry.js'

describe('validICO (rovnaký algoritmus ako backend)', () => {
  it.each([
    ['31322832', true],
    ['31 322 832', true],
    ['31322833', false],
    ['612341', true], // 6-miestne → 00612341
    ['1234567', false],
    ['', false],
  ])('%s → %s', (ico, ok) => expect(validICO(ico)).toBe(ok))

  it('doplní nuly', () => expect(normalizeICO('612341')).toBe('00612341'))
})

describe('fillFromRegistry', () => {
  const reg = { ico: '31322832', name: 'SLOVNAFT, a.s.', street: 'Vlčie hrdlo 1', zip: '824 12', city: 'Bratislava', country: 'SK' }

  it('prepíše názov a adresu, zachová DIČ a e-mail', () => {
    const out = fillFromRegistry({ name: 'slovnaft', ico: '31322832', dic: '2020372640', email: 'x@y.sk', country: 'SK', street: 'stará' }, reg)
    expect(out).toEqual({ name: 'SLOVNAFT, a.s.', ico: '31322832', dic: '2020372640', email: 'x@y.sk', country: 'SK', street: 'Vlčie hrdlo 1', zip: '824 12', city: 'Bratislava' })
  })

  it.each([
    ['DIČ z registra prepíše prázdne', { dic: '' }, '2020372640', '2020372640'],
    ['bez DIČ v registri ostane zadané', { dic: '1234567890' }, undefined, '1234567890'],
    ['DIČ z registra má prednosť', { dic: '1234567890' }, '2020372640', '2020372640'],
  ])('%s', (_, target, dic, want) => {
    expect(fillFromRegistry({ name: '', ...target }, { ...reg, dic }).dic).toBe(want)
  })

  it('formulár firmy bez krajiny nedostane pole country', () => {
    expect('country' in fillFromRegistry({ name: '' }, reg)).toBe(false)
  })
})

describe('fillFromVIES', () => {
  const cz = { vatId: 'CZ27654321', country: 'CZ', valid: true, name: 'Bohemia Control s.r.o.', street: 'Purkyňova 99', zip: '61200', city: 'Brno' }
  it('zahraničný partner: názov, adresa, krajina', () => {
    expect(fillFromVIES({ name: '', street: '', zip: '', city: '', country: 'SK', icdph: 'cz27654321', email: 'a@b.cz' }, cz)).toEqual({
      name: 'Bohemia Control s.r.o.', street: 'Purkyňova 99', zip: '61200', city: 'Brno', country: 'CZ', icdph: 'CZ27654321', email: 'a@b.cz',
    })
  })
  it('slovenský: adresu nemení, doplní iba chýbajúci názov', () => {
    const sk = { vatId: 'SK2020372640', country: 'SK', valid: true, name: 'SLOVNAFT, a.s.', street: 'X', city: 'Y' }
    expect(fillFromVIES({ name: 'Slovnaft', street: 'Vlčie hrdlo 1', icdph: 'sk 2020372640' }, sk)).toEqual({ name: 'Slovnaft', street: 'Vlčie hrdlo 1', icdph: 'SK2020372640' })
    expect(fillFromVIES({ name: '' }, sk).name).toBe('SLOVNAFT, a.s.')
  })
  it('neplatné IČ DPH nič nemení', () => {
    const t = { name: 'A' }
    expect(fillFromVIES(t, { valid: false })).toBe(t)
  })
  it('IČ DPH z registra (VIES potvrdil platiteľa)', () => {
    expect(fillFromRegistry({ name: '', icdph: '' }, { ico: '1', name: 'A', street: '', zip: '', city: '', icdph: 'SK2020372640' }).icdph).toBe('SK2020372640')
  })
})
