// Platobný príkaz z prijatých faktúr: predvýber a súčet vybraných.
// O tom, čo sa do príkazu smie zaradiť, rozhoduje server (pole problem).

/** Predvolene vybrané: bez problému a splatné najneskôr o `days` dní. */
export function defaultSelection(cands, todayISO, days = 7) {
  const limit = new Date(todayISO + 'T00:00:00Z')
  limit.setUTCDate(limit.getUTCDate() + days)
  const lim = limit.toISOString().slice(0, 10)
  return cands.filter((c) => !c.problem && (!c.dueDate || c.dueDate <= lim)).map((c) => c.id)
}

/** Súčet súm vybraných faktúr (sumy sú hotové zo servera). */
export function selectedTotal(cands, ids) {
  const set = new Set(ids)
  return cands.filter((c) => set.has(c.id)).reduce((a, c) => a + Number(c.amount), 0)
}
