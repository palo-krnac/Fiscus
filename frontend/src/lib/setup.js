// Úvodné nastavenie firmy (sprievodca) a druh účtovníctva.

export const bookkeepingOptions = [
  {
    value: 'simple',
    label: 'Jednoduché účtovníctvo',
    who: 'Živnostník, ktorý uplatňuje skutočné výdavky alebo vedie jednoduché účtovníctvo',
    what: 'Peňažný denník, výkazy o príjmoch a výdavkoch a o majetku a záväzkoch',
  },
  {
    value: 'double',
    label: 'Podvojné účtovníctvo',
    who: 's.r.o., a.s. a ďalšie právnické osoby, živnostník nad hranicou obratu',
    what: 'Účtový rozvrh, účtovný denník a hlavná kniha, súvaha a výkaz ziskov a strát',
  },
]

/** Popis druhu účtovníctva ('' = nezvolené). */
export function bookkeepingLabel(v) {
  return bookkeepingOptions.find((o) => o.value === v)?.label ?? 'Nezvolené (zobrazuje sa oboje)'
}

/** Patrí položka menu k zvolenému účtovníctvu? Nezvolené = všetko. */
export function bookAllows(item, bookkeeping) {
  return !item.book || !bookkeeping || item.book === bookkeeping
}

export const setupSteps = ['Firma', 'DPH', 'Účtovníctvo', 'Začiatok']

/** Chyby kroku sprievodcu (prázdny objekt = dá sa pokračovať). */
export function stepErrors(step, f) {
  const e = {}
  if (step === 0 && !String(f.name ?? '').trim()) e.name = 'Zadajte názov firmy alebo meno živnostníka'
  if (step === 1 && f.vatPayer && !/^SK\d{10}$/.test(String(f.icdph ?? '').replace(/\s/g, '').toUpperCase())) e.icdph = 'IČ DPH je SK a 10 číslic'
  if (step === 2 && !f.bookkeeping) e.bookkeeping = 'Vyberte druh účtovníctva'
  return e
}

/** Kam ísť po dokončení sprievodcu. */
export function afterSetup(start) {
  return start === 'migrate' ? '/prenos' : '/faktury'
}

/** Text o prenose dokladov z iného programu (null = ešte nebol). */
export function migrationNote(company) {
  if (!company?.migratedAt) return null
  const d = new Date(company.migratedAt)
  const when = Number.isNaN(d.getTime()) ? company.migratedAt : `${d.getDate()}. ${d.getMonth() + 1}. ${d.getFullYear()}`
  return `Doklady sa už preniesli z programu ${company.migratedFrom} (${when}). Ďalší prenos dokladov nie je možný — adresár a cenník sa prenášajú kedykoľvek.`
}
