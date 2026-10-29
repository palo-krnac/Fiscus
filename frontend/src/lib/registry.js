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
 * Prepíše názov a adresu; DIČ doplní, ak ho register pozná (Register
 * účtovných závierok), IČ DPH, ak VIES potvrdí platiteľa DPH. E-mail
 * a ďalšie polia nechá tak.
 */
export function fillFromRegistry(target, company) {
  return {
    ...target,
    ico: company.ico,
    name: company.name || target.name,
    street: company.street,
    zip: company.zip,
    city: company.city,
    ...('dic' in target || company.dic ? { dic: company.dic || target.dic || '' } : {}),
    ...('icdph' in target || company.icdph ? { icdph: company.icdph || target.icdph || '' } : {}),
    ...('country' in target || company.country !== 'SK' ? { country: company.country || 'SK' } : {}),
  }
}

/**
 * Doplní údaje z VIES k zadanému IČ DPH. Pri zahraničnom partnerovi prepíše
 * názov, adresu a krajinu; pri slovenskom iba doplní chýbajúci názov
 * (adresa je presnejšia z RPO).
 */
export function fillFromVIES(target, check) {
  if (!check?.valid) return target
  const out = { ...target, icdph: check.vatId }
  if (check.country !== 'SK') {
    Object.assign(out, {
      name: check.name || target.name,
      street: check.street || target.street,
      zip: check.zip || target.zip,
      city: check.city || target.city,
    })
    if ('country' in target) out.country = check.country
  } else if (!target.name && check.name) {
    out.name = check.name
  }
  return out
}
