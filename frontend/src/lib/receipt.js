// Bloček z mobilu: zmenšenie fotky a prečítanie QR kódu (jsQR) v prehliadači.

/** Rozmery zmenšené tak, aby dlhšia strana mala najviac max (nikdy nezväčšuje). */
export function fitSize(w, h, max) {
  const k = Math.min(1, max / Math.max(w, h))
  return { w: Math.max(1, Math.round(w * k)), h: Math.max(1, Math.round(h * k)) }
}

/**
 * Prečíta QR kód z obrázka. read(w, h) vráti ImageData v danej veľkosti;
 * skúša viac veľkostí (malý kód na veľkej fotke sa lepšie číta vo väčšej,
 * rozmazaná fotka v menšej). Vráti text kódu alebo ''.
 */
export function findQR(jsQR, width, height, read, sizes = [1600, 1000, 2400, 700]) {
  const tried = new Set()
  for (const max of sizes) {
    const { w, h } = fitSize(width, height, max)
    if (tried.has(w)) continue
    tried.add(w)
    const img = read(w, h)
    const code = jsQR(img.data, w, h, { inversionAttempts: 'attemptBoth' })
    if (code?.data) return code.data
  }
  return ''
}

/** Je text identifikátor online bločku eKasa (O-…)? Rovnaké pravidlo ako server. */
export function receiptId(text) {
  const m = /\bO-[0-9A-F]{32}\b/i.exec(String(text ?? ''))
  return m ? 'O-' + m[0].slice(2).toUpperCase() : ''
}

export const payments = [
  { label: 'Hotovosť', value: 'cash' },
  { label: 'Karta', value: 'card' },
]
