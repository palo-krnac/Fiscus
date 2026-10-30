// Suma slovom pre pokladničné doklady: „dvestopäťdesiattri eur 50 centov“.

const ONES = ['', 'jeden', 'dva', 'tri', 'štyri', 'päť', 'šesť', 'sedem', 'osem', 'deväť']
const TEENS = ['desať', 'jedenásť', 'dvanásť', 'trinásť', 'štrnásť', 'pätnásť', 'šestnásť', 'sedemnásť', 'osemnásť', 'devätnásť']
const TENS = ['', '', 'dvadsať', 'tridsať', 'štyridsať', 'päťdesiat', 'šesťdesiat', 'sedemdesiat', 'osemdesiat', 'deväťdesiat']
const HUNDREDS = ['', 'sto', 'dvesto', 'tristo', 'štyristo', 'päťsto', 'šesťsto', 'sedemsto', 'osemsto', 'deväťsto']

/** 0–999 slovom (jedným slovom, ako sa píše na dokladoch). */
function upTo999(n, one = 'jeden', two = 'dva') {
  const h = Math.floor(n / 100)
  const rest = n % 100
  let s = HUNDREDS[h]
  if (rest >= 10 && rest < 20) return s + TEENS[rest - 10]
  s += TENS[Math.floor(rest / 10)]
  const u = rest % 10
  if (u === 1) s += one
  else if (u === 2) s += two
  else s += ONES[u]
  return s
}

/** Celé číslo slovom (do 999 999 999). */
export function numberInWords(n) {
  n = Math.floor(Math.abs(n))
  if (n === 0) return 'nula'
  const millions = Math.floor(n / 1e6)
  const thousands = Math.floor((n % 1e6) / 1000)
  const rest = n % 1000
  let s = ''
  if (millions) {
    if (millions === 1) s += 'jedenmilión'
    else if (millions < 5) s += upTo999(millions, 'jeden', 'dva') + 'milióny'
    else s += upTo999(millions) + 'miliónov'
  }
  if (thousands) {
    // „tisíc“ (nie „jedentisíc“), „dvetisíc“
    s += thousands === 1 ? 'tisíc' : upTo999(thousands, 'jeden', 'dve') + 'tisíc'
  }
  if (rest) s += upTo999(rest)
  return s
}

function form(n, one, few, many) {
  if (n === 1) return one
  if (n >= 2 && n <= 4) return few
  return many
}

/** Suma (reťazec z API, napr. "253.50") slovom v eurách a centoch. */
export function amountInWords(amount) {
  const [whole, frac = '0'] = String(amount).replace('-', '').split('.')
  const eur = Number(whole)
  const cents = Math.round(Number(`0.${frac}`) * 100)
  // euro je stredného rodu: jedno euro, dve eurá
  const num = eur === 1 ? 'jedno' : eur === 2 ? 'dve' : numberInWords(eur)
  let s = `${num} ${form(eur, 'euro', 'eurá', 'eur')}`
  if (cents) s += ` ${cents} ${form(cents, 'cent', 'centy', 'centov')}`
  return s
}

/** Suma slovom v mene dokladu: v eurách ako amountInWords, inak „dvetisícpäťsto CZK 50/100“. */
export function amountInWordsIn(amount, currency = 'EUR') {
  if (!currency || currency === 'EUR') return amountInWords(amount)
  const [whole, frac = '0'] = String(amount).replace('-', '').split('.')
  const cents = Math.round(Number(`0.${frac}`) * 100)
  return `${numberInWords(Number(whole))} ${currency}${cents ? ` ${cents}/100` : ''}`
}
