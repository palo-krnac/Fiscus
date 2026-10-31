// Stĺpce exportov jednotlivých zoznamov (pozri lib/export.js).
import { codes, countryName, paymentLabel } from './codes.js'
import { purchaseStatusOf, statusOf } from './format.js'
import { actionLabel, channelLabel } from './audit.js'
import { quoteStatusOf } from './quotes.js'
import { intervalLabel, recurringStatusOf } from './recurring.js'
import { levelShort } from './reminders.js'
import { kindLabel } from './advance.js'
import { assetStatus, disposalOptions, periodLabel } from './assets.js'
import { tripTime } from './logbook.js'
import { kindLabel as calendarKind, repeatLabel, stateOf } from './calendar.js'
import { docKindLabel, emailStatus } from './emails.js'
import { eurBalance, eurSummary, isForeignCurrency, purchaseEUR, sourceLabel } from './fx.js'

const t = (header, value, width) => ({ header, value, width })
const money = (header, value) => ({ header, value, type: 'money', width: 14 })
const date = (header, value) => ({ header, value, type: 'date', width: 12 })
const yes = (b) => (b ? 'áno' : '')

export const invoiceColumns = [
  t('Číslo', (r) => r.number || 'koncept', 14),
  t('Druh', (r) => kindLabel(r.kind), 16),
  t('Odberateľ', (r) => r.customer?.name, 32),
  t('Predmet', (r) => r.subject, 30),
  t('IČO', (r) => r.customer?.ico, 11),
  t('DIČ', (r) => r.customer?.dic, 12),
  t('IČ DPH', (r) => r.customer?.icdph, 14),
  t('Obec', (r) => r.customer?.city, 16),
  t('Krajina', (r) => countryName(r.customer?.country || 'SK'), 12),
  t('Variabilný symbol', (r) => r.variableSymbol, 12),
  date('Vystavená', (r) => r.issueDate),
  date('Dodanie', (r) => r.deliveryDate),
  date('Splatná', (r) => r.dueDate),
  date('Uhradená', (r) => r.paidDate),
  t('Úhrada', (r) => paymentLabel(r.paymentMethod), 16),
  money('Základ', (r) => r.summary?.base),
  money('DPH', (r) => r.summary?.vat),
  money('Spolu', (r) => r.summary?.total),
  t('Mena', (r) => r.currency || 'EUR', 6),
  { header: 'Kurz', value: (r) => (isForeignCurrency(r.currency) ? r.fxRate : ''), type: 'number', width: 10 },
  money('Základ €', (r) => eurSummary(r)?.base),
  money('DPH €', (r) => eurSummary(r)?.vat),
  money('Spolu €', (r) => eurSummary(r)?.total),
  money('Uhradené', (r) => r.paidAmount),
  money('Zostáva €', (r) => (r.status === 'issued' ? eurBalance(r) : '0')),
  t('Prenesenie DPH', (r) => yes(r.reverseCharge), 8),
  t('Stav', (r) => statusOf(r).label, 18),
  t('Poznámka', (r) => r.note, 30),
]

export const purchaseColumns = [
  t('Číslo', (r) => r.number, 12),
  t('Číslo dodávateľa', (r) => r.supplierNumber, 16),
  t('Dodávateľ', (r) => r.supplier?.name, 32),
  t('IČO', (r) => r.supplier?.ico, 11),
  t('IČ DPH', (r) => r.supplier?.icdph, 14),
  t('Variabilný symbol', (r) => r.variableSymbol, 12),
  t('Predmet', (r) => r.description, 28),
  date('Vyhotovená', (r) => r.issueDate),
  date('Dodanie', (r) => r.deliveryDate),
  date('Doručená', (r) => r.receivedDate),
  date('Splatná', (r) => r.dueDate),
  date('Uhradená', (r) => r.paidDate),
  t('Mena', (r) => r.currency || 'EUR', 6),
  { header: 'Kurz', value: (r) => (isForeignCurrency(r.currency) ? r.fxRate : ''), type: 'number', width: 10 },
  money('Spolu v mene', (r) => r.summary?.total),
  money('Základ €', (r) => purchaseEUR(r)?.base),
  money('DPH €', (r) => purchaseEUR(r)?.vat),
  money('Spolu €', (r) => purchaseEUR(r)?.total),
  t('Prenesenie DPH', (r) => yes(r.reverseCharge), 8),
  t('Bez odpočtu', (r) => yes(r.noDeduction), 8),
  t('Stav', (r) => purchaseStatusOf(r).label, 18),
  t('Druh výdavku', (r) => codes.expenseKinds?.find((k) => k.code === (r.expense || 'services'))?.label ?? '', 18),
]

