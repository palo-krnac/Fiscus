import { describe, expect, it } from 'vitest'
import { publicVerdict, shareInfo } from './quotes.js'

describe('shareInfo', () => {
  it('bez odkazu', () => expect(shareInfo(null)).toBeNull())
  it('neotvorená', () => expect(shareInfo({ token: 't' }).viewed).toBe('Zákazník odkaz ešte neotvoril.'))
  it('prijatá s objednávkou', () => {
    const i = shareInfo({ viewed: '2026-10-29T10:00:00Z', response: { accepted: true, name: 'Ján', order: 'OBJ-7', note: 'ok', at: '2026-10-29T11:00:00Z' } })
    expect(i.response).toMatch(/^Prijal Ján, /)
    expect(i.order).toBe('Objednávka OBJ-7')
    expect(i.viewed).toMatch(/^Zákazník ponuku otvoril/)
  })
})

describe('publicVerdict', () => {
  it.each([
    [{ status: 'issued' }, 'open'],
    [{ status: 'issued', expired: true }, 'expired'],
    [{ status: 'accepted', response: { accepted: true } }, 'accepted'],
    [{ status: 'rejected', response: { accepted: false } }, 'rejected'],
    [{ status: 'invoiced' }, 'accepted'],
  ])('%o', (v, want) => expect(publicVerdict(v)).toBe(want))
})
