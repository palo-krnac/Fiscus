// Koeficient pomerného odpočítania (§ 50) — iba zobrazenie, počíta server.

const months = ['januári', 'februári', 'marci', 'apríli', 'máji', 'júni', 'júli', 'auguste', 'septembri', 'októbri', 'novembri', 'decembri']

/** Obdobie vysporiadania: "2026-12" → „decembri 2026“, "2026-Q4" → „4. štvrťroku 2026“ (po predložke v). */
export function coefPeriodLabel(p) {
  const [y, rest] = String(p ?? '').split('-')
  if (!rest) return ''
  if (rest.startsWith('Q')) return `${rest.slice(1)}. štvrťroku ${y}`
  return `${months[Number(rest) - 1]} ${y}`
}

/** Rok, ktorého koeficient sa týka obdobia "2026-10" / "2026-Q3". */
export const coefYear = (period) => Number(String(period ?? '').slice(0, 4)) || new Date().getFullYear()
