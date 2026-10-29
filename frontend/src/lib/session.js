// Spoločný stav aplikácie: edícia, firmy, aktuálna firma.
import { reactive } from 'vue'
import { api } from './api.js'
import { loadCodes } from './codes.js'

const KEY = 'fiskus.companyId'

export const session = reactive({
  ready: false,
  error: null,
  caps: { mode: 'desktop', multiCompany: false },
  companies: [],
  companyId: null,

  get company() {
    return this.companies.find((c) => c.id === this.companyId) ?? null
  },
})

export async function loadSession() {
  try {
    const [caps, companies] = await Promise.all([api.capabilities(), api.companies(), loadCodes()])
    session.caps = caps
    session.companies = companies
    let saved = null
    try {
      saved = Number(localStorage.getItem(KEY))
    } catch {
      /* úložisko prehliadača nemusí byť dostupné */
    }
    session.companyId = companies.some((c) => c.id === saved) ? saved : (companies[0]?.id ?? null)
    session.error = null
  } catch (e) {
    session.error = e.message
  } finally {
    session.ready = true
  }
}

export function selectCompany(id) {
  session.companyId = id
  try {
    localStorage.setItem(KEY, String(id))
  } catch {
    /* ignorovať */
  }
}

/** Po uložení nastavení nahradí firmu v zozname. */
export function replaceCompany(c) {
  const i = session.companies.findIndex((x) => x.id === c.id)
  if (i >= 0) session.companies.splice(i, 1, c)
}
