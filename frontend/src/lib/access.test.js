import { describe, expect, it } from 'vitest'
import { hasPerm, permsFor, roleFor, visibleModules } from './access.js'

const me = {
  user: { id: 2, name: 'Fero' },
  access: [
    { companyId: 1, role: 'invoicer', perms: ['invoices.read', 'invoices.write'] },
    { companyId: 2, role: 'viewer', perms: ['invoices.read'] },
  ],
}

describe('práva', () => {
  it('podľa firmy', () => {
    expect(hasPerm(me, 1, 'invoices.write')).toBe(true)
    expect(hasPerm(me, 2, 'invoices.write')).toBe(false)
    expect(hasPerm(me, 3, 'invoices.read')).toBe(false)
    expect(roleFor(me, 2)).toBe('viewer')
    expect(roleFor(me, 9)).toBe('')
  })
  it('bez prihlásenia nič', () => {
    expect(permsFor(null, 1)).toEqual([])
    expect(hasPerm(null, 1, 'invoices.read')).toBe(false)
  })
  it('prázdne právo stačí prístup', () => {
    expect(hasPerm(me, 1, '')).toBe(true)
  })
  it('menu bez nepovolených položiek a prázdnych skupín', () => {
    const mods = [
      { group: 'Predaj', items: [{ path: '/faktury', perm: 'invoices.read' }] },
      { group: 'Nákup', items: [{ path: '/prijate', perm: 'purchases.read' }] },
    ]
    const out = visibleModules(mods, (i) => hasPerm(me, 1, i.perm))
    expect(out.map((g) => g.group)).toEqual(['Predaj'])
  })
})
