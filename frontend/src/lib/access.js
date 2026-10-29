// Práva prihláseného používateľa. Backend práva vždy kontroluje sám,
// rozhranie podľa nich iba skrýva akcie, ktoré by aj tak odmietol.

/** Práva používateľa vo firme podľa odpovede /api/me. */
export function permsFor(me, companyId) {
  if (!me) return []
  return me.access?.find((a) => a.companyId === companyId)?.perms ?? []
}

/** Rola používateľa vo firme ('' = nemá prístup). */
export function roleFor(me, companyId) {
  return me?.access?.find((a) => a.companyId === companyId)?.role ?? ''
}

/** Môže používateľ vykonať akciu? Prázdne právo = stačí prístup do firmy. */
export function hasPerm(me, companyId, perm) {
  if (!perm) return true
  return permsFor(me, companyId).includes(perm)
}

/** Odfiltruje položky menu, na ktoré používateľ nemá právo. Prázdne skupiny vynechá. */
export function visibleModules(modules, allowed) {
  return modules
    .map((g) => ({ ...g, items: g.items.filter((i) => allowed(i)) }))
    .filter((g) => g.items.length > 0)
}
