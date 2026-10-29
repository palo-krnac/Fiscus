// Fulltextové hľadanie v zoznamoch: bez ohľadu na veľkosť písmen a diakritiku,
// každé slovo hľadaného výrazu sa musí niekde nájsť („nitra 2026“).

/** Text bez diakritiky a malými písmenami. */
export function fold(s) {
  return String(s ?? '')
    .normalize('NFD')
    .replace(/\p{M}/gu, '')
    .toLowerCase()
}

/** Pripraví hľadaný výraz (null = nehľadá sa). */
export function searchTerms(q) {
  const words = fold(q).split(/\s+/).filter(Boolean)
  return words.length ? words : null
}

/** Vyhovuje záznam hľadaniu? fields sú hodnoty, v ktorých sa hľadá. */
export function matches(terms, fields) {
  if (!terms) return true
  const hay = fold(fields.filter((f) => f !== undefined && f !== null && f !== '').join(' '))
  // sumy aj v zápise s čiarkou: 1234.50 → nájde „1234,5“
  const alt = hay.replace(/(\d)\.(\d)/g, '$1,$2')
  return terms.every((t) => hay.includes(t) || alt.includes(t))
}
