import { describe, expect, it } from 'vitest'
import { docRoute, emailStatus, filterEmails, guessProvider, portFor } from './emails.js'

const rows = [
  { id: 1, docKind: 'invoice', docId: 7, docNumber: 'FA20260007', to: 'uctaren@zeleny.sk', subject: 'Faktúra FA20260007 – Krnáč', user: 'Palo', error: '' },
  { id: 2, docKind: 'reminder', docId: 3, docNumber: 'UP20260003', to: 'dlznik@firma.sk', subject: 'Upomienka', user: 'automaticky', error: '535 zlé heslo' },
  { id: 3, docKind: 'test', docId: 0, to: 'ja@firma.sk', subject: 'Skúšobná správa', user: 'Palo', error: '' },
]

describe('filterEmails', () => {
  it.each([
    ['all', '', [1, 2, 3]],
    ['failed', '', [2]],
    ['all', 'zeleny', [1]],
    ['all', 'faktura krnac', [1]],
    ['all', 'upomienka zle', [2]],
    ['all', 'skusobna', [3]],
    ['all', 'neodoslany', [2]],
    ['failed', 'zeleny', []],
  ])('%s „%s“', (view, q, ids) => {
    expect(filterEmails(rows, view, q).map((r) => r.id)).toEqual(ids)
  })
})

describe('pomocníci', () => {
  it('cesta k dokladu', () => {
    expect(rows.map(docRoute)).toEqual(['/faktury/7', '/upomienky/3/tlac', null])
    expect(docRoute({ docKind: 'quote', docId: 5 })).toBe('/ponuky/5')
    expect(docRoute({ docKind: 'notice', docId: 0 })).toBe('/pravidelne')
  })
  it('stav', () => {
    expect(emailStatus(rows[0]).tone).toBe('paid')
    expect(emailStatus(rows[1]).label).toBe('Neodoslaný')
  })
  it.each([
    ['starttls', 587],
    ['tls', 465],
    ['none', 25],
    [undefined, 587],
  ])('port %s → %d', (sec, port) => expect(portFor(sec)).toBe(port))
  it.each([
    ['palo@gmail.com', 'smtp.gmail.com', 587],
    ['x@Azet.sk', 'smtp.azet.sk', 465],
    ['faktury@krnac.sk', null, null],
    ['', null, null],
  ])('poskytovateľ %s', (email, host, port) => {
    const p = guessProvider(email)
    expect(p?.host ?? null).toBe(host)
    expect(p?.port ?? null).toBe(port)
  })
})
