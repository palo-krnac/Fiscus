import { describe, expect, it } from 'vitest'
import { deviceName, pairCode, remaining } from './mobile.js'

describe('mobil v sieti', () => {
  it.each([
    ['Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)', 'iPhone'],
    ['Mozilla/5.0 (Linux; Android 14; SM-S911B Build/UP1A) AppleWebKit', 'SM-S911B'],
    ['Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36', 'Android'],
    ['Mozilla/5.0 (X11; Linux x86_64)', 'Telefón'],
  ])('meno zariadenia %#', (ua, want) => expect(deviceName(ua)).toBe(want))

  it('zostávajúci čas', () => {
    const now = Date.parse('2026-10-31T17:00:00Z')
    expect(remaining('2026-10-31T17:05:00Z', now)).toBe('5:00')
    expect(remaining('2026-10-31T17:00:09Z', now)).toBe('0:09')
    expect(remaining('2026-10-31T16:59:00Z', now)).toBe('')
  })

  it.each([
    ['?kod=ab3k9z', 'AB3K9Z'],
    ['?kod=AB<script>', 'ABSCRI'],
    ['', ''],
  ])('kód %s', (q, want) => expect(pairCode(q)).toBe(want))
})
