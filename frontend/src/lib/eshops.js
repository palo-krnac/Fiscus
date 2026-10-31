// Napojenie e-shopov: pomocné funkcie pre nastavenie a výsledky načítania.

/** Nové napojenie (Since prázdne = server dá posledných 30 dní). */
export function blankConn(platform = 'woocommerce') {
  return { id: '', platform, name: '', url: '', key: '', secret: '', hasKey: false, hasSecret: false, onlyPaid: true, since: '' }
}

/** Údaje na uloženie: bez stavových polí; prázdny kľúč a tajomstvo server ponechá. */
export function toSave(conns) {
  return conns.map(({ id, platform, name, url, key, secret, onlyPaid, since }) => ({
    id,
    platform,
    name,
    url,
    key: key ?? '',
    secret: secret ?? '',
    onlyPaid: !!onlyPaid,
    since: since || '',
  }))
}

/** Chyby polí jedného obchodu z odpovede servera (conns[i].url → url). */
export function connErrors(fieldMap, index) {
  const prefix = `conns[${index}].`
  const out = {}
  for (const [k, v] of Object.entries(fieldMap ?? {})) if (k.startsWith(prefix)) out[k.slice(prefix.length)] = v
  return out
}

const statuses = {
  created: { label: 'Koncept faktúry', tone: 'paid' },
  waiting: { label: 'Čaká na úhradu', tone: 'draft' },
  skipped: { label: 'Vynechaná', tone: 'cancelled' },
  error: { label: 'Chyba', tone: 'overdue' },
}

/** Popis a farba stavu objednávky po načítaní. */
export function orderStatus(s) {
  return statuses[s] ?? { label: s, tone: 'cancelled' }
}

/** Čas posledného načítania „29. 10. 2026 14:05“ (prázdny = nikdy). */
export function formatWhen(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getDate()}. ${d.getMonth() + 1}. ${d.getFullYear()} ${p(d.getHours())}:${p(d.getMinutes())}`
}

/** Krátke zhrnutie odoslania zásob. */
export function stockSummary(r) {
  const parts = [`Aktualizované: ${r.updated}`]
  if (r.missing?.length) parts.push(`v obchode chýba ${r.missing.length}: ${r.missing.slice(0, 8).join(', ')}${r.missing.length > 8 ? '…' : ''}`)
  if (r.errors?.length) parts.push(`chyby: ${r.errors.length}`)
  return parts.join(' · ')
}
