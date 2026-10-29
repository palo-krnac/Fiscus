import { describe, expect, it } from 'vitest'
import { fillFromRegistry, normalizeICO, validICO } from './registry.js'

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

  it('formulár firmy bez krajiny nedostane pole country', () => {
    expect('country' in fillFromRegistry({ name: '' }, reg)).toBe(false)
  })
})
