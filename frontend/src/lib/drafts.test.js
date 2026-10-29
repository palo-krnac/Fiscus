import { describe, expect, it } from 'vitest'
import { setPurchaseDraft, takePurchaseDraft } from './drafts.js'

describe('koncept z dokladu', () => {
  it('sa použije iba raz', () => {
    setPurchaseDraft({ purchase: { supplierNumber: 'A1' }, warnings: [] })
    expect(takePurchaseDraft().purchase.supplierNumber).toBe('A1')
    expect(takePurchaseDraft()).toBeNull()
  })
})
