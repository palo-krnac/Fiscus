// Bloky dodávateľa a odberateľa na tlači dokladu (rovnako ako PDF zo servera,
// internal/printout): adresa, identifikátory a kontakt pod sebou s popismi,
// kontaktná osoba a dodacia adresa odberateľa.

const nonEmpty = (pairs) => pairs.filter(([, v]) => v && String(v).trim())

export const addressLines = (p) => [p?.street, [p?.zip, p?.city].filter(Boolean).join(' ')].filter(Boolean)

/** Skupiny [popis, hodnota] dodávateľa: identifikátory, kontakt. */
export function supplierGroups(c) {
  return [
    nonEmpty([['IČO', c?.ico], ['DIČ', c?.dic], ['IČ DPH', c?.icdph]]),
    nonEmpty([['Tel.', c?.phone], ['E-mail', c?.email]]),
  ].filter((g) => g.length)
}

/** Skupiny odberateľa: identifikátory, kontaktná osoba alebo e-mail. */
export function customerGroups(inv) {
  const c = inv?.customer ?? {}
  const p = inv?.attention
  const contact = p ? [['Kontakt', p.name], ['Tel.', p.phone], ['E-mail', p.email || c.email]] : [['E-mail', c.email]]
  return [nonEmpty([['IČO', c.ico], ['DIČ', c.dic], ['IČ DPH', c.icdph]]), nonEmpty(contact)].filter((g) => g.length)
}

/** Riadky dodacej adresy (prázdne pole, ak nie je). */
export function deliveryLines(inv, countryName = (x) => x) {
  const d = inv?.delivery
  if (!d || ![d.name, d.street, d.city, d.zip].some((x) => x && x.trim())) return []
  const out = [d.name, ...addressLines(d)].filter(Boolean)
  if (d.country && d.country !== 'SK') out.push(countryName(d.country))
  return out
}
