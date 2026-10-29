// Pomôcky na zobrazenie auditu: názvy akcií a kanálov, rozdiel pred/po.

export const actionLabels = {
  create: 'vytvorenie',
  email: 'e-mail',
  import: 'import',
  update: 'zmena',
  delete: 'zmazanie',
  issue: 'vystavenie',
  pay: 'úhrada',
  unpay: 'zrušenie úhrady',
  cancel: 'storno',
  login: 'prihlásenie',
  login_failed: 'neúspešné prihlásenie',
  logout: 'odhlásenie',
  password: 'zmena hesla',
  grant: 'pridanie prístupu',
  role: 'zmena roly',
  revoke: 'odobratie prístupu',
  backup: 'záloha',
  restore: 'obnova zo zálohy',
}

export const channelLabels = {
  web: 'Web',
  api: 'API',
  mcp: 'AI (MCP)',
  system: 'Systém',
}

export const actionLabel = (a) => actionLabels[a] ?? a
export const channelLabel = (c) => channelLabels[c] ?? c

/** Akcie, ktoré treba zvýrazniť (bezpečnosť, mazanie). */
export const isAlert = (e) => e.action === 'login_failed' || e.action === 'delete' || e.action === 'revoke'

function flatten(obj, prefix = '', out = {}) {
  if (obj === null || obj === undefined) return out
  if (typeof obj !== 'object' || Array.isArray(obj)) {
    out[prefix] = obj
    return out
  }
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) flatten(v, key, out)
    else out[key] = v
  }
  return out
}

const show = (v) => {
  if (v === undefined || v === null || v === '') return '—'
  if (Array.isArray(v) || typeof v === 'object') return JSON.stringify(v)
  if (v === true) return 'áno'
  if (v === false) return 'nie'
  return String(v)
}

/**
 * Zoznam zmenených polí medzi stavom pred a po.
 * Pri vytvorení (bez „pred“) vráti všetky vyplnené polia.
 */
export function diffFields(before, after) {
  const b = flatten(before)
  const a = flatten(after)
  const keys = [...new Set([...Object.keys(b), ...Object.keys(a)])]
  return keys
    .filter((k) => JSON.stringify(b[k]) !== JSON.stringify(a[k]))
    .filter((k) => before || (a[k] !== '' && a[k] !== null && a[k] !== false && a[k] !== 0))
    .map((k) => ({ field: k, before: show(b[k]), after: show(a[k]) }))
}
