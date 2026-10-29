// Spoločný stav aplikácie: edícia, prihlásený používateľ, firmy, aktuálna firma.
import { reactive } from 'vue'
import { api, setUnauthorizedHandler } from './api.js'
import { loadCodes } from './codes.js'
import { hasPerm, roleFor } from './access.js'

const KEY = 'fiskus.companyId'

export const session = reactive({
  ready: false,
  error: null,
  caps: { mode: 'desktop', multiCompany: false },
  companies: [],
  companyId: null,
  me: null, // odpoveď /api/me
  needLogin: false, // serverová verzia bez platnej relácie
  logoVersion: 0, // zvýši sa po zmene loga (obnoví obrázok bez cache)

  get company() {
    return this.companies.find((c) => c.id === this.companyId) ?? null
  },
})

/** Je prihlásený správca inštalácie (v desktopovej verzii vždy)? */
export function isSuperAdmin() {
  return !!session.me?.user?.superAdmin
}

/** Má prihlásený používateľ v aktuálnej firme právo? */
export function can(perm) {
  return hasPerm(session.me, session.companyId, perm)
}

export function currentRole() {
  return roleFor(session.me, session.companyId)
}

setUnauthorizedHandler(() => {
  session.needLogin = true
  session.me = null
})

export async function loadSession() {
  try {
    const [caps] = await Promise.all([api.capabilities(), loadCodes()])
    session.caps = caps
    let me
    try {
      me = await api.me()
    } catch (e) {
      if (e.status === 401) {
        session.needLogin = true
        session.me = null
        return
      }
      throw e
    }
    const companies = await api.companies()
    session.me = me
    session.needLogin = false
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

export async function logout() {
  try {
    await api.logout()
  } finally {
    session.me = null
    session.companies = []
    session.companyId = null
    session.needLogin = true
  }
}
