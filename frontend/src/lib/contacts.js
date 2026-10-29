// Pomocné funkcie pre adresár (bez závislosti na Vue — dajú sa testovať).

const FOLD = {
  á: 'a', ä: 'a', č: 'c', ď: 'd', é: 'e', ě: 'e', í: 'i', ĺ: 'l', ľ: 'l', ň: 'n',
  ó: 'o', ô: 'o', ö: 'o', ŕ: 'r', ř: 'r', š: 's', ť: 't', ú: 'u', ů: 'u', ü: 'u', ý: 'y', ž: 'z',
}

/** "Kováč" → "kovac" — rovnaké pravidlá ako store.SearchKey v Go. */
export function searchKey(s) {
  return String(s ?? '')
    .toLowerCase()
    .replace(/[^\x00-\x7f]/g, (ch) => FOLD[ch] ?? ch)
    .split(/\s+/)
    .filter(Boolean)
    .join(' ')
}

/** Filtruje kontakty podľa názvu, IČO a obce. */
export function filterContacts(list, query) {
  const q = searchKey(query)
  if (!q) return list
  return list.filter((c) => searchKey(`${c.name} ${c.ico} ${c.city}`).includes(q))
}

/** Kontakt z adresára → odberateľ na faktúre (kópia údajov). */
export function customerFromContact(c) {
  return {
    name: c.name, street: c.street, city: c.city, zip: c.zip, country: c.country || 'SK',
    ico: c.ico, dic: c.dic, icdph: c.icdph, email: c.email,
  }
}

/** Odberateľ z faktúry → nový kontakt do adresára. */
export function contactFromCustomer(p) {
  return {
    name: p.name ?? '', street: p.street ?? '', city: p.city ?? '', zip: p.zip ?? '', country: p.country || 'SK',
    ico: p.ico ?? '', dic: p.dic ?? '', icdph: p.icdph ?? '', email: p.email ?? '',
    phone: '', iban: '', dueDays: 0, note: '',
  }
}

/** Splatnosť v dňoch pre faktúru: kontakt má prednosť pred firmou. */
export function dueDaysFor(contact, company) {
  return contact?.dueDays || company?.defaultDueDays || 14
}