export const contactColumns = [
  t('Názov', (r) => r.name, 32),
  t('IČO', (r) => r.ico, 11),
  t('DIČ', (r) => r.dic, 12),
  t('IČ DPH', (r) => r.icdph, 14),
  t('Ulica', (r) => r.street, 24),
  t('PSČ', (r) => r.zip, 8),
  t('Obec', (r) => r.city, 16),
  t('Krajina', (r) => countryName(r.country || 'SK'), 12),
  t('E-mail', (r) => r.email, 24),
  t('Telefón', (r) => r.phone, 16),
  t('IBAN', (r) => r.iban, 28),
  { header: 'Splatnosť (dni)', value: (r) => r.dueDays || '', type: 'number', width: 10 },
  t('Poznámka', (r) => r.note, 30),
]

const roleLabel = (code) => codes.roles?.find((r) => r.code === code)?.label ?? code

export const memberColumns = [
  t('Meno', (r) => r.user.name, 24),
  t('E-mail', (r) => r.user.email, 28),
  t('Rola', (r) => roleLabel(r.role), 14),
  t('Aktívny', (r) => (r.user.active ? 'áno' : 'nie'), 8),
  t('2FA', (r) => (r.user.twoFactor ? 'áno' : 'nie'), 6),
]

export const companyColumns = [
  t('Firma', (r) => r.name, 32),
  t('IČO', (r) => r.ico, 11),
  t('DIČ', (r) => r.dic, 12),
  t('IČ DPH', (r) => r.icdph, 14),
  t('Ulica', (r) => r.street, 24),
  t('PSČ', (r) => r.zip, 8),
  t('Obec', (r) => r.city, 16),
  t('Platiteľ DPH', (r) => yes(r.vatPayer), 8),
]

const fmtTime = new Intl.DateTimeFormat('sk-SK', {
  timeZone: 'Europe/Bratislava',
  dateStyle: 'short',
  timeStyle: 'medium',
})

/** Stĺpce auditu; companyName prevedie ID firmy na názov. */
export function auditColumns(companyName) {
  const catLabel = (c) => codes.auditCategories?.find((x) => x.code === c)?.label ?? c
  return [
    t('Čas', (r) => fmtTime.format(new Date(r.time)), 20),
    t('Firma', (r) => companyName(r.companyId), 24),
    t('Kategória', (r) => catLabel(r.category), 16),
    t('Používateľ', (r) => r.userName, 20),
    t('Kanál', (r) => channelLabel(r.channel), 10),
    t('IP adresa', (r) => r.ip, 15),
    t('Akcia', (r) => actionLabel(r.action), 18),
    t('Popis', (r) => r.summary, 60),
    t('Objekt', (r) => (r.entityType ? `${r.entityType} #${r.entityId}` : ''), 14),
    t('Odtlačok', (r) => r.hash, 20),
  ]
}

export const vatRowColumns = [
  t('Riadok', (r) => r.code.replace(/^r/, ''), 8),
  t('Popis', (r) => r.label, 60),
  money('Suma', (r) => r.value),
]

