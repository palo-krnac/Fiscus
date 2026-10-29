// Číselníky. Systémové (sadzby DPH, spôsoby úhrady…) prichádzajú z API
// /api/codes; zoznam krajín je statický (ISO 3166-1).
import { reactive } from 'vue'
import { api } from './api.js'

export const codes = reactive({
  vatPeriods: [],
  paymentMethods: [],
  defaultUnits: [],
  defaultFormats: {},
  currencies: [],
  domesticRCs: [],
})

export async function loadCodes() {
  Object.assign(codes, await api.codes())
}

/** Sadzby DPH platné k dátumu dodania — rovnaké pravidlo ako vat.RatesOn v Go. */
export function ratesOn(date, periods = codes.vatPeriods) {
  if (!periods.length) return [23, 19, 5, 0]
  const d = date || new Date().toISOString().slice(0, 10)
  const p = periods.find((x) => d >= x.from && (!x.to || d <= x.to))
  if (p) return p.rates
  return d < periods[0].from ? periods[0].rates : periods[periods.length - 1].rates
}

export function paymentLabel(code) {
  return codes.paymentMethods.find((m) => m.code === code)?.label ?? 'Prevodom na účet'
}

/**
 * Náhľad čísla dokladu podľa formátu — iba na zobrazenie v nastaveniach.
 * Skutočné číslo prideľuje backend pri vystavení.
 */
export function previewNumber(format, date = new Date(), seq = 1) {
  if (!format) return ''
  const y = date.getFullYear()
  return format
    .replace(/\{(N{1,8})\}/g, (_, n) => String(seq).padStart(n.length, '0'))
    .replaceAll('{RRRR}', String(y))
    .replaceAll('{RR}', String(y % 100).padStart(2, '0'))
    .replaceAll('{MM}', String(date.getMonth() + 1).padStart(2, '0'))
}

// Krajiny: najčastejšie hore, ďalej EÚ a okolie.
export const countries = [
  ['SK', 'Slovensko'], ['CZ', 'Česko'], ['HU', 'Maďarsko'], ['PL', 'Poľsko'], ['AT', 'Rakúsko'],
  ['DE', 'Nemecko'], ['UA', 'Ukrajina'],
  ['BE', 'Belgicko'], ['BG', 'Bulharsko'], ['HR', 'Chorvátsko'], ['CY', 'Cyprus'], ['DK', 'Dánsko'],
  ['EE', 'Estónsko'], ['FI', 'Fínsko'], ['FR', 'Francúzsko'], ['GR', 'Grécko'], ['IE', 'Írsko'],
  ['IT', 'Taliansko'], ['LV', 'Lotyšsko'], ['LT', 'Litva'], ['LU', 'Luxembursko'], ['MT', 'Malta'],
  ['NL', 'Holandsko'], ['PT', 'Portugalsko'], ['RO', 'Rumunsko'], ['SI', 'Slovinsko'], ['ES', 'Španielsko'],
  ['SE', 'Švédsko'], ['CH', 'Švajčiarsko'], ['NO', 'Nórsko'], ['GB', 'Spojené kráľovstvo'],
  ['RS', 'Srbsko'], ['US', 'Spojené štáty'], ['CN', 'Čína'],
].map(([code, name]) => ({ code, name, label: `${name} (${code})` }))

export const EU = new Set(['AT', 'BE', 'BG', 'HR', 'CY', 'CZ', 'DK', 'EE', 'FI', 'FR', 'DE', 'GR', 'HU', 'IE', 'IT', 'LV', 'LT', 'LU', 'MT', 'NL', 'PL', 'PT', 'RO', 'SK', 'SI', 'ES', 'SE'])

export function countryName(code) {
  return countries.find((c) => c.code === code)?.name ?? code
}

export const supplyKinds = [
  { label: 'Tovar', value: 'goods' },
  { label: 'Služba', value: 'services' },
]

/** Je partner zo zahraničia? (prázdna krajina = Slovensko) */
export const isForeign = (country) => !!country && country !== 'SK'

/**
 * Vysvetlenie, ako sa zahraničný obchod zaradí do priznania.
 * side: 'sale' (vydaná faktúra) | 'purchase' (prijatá faktúra)
 */
export function supplyHint(side, country, kind) {
  if (!isForeign(country) || !kind) return ''
  const eu = EU.has(country)
  if (side === 'sale') {
    if (kind === 'goods') return eu ? 'Dodanie tovaru do EÚ: oslobodené (§ 43), r. 14 a súhrnný výkaz – kód 0. Odberateľ musí mať IČ DPH.' : 'Vývoz tovaru: oslobodené (§ 47), r. 15.'
    return eu ? 'Služba do EÚ: daň platí odberateľ, súhrnný výkaz – kód 2. Odberateľ musí mať IČ DPH.' : 'Služba mimo EÚ: miesto dodania v zahraničí, do priznania sa neuvádza.'
  }
  if (kind === 'goods') return eu ? 'Nadobudnutie tovaru z EÚ: daň priznávate a odpočítavate vy (r. 07–08, KV B.1).' : 'Dovoz tovaru: DPH platená na colnici sa zatiaľ eviduje ručne.'
  return 'Služba zo zahraničia: daň priznávate a odpočítavate vy (§ 69 ods. 3, r. 09–10, KV B.1).'
}

/** Povinný text na faktúre pri plnení bez DPH (prenesenie, oslobodenie). */
export function vatExemptionNote(inv) {
  if (!inv.reverseCharge) return ''
  const country = inv.customer?.country
  if (!isForeign(country)) return 'Prenesenie daňovej povinnosti podľa § 69 ods. 12 zákona o DPH. Daň odvedie odberateľ.'
  const eu = EU.has(country)
  if (inv.supplyKind === 'goods') {
    return eu
      ? 'Dodanie tovaru do iného členského štátu oslobodené od dane podľa § 43 zákona o DPH (čl. 138 smernice 2006/112/ES).'
      : 'Vývoz tovaru oslobodený od dane podľa § 47 zákona o DPH.'
  }
  return eu
    ? 'Prenesenie daňovej povinnosti – daň platí príjemca služby (reverse charge, čl. 196 smernice 2006/112/ES).'
    : 'Miesto dodania služby je mimo územia SR (§ 15 ods. 1 zákona o DPH).'
}

/** Druhy tuzemského prenesenia (§ 69 ods. 12) na výber pri položke. */
export function domesticRCOptions(kinds = codes.domesticRCs) {
  return [{ code: '', label: '— bežná DPH' }, ...kinds.map((k) => ({ code: k.code, label: `${k.letter}) ${k.label}` }))]
}

/** Druh prenesenia s kódom colného sadzobníka v KV (plodiny, kovy). */
export const needsCommodity = (code) => code === 'crops' || code === 'metals'

/** Povinný text pri položkách s tuzemským prenesením — rovnaký ako invoice.DomesticReverseNote v Go. */
export function domesticReverseNote(lines = [], kinds = codes.domesticRCs) {
  const letters = kinds.filter((k) => lines.some((l) => l.reverse === k.code)).map((k) => `${k.letter})`)
  if (!letters.length) return ''
  return `PDP – prenesenie daňovej povinnosti podľa § 69 ods. 12 písm. ${letters.join(', ')} zákona o DPH: daň z týchto položiek odvedie odberateľ.`
}
