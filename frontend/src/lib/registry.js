// Vyhľadanie firmy v registri (RPO) — pomocné funkcie bez závislosti na Vue.

/** IČO: 8 číslic (staré 6-miestne sa doplnia nulami), kontrolná číslica mod 11. */
export function normalizeICO(s) {
  let v = String(s ?? '').replace(/\s/g, '')
  if (/^\d{6}$/.test(v)) v = '00' + v
  return v
}

export function validICO(s) {
  const v = normalizeICO(s)
  if (!/^\d{8}$/.test(v)) return false
  let sum = 0
  for (let i = 0; i < 7; i++) sum += Number(v[i]) * (8 - i)
  return Number(v[7]) === ((11 - (sum % 11)) % 10)
}

/**
 * Doplní údaje z registra do formulára (kontakt, odberateľ, firma).
 * Prepíše názov a adresu; DIČ, IČ DPH, e-mail a ďalšie polia nechá tak,
 * lebo RPO ich neobsahuje.
 */
export function fillFromRegistry(target, company) {
  return {
    ...target,
    ico: company.ico,
    name: company.name || target.name,
    street: company.street,
    zip: company.zip,
    city: company.city,
    ...('country' in target || company.country !== 'SK' ? { country: company.country || 'SK' } : {}),
  }
}
