import { describe, expect, it } from 'vitest'
import { actionLabel, channelLabel, diffFields, isAlert } from './audit.js'

describe('audit', () => {
  it('zmenené polia vrátane vnorených', () => {
    const before = { name: 'Firma', customer: { city: 'Nitra', zip: '949 01' }, lines: [1] }
    const after = { name: 'Firma', customer: { city: 'Trnava', zip: '949 01' }, lines: [1, 2] }
    expect(diffFields(before, after)).toEqual([
      { field: 'customer.city', before: 'Nitra', after: 'Trnava' },
      { field: 'lines', before: '[1]', after: '[1,2]' },
    ])
  })
  it('pri vytvorení iba vyplnené polia', () => {
    expect(diffFields(null, { name: 'Nový', note: '', active: true })).toEqual([
      { field: 'name', before: '—', after: 'Nový' },
      { field: 'active', before: '—', after: 'áno' },
    ])
  })
  it('pri zmazaní všetko pred', () => {
    expect(diffFields({ name: 'X' }, null)).toEqual([{ field: 'name', before: 'X', after: '—' }])
  })
  it('názvy', () => {
    expect(actionLabel('issue')).toBe('vystavenie')
    expect(actionLabel('neznama')).toBe('neznama')
    expect(channelLabel('mcp')).toBe('AI (MCP)')
    expect(isAlert({ action: 'login_failed' })).toBe(true)
    expect(isAlert({ action: 'update' })).toBe(false)
  })
})