export const kvColumns = [
  t('Časť', (r) => r.section, 6),
  t('IČ DPH', (r) => r.partner, 14),
  t('Názov', (r) => r.name, 30),
  t('Číslo faktúry', (r) => r.number, 16),
  t('Pôvodná faktúra', (r) => r.original, 16),
  date('Dátum dodania', (r) => r.date),
  money('Základ', (r) => r.base ?? r.baseDiff),
  money('Daň', (r) => (r.a2 ? '' : r.vat ?? r.vatDiff)),
  { header: 'Sadzba %', value: (r) => (r.a2 ? '' : r.rate), type: 'number', width: 8 },
  money('Odpočítaná daň', (r) => (r.section === 'C.2' ? r.deductedDiff : r.deducted)),
  t('Prenesenie – plnenie', (r) => r.label, 22),
  t('Kód SCS', (r) => r.code, 8),
  t('Druh tovaru', (r) => r.goods, 8),
  { header: 'Množstvo', value: (r) => r.quantity, type: 'number', width: 10 },
  t('MJ', (r) => r.unit, 5),
]

const bankStatus = { open: 'Nespárované', matched: 'Spárované', ignored: 'Nepárované' }

export const bankColumns = [
  date('Dátum', (r) => r.date),
  money('Suma', (r) => (r.direction === 'out' ? '-' : '') + r.amount),
  t('Mena', (r) => r.currency, 6),
  t('Protistrana', (r) => r.counterName, 28),
  t('IBAN protistrany', (r) => r.counterIban, 26),
  t('VS', (r) => r.vs, 12),
  t('ŠS', (r) => r.ss, 8),
  t('KS', (r) => r.ks, 6),
  t('Správa', (r) => r.message, 40),
  t('Stav', (r) => bankStatus[r.status] ?? r.status, 14),
  t('Doklad', (r) => r.match?.number, 14),
  t('Poznámka', (r) => r.note, 24),
  t('Účet', (r) => r.account, 26),
]

export const svColumns = [
  t('Štát', (r) => r.country, 6),
  t('IČ DPH', (r) => r.vatId, 16),
  t('Odberateľ', (r) => r.name, 30),
  money('Hodnota', (r) => r.value),
  { header: 'Kód plnenia', value: (r) => r.code, type: 'number', width: 8 },
  { header: 'Počet dokladov', value: (r) => r.count, type: 'number', width: 8 },
]

const unixDate = (sec) => (sec ? new Date(sec * 1000).toISOString().slice(0, 10) : '')

export const apiKeyColumns = [
  t('Názov', (r) => r.name, 24),
  t('Kľúč', (r) => (r.prefix === 'OAuth' ? 'pripojená aplikácia (OAuth)' : r.prefix + '…'), 14),
  t('Práva', (r) => (r.readOnly ? 'len čítanie' : 'ako používateľ'), 14),
  date('Vytvorený', (r) => unixDate(r.createdAt)),
  date('Naposledy použitý', (r) => unixDate(r.lastUsedAt)),
  t('Stav', (r) => (r.revoked ? 'zrušený' : 'aktívny'), 10),
]

export const cashColumns = [
  t('Číslo', (r) => r.number, 14),
  date('Dátum', (r) => r.date),
  t('Druh', (r) => (r.kind === 'in' ? 'príjem' : 'výdavok'), 9),
  money('Príjem', (r) => (r.kind === 'in' ? r.amount : '')),
  money('Výdavok', (r) => (r.kind === 'out' ? r.amount : '')),
  t('Partner', (r) => r.partner, 24),
  t('Účel', (r) => r.purpose, 36),
  t('Mena', (r) => r.currency || 'EUR', 6),
  money('Suma €', (r) => (r.kind === 'out' ? `-${r.amountEur ?? r.amount}` : r.amountEur ?? r.amount)),
  t('Doklad', (r) => r.linkNumber, 14),
  t('Stav', (r) => (r.cancelled ? 'stornovaný' : ''), 10),
  t('Poznámka', (r) => r.note, 24),
]

