// Spoločný stav aplikácie: edícia, firmy, aktuálna firma.
import { reactive } from 'vue'
import { api } from './api.js'

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
    const [caps, companies] = await Promise.all([api.capabilities(), api.companies()])
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
