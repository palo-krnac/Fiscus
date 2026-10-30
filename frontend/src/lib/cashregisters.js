// Pokladne firmy: výber v pokladni a úprava zoznamu. Zostatky počíta server.

/** Popis pokladne do výberu: „Pokladňa CZK (CZK)“, archivovaná s poznámkou. */
export function registerLabel(r) {
  const cur = r.currency && r.currency !== 'EUR' ? ` (${r.currency})` : ''
  return `${r.name}${cur}${r.archived ? ' – archivovaná' : ''}`
}

/** Pokladne do výberu: aktívne, archivované iba s dokladmi alebo práve zvolená. */
export function registerOptions(list, selected) {
  return list
    .filter((r) => !r.archived || r.documents > 0 || r.id === selected)
    .map((r) => ({ value: r.id, label: registerLabel(r) }))
}

/** Nová pokladňa do zoznamu: záporné ID (server pridelí skutočné). */
export function blankRegister(list) {
  const min = Math.min(0, ...list.map((r) => r.id))
  return { id: min - 1, name: '', currency: 'EUR', prefix: '', archived: false, documents: 0 }
}

/** Menu a predponu pokladne s dokladmi už meniť nemožno. */
export const registerLocked = (r) => r.documents > 0

/** Dáta na uloženie (bez zostatkov). */
export const registersPayload = (list) => list.map(({ id, name, currency, prefix, archived }) => ({ id, name, currency, prefix, archived }))
