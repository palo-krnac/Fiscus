// Kalendár termínov: mriežka mesiaca, druhy termínov, filtre a odkaz na odber.
// Termíny (aj posun lehôt na pracovný deň) počíta server; tu sa iba zobrazujú.

export const kinds = [
  { id: 'tax', label: 'Dane', short: 'Daň' },
  { id: 'own', label: 'Vlastné', short: 'Vlastný' },
  { id: 'pay', label: 'Na úhradu', short: 'Úhrada' },
  { id: 'receive', label: 'Pohľadávky', short: 'Splatnosť' },
  { id: 'recurring', label: 'Pravidelné faktúry', short: 'Pravidelná' },
  { id: 'quote', label: 'Ponuky', short: 'Ponuka' },
]

export const kindLabel = (k) => kinds.find((x) => x.id === k)?.label ?? k

export const monthNames = ['január', 'február', 'marec', 'apríl', 'máj', 'jún', 'júl', 'august', 'september', 'október', 'november', 'december']
export const weekdays = ['Po', 'Ut', 'St', 'Št', 'Pi', 'So', 'Ne']

const pad = (n) => String(n).padStart(2, '0')
/** ISO dátum "2026-10-05". */
export const iso = (y, m, d) => `${y}-${pad(m)}-${pad(d)}`

/** Prvý a posledný deň mesiaca (m = 1–12). */
export function monthRange(y, m) {
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate()
  return { from: iso(y, m, 1), to: iso(y, m, last) }
}

/** Posun mesiaca o delta: { y, m }. */
export function shiftMonth(y, m, delta) {
  const t = y * 12 + (m - 1) + delta
  return { y: Math.floor(t / 12), m: (t % 12) + 1 }
}

/**
 * Mriežka mesiaca po týždňoch (pondelok prvý). Každý deň:
 * { date, day, inMonth, weekend }. Vždy celé týždne.
 */
export function monthGrid(y, m) {
  const first = new Date(Date.UTC(y, m - 1, 1))
  const lead = (first.getUTCDay() + 6) % 7 // počet dní pred 1. (pondelok = 0)
  const start = new Date(Date.UTC(y, m - 1, 1 - lead))
  const days = new Date(Date.UTC(y, m, 0)).getUTCDate()
  const cells = Math.ceil((lead + days) / 7) * 7
  const weeks = []
  for (let i = 0; i < cells; i++) {
    const d = new Date(start.getTime() + i * 86400000)
    if (i % 7 === 0) weeks.push([])
    weeks.at(-1).push({
      date: iso(d.getUTCFullYear(), d.getUTCMonth() + 1, d.getUTCDate()),
      day: d.getUTCDate(),
      inMonth: d.getUTCMonth() === m - 1,
      weekend: i % 7 >= 5,
    })
  }
  return weeks
}

/** Termíny podľa dňa: { "2026-10-26": [..] }. */
export function byDate(events) {
  const out = {}
  for (const e of events) (out[e.date] ??= []).push(e)
  return out
}

/** Stav termínu na zobrazenie a export. */
export function stateOf(e) {
  if (e.done) return { tone: 'paid', text: e.kind === 'tax' ? 'podané' : 'splnené' }
  if (e.overdue) return { tone: 'overdue', text: 'po termíne' }
  return { tone: '', text: '' }
}

/** Text hľadania pre termín. */
export const searchText = (e) => [e.title, e.detail, kindLabel(e.kind), e.date]

/** Krátky dátum „po 26. 10.“ pre zoznam. */
export function dayLabel(isoDate) {
  const [y, m, d] = isoDate.split('-').map(Number)
  const wd = weekdays[(new Date(Date.UTC(y, m - 1, d)).getUTCDay() + 6) % 7]
  return `${wd} ${d}. ${m}.`
}

/** Počet dní medzi dvoma ISO dátumami (b − a). */
export function daysBetween(a, b) {
  return Math.round((Date.parse(b + 'T00:00:00Z') - Date.parse(a + 'T00:00:00Z')) / 86400000)
}