/** Peňažný denník: pohyb, platidlo a suma v každom stĺpci (§ 4 ods. 6). */
export function journalExportColumns(cols = codes.journalColumns ?? []) {
  const amountIn = (r, code) => r.lines?.find((l) => l.column === code)?.amount ?? ''
  return [
    date('Dátum', (r) => r.date),
    t('Doklad', (r) => r.number, 16),
    t('Uhrádza', (r) => r.doc?.number ?? '', 14),
    t('Platidlo', (r) => (r.means === 'cash' ? 'pokladnica' : 'banka'), 10),
    t('Partner', (r) => r.partner, 24),
    t('Text', (r) => r.text, 30),
    money('Suma', (r) => r.amount),
    ...cols.map((c) => money(c.label, (r) => amountIn(r, c.code))),
    t('Rozpis', (r) => (!r.lines?.length ? 'nerozpísaný' : r.auto ? 'návrh' : 'ručne'), 11),
    t('Poznámka', (r) => r.note, 24),
  ]
}

/** Kniha pohľadávok alebo záväzkov k dňu. */
export function openItemColumns(payables = false) {
  return [
    t('Číslo', (r) => r.number, 14),
    t('Druh', (r) => (r.kind === 'purchase' ? 'prijatá faktúra' : kindLabel(r.kind)), 18),
    t(payables ? 'Dodávateľ' : 'Odberateľ', (r) => r.partner, 30),
    t('IČO', (r) => r.ico, 11),
    date(payables ? 'Doručená' : 'Vystavená', (r) => r.issueDate),
    date('Splatná', (r) => r.dueDate),
    t('Mena', (r) => r.currency, 6),
    money('Suma', (r) => r.total),
    money('Uhradené', (r) => r.paid),
    money('Zostáva', (r) => r.remaining),
    money('Zostáva €', (r) => r.remainingEur),
    { header: 'Dni po splatnosti', value: (r) => r.daysOverdue || '', type: 'number', width: 10 },
  ]
}

/** Výkazy jednoduchého účtovníctva (celé eurá). */
export function statementColumns(previousEmpty = false) {
  const cols = [
    t('Výkaz', (r) => r.statement, 30),
    t('Riadok', (r) => r.code, 6),
    t('Názov', (r) => r.label, 50),
    { header: 'Bežné obdobie', value: (r) => Number(r.current), type: 'number', width: 14 },
  ]
  if (!previousEmpty) cols.push({ header: 'Predchádzajúce obdobie', value: (r) => Number(r.previous), type: 'number', width: 14 })
  return cols
}

export const backupColumns = [
  t('Záloha', (r) => r.name, 32),
  t('Čas', (r) => new Date(r.time).toLocaleString('sk-SK'), 20),
  { header: 'Veľkosť (kB)', value: (r) => Math.round(r.size / 1024), type: 'number', width: 12 },
]

export const quoteColumns = [
  t('Číslo', (r) => r.number || 'koncept', 14),
  t('Odberateľ', (r) => r.customer?.name, 32),
  t('Predmet', (r) => r.subject, 30),
  t('IČO', (r) => r.customer?.ico, 11),
  t('IČ DPH', (r) => r.customer?.icdph, 14),
  t('Obec', (r) => r.customer?.city, 16),
  date('Dátum ponuky', (r) => r.issueDate),
  date('Platná do', (r) => r.validUntil),
  money('Základ', (r) => r.summary?.base),
  money('DPH', (r) => r.summary?.vat),
  money('Spolu', (r) => r.summary?.total),
  t('Mena', (r) => r.currency || 'EUR', 6),
  { header: 'Kurz', value: (r) => (isForeignCurrency(r.currency) ? r.fxRate : ''), type: 'number', width: 10 },
  money('Základ €', (r) => eurSummary(r)?.base),
  money('DPH €', (r) => eurSummary(r)?.vat),
  money('Spolu €', (r) => eurSummary(r)?.total),
  t('Stav', (r) => quoteStatusOf(r).label, 16),
  t('Faktúra', (r) => (r.invoiceId ? r.invoiceNumber || 'koncept' : ''), 14),
  t('Poznámka', (r) => r.note, 30),
]

