// E-faktúry: možnosti pripojenia k poštárovi a stav odoslania.
import { formatDate } from './format.js'

/** Spôsoby pripojenia. Priečinky na serveri iba pre správcu inštalácie. */
export function postmanModes(mode, superAdmin) {
  const out = [{ label: 'Vypnuté', value: '' }]
  if (mode !== 'server' || superAdmin) out.push({ label: 'Priečinky', value: 'folder' })
  out.push({ label: 'API poskytovateľa', value: 'http' })
  return out
}

/** Text o poslednom odoslaní e-faktúry („Odoslaná 29. 10. 2026“). */
export function sentText(list = []) {
  const last = list.find((e) => !e.error) ?? list[0]
  if (!last) return ''
  const day = formatDate(String(last.created).slice(0, 10))
  return last.error ? `E-faktúra neodoslaná ${day}: ${last.error}` : `E-faktúra odoslaná ${day}${last.peppolId ? ` na ${last.peppolId}` : ''}`
}
