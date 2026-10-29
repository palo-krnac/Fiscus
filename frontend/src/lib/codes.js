// Číselníky. Systémové (sadzby DPH, spôsoby úhrady…) prichádzajú z API
// /api/codes; zoznam krajín je statický (ISO 3166-1).
import { reactive } from 'vue'
import { api } from './api.js'

export const codes = reactive({
  vatPeriods: [],
  paymentMethods: [],
  defaultUnits: [],
  defaultFormats: {},
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
