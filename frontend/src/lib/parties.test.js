import { describe, expect, it } from 'vitest'
import { customerGroups, deliveryLines, supplierGroups } from './parties.js'

describe('strany na tlači', () => {
  it('dodávateľ: identifikátory a kontakt pod sebou', () => {
    expect(supplierGroups({ ico: '12345678', icdph: 'SK2020123456', phone: '+421 55 123', email: 'a@b.sk' })).toEqual([
      [['IČO', '12345678'], ['IČ DPH', 'SK2020123456']],
      [['Tel.', '+421 55 123'], ['E-mail', 'a@b.sk']],
    ])
    expect(supplierGroups({ ico: '1' })).toEqual([[['IČO', '1']]])
  })
  it.each([
    ['bez osoby', { customer: { ico: '1', email: 'f@d.sk' } }, [[['IČO', '1']], [['E-mail', 'f@d.sk']]]],
    ['s osobou', { customer: { email: 'f@d.sk' }, attention: { name: 'Peter', phone: '0900' } }, [[['Kontakt', 'Peter'], ['Tel.', '0900'], ['E-mail', 'f@d.sk']]]],
    ['nič', { customer: {} }, []],
  ])('odberateľ %s', (_, inv, want) => expect(customerGroups(inv)).toEqual(want))
  it.each([
    ['bez adresy', {}, []],
    ['prázdna', { delivery: { street: ' ' } }, []],
    ['domáca', { delivery: { name: 'Sklad', street: 'Krátka 2', zip: '85101', city: 'Bratislava', country: 'SK' } }, ['Sklad', 'Krátka 2', '85101 Bratislava']],
    ['zahraničná', { delivery: { street: 'Hauptstr. 1', zip: '1010', city: 'Wien', country: 'AT' } }, ['Hauptstr. 1', '1010 Wien', 'AT!']],
  ])('dodacia adresa %s', (_, inv, want) => expect(deliveryLines(inv, (c) => c + '!')).toEqual(want))
})