export const recurringColumns = [
  t('Názov', (r) => r.name, 28),
  t('Odberateľ', (r) => r.customer?.name, 32),
  t('IČO', (r) => r.customer?.ico, 11),
  t('Interval', (r) => intervalLabel(r.interval), 12),
  date('Ďalšia faktúra', (r) => r.nextDate),
  date('Koniec', (r) => r.endDate),
  money('Základ', (r) => r.summary?.base),
  money('Spolu', (r) => r.summary?.total),
  t('Vystaviť automaticky', (r) => yes(r.autoIssue), 10),
  t('Splatnosť (dni)', (r) => r.dueDays || '', 10),
  { header: 'Vytvorených faktúr', value: (r) => r.generated ?? 0, type: 'number', width: 10 },
  t('Stav', (r) => recurringStatusOf(r).label, 14),
  t('Položky', (r) => (r.lines ?? []).map((l) => l.description).join('; '), 40),
]

export const reminderColumns = [
  t('Číslo', (r) => r.number, 14),
  date('Dátum', (r) => r.date),
  t('Stupeň', (r) => levelShort(r.level), 12),
  t('Odberateľ', (r) => r.customer?.name, 32),
  t('IČO', (r) => r.customer?.ico, 11),
  t('Faktúry', (r) => (r.items ?? []).map((i) => i.number).join(', '), 30),
  money('Dlžná suma', (r) => r.principal),
  money('Úrok z omeškania', (r) => r.interestTotal),
  money('Paušálna náhrada', (r) => r.fees),
  money('Spolu', (r) => r.total),
  date('Uhradiť do', (r) => r.payBy),
  t('Poznámka', (r) => r.note, 30),
]

export const overdueColumns = [
  t('Odberateľ', (r) => r.customer?.name, 32),
  t('IČO', (r) => r.customer?.ico, 11),
  t('Faktúra', (r) => r.number, 14),
  date('Vystavená', (r) => r.issueDate),
  date('Splatná', (r) => r.dueDate),
  { header: 'Dní po splatnosti', value: (r) => r.days, type: 'number', width: 10 },
  money('Suma', (r) => r.total),
  t('Upomenutá', (r) => (r.reminded ? levelShort(r.reminded) : ''), 12),
  date('Posledná upomienka', (r) => r.lastReminded),
]

export const ecbColumns = [
  date('Platí od', (r) => r.from),
  { header: 'Sadzba ECB (%)', value: (r) => Number(r.rate), type: 'number', width: 14 },
]

// Kniha jázd (riadky jázd s nákladmi cost = { fuel, wear, total } zo servera).
export const tripColumns = [
  date('Dátum', (r) => r.date),
  t('Čas', (r) => tripTime(r), 12),
  t('Odkiaľ', (r) => r.from, 22),
  t('Kam', (r) => r.to, 22),
  t('Účel', (r) => r.purpose, 30),
  t('Druh', (r) => (r.kind === 'private' ? 'súkromná' : 'služobná'), 10),
  t('Vodič', (r) => r.driver, 18),
  { header: 'Tachometer začiatok', value: (r) => r.odoStart || '', type: 'number', width: 12 },
  { header: 'Tachometer koniec', value: (r) => r.odoEnd || '', type: 'number', width: 12 },
  { header: 'Km', value: (r) => r.km, type: 'number', width: 8 },
  money('PHM €', (r) => r.cost?.fuel ?? ''),
  money('Opotrebenie €', (r) => r.cost?.wear ?? ''),
  money('Náklady spolu €', (r) => r.cost?.total ?? ''),
  t('Poznámka', (r) => r.note, 24),
]

export const fuelColumns = [
  date('Dátum', (r) => r.date),
  { header: 'Množstvo', value: (r) => Number(r.quantity), type: 'number', width: 10 },
  money('Cena s DPH €', (r) => r.amount),
  { header: 'Tachometer', value: (r) => r.odometer || '', type: 'number', width: 12 },
  t('Plná nádrž', (r) => yes(r.full), 8),
  t('Čerpacia stanica', (r) => r.station, 22),
  t('Doklad', (r) => r.docNumber, 16),
  t('Poznámka', (r) => r.note, 24),
]

