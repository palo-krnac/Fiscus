import { describe, expect, it } from 'vitest'
import jsQR from 'jsqr'
import QRCode from 'qrcode'
import { formatSecret, isAppCode, mfaQR, recoveryFile } from './mfa.js'

describe('dvojfázové overenie', () => {
  it.each([
    ['JBSWY3DPEHPK3PXP', 'JBSW Y3DP EHPK 3PXP'],
    ['ABCDE', 'ABCD E'],
    ['', ''],
    [undefined, ''],
  ])('kľúč %s', (s, want) => {
    expect(formatSecret(s)).toBe(want)
  })
  it.each([
    ['123456', true],
    ['123 456', true],
    ['12345', false],
    ['abcde-fghjk', false],
  ])('kód z aplikácie %s', (c, want) => {
    expect(isAppCode(c)).toBe(want)
  })
  it('súbor so záložnými kódmi', () => {
    const t = recoveryFile(['aaaaa-bbbbb', 'ccccc-ddddd'], 'palo@example.sk', new Date(2026, 9, 29))
    expect(t).toContain('palo@example.sk')
    expect(t).toContain('29. 10. 2026')
    expect(t.split('\n')).toContain('ccccc-ddddd')
  })
  it('QR kód obsahuje adresu otpauth', async () => {
    const uri = 'otpauth://totp/Fi%C5%A1kus:palo@example.sk?secret=JBSWY3DPEHPK3PXP&issuer=Fi%C5%A1kus'
    expect(await mfaQR(uri)).toContain('<svg')
    // rovnaké nastavenie ako mfaQR, iba ako obrázok na prečítanie
    const { PNG } = await import('pngjs')
    const png = PNG.sync.read(await QRCode.toBuffer(uri, { errorCorrectionLevel: 'M', margin: 4, scale: 6 }))
    expect(jsQR(new Uint8ClampedArray(png.data), png.width, png.height)?.data).toBe(uri)
  })
})
