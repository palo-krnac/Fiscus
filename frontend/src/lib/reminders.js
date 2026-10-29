// Upomienky: názvy stupňov, texty listu a právne odkazy.
import { formatDate } from './format.js'

export const levelTitle = (level) =>
  ({ 1: 'Upomienka', 2: '2. upomienka', 3: 'Posledná upomienka pred vymáhaním' })[level] ?? 'Upomienka'

export const levelShort = (level) => ({ 1: '1. upomienka', 2: '2. upomienka', 3: 'posledná' })[level] ?? `${level}.`

/** Úvodný text listu podľa stupňa. */
export function introText(r) {
  const by = formatDate(r.payBy)
  switch (r.level) {
    case 2:
      return `Napriek predchádzajúcej upomienke stále evidujeme neuhradené faktúry uvedené nižšie. Žiadame Vás o ich úhradu najneskôr do ${by}.`
    case 3:
      return `Toto je posledná upomienka. Ak dlžnú sumu neuhradíte najneskôr do ${by}, budeme nútení uplatniť pohľadávku súdnou cestou alebo prostredníctvom inkasnej spoločnosti, čo Vám spôsobí ďalšie náklady.`
    default:
      return `Dovoľujeme si Vás upozorniť, že podľa našej evidencie nemáme uhradené faktúry uvedené nižšie, ktorých splatnosť už uplynula. Prosíme Vás o ich úhradu najneskôr do ${by}. Ak ste ich medzičasom uhradili, považujte túto upomienku za bezpredmetnú.`
  }
}

/** Poznámka o úroku z omeškania a paušálnej náhrade (právny základ). */
export function legalNotes(r) {
  const out = []
  if (r.interestMode === 'statutory' && Number(r.interestTotal) > 0) {
    out.push(
      r.business
        ? 'Úrok z omeškania podľa § 369 Obchodného zákonníka a nariadenia vlády č. 21/2013 Z. z. (základná úroková sadzba ECB platná k prvému dňu omeškania zvýšená o 9 percentuálnych bodov), vypočítaný ku dňu vystavenia upomienky.'
        : 'Úrok z omeškania podľa § 517 Občianskeho zákonníka a nariadenia vlády č. 87/1995 Z. z. (základná úroková sadzba ECB platná k prvému dňu omeškania zvýšená o 5 percentuálnych bodov), vypočítaný ku dňu vystavenia upomienky.',
    )
  } else if (r.interestMode === 'custom' && Number(r.interestTotal) > 0) {
    out.push(`Úrok z omeškania vo výške ${String(r.customRate).replace('.', ',')} % ročne, vypočítaný ku dňu vystavenia upomienky.`)
  }
  if (Number(r.fees) > 0) {
    out.push('Paušálna náhrada nákladov spojených s uplatnením pohľadávky podľa § 369c Obchodného zákonníka (40 € za každú faktúru).')
  }
  if (Number(r.interestTotal) > 0) {
    out.push('Úrok z omeškania plynie až do zaplatenia; uvedená suma je k dátumu upomienky.')
  }
  return out
}

export const interestModes = [
  { value: 'statutory', label: 'Zákonný úrok' },
  { value: 'custom', label: 'Dohodnutá sadzba' },
  { value: 'none', label: 'Bez úroku' },
]
