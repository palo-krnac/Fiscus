import { describe, expect, it } from 'vitest'
import jsQR from 'jsqr'
import QRCode from 'qrcode'
import { findQR, fitSize, receiptId } from './receipt.js'

describe('bloček z mobilu', () => {
  it.each([
    [4000, 3000, 1600, { w: 1600, h: 1200 }],
    [3000, 4000, 1000, { w: 750, h: 1000 }],
    [800, 600, 1600, { w: 800, h: 600 }],
  ])('fitSize(%i, %i, %i)', (w, h, max, want) => expect(fitSize(w, h, max)).toEqual(want))

  it.each([
    ['O-7DBCDA8A56EE426DBCDA8A56EE426D1A', 'O-7DBCDA8A56EE426DBCDA8A56EE426D1A'],
    ['x o-7dbcda8a56ee426dbcda8a56ee426d1a y', 'O-7DBCDA8A56EE426DBCDA8A56EE426D1A'],
    ['OKP:1:2', ''],
    ['', ''],
  ])('receiptId(%s)', (t, want) => expect(receiptId(t)).toBe(want))

  it('prečíta QR kód bločku z obrázka', async () => {
    const id = 'O-7DBCDA8A56EE426DBCDA8A56EE426D1A'
    const qr = QRCode.create(id, { errorCorrectionLevel: 'M' })
    const n = qr.modules.size
    const scale = 8
    const pad = 4 * scale
    const size = n * scale + 2 * pad
    // biely obrázok s kódom (RGBA)
    const data = new Uint8ClampedArray(size * size * 4).fill(255)
    for (let y = 0; y < n; y++)
      for (let x = 0; x < n; x++)
        if (qr.modules.get(y, x))
          for (let dy = 0; dy < scale; dy++)
            for (let dx = 0; dx < scale; dx++) {
              const i = ((pad + y * scale + dy) * size + pad + x * scale + dx) * 4
              data[i] = data[i + 1] = data[i + 2] = 0
            }
    const calls = []
    const read = (w, h) => {
      calls.push(w)
      return { data, width: size, height: size } // test číta vždy originál
    }
    expect(findQR(jsQR, size, size, read, [size])).toBe(id)
    expect(calls).toEqual([size])
    expect(findQR(() => null, 100, 100, () => ({ data: new Uint8ClampedArray(4) }), [100, 50])).toBe('')
  })
})