export const kmRateColumns = [
  date('Platí od', (r) => r.from),
  { header: 'Osobné vozidlo (€/km)', value: (r) => Number(r.car), type: 'number', width: 14 },
  { header: 'Jednostopové (€/km)', value: (r) => Number(r.moto), type: 'number', width: 14 },
]

export const holidayColumns = [
  t('Deň', (r) => r.day, 10),
  t('Názov', (r) => r.name, 40),
  { header: 'Od roku', value: (r) => r.from || '', type: 'number', width: 9 },
  { header: 'Do roku', value: (r) => r.to || '', type: 'number', width: 9 },
  t('Okrem rokov', (r) => r.exceptText, 14),
]

export const vatPeriodColumns = [
  date('Platí od', (r) => r.from),
  date('Platí do', (r) => r.to),
  t('Sadzby DPH (%)', (r) => (r.rates ?? []).join(', '), 20),
]

export const itemColumns = [
  t('Kód', (r) => r.code, 10),
  t('Názov', (r) => r.name, 40),
  t('MJ', (r) => r.unit, 6),
  money('Cena bez DPH', (r) => r.priceNet),
  { header: 'DPH (%)', value: (r) => r.vatRate, type: 'number', width: 8 },
  money('Cena s DPH', (r) => r.priceGross),
  t('Zadaná cena', (r) => (r.priceMode === 'gross' ? 's DPH' : 'bez DPH'), 10),
  t('Archivovaná', (r) => yes(r.archived), 10),
  t('Poznámka', (r) => r.note, 30),
]

// Register majetku za rok (riadky z /assets?year=); stav sa ráta k roku riadku.
export const assetColumns = [
  t('Inventárne číslo', (r) => r.number, 12),
  t('Názov', (r) => r.name, 32),
  t('Druh', (r) => (r.kind === 'intangible' ? 'nehmotný' : 'hmotný'), 10),
  t('Skupina / doba', (r) => periodLabel(r), 16),
  date('Zaradený', (r) => r.acquired),
  money('Vstupná cena', (r) => r.cost),
  { header: 'Podiel na podnikaní (%)', value: (r) => r.share, type: 'number', width: 10 },
  money('ZC na začiatku roka', (r) => r.opening),
  money('Odpis v roku', (r) => r.thisYear?.claimed),
  money('ZC vyradeného (výdavok)', (r) => r.thisYear?.disposal),
  money('ZC na konci roka', (r) => r.residual),
  date('Vyradený', (r) => r.disposed),
  t('Dôvod vyradenia', (r) => disposalOptions.find((o) => o.value === r.disposal)?.label ?? '', 14),
  t('Stav', (r) => assetStatus(r, r.thisYear?.year).label, 12),
  t('Prijatá faktúra', (r) => r.purchaseNumber, 14),
  t('Poznámka', (r) => r.note, 30),
]

export const emailColumns = [
  t('Odoslaný', (r) => fmtTime.format(new Date(r.sent)), 18),
  t('Doklad', (r) => docKindLabel(r.docKind), 16),
  t('Číslo', (r) => r.docNumber, 14),
  t('Komu', (r) => r.to, 32),
  t('Kópia', (r) => r.cc, 24),
  t('Predmet', (r) => r.subject, 40),
  t('Príloha', (r) => r.file, 26),
  t('Poslal', (r) => r.user, 18),
  t('Stav', (r) => emailStatus(r).label, 12),
  t('Chyba', (r) => r.error, 40),
]

export const rateColumns = [
  date('Dátum', (r) => r.date),
  t('Mena', (r) => r.currency, 6),
  { header: 'Kurz (za 1 EUR)', value: (r) => r.rate, type: 'number', width: 14 },
  t('Zdroj', (r) => sourceLabel(r.source), 8),
]

