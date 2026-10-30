// Kniha jázd: číselníky, popisy a hľadanie. Kilometre, náklady a náhrady
// počíta server (internal/logbook), tu sa iba zobrazujú.
import { matches, searchTerms } from './search.js'
import { formatDate } from './format.js'

export const fuelKinds = [
  { value: 'diesel', label: 'Nafta', unit: 'l' },
  { value: 'petrol', label: 'Benzín', unit: 'l' },
  { value: 'lpg', label: 'LPG', unit: 'l' },
  { value: 'cng', label: 'CNG', unit: 'kg' },
  { value: 'electric', label: 'Elektrina', unit: 'kWh' },
]

export const fuelUnit = (fuel) => fuelKinds.find((f) => f.value === fuel)?.unit ?? 'l'

export const ownerships = [
  { value: 'business', label: 'V obchodnom majetku' },
  { value: 'leased', label: 'Prenajaté, leasing' },
  { value: 'private', label: 'Súkromné' },
]

/** Spôsob uplatnenia PHM; paušál je 80 % (v majetku, prenajaté) alebo 50 % (súkromné). */
export function methodOptions(ownership) {
  const flat = ownership === 'private' ? 50 : 80
  return [
    { value: 'log', label: 'Podľa knihy jázd' },
    { value: 'flat', label: `Paušál ${flat} % PHM` },
  ]
}

/** Vysvetlenie, ako sa z knihy počítajú výdavky. */
export function methodHint(v) {
  if (!v) return ''
  if (v.ownership === 'private') {
    return v.method === 'flat'
      ? 'Súkromné vozidlo, paušál: výdavkom je 50 % ceny PHM z dokladov, bez základnej náhrady za km. Treba stav tachometra na začiatku a konci roka.'
      : 'Súkromné vozidlo: za služobné jazdy cestovné náhrady — základná náhrada za každý km (opotrebenie) a PHM podľa spotreby z technického preukazu a ceny z dokladov o tankovaní.'
  }
  return v.method === 'flat'
    ? 'Paušál: výdavkom je 80 % ceny PHM (pri nižšom podiele služobných jázd podľa knihy menej), kniha jázd nie je povinná — stačí stav tachometra na začiatku a konci roka. Odpisy a ostatné výdavky v podiele podnikania.'
    : 'Podľa knihy jázd: výdavkom je PHM na služobné jazdy, najviac podľa spotreby z technického preukazu (nadspotreba nie je daňový výdavok). PHM na súkromné jazdy sa z výdavkov vylúči.'
}

/** Popis vozidla do výberu: „BB-123AB Škoda Octavia“. */
export const vehicleLabel = (v) => `${v.plate} ${v.name}${v.archived ? ' – archivované' : ''}`

/** Trasa jazdy: „Banská Bystrica – Zvolen“. */
export const tripRoute = (t) => [t.from, t.to].filter(Boolean).join(' – ')

/** Čas jazdy „07:30–08:10“. */
export const tripTime = (t) => (t.depart || t.arrive ? `${t.depart || '?'}–${t.arrive || '?'}` : '')

export const tripKinds = [
  { value: 'business', label: 'Služobná' },
  { value: 'private', label: 'Súkromná' },
]

/** Hľadanie v jazdách (bez diakritiky, viac slov) a filter druhu. */
export function filterTrips(trips, query, kind = 'all') {
  const terms = searchTerms(query)
  return trips.filter(
    (t) => (kind === 'all' || t.kind === kind) && matches(terms, [formatDate(t.date), t.from, t.to, t.purpose, t.driver, t.note, String(t.km)]),
  )
}

/** Hľadanie v tankovaniach. */
export function filterFuel(list, query) {
  const terms = searchTerms(query)
  return list.filter((f) => matches(terms, [formatDate(f.date), f.station, f.docNumber, f.note, f.amount, f.quantity]))
}

/** Účely služobných jázd na našepkávanie (najčastejšie prvé). */
export function purposeSuggestions(trips, limit = 12) {
  const count = new Map()
  for (const t of trips) if (t.purpose) count.set(t.purpose, (count.get(t.purpose) ?? 0) + 1)
  return [...count.entries()].sort((a, b) => b[1] - a[1]).slice(0, limit).map(([p]) => p)
}

/** Nová jazda: dátum dnes, tachometer od posledného stavu, vodič vozidla. */
export function blankTrip(vehicle, today) {
  return { id: 0, vehicleId: vehicle.id, date: today, depart: '', arrive: '', from: '', to: '', purpose: '', kind: 'business',
    driver: vehicle.driver ?? '', odoStart: vehicle.lastOdometer || null, odoEnd: null, km: null, note: '' }
}

/** Spiatočná jazda k uloženej: opačná trasa, tachometer pokračuje. */
export function returnTrip(t) {
  return { ...t, id: 0, from: t.to, to: t.from, depart: '', arrive: '', odoStart: t.odoEnd || null, odoEnd: t.odoEnd && t.km ? t.odoEnd + t.km : null }
}

/** Stav zaúčtovania vyúčtovania do uzávierky (sumy zo servera). */
export function logbookPosting(r) {
  if (!r) return null
  const posting = Number(r.posting ?? 0)
  const posted = Number(r.posted ?? 0)
  if (posting === 0 && posted === 0) return { tone: 'draft', label: 'Nie je čo zaúčtovať', action: '' }
  if (posted === 0) return { tone: 'overdue', label: 'Nezaúčtované v uzávierke', action: 'Zaúčtovať do uzávierky' }
  if (posted !== posting) return { tone: 'overdue', label: 'Kniha sa zmenila po zaúčtovaní', action: 'Zaúčtovať znova' }
  return { tone: 'paid', label: 'Zaúčtované v uzávierke (stĺpec e6)', action: 'Zaúčtovať znova' }
}
