import { describe, expect, it } from 'vitest'
import { attention, badgeText, refreshDrafts } from './attention.js'

describe('badgeText', () => {
  it.each([
    [0, ''],
    [undefined, ''],
    [3, '3'],
    [120, '99+'],
  ])('%s → %s', (n, want) => expect(badgeText(n)).toBe(want))
})

describe('refreshDrafts', () => {
  it('zapíše počet a pri chybe nulu', async () => {
    await refreshDrafts(async () => [{}, {}])
    expect(attention['/pravidelne']).toBe(2)
    await refreshDrafts(async () => {
      throw new Error('x')
    })
    expect(attention['/pravidelne']).toBe(0)
  })
})
