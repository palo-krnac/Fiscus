// Export zoznamov do Excelu (.xlsx) a CSV. Exportuje sa presne to, čo je
// v zozname po filtri a hľadaní. Sumy prichádzajú z API ako reťazce a do
// Excelu idú ako čísla s formátom (dá sa s nimi počítať), dátumy ako dátumy.
//
// Stĺpec: { header, value: (riadok) => hodnota, type: 'text'|'money'|'number'|'date', width }

const MONEY = '#,##0.00'
const DATE = 'dd.mm.yyyy'

function toDate(iso) {
  if (!iso) return null
  const [y, m, d] = String(iso).slice(0, 10).split('-').map(Number)
  if (!y || !m || !d) return null
  return new Date(Date.UTC(y, m - 1, d))
}

function toNumber(v) {
  if (v === null || v === undefined || v === '') return null
  const n = Number(String(v).replace(/\s/g, '').replace(',', '.'))
  return Number.isFinite(n) ? n : null
}

/** Bunka pre Excel. */
export function xlsxCell(col, row) {
  const v = col.value(row)
  switch (col.type) {
    case 'money': {
      const n = toNumber(v)
      return n === null ? null : { value: n, type: Number, format: MONEY }
    }
    case 'number': {
      const n = toNumber(v)
      return n === null ? null : { value: n, type: Number }
    }
    case 'date': {
      const d = toDate(v)
      return d ? { value: d, type: Date, format: DATE } : null
    }
    default:
      return v === null || v === undefined || v === '' ? null : { value: String(v), type: String }
  }
}

/** Údaje hárku: hlavička + riadky. */
export function sheetData(columns, rows) {
  return [
    columns.map((c) => ({ value: c.header, type: String, fontWeight: 'bold' })),
    ...rows.map((r) => columns.map((c) => xlsxCell(c, r))),
  ]
}

/** Text bunky pre CSV v slovenskom zápise (desatinná čiarka, dátum d.m.rrrr). */
export function csvText(col, row) {
  const v = col.value(row)
  if (v === null || v === undefined || v === '') return ''
  switch (col.type) {
    case 'money': {
      const n = toNumber(v)
      return n === null ? '' : n.toFixed(2).replace('.', ',')
    }
    case 'number': {
      const n = toNumber(v)
      return n === null ? '' : String(n).replace('.', ',')
    }
    case 'date': {
      const d = toDate(v)
      return d ? `${d.getUTCDate()}.${d.getUTCMonth() + 1}.${d.getUTCFullYear()}` : ''
    }
  }
  let s = String(v)
  // ochrana pred vložením vzorca do tabuľkového programu (CSV injection)
  if (/^[=+\-@\t\r]/.test(s)) s = `'${s}`
  return s
}

function quote(s) {
  return /[";\r\n]/.test(s) ? `"${s.replaceAll('"', '""')}"` : s
}

/** CSV pre Excel v slovenskom prostredí: oddeľovač „;“, UTF-8 s BOM, CRLF. */
export function toCSV(columns, rows) {
  const lines = [columns.map((c) => quote(c.header)), ...rows.map((r) => columns.map((c) => quote(csvText(c, r))))]
  return '﻿' + lines.map((l) => l.join(';')).join('\r\n') + '\r\n'
}

/** Názov súboru: faktury-2026-10-29.xlsx (bez diakritiky a medzier). */
export function exportFileName(base, ext, today = new Date()) {
  const slug = String(base)
    .normalize('NFD')
    .replace(/\p{M}/gu, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
  const d = today.toISOString().slice(0, 10)
  return `${slug || 'export'}-${d}.${ext}`
}

function save(blob, name) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export async function downloadXLSX(base, columns, rows, sheet = 'Export') {
  const { default: writeExcelFile } = await import('write-excel-file/browser')
  const blob = await writeExcelFile(sheetData(columns, rows), {
    columns: columns.map((c) => ({ width: c.width ?? (c.type === 'money' ? 14 : c.type === 'date' ? 12 : 20) })),
    sheet: sheet.slice(0, 31),
    stickyRowsCount: 1,
    fontFamily: 'Calibri',
    fontSize: 11,
  }).toBlob()
  save(blob, exportFileName(base, 'xlsx'))
}

export function downloadCSV(base, columns, rows) {
  save(new Blob([toCSV(columns, rows)], { type: 'text/csv;charset=utf-8' }), exportFileName(base, 'csv'))
}
