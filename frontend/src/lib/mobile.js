// Mobil v sieti: pomocné funkcie pre spárovanie telefónu.

/** Odhad mena zariadenia z prehliadača (dá sa prepísať). */
export function deviceName(ua = '') {
  if (/iPhone/i.test(ua)) return 'iPhone'
  if (/iPad/i.test(ua)) return 'iPad'
  const m = /Android[^;)]*;\s*([^;)]+?)(?:\s+Build|\))/i.exec(ua)
  if (m && !/^(K|wv)$/.test(m[1].trim())) return m[1].trim()
  if (/Android/i.test(ua)) return 'Android'
  return 'Telefón'
}

/** Zostávajúci čas platnosti kódu „4:59“ (prázdny = vypršal). */
export function remaining(expires, now = Date.now()) {
  const ms = new Date(expires).getTime() - now
  if (!(ms > 0)) return ''
  const s = Math.ceil(ms / 1000)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

/** Kód z adresy /parovanie?kod=… (iba povolené znaky). */
export function pairCode(search) {
  const k = new URLSearchParams(search).get('kod') ?? ''
  return k.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 6)
}
