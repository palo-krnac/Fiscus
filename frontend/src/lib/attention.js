// Počty vecí, ktoré čakajú na používateľa (odznak v menu pri položke).
import { reactive } from 'vue'

export const attention = reactive({})

/** Text odznaku: nič pri nule, „99+“ pri veľkom počte. */
export function badgeText(n) {
  if (!n || n < 0) return ''
  return n > 99 ? '99+' : String(n)
}

/** Obnoví počet konceptov z pravidelnej fakturácie (chyba odznak iba skryje). */
export async function refreshDrafts(load) {
  try {
    attention['/pravidelne'] = (await load()).length
  } catch {
    attention['/pravidelne'] = 0
  }
}

/** Obnoví počet termínov do 7 dní a zmeškaných (kalendár). */
export async function refreshCalendar(load, count) {
  try {
    attention['/kalendar'] = count(await load())
  } catch {
    attention['/kalendar'] = 0
  }
}
