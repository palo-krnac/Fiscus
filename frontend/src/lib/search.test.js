import { describe, expect, it } from 'vitest'
import { fold, matches, searchTerms } from './search.js'

describe('hľadanie', () => {
  it('bez diakritiky a veľkosti písmen', () => {
    expect(fold('Žilina ŠTÚROVA')).toBe('zilina sturova')
  })
  it.each([
    ['', true],
    ['  ', true],
    ['zilina', true],
    ['ŽILINA fa2026', true],
    ['zilina 2025', false],
    ['1234,5', true],
    ['1234.50', true],
    ['bratislava', false],
  ])('%s → %s', (q, want) => {
    expect(matches(searchTerms(q), ['FA20260012', 'Elektro Žilina s.r.o.', null, '1234.50'])).toBe(want)
  })
})
