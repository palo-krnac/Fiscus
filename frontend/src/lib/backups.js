// Nastavenie záloh vo formulári: kontrola zopakovaného hesla a upozornenia
// na následky zmeny (pravidlá hesla overuje server).

/** Chyba zopakovaného hesla alebo '' . */
export function confirmError(password, confirm) {
  if (!password) return ''
  return password === confirm ? '' : 'Heslá sa nezhodujú'
}

/** Upozornenia pred uložením: staršie zálohy pri zmene hesla alebo priečinka. */
export function changeNotes({ count, passwordSet, dirChanged, passwordChanged }) {
  const notes = []
  if (count > 0 && passwordSet && passwordChanged) notes.push('Staršie zálohy ostanú zašifrované pôvodným heslom — uschovajte si ho.')
  if (count > 0 && dirChanged) notes.push('Staršie zálohy ostanú v pôvodnom priečinku; zoznam ukáže iba zálohy v novom.')
  return notes
}

/** Krátky názov cloudového priečinka pre tlačidlo („OneDrive“, „Dropbox“…). */
export function cloudLabel(path) {
  const parts = String(path).split(/[\\/]/).filter(Boolean)
  const i = parts.findIndex((p) => /^(OneDrive|Dropbox|Google Drive|iCloudDrive|com~apple~CloudDocs)/.test(p))
  if (i < 0) return parts.at(-2) ?? path
  const p = parts[i]
  if (p === 'com~apple~CloudDocs' || p === 'iCloudDrive') return 'iCloud'
  if (p === 'Google Drive') return 'Google Disk'
  return p
}

// Kópia záloh v cloude (WebDAV, S3). Prístupové údaje overuje server skúšobným
// súborom; tajný údaj sa do prehliadača nikdy nevracia.

/** Poskytovatelia S3 s adresou podľa regiónu (null = adresu zadá používateľ). */
export const s3Providers = [
  { id: 'aws', label: 'Amazon S3', region: 'eu-central-1', url: (r) => `https://s3.${r}.amazonaws.com` },
  { id: 'wasabi', label: 'Wasabi', region: 'eu-central-1', url: (r) => `https://s3.${r}.wasabisys.com` },
  { id: 'b2', label: 'Backblaze B2', region: 'eu-central-003', url: (r) => `https://s3.${r}.backblazeb2.com` },
  { id: 'hetzner', label: 'Hetzner', region: 'fsn1', url: (r) => `https://${r}.your-objectstorage.com` },
  { id: 'other', label: 'Iný (MinIO, R2…)', region: '', url: null },
]

/** Adresa S3 pre poskytovateľa a región; '' ak ju treba zadať ručne. */
export function s3Endpoint(providerId, region) {
  const p = s3Providers.find((x) => x.id === providerId)
  const r = String(region ?? '').trim()
  return p?.url && r ? p.url(r) : ''
}

/** Rozpozná poskytovateľa podľa uloženej adresy. */
export function s3ProviderOf(url, region) {
  for (const p of s3Providers) if (p.url && region && p.url(region) === url) return p.id
  return url ? 'other' : 'aws'
}

/** Krátky opis cieľa bez prístupových údajov. */
export function remoteTarget(r) {
  if (!r?.kind) return ''
  let host = r.url
  try {
    const u = new URL(r.url)
    host = r.kind === 'webdav' ? u.host + decodeURI(u.pathname).replace(/\/$/, '') : u.host
  } catch {
    /* ponechať, ako je */
  }
  if (r.kind === 's3') return `S3 ${host} · ${r.bucket}${r.prefix ? '/' + r.prefix : ''}`
  return `WebDAV ${host}`
}

const fmt = new Intl.DateTimeFormat('sk-SK', { dateStyle: 'medium', timeStyle: 'short' })

/** Stav poslednej kópie: { tone: 'ok'|'error'|'wait', text }. */
export function remoteStateText(state) {
  const ok = state?.lastOk ? new Date(state.lastOk) : null
  const bad = state?.errorAt ? new Date(state.errorAt) : null
  if (bad && (!ok || bad > ok)) {
    return { tone: 'error', text: `Posledná kópia ${fmt.format(bad)} zlyhala: ${state.lastError}` }
  }
  if (ok) {
    const n = state.count
    const word = n === 1 ? 'záloha' : n >= 2 && n <= 4 ? 'zálohy' : 'záloh'
    return { tone: 'ok', text: `Posledná kópia ${fmt.format(ok)} · v cloude ${n} ${word}` }
  }
  return { tone: 'wait', text: 'Kópia sa vytvorí pri najbližšej zálohe.' }
}
