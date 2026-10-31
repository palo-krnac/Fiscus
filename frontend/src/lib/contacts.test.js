import { describe, expect, it } from 'vitest'
import { addressKindLabel, addressText, blankAddress, blankPerson, contactFromCustomer, customerFromContact, defaultDelivery, defaultPerson, dueDaysFor, filterContacts, personsText, searchKey, setPrimary } from './contacts.js'

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

describe('osoby a adresy kontaktu', () => {
  const k = {
    name: 'Kováč a syn',
    persons: [{ name: 'Peter Kováč', role: 'konateľ', email: 'peter@kovac.sk', primary: true }, { name: 'Eva Malá', phone: '0900' }],
    addresses: [
      { kind: 'postal', street: 'P.O. Box 1', city: 'Nitra', default: true },
      { kind: 'delivery', name: 'Sklad', street: 'Priemyselná 1', zip: '94901', city: 'Nitra', default: true },
    ],
  }
  it.each([
    ['meno osoby', 'eva mala', 1],
    ['e-mail osoby', 'peter@kovac', 1],
    ['dodacia adresa', 'priemyselna', 1],
    ['nič', 'zilina', 0],
  ])('hľadanie: %s', (_, q, n) => expect(filterContacts([k], q)).toHaveLength(n))
  it('texty do zoznamu a exportu', () => {
    expect(personsText(k)).toBe('Peter Kováč (konateľ), Eva Malá')
    expect(addressText(k.addresses[1])).toBe('Sklad, Priemyselná 1, 94901 Nitra')
  })
  it('predvolené na doklad', () => {
    expect(defaultDelivery(k)).toEqual({ name: 'Sklad', street: 'Priemyselná 1', city: 'Nitra', zip: '94901', country: 'SK' })
    expect(defaultPerson(k)).toEqual({ name: 'Peter Kováč', email: 'peter@kovac.sk', phone: undefined })
    expect(defaultDelivery({})).toBeNull()
    expect(defaultPerson({ persons: [{ name: 'X' }] })).toBeNull()
  })
  it('jedna predvolená osoba a jedna adresa každého druhu', () => {
    const ps = setPrimary(k.persons, 1)
    expect(ps.map((p) => p.primary)).toEqual([false, true])
    expect(setPrimary(ps, 1).map((p) => p.primary)).toEqual([false, false])
    const as = setPrimary([...k.addresses, { kind: 'delivery', street: 'x', default: false }], 2, 'default')
    expect(as.map((a) => a.default)).toEqual([true, false, true])
  })
  it('prázdne riadky', () => {
    expect(blankPerson()).toMatchObject({ name: '', primary: false })
    expect(blankAddress().kind).toBe('delivery')
    expect(addressKindLabel('branch')).toBe('Prevádzka')
  })
})
