import { describe, expect, it } from 'vitest'
import { contactFromCustomer, customerFromContact, dueDaysFor, filterContacts, searchKey } from './contacts.js'

describe('searchKey', () => {
  it.each([
    ['Kováč', 'kovac'],
    ['ŽILINA', 'zilina'],
    ['  Ľubica   Ďurčová ', 'lubica durcova'],
    [null, ''],
  ])('%s → %s', (input, out) => expect(searchKey(input)).toBe(out))
})

describe('filterContacts', () => {
  const list = [
    { name: 'Ing. Peter Kováč', ico: '', city: 'Banská Bystrica' },
    { name: 'Slovnaft, a.s.', ico: '31322832', city: 'Bratislava' },
  ]
  it('bez diakritiky a podľa IČO', () => {
    expect(filterContacts(list, 'kovac')).toHaveLength(1)
    expect(filterContacts(list, '3132')[0].name).toBe('Slovnaft, a.s.')
    expect(filterContacts(list, 'bystrica')).toHaveLength(1)
    expect(filterContacts(list, '')).toHaveLength(2)
    expect(filterContacts(list, 'košice')).toHaveLength(0)
  })
})

it('prevod kontakt ↔ odberateľ zachová údaje', () => {
  const c = { id: 5, name: 'A', street: 'S 1', city: 'BB', zip: '974 01', country: 'SK', ico: '1', dic: '2', icdph: 'SK3', email: 'a@b.sk', dueDays: 30 }
  const p = customerFromContact(c)
  expect(p).toEqual({ name: 'A', street: 'S 1', city: 'BB', zip: '974 01', country: 'SK', ico: '1', dic: '2', icdph: 'SK3', email: 'a@b.sk' })
  expect(contactFromCustomer(p)).toMatchObject({ name: 'A', ico: '1', icdph: 'SK3', dueDays: 0 })
})

it('splatnosť: kontakt → firma → 14 dní', () => {
  expect(dueDaysFor({ dueDays: 30 }, { defaultDueDays: 10 })).toBe(30)
  expect(dueDaysFor({ dueDays: 0 }, { defaultDueDays: 10 })).toBe(10)
  expect(dueDaysFor(null, null)).toBe(14)
})