export const calendarColumns = [
  date('Dátum', (r) => r.date),
  t('Druh', (r) => calendarKind(r.kind), 16),
  t('Termín', (r) => r.title, 48),
  t('Podrobnosti', (r) => r.detail, 40),
  money('Suma', (r) => r.amount ?? ''),
  t('Mena', (r) => (r.amount ? r.currency || 'EUR' : ''), 6),
  t('Stav', (r) => stateOf(r).text, 12),
  date('Pôvodná lehota', (r) => r.shifted),
]

export const ownEventColumns = [
  t('Termín', (r) => r.title, 40),
  date('Prvý termín', (r) => r.date),
  t('Opakovanie', (r) => repeatLabel(r.repeat), 16),
  date('Do', (r) => r.until),
  t('Posun na pracovný deň', (r) => yes(r.workday), 10),
  t('Poznámka', (r) => r.note, 40),
]

export const accountColumns = [
  t('Účet', (r) => (r.number.length > 3 ? `${r.number.slice(0, 3)}.${r.number.slice(3)}` : r.number), 10),
  t('Názov', (r) => r.name, 48),
  t('Druh', (r) => ({ A: 'aktívny', P: 'pasívny', X: 'aktívny / pasívny', N: 'nákladový', V: 'výnosový', Z: 'závierkový', S: 'podsúvahový' })[r.type] ?? r.type, 16),
  t('Nedaňový', (r) => yes(r.nonTax), 9),
  t('Používa sa', (r) => yes(r.active), 9),
  t('Poznámka', (r) => r.note, 30),
]

export const centerColumns = [
  t('Kód', (r) => r.code, 10),
  t('Stredisko', (r) => r.name, 36),
  t('Aktívne', (r) => yes(r.active), 8),
  t('Poznámka', (r) => r.note, 30),
]

export const jobColumns = [
  t('Kód', (r) => r.code, 14),
  t('Zákazka', (r) => r.name, 36),
  t('Odberateľ', (r) => r.customer, 28),
  date('Od', (r) => r.from),
  date('Do', (r) => r.to),
  t('Aktívna', (r) => yes(r.active), 8),
  t('Poznámka', (r) => r.note, 30),
]

export const templateColumns = [
  t('Kód', (r) => r.code, 14),
  t('Predkontácia', (r) => r.name, 40),
  t('Doklad', (r) => ({ sale: 'Vydaná faktúra', purchase: 'Prijatá faktúra', bank: 'Banka', cash: 'Pokladňa', internal: 'Interný doklad' })[r.doc] ?? r.doc, 16),
  t('Má dať', (r) => r.lines.filter((l) => l.side === 'MD').map((l) => l.account).join(', '), 20),
  t('Dal', (r) => r.lines.filter((l) => l.side === 'D').map((l) => l.account).join(', '), 20),
  t('Používa sa', (r) => yes(r.active), 9),
  t('Poznámka', (r) => r.note, 30),
]

const docName = (d) => ({ sale: 'Vydaná faktúra', purchase: 'Prijatá faktúra', bank: 'Banka', cash: 'Pokladňa', internal: 'Interný doklad' })[d] ?? d
const accNo = (n) => (n && n.length > 3 ? `${n.slice(0, 3)}.${n.slice(3)}` : n ?? '')
const nz = (v) => (Number(v) ? v : '')

/** Účtovný denník — riadky z lib/ledger.js postingRows. */
export const postingColumns = [
  date('Dátum', (r) => r.date),
  t('Doklad', (r) => r.number, 16),
  t('Druh', (r) => docName(r.doc), 16),
  t('Partner', (r) => r.partner, 28),
  t('Text', (r) => r.text, 32),
  t('Účet', (r) => accNo(r.line.account), 10),
  t('Názov účtu', (r) => r.line.name, 30),
  money('MD', (r) => nz(r.line.debit)),
  money('D', (r) => nz(r.line.credit)),
  t('Stredisko', (r) => r.line.center, 10),
  t('Zákazka', (r) => r.line.job, 12),
  t('Predkontácia', (r) => (r.manual ? 'ručne' : r.template), 14),
  t('Problém', (r) => r.problem, 30),
]