/** Kedy: „dnes“, „zajtra“, „o 5 dní“, „pred 3 dňami“. */
export function relative(today, date) {
  const n = daysBetween(today, date)
  if (n === 0) return 'dnes'
  if (n === 1) return 'zajtra'
  if (n === -1) return 'včera'
  if (n > 0) return n < 5 ? `o ${n} dni` : `o ${n} dní`
  return `pred ${-n} dňami`
}

/** Termín je povinnosť používateľa (nie pohľadávka, ponuka ani pravidelná faktúra). */
export const mine = (e) => e.kind === 'tax' || e.kind === 'own' || e.kind === 'pay'

/**
 * Odznak v menu: vlastné povinnosti (dane, úhrady, vlastné termíny) do 7 dní
 * a zmeškané. Pohľadávky sú v Pohľadávkach a upomienkach.
 */
export function soonCount(view) {
  if (!view) return 0
  const n = (e) => daysBetween(view.today, e.date)
  const todo = [...view.overdue, ...view.events].filter((e) => mine(e) && !e.done)
  return todo.filter((e) => e.overdue || (n(e) >= 0 && n(e) <= 7)).length
}

/** Rozsah pre odznak v menu: od začiatku mesiaca po 8 dní dopredu (d = Date). */
export function badgeRange(d) {
  const today = iso(d.getFullYear(), d.getMonth() + 1, d.getDate())
  const end = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate() + 8))
  return { from: today.slice(0, 8) + '01', to: iso(end.getUTCFullYear(), end.getUTCMonth() + 1, end.getUTCDate()) }
}

/** Adresa odberu: https://… pre Google a Outlook, webcal://… pre Apple. */
export function feedURLs(origin, cid, token) {
  const path = `/api/public/calendar/${cid}/${token}/fiskus.ics`
  const https = origin.replace(/\/$/, '') + path
  return { https, webcal: https.replace(/^https?:/, 'webcal:') }
}

/** Adresa je dostupná iba z tohto počítača (desktop) — Google ju nestiahne. */
export function localOnly(origin) {
  try {
    const h = new URL(origin).hostname
    return h === 'localhost' || h === '127.0.0.1' || h === '::1' || h === '[::1]'
  } catch {
    return false
  }
}

export const repeats = [
  { value: '', label: 'Neopakuje sa' },
  { value: 'monthly', label: 'Každý mesiac' },
  { value: 'quarterly', label: 'Každý štvrťrok' },
  { value: 'yearly', label: 'Každý rok' },
]
export const repeatLabel = (r) => repeats.find((x) => x.value === (r || ''))?.label ?? r

/**
 * Predvolené vlastné termíny pre živnostníka a s. r. o. (year = rok prvého
 * termínu). Lehoty, ktoré sa posúvajú na pracovný deň, majú workday.
 */
export function presets(year) {
  return [
    { title: 'Odvody do Sociálnej poisťovne', note: 'Preddavok SZČO za predchádzajúci mesiac (do 8. dňa).', date: iso(year, 1, 8), repeat: 'monthly', workday: true },
    { title: 'Preddavok na zdravotné poistenie', note: 'Za predchádzajúci mesiac (do 8. dňa).', date: iso(year, 1, 8), repeat: 'monthly', workday: true },
    { title: 'Preddavok na daň z príjmov – štvrťročne', note: 'Ak bola posledná známa daňová povinnosť vyššia ako 5 000 € (najviac 16 600 €), do konca štvrťroka.', date: iso(year, 3, 31), repeat: 'quarterly', workday: true },
    { title: 'Preddavok na daň z príjmov – mesačne', note: 'Ak bola posledná známa daňová povinnosť vyššia ako 16 600 €, do konca mesiaca.', date: iso(year, 1, 31), repeat: 'monthly', workday: true },
    { title: 'Mzdy: odvody a preddavok na daň', note: 'Poistné v deň výplaty, preddavok na daň zo mzdy do 5 dní po výplate.', date: '', repeat: 'monthly', workday: true },
    { title: 'Daň z nehnuteľností', note: 'Podľa rozhodnutia obce.', date: iso(year, 5, 31), repeat: 'yearly', workday: true },
    { title: 'STK a emisná kontrola', note: '', date: '', repeat: 'yearly', workday: false },
    { title: 'Poistka vozidla (PZP, havarijná)', note: '', date: '', repeat: 'yearly', workday: false },
  ]
}
