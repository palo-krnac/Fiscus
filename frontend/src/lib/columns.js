// Stĺpce exportov jednotlivých zoznamov (pozri lib/export.js).
import { codes, countryName, paymentLabel } from './codes.js'
import { purchaseStatusOf, statusOf } from './format.js'
import { actionLabel, channelLabel } from './audit.js'
import { quoteStatusOf } from './quotes.js'

const t = (header, value, width) => ({ header, value, width })
const money = (header, value) => ({ header, value, type: 'money', width: 14 })
const date = (header, value) => ({ header, value, type: 'date', width: 12 })
const yes = (b) => (b ? 'áno' : '')

export const invoiceColumns = [
  t('Číslo', (r) => r.number || 'koncept', 14),
  t('Druh', (r) => (r.kind === 'credit_note' ? 'dobropis' : 'faktúra'), 10),
  t('Odberateľ', (r) => r.customer?.name, 32),
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
  money('Základ', (r) => r.summary?.base),
  money('DPH', (r) => r.summary?.vat),
  money('Spolu', (r) => r.summary?.total),
  t('Prenesenie DPH', (r) => yes(r.reverseCharge), 8),
  t('Bez odpočtu', (r) => yes(r.noDeduction), 8),
  t('Stav', (r) => purchaseStatusOf(r).label, 18),
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
  money('Daň', (r) => r.vat ?? r.vatDiff),
  { header: 'Sadzba %', value: (r) => r.rate, type: 'number', width: 8 },
  money('Odpočítaná daň', (r) => r.deducted),
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
  t('Kľúč', (r) => r.prefix + '…', 14),
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
  t('Doklad', (r) => r.linkNumber, 14),
  t('Stav', (r) => (r.cancelled ? 'stornovaný' : ''), 10),
  t('Poznámka', (r) => r.note, 24),
]

export const backupColumns = [
  t('Záloha', (r) => r.name, 32),
  t('Čas', (r) => new Date(r.time).toLocaleString('sk-SK'), 20),
  { header: 'Veľkosť (kB)', value: (r) => Math.round(r.size / 1024), type: 'number', width: 12 },
]

export const quoteColumns = [
  t('Číslo', (r) => r.number || 'koncept', 14),
  t('Odberateľ', (r) => r.customer?.name, 32),
  t('IČO', (r) => r.customer?.ico, 11),
  t('IČ DPH', (r) => r.customer?.icdph, 14),
  t('Obec', (r) => r.customer?.city, 16),
  date('Dátum ponuky', (r) => r.issueDate),
  date('Platná do', (r) => r.validUntil),
  money('Základ', (r) => r.summary?.base),
  money('DPH', (r) => r.summary?.vat),
  money('Spolu', (r) => r.summary?.total),
  t('Mena', (r) => r.currency || 'EUR', 6),
  t('Stav', (r) => quoteStatusOf(r).label, 16),
  t('Faktúra', (r) => (r.invoiceId ? r.invoiceNumber || 'koncept' : ''), 14),
  t('Poznámka', (r) => r.note, 30),
]
