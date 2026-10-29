// @vitest-environment node
import { describe, expect, it } from 'vitest'
import writeExcelFile from 'write-excel-file/node'
import readExcelFile from 'read-excel-file/node'
import { csvText, exportFileName, sheetData, toCSV } from './export.js'

const columns = [
  { header: 'Číslo', value: (r) => r.number },
  { header: 'Odberateľ', value: (r) => r.customer },
  { header: 'Vystavená', value: (r) => r.issueDate, type: 'date' },
  { header: 'Spolu', value: (r) => r.total, type: 'money' },
]
const rows = [
  { number: 'FA20260001', customer: 'Elektro "Hron"; s.r.o.', issueDate: '2026-10-29', total: '1234.5' },
  { number: '', customer: '=HYPERLINK("x")', issueDate: '', total: '-309.96' },
]

describe('CSV', () => {
  it('slovenský zápis, BOM, bodkočiarka, úvodzovky', () => {
    const csv = toCSV(columns, rows)
    expect(csv.startsWith('﻿')).toBe(true)
    const lines = csv.slice(1).split('\r\n')
    expect(lines[0]).toBe('Číslo;Odberateľ;Vystavená;Spolu')
    expect(lines[1]).toBe('FA20260001;"Elektro ""Hron""; s.r.o.";29.10.2026;1234,50')
    expect(lines[2]).toBe(`;"'=HYPERLINK(""x"")";;-309,96`)
  })
  it('vzorec sa neprenesie', () => {
    expect(csvText(columns[1], rows[1])).toBe(`'=HYPERLINK("x")`)
    expect(csvText(columns[3], rows[1])).toBe('-309,96') // záporná suma nie je vzorec
  })
})

describe('Excel', () => {
  it('čísla a dátumy sa dajú prečítať späť', async () => {
    const buf = await writeExcelFile(sheetData(columns, rows), { sheet: 'Faktúry' }).toBuffer()
    const sheets = await readExcelFile(buf)
    const data = Array.isArray(sheets[0]?.data) ? sheets[0].data : sheets
    expect(data[0]).toEqual(['Číslo', 'Odberateľ', 'Vystavená', 'Spolu'])
    expect(data[1][0]).toBe('FA20260001')
    expect(data[1][3]).toBe(1234.5)
    expect(new Date(data[1][2]).toISOString().slice(0, 10)).toBe('2026-10-29')
    expect(data[2][3]).toBe(-309.96)
    expect(data[2][1]).toBe('=HYPERLINK("x")') // text, nie vzorec
  })
})

describe('názov súboru', () => {
  it('bez diakritiky', () => {
    expect(exportFileName('Prijaté faktúry 2026', 'xlsx', new Date('2026-10-29T10:00:00Z'))).toBe('prijate-faktury-2026-2026-10-29.xlsx')
  })
})
