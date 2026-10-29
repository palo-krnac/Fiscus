// E-maily: druhy dokladov, stav odoslania, hľadanie a nastavenie servera SMTP.
import { fold, matches, searchTerms } from './search.js'

export const docKinds = {
  invoice: { label: 'Faktúra', url: 'invoices' },
  quote: { label: 'Cenová ponuka', url: 'quotes' },
  reminder: { label: 'Upomienka', url: 'reminders' },
  test: { label: 'Skúšobná správa', url: '' },
  notice: { label: 'Upozornenie', url: '' },
}

export const docKindLabel = (k) => docKinds[k]?.label ?? k

/** Cesta k dokladu v programe (null pri skúšobnej správe). */
export function docRoute(e) {
  switch (e.docKind) {
    case 'invoice':
      return `/faktury/${e.docId}`
    case 'quote':
      return `/ponuky/${e.docId}`
    case 'reminder':
      return `/upomienky/${e.docId}/tlac`
    case 'notice':
      return '/pravidelne'
  }
  return null
}

export const emailStatus = (e) => (e.error ? { label: 'Neodoslaný', tone: 'overdue' } : { label: 'Odoslaný', tone: 'paid' })

export const emailViews = [
  { label: 'Všetky', value: 'all' },
  { label: 'Neodoslané', value: 'failed' },
]

/** Filter zoznamu podľa zobrazenia a hľadania (bez diakritiky, viac slov). */
export function filterEmails(rows, view, search) {
  const terms = searchTerms(search)
  return rows.filter(
    (e) =>
      (view !== 'failed' || !!e.error) &&
      matches(terms, [e.docNumber, docKindLabel(e.docKind), e.to, e.cc, e.subject, e.file, e.user, e.error, emailStatus(e).label]),
  )
}

export const securityOptions = [
  { label: 'STARTTLS', value: 'starttls', port: 587 },
  { label: 'SSL/TLS', value: 'tls', port: 465 },
  { label: 'Bez šifrovania', value: 'none', port: 25 },
]

/** Predvolený port pre zabezpečenie. */
export const portFor = (security) => securityOptions.find((o) => o.value === security)?.port ?? 587

/** Známe poskytovatelia — predvyplnenie servera podľa domény odosielateľa. */
const providers = [
  { match: ['gmail.com', 'googlemail.com'], host: 'smtp.gmail.com', security: 'starttls', note: 'Gmail vyžaduje heslo aplikácie (Účet Google → Zabezpečenie → Heslá aplikácií).' },
  { match: ['outlook.com', 'hotmail.com', 'live.com', 'outlook.sk'], host: 'smtp-mail.outlook.com', security: 'starttls', note: '' },
  { match: ['azet.sk', 'atlas.sk', 'centrum.sk'], host: 'smtp.azet.sk', security: 'tls', note: '' },
  { match: ['zoznam.sk'], host: 'smtp.zoznam.sk', security: 'tls', note: '' },
  { match: ['seznam.cz', 'email.cz'], host: 'smtp.seznam.cz', security: 'tls', note: '' },
]

/** Návrh servera podľa e-mailu odosielateľa (null = neznámy poskytovateľ). */
export function guessProvider(email) {
  const domain = fold(String(email ?? '').split('@')[1] ?? '').trim()
  if (!domain) return null
  const p = providers.find((x) => x.match.includes(domain))
  return p ? { host: p.host, security: p.security, port: portFor(p.security), note: p.note } : null
}

/** Zástupné znaky v šablónach (pomoc v nastavení). */
export const placeholders = [
  ['{cislo}', 'číslo dokladu'],
  ['{suma}', 'suma s menou'],
  ['{splatnost}', 'splatnosť / lehota úhrady'],
  ['{platba}', 'veta s IBAN a VS (faktúra)'],
  ['{platna_do}', 'platnosť ponuky'],
  ['{Doklad}', 'názov dokladu (Faktúra, Dobropis…)'],
  ['{doklad}', 'názov vo 4. páde (faktúru…)'],
  ['{odberatel}', 'odberateľ'],
  ['{firma}', 'vaša firma'],
  ['{iban}', 'váš IBAN'],
  ['{vs}', 'variabilný symbol'],
  ['{datum}', 'dátum dokladu'],
]

export const templateKinds = [
  { kind: 'invoice', label: 'Faktúry' },
  { kind: 'quote', label: 'Cenové ponuky' },
  { kind: 'reminder', label: 'Upomienky' },
]
