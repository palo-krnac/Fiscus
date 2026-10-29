// @vitest-environment node
import { describe, expect, it } from 'vitest'
import { decode } from 'bysquare/pay'
import QRCode from 'qrcode'
import jsQR from 'jsqr'
import { PNG } from 'pngjs'
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
  ])('%s → bez QR', (_, inv, comp) => {
    expect(payModel(inv, comp)).toBeNull()
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
