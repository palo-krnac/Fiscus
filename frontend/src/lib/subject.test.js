import { describe, expect, it } from 'vitest'
import { partyCell, rowHeight, stackedRowHeight, subjectColumn } from './subject.js'

describe('predmet v zozname', () => {
  it.each([
    ['Alfa s.r.o.', 'Údržba webu', ['Alfa s.r.o.', 'Údržba webu']],
    ['Alfa s.r.o.', '', ['Alfa s.r.o.']],
    ['A <b> & B', '<i>x</i>', ['A <b> & B', '<i>x</i>']],
  ])('bunka %s / %s', (name, subject, texts) => {
    const el = partyCell(name, subject)
    expect([...el.children].map((c) => c.textContent)).toEqual(texts)
    expect(el.querySelector('b, i')).toBeNull()
  })
  it('stĺpec', () => {
    expect(subjectColumn.valueGetter({ data: { subject: 'Servis' } })).toBe('Servis')
    expect(subjectColumn.valueGetter({ data: {} })).toBe('')
  })
  it.each([
    [true, {}, stackedRowHeight],
    [true, { _totals: true }, undefined],
    [false, {}, undefined],
  ])('výška riadku na mobile=%s %o', (narrow, data, h) => {
    expect(rowHeight({ matches: narrow })({ data })).toBe(h)
  })
})
