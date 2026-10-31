// Pomocné funkcie pre adresár (bez závislosti na Vue — dajú sa testovať).
import { matches, searchTerms } from './search.js'

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

/** Filtruje kontakty podľa názvu, IČO, DIČ, IČ DPH, adresy, e-mailu a telefónu. */
export function filterContacts(list, query) {
  const terms = searchTerms(query)
  if (!terms) return list
  return list.filter((c) =>
    matches(terms, [
      c.name, c.ico, c.dic, c.icdph, c.street, c.city, c.zip, c.email, c.phone, c.note,
      ...(c.persons ?? []).flatMap((p) => [p.name, p.role, p.email, p.phone]),
      ...(c.addresses ?? []).flatMap((a) => [a.name, a.street, a.city]),
    ]),
  )
}

/** Druhy ďalších adries kontaktu. */
export const addressKinds = [
  { value: 'delivery', label: 'Dodacia' },
  { value: 'postal', label: 'Korešpondenčná' },
  { value: 'branch', label: 'Prevádzka' },
  { value: 'other', label: 'Iná' },
]
export const addressKindLabel = (k) => addressKinds.find((x) => x.value === k)?.label ?? k

export const blankPerson = () => ({ name: '', role: '', email: '', phone: '', note: '', primary: false })
export const blankAddress = (kind = 'delivery') => ({ kind, name: '', street: '', city: '', zip: '', country: 'SK', note: '', default: false })

/** Označí jednu predvolenú položku (osobu alebo adresu daného druhu). */
export function setPrimary(list, index, key = 'primary') {
  const kind = list[index]?.kind
  return list.map((x, i) => (key === 'default' && x.kind !== kind ? x : { ...x, [key]: i === index ? !x[key] : false }))
}

/** Osoby ako text („Peter Kováč (konateľ), Eva Malá“) — do zoznamu a exportu. */
export function personsText(c) {
  return (c.persons ?? []).map((p) => (p.role ? `${p.name} (${p.role})` : p.name)).join(', ')
}

/** Jednoriadková adresa („Sklad, Priemyselná 1, 94901 Nitra“). */
export function addressText(a) {
  return [a.name, a.street, [a.zip, a.city].filter(Boolean).join(' ')].filter(Boolean).join(', ')
}

/** Predvolená dodacia adresa kontaktu ako adresa na doklad (null, ak nie je). */
export function defaultDelivery(c) {
  const a = (c?.addresses ?? []).find((x) => x.kind === 'delivery' && x.default)
  return a ? { name: a.name, street: a.street, city: a.city, zip: a.zip, country: a.country || 'SK' } : null
}

/** Predvolená kontaktná osoba na doklad (null, ak nie je). */
export function defaultPerson(c) {
  const p = (c?.persons ?? []).find((x) => x.primary)
  return p ? { name: p.name, email: p.email, phone: p.phone } : null
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
