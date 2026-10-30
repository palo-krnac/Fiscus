import { describe, expect, it } from 'vitest'
import { accountLines, apiLines, editLines, filterPostings, filterTurnovers, openingLines, postingRows, sideOf } from './ledger.js'

const postings = [
  { key: 'sale:1', date: '2026-10-05', doc: 'sale', number: 'FA001', partner: 'Kováč s.r.o.', text: 'Vydaná faktúra', template: 'FV-SLUZBY', amount: '492.00',
    lines: [{ account: '311', name: 'Odberatelia', debit: '492.00', credit: '0.00' }, { account: '602', debit: '0.00', credit: '400.00' }, { account: '343', debit: '0.00', credit: '92.00' }] },
  { key: 'bank:1', date: '2026-11-02', doc: 'bank', number: 'B1', partner: 'Sociálna poisťovňa', amount: '-216.12', problem: 'Vyberte predkontáciu', lines: [] },
  { key: 'bank:2', date: '2026-11-03', doc: 'bank', number: 'B2', manual: true, amount: '-10',
    lines: [{ account: '221001', debit: '0.00', credit: '10.00' }, { account: '568', debit: '10.00', credit: '0.00' }] },
]

describe('účtovný denník', () => {
  it('hľadanie a filtre', () => {
    expect(filterPostings(postings, { search: 'kovac' })).toHaveLength(1)
    expect(filterPostings(postings, { search: '221.001' })[0].key).toBe('bank:2')
    expect(filterPostings(postings, { filter: 'problem' })[0].key).toBe('bank:1')
    expect(filterPostings(postings, { filter: 'manual' })).toHaveLength(1)
    expect(filterPostings(postings, { filter: 'bank' })).toHaveLength(2)
    expect(filterPostings(postings, { month: 10 })).toHaveLength(1)
  })
  it('export po riadkoch', () => {
    const rows = postingRows(postings)
    expect(rows).toHaveLength(6)
    expect(rows[1].line.account).toBe('602')
  })
  it('riadky účtu', () => {
    expect(accountLines(postings, '221')).toHaveLength(1)
    expect(accountLines(postings, '311')[0].line.debit).toBe('492.00')
  })
})

describe('hlavná kniha', () => {
  it('strana stavu', () => {
    expect(sideOf('100.00')).toEqual({ debit: '100.00', credit: '' })
    expect(sideOf('-100.00')).toEqual({ debit: '', credit: '100.00' })
    expect(sideOf('0.00')).toEqual({ debit: '', credit: '' })
  })
  it('hľadanie účtu', () => {
    expect(filterTurnovers([{ account: '221001', name: 'Tatra' }, { account: '311', name: 'Odberatelia' }], '221.001')).toHaveLength(1)
  })
  it('riadky formulára', () => {
    expect(editLines([])).toHaveLength(2)
    expect(apiLines([{ account: '221', debit: '1 000,50', credit: '' }, { account: '', debit: '', credit: '' }])).toEqual([{ account: '221', debit: '1000.50', credit: '0' }])
    expect(openingLines({ '411': '-1000.00', '221': '1000.00' })).toEqual([{ account: '221', debit: '1000.00', credit: '' }, { account: '411', debit: '', credit: '1000.00' }])
  })
})
