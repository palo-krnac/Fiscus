// Zdaňovacie obdobia DPH pre výber v rozhraní. Výpočet priznania je v backende.

const MONTHS = ['január', 'február', 'marec', 'apríl', 'máj', 'jún', 'júl', 'august', 'september', 'október', 'november', 'december']

/** Prvé obdobie na tlačive DPHv25. */
export const FIRST = { year: 2025, month: 7 }

export function periodLabel(value) {
  const [y, rest] = value.split('-')
  if (rest.startsWith('Q')) return `${rest.slice(1)}. štvrťrok ${y}`
  return `${MONTHS[Number(rest) - 1]} ${y}`
}

/**
 * Ponuka období od najnovšieho (aktuálne obdobie) po júl 2025.
 * kind: 'month' | 'quarter'
 */
export function periodOptions(kind, today = new Date()) {
  const out = []
  let y = today.getFullYear()
  if (kind === 'quarter') {
    let q = Math.floor(today.getMonth() / 3) + 1
    while (y > FIRST.year || (y === FIRST.year && q >= 3)) {
      const value = `${y}-Q${q}`
      out.push({ value, label: periodLabel(value) })
      if (--q === 0) {
        q = 4
        y--
      }
    }
    return out
  }
  let mo = today.getMonth() + 1
  while (y > FIRST.year || (y === FIRST.year && mo >= FIRST.month)) {
    const value = `${y}-${String(mo).padStart(2, '0')}`
    out.push({ value, label: periodLabel(value) })
    if (--mo === 0) {
      mo = 12
      y--
    }
  }
  return out
}

/** Predvolené obdobie: predchádzajúce (to sa práve podáva), ak existuje. */
export function defaultPeriod(kind, today = new Date()) {
  const opts = periodOptions(kind, today)
  return (opts[1] ?? opts[0])?.value ?? ''
}

/** Termín podania: 25. deň po skončení obdobia. */
export function dueDate(value) {
  const [y, rest] = value.split('-')
  let year = Number(y)
  let month = rest.startsWith('Q') ? Number(rest.slice(1)) * 3 + 1 : Number(rest) + 1
  if (month > 12) {
    month -= 12
    year++
  }
  return `${year}-${String(month).padStart(2, '0')}-25`
}

/** Druhy priznania (zhodné s backendom). */
export const returnKinds = [
  { label: 'Riadne', value: 'riadne' },
  { label: 'Opravné', value: 'opravne' },
  { label: 'Dodatočné', value: 'dodatocne' },
]

/**
 * Navrhnutý druh priznania: bez podania riadne; po podaní pred lehotou
 * opravné, po lehote dodatočné (lehota z backendu, ISO dátumy).
 */
export function suggestKind(filings, deadlineISO, todayISO) {
  if (!filings?.length) return 'riadne'
  return todayISO > deadlineISO ? 'dodatocne' : 'opravne'
}

/** Posledná známa povinnosť (+) / nadmerný odpočet (−) z posledného podania ('' bez podania). */
export const lastKnownOf = (filings) => (filings?.length ? String(filings[filings.length - 1].result) : '')

/** Parametre dotazu pre druh priznania (prázdne hodnoty vynechá query()). */
export function kindParams(s) {
  if (!s || s.druh === 'riadne') return {}
  const num = (v) => String(v ?? '').replace(/\s/g, '').replace(',', '.')
  if (s.druh === 'opravne') return { druh: 'opravne' }
  return { druh: 'dodatocne', zistene: s.zistene || '', posledne: num(s.posledne), r37: num(s.r37) }
}
