import { describe, expect, it } from 'vitest'
import { introText, legalNotes, levelTitle } from './reminders.js'

describe('upomienky', () => {
  it('nadpis podľa stupňa', () => {
    expect(levelTitle(1)).toBe('Upomienka')
    expect(levelTitle(3)).toBe('Posledná upomienka pred vymáhaním')
  })
  it('úvodný text obsahuje lehotu', () => {
    expect(introText({ level: 1, payBy: '2026-11-05' })).toContain('5. 11. 2026')
    expect(introText({ level: 3, payBy: '2026-11-05' })).toMatch(/súdnou cestou/)
  })
  it.each([
    [{ interestMode: 'statutory', business: true, interestTotal: '4.55', fees: '40.00' }, ['§ 369 Obchodného', '§ 369c', 'plynie až do zaplatenia']],
    [{ interestMode: 'statutory', business: false, interestTotal: '1.00', fees: '0.00' }, ['§ 517 Občianskeho', 'plynie']],
    [{ interestMode: 'custom', customRate: '12.50', interestTotal: '3.00', fees: '0' }, ['12,50 % ročne', 'plynie']],
    [{ interestMode: 'none', interestTotal: '0.00', fees: '0.00' }, []],
  ])('právne poznámky %o', (r, parts) => {
    const notes = legalNotes(r).join(' ')
    expect(legalNotes(r).length).toBe(parts.length)
    for (const p of parts) expect(notes).toContain(p)
  })
})
