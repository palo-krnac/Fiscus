// Pravidelná fakturácia: intervaly, stav šablóny a zástupné texty.

export const intervals = [
  { value: 'monthly', label: 'Mesačne', months: 1 },
  { value: 'quarterly', label: 'Štvrťročne', months: 3 },
  { value: 'halfyear', label: 'Polročne', months: 6 },
  { value: 'yearly', label: 'Ročne', months: 12 },
]

export const intervalLabel = (v) => intervals.find((i) => i.value === v)?.label ?? v ?? ''

/** Zástupné texty, ktoré backend nahradí v popise položky a v poznámke. */
export const placeholders = [
  { token: '{obdobie}', hint: 'október 2026 / 4. štvrťrok 2026 / 2026' },
  { token: '{mesiac}', hint: 'október' },
  { token: '{rok}', hint: '2026' },
  { token: '{minule_obdobie}', hint: 'september 2026' },
]

/** Stav šablóny: aktívna, pozastavená alebo skončená (bez ďalšieho obdobia). */
export function recurringStatusOf(t) {
  if (t.id && !t.nextDate) return { label: 'Skončená', tone: 'cancelled' }
  if (!t.active) return { label: 'Pozastavená', tone: 'draft' }
  return { label: 'Aktívna', tone: 'paid' }
}

/** Prepočet sumy šablóny na mesiac (súčet už vypočítanej sumy / dĺžka intervalu). */
export function monthlyAmount(t) {
  const m = intervals.find((i) => i.value === t.interval)?.months
  return m ? Number(t.summary?.total ?? 0) / m : 0
}

/** Prvý deň nasledujúceho mesiaca (predvolený dátum prvej faktúry), ISO. */
export function firstOfNextMonth(today = new Date()) {
  const d = new Date(Date.UTC(today.getFullYear(), today.getMonth() + 1, 1))
  return d.toISOString().slice(0, 10)
}