const side = (v, s) => (Number(v) * s > 0 ? String(v).replace('-', '') : '')

/** Hlavná kniha — obraty a zostatky účtov. */
export const turnoverColumns = [
  t('Účet', (r) => accNo(r.account), 10),
  t('Názov', (r) => r.name, 36),
  money('PS MD', (r) => side(r.opening, 1)),
  money('PS D', (r) => side(r.opening, -1)),
  money('Obrat MD', (r) => nz(r.debit)),
  money('Obrat D', (r) => nz(r.credit)),
  money('KS MD', (r) => side(r.closing, 1)),
  money('KS D', (r) => side(r.closing, -1)),
]

/** Súvaha a výkaz ziskov a strát (podvojné, celé eurá). */
export function ledgerStatementColumns(balance = true, previousEmpty = false) {
  const n = (header, value) => ({ header, value, type: 'number', width: 14 })
  const cols = [t('Výkaz', (r) => r.statement, 22), t('Riadok', (r) => r.code, 6), t('Položka', (r) => r.label, 60)]
  if (balance) cols.push(n('Brutto', (r) => (r.statement.endsWith('aktíva') ? Number(r.brutto) : '')), n('Korekcia', (r) => (r.statement.endsWith('aktíva') ? Number(r.korekcia) : '')))
  cols.push(n('Bežné obdobie', (r) => Number(r.netto)))
  if (!previousEmpty) cols.push(n('Minulé obdobie', (r) => Number(r.previous)))
  cols.push(t('Účty', (r) => r.accounts, 40))
  return cols
}

/** Náklady a výnosy po strediskách a zákazkách. */
export const dimColumns = [
  t('Druh', (r) => (r.kind === 'center' ? 'stredisko' : 'zákazka'), 10),
  t('Kód', (r) => r.code, 14),
  t('Názov', (r) => r.name, 36),
  money('Náklady', (r) => r.costs),
  money('Výnosy', (r) => r.revenues),
  money('Výsledok', (r) => r.result),
]

const stockKind = (r) => (r.return ? 'Príjemka – vrátenie' : { receipt: 'Príjemka', issue: 'Výdajka', count: 'Inventúrny rozdiel' }[r.kind] ?? r.kind)
const num = (header, value) => ({ header, value: (r) => Number(value(r)), type: 'number', width: 12 })

/** Skladové karty. */
export const stockCardColumns = [
  t('Kód', (r) => r.code, 10),
  t('Položka', (r) => r.name, 36),
  t('Druh', (r) => ({ goods: 'tovar', material: 'materiál', product: 'výrobky' })[r.kind] ?? r.kind, 10),
  t('Sklad', (r) => r.warehouse, 8),
  num('Množstvo', (r) => r.quantity),
  t('MJ', (r) => r.unit, 6),
  money('Priemerná cena', (r) => r.avgCost),
  money('Hodnota', (r) => r.value),
  num('Minimum', (r) => r.min),
  t('Pod minimom', (r) => yes(r.low), 10),
]

/** Skladové doklady — riadky z lib/stock.js docRows. */
export const stockDocColumns = [
  date('Dátum', (r) => r.date),
  t('Číslo', (r) => r.number, 12),
  t('Druh', stockKind, 18),
  t('Sklad', (r) => r.warehouse, 8),
  t('Partner', (r) => r.partner, 26),
  t('Doklad', (r) => r.linkNumber, 14),
  t('Položka', (r) => r.line.name, 32),
  num('Množstvo', (r) => r.line.quantity),
  t('MJ', (r) => r.line.unit, 6),
  money('Cena za MJ', (r) => r.line.unitCost),
  money('Spolu', (r) => r.line.total),
  t('Poznámka', (r) => r.note, 24),
]
