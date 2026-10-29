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
