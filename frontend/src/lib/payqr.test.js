// @vitest-environment node
import { describe, expect, it } from 'vitest'
import { decode } from 'bysquare/pay'
import QRCode from 'qrcode'
import jsQR from 'jsqr'
import { PNG } from 'pngjs'
import { readFileSync } from 'node:fs'
import { payBySquare, payModel } from './payqr.js'

const company = {
  name: 'Demo elektroservis', street: 'Námestie SNP 1', zip: '974 01', city: 'Banská Bystrica',
  iban: 'SK31 1200 0000 1987 4263 7541',
}
const invoice = {
  status: 'issued', number: 'FA20260039', variableSymbol: '20260039', currency: 'EUR',
  dueDate: '2026-11-12', summary: { total: '269.12' },
}

describe('payModel', () => {
  it('poskladá platbu z faktúry', () => {
    const m = payModel(invoice, company)
    const p = m.payments[0]
    expect(m.invoiceId).toBe('FA20260039')
    expect(p.amount).toBe(269.12)
    expect(p.variableSymbol).toBe('20260039')
    expect(p.paymentDueDate).toBe('20261112')
    expect(p.bankAccounts[0].iban).toBe('SK3112000000198742637541')
    expect(p.beneficiary).toEqual({ name: 'Demo elektroservis', street: 'Námestie SNP 1', city: '974 01 Banská Bystrica' })
  })

  it.each([
    ['koncept', { ...invoice, status: 'draft' }, company],
    ['uhradená', { ...invoice, status: 'paid' }, company],
    ['stornovaná', { ...invoice, status: 'cancelled' }, company],
    ['nulová suma', { ...invoice, summary: { total: '0.00' } }, company],
    ['dobropis', { ...invoice, summary: { total: '-50.00' } }, company],
    ['bez IBAN', invoice, { ...company, iban: '' }],
    ['platba v hotovosti', { ...invoice, paymentMethod: 'cash' }, company],
    ['uhradená čiastočne do nuly', { ...invoice, balance: '0.00' }, company],
  ])('%s → bez QR', (_, inv, comp) => {
    expect(payModel(inv, comp)).toBeNull()
  })
})

describe('čiastočná úhrada', () => {
  it('QR je na zostatok', () => {
    expect(payModel({ ...invoice, balance: '69.12' }, company).payments[0].amount).toBe(69.12)
  })
})

describe('PAY by square', () => {
  it('kód sa dá dekódovať späť na rovnakú platbu (bez diakritiky)', () => {
    const code = payBySquare(invoice, company)
    expect(code).toMatch(/^[0-9A-V]+$/) // base32hex
    const back = decode(code)
    const p = back.payments[0]
    expect(back.invoiceId).toBe('FA20260039')
    expect(p.amount).toBe(269.12)
    expect(p.currencyCode).toBe('EUR')
    expect(p.variableSymbol).toBe('20260039')
    expect(p.paymentDueDate).toBe('20261112')
    expect(p.bankAccounts[0].iban).toBe('SK3112000000198742637541')
    expect(p.beneficiary.name).toBe('Demo elektroservis')
    expect(p.beneficiary.city).toBe('974 01 Banska Bystrica')
  })

  it('QR obrázok sa dá prečítať čítačkou QR kódov', async () => {
    const code = payBySquare(invoice, company)
    const buf = await QRCode.toBuffer(code, { errorCorrectionLevel: 'M', margin: 4, scale: 6 })
    const png = PNG.sync.read(buf)
    const read = jsQR(new Uint8ClampedArray(png.data), png.width, png.height)
    expect(read?.data).toBe(code)
  })
})

// QR do PDF skladá server (backend/internal/payqr). Jeho vzorky musia prečítať
// rovnaké knižnice ako QR z frontendu: bysquare (obsah) a jsQR (obrázok).
describe('PAY by square zo servera', () => {
  const deburr = (t) => t.normalize('NFD').replace(/\p{M}/gu, '')
  const golden = JSON.parse(readFileSync(new URL('../../../backend/internal/payqr/testdata/golden.json', import.meta.url), 'utf8'))

  function render(rows, scale = 4, quiet = 4) {
    const n = rows.length
    const size = (n + 2 * quiet) * scale
    const png = new PNG({ width: size, height: size })
    for (let y = 0; y < size; y++) {
      for (let x = 0; x < size; x++) {
        const mx = Math.floor(x / scale) - quiet
        const my = Math.floor(y / scale) - quiet
        const dark = mx >= 0 && my >= 0 && mx < n && my < n && rows[my][mx] === '1'
        const i = (y * size + x) * 4
        png.data[i] = png.data[i + 1] = png.data[i + 2] = dark ? 0 : 255
        png.data[i + 3] = 255
      }
    }
    return png
  }

  it.each(golden.map((g) => [g.name, g]))('%s', (_, g) => {
    const back = decode(g.text)
    const p = back.payments[0]
    expect(back.invoiceId).toBe(g.id)
    expect(p.amount).toBe(Number(g.payment.Amount))
    expect(p.currencyCode).toBe(g.payment.Currency)
    expect(p.paymentDueDate).toBe(g.payment.DueDate)
    expect(p.variableSymbol).toBe(g.payment.VariableSymbol)
    expect(p.bankAccounts[0].iban).toBe(g.payment.IBAN.replace(/\s/g, '').toUpperCase())
    expect(p.beneficiary.name).toBe(deburr(g.payment.Name))
    expect(p.paymentNote).toBe(deburr(g.payment.Note))

    expect(g.rows.length).toBe(g.version * 4 + 17)
    const png = render(g.rows)
    const read = jsQR(new Uint8ClampedArray(png.data), png.width, png.height)
    expect(read?.data).toBe(g.text)
  })
})
