// Tenký klient pre Go API. Chyby prevádza na ApiError s chybami polí.

export class ApiError extends Error {
  constructor(status, message, fields = []) {
    super(message)
    this.status = status
    this.fields = fields
  }

  /** { 'customer.name': 'Zadajte odberateľa', ... } */
  fieldMap() {
    return Object.fromEntries(this.fields.map((f) => [f.field, f.message]))
  }
}

let onUnauthorized = null

/** Zavolá sa pri odpovedi 401 (vypršaná relácia) — aplikácia zobrazí prihlásenie. */
export function setUnauthorizedHandler(fn) {
  onUnauthorized = fn
}

export async function request(method, path, body, fetchImpl = globalThis.fetch) {
  const init = { method, headers: {}, credentials: 'same-origin' }
  // ochrana proti CSRF: cudzia stránka túto hlavičku poslať nevie
  if (method !== 'GET') init.headers['X-Fiskus'] = '1'

  if (typeof Blob !== 'undefined' && body instanceof Blob) {
    // súbor (napr. bankový výpis) sa posiela tak, ako je
    init.headers['Content-Type'] = body.type || 'application/octet-stream'
    init.body = body
  } else if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }
  let res
  try {
    res = await fetchImpl(path, init)
  } catch {
    throw new ApiError(0, 'Server neodpovedá. Skontrolujte, či program Fiškus beží.')
  }
  const text = await res.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }
  if (!res.ok) {
    if (res.status === 401 && onUnauthorized && !path.startsWith('/api/auth/')) onUnauthorized()
    throw new ApiError(res.status, data?.error ?? `Chyba servera (${res.status})`, data?.fields ?? [])
  }
  return data
}

/** Stiahne súbor ako Blob s menom z hlavičky Content-Disposition. */
export async function download(path, fallbackName, fetchImpl = globalThis.fetch, body) {
  const init = { credentials: 'same-origin' }
  if (body !== undefined) {
    // POST s JSON telom (napr. platobný príkaz z vybraných faktúr)
    Object.assign(init, { method: 'POST', headers: { 'X-Fiskus': '1', 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
  }
  const res = await fetchImpl(path, init)
  if (!res.ok) {
    let data = null
    try {
      data = await res.json()
    } catch {
      /* telo nie je JSON */
    }
    throw new ApiError(res.status, data?.error ?? `Chyba servera (${res.status})`, data?.fields ?? [])
  }
  const name = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '')?.[1] ?? fallbackName
  return { blob: await res.blob(), name }
}

const inv = (cid) => `/api/companies/${cid}/invoices`

/** Query string z filtra (prázdne hodnoty vynechá). */
export function query(params = {}) {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') p.set(k, v)
  }
  const qs = p.toString()
  return qs ? `?${qs}` : ''
}

export const api = {
  capabilities: () => request('GET', '/api/capabilities'),
  desktopPing: () => request('POST', '/api/desktop/ping'),
  desktopQuit: () => request('POST', '/api/desktop/quit'),
  me: () => request('GET', '/api/me'),
  keys: () => request('GET', '/api/me/keys'),
  backups: () => request('GET', '/api/backups'),
  createBackup: () => request('POST', '/api/backups'),
  restoreBackup: (name) => request('POST', `/api/backups/${encodeURIComponent(name)}/restore`),
  cancelRestore: () => request('DELETE', '/api/backups/restore'),
  setBackupSettings: (s) => request('PUT', '/api/backups/settings', s),
  setBackupRemote: (r) => request('PUT', '/api/backups/remote', r),
  remoteBackups: () => request('GET', '/api/backups/remote'),
  fetchRemoteBackup: (name) => request('POST', `/api/backups/remote/${encodeURIComponent(name)}/fetch`),
  vatCoefficient: (cid, year) => request('GET', `/api/companies/${cid}/vat/coefficient/${year}`),
  setVATCoefficient: (cid, year, c) => request('PUT', `/api/companies/${cid}/vat/coefficient/${year}`, c),
  backupURL: (name) => `/api/backups/${encodeURIComponent(name)}`,
  calendar: (cid, from, to) => request('GET', `/api/companies/${cid}/calendar${query({ from, to })}`),
  calendarICSURL: (cid) => `/api/companies/${cid}/calendar.ics`,
  ownEvents: (cid) => request('GET', `/api/companies/${cid}/calendar/own`),
  saveOwnEvent: (cid, o) =>
    o.id ? request('PUT', `/api/companies/${cid}/calendar/own/${o.id}`, o) : request('POST', `/api/companies/${cid}/calendar/own`, o),
  deleteOwnEvent: (cid, id) => request('DELETE', `/api/companies/${cid}/calendar/own/${id}`),
  markOwnEvent: (cid, id, date, done) => request('POST', `/api/companies/${cid}/calendar/own/${id}/done`, { date, done }),
  calendarFeed: () => request('GET', '/api/me/calendar-feed'),
  calendarSync: (cid) => request('GET', `/api/companies/${cid}/calendar/sync`),
  connectCalendarSync: (cid, provider) => request('POST', `/api/companies/${cid}/calendar/sync/connect`, { provider }),
  runCalendarSync: (cid) => request('POST', `/api/companies/${cid}/calendar/sync/run`, {}),
  disconnectCalendarSync: (cid, remove) => request('DELETE', `/api/companies/${cid}/calendar/sync` + (remove ? '?remove=1' : '')),
  setCalendarSyncConfig: (c) => request('PUT', '/api/calendar-sync/config', c),
  createCalendarFeed: () => request('POST', '/api/me/calendar-feed'),
  revokeCalendarFeed: () => request('DELETE', '/api/me/calendar-feed'),
  createKey: (name, readOnly) => request('POST', '/api/me/keys', { name, readOnly }),
  revokeKey: (id) => request('DELETE', `/api/me/keys/${id}`),
  license: () => request('GET', '/api/license'),
  setLicense: (key) => request('POST', '/api/license', { key }),
  login: (email, password) => request('POST', '/api/auth/login', { email, password }),
  logout: () => request('POST', '/api/auth/logout'),
  changePassword: (current, password) => request('POST', '/api/auth/password', { current, password }),
  // dvojfázové overenie: po hesle príde { mfa: true, ticket }, reláciu vytvorí až kód
  loginMFA: (ticket, code) => request('POST', '/api/auth/mfa', { ticket, code }),
  mfa: () => request('GET', '/api/me/mfa'),
  mfaSetup: (password) => request('POST', '/api/me/mfa/setup', { password }),
  mfaEnable: (code) => request('POST', '/api/me/mfa/enable', { code }),
  mfaDisable: (password, code) => request('POST', '/api/me/mfa/disable', { password, code }),
  mfaRecovery: (password) => request('POST', '/api/me/mfa/recovery', { password }),
  // zabezpečenie: PIN desktopu, povinné 2FA servera
  unlockPIN: (pin) => request('POST', '/api/auth/pin', { pin }),
  lock: () => request('POST', '/api/auth/lock'),
  security: () => request('GET', '/api/security'),
  setPIN: (current, pin) => request('PUT', '/api/security/pin', { current, pin }),
  setRequireMFA: (on) => request('PUT', '/api/security/require-mfa', { on }),
  // súhlas s pripojením aplikácie cez OAuth (query = parametre /oauth/authorize)
  oauthRequest: (query) => request('GET', `/api/oauth/request?${query}`),
  oauthDecision: (query, allow, readOnly) => request('POST', '/api/oauth/decision', { query, allow, readOnly }),
  mfaReset: (uid) => request('POST', `/api/users/${uid}/mfa-reset`),

  members: (cid) => request('GET', `/api/companies/${cid}/users`),
  addMember: (cid, m) => request('POST', `/api/companies/${cid}/users`, m),
  setRole: (cid, uid, role) => request('PUT', `/api/companies/${cid}/users/${uid}`, { role }),
  removeMember: (cid, uid) => request('DELETE', `/api/companies/${cid}/users/${uid}`),

  /** Audit firmy; cid = 0 → celá inštalácia (iba správca inštalácie). */
  audit: (cid, filter) => request('GET', (cid ? `/api/companies/${cid}/audit` : '/api/audit') + query(filter)),
  verifyAudit: () => request('GET', '/api/audit/verify'),

  journal: (cid, year) => request('GET', `/api/companies/${cid}/journal` + query({ year })),
  classifyJournal: (cid, key, lines, note) => request('PUT', `/api/companies/${cid}/journal/${encodeURIComponent(key)}`, { lines, note }),
  addClosing: (cid, year, text, lines) => request('POST', `/api/companies/${cid}/journal/closing`, { year, text, lines }),
  setJournalOpening: (cid, year, bank) => request('PUT', `/api/companies/${cid}/journal/opening`, { year, bank }),
  statements: (cid, year) => request('GET', `/api/companies/${cid}/statements` + query({ year })),
  setStatementInputs: (cid, year, inputs) => request('PUT', `/api/companies/${cid}/statements/${year}`, inputs),
  openItems: (cid, date) => request('GET', `/api/companies/${cid}/open-items` + query({ date })),
  chart: (cid) => request('GET', `/api/companies/${cid}/chart`),
  saveAccount: (cid, old, a) =>
    old ? request('PUT', `/api/companies/${cid}/chart/accounts/${old}`, a) : request('POST', `/api/companies/${cid}/chart/accounts`, a),
  deleteAccount: (cid, number) => request('DELETE', `/api/companies/${cid}/chart/accounts/${number}`),
  saveCenter: (cid, old, c) =>
    old ? request('PUT', `/api/companies/${cid}/chart/centers/${encodeURIComponent(old)}`, c) : request('POST', `/api/companies/${cid}/chart/centers`, c),
  deleteCenter: (cid, code) => request('DELETE', `/api/companies/${cid}/chart/centers/${encodeURIComponent(code)}`),
  saveJob: (cid, old, j) =>
    old ? request('PUT', `/api/companies/${cid}/chart/jobs/${encodeURIComponent(old)}`, j) : request('POST', `/api/companies/${cid}/chart/jobs`, j),
  deleteJob: (cid, code) => request('DELETE', `/api/companies/${cid}/chart/jobs/${encodeURIComponent(code)}`),
  stock: (cid) => request('GET', `/api/companies/${cid}/stock`),
  saveStockConfig: (cid, c) => request('PUT', `/api/companies/${cid}/stock/config`, c),
  saveStockDoc: (cid, d) =>
    d.id ? request('PUT', `/api/companies/${cid}/stock/docs/${d.id}`, d) : request('POST', `/api/companies/${cid}/stock/docs`, d),
  deleteStockDoc: (cid, id) => request('DELETE', `/api/companies/${cid}/stock/docs/${id}`),
  createStockFeed: (cid) => request('POST', `/api/companies/${cid}/stock/feed`, {}),
  revokeStockFeed: (cid) => request('DELETE', `/api/companies/${cid}/stock/feed`),
  stockCard: (cid, warehouse, item) => request('GET', `/api/companies/${cid}/stock/cards/${encodeURIComponent(warehouse)}/${item}`),
  ledger: (cid, year) => request('GET', `/api/companies/${cid}/ledger` + query({ year })),
  setPosting: (cid, year, key, body) => request('PUT', `/api/companies/${cid}/ledger/${year}/postings/${encodeURIComponent(key)}`, body),
  setMoneyAccounts: (cid, m) => request('PUT', `/api/companies/${cid}/chart/money`, m),
  fxRevaluation: (cid, year) => request('GET', `/api/companies/${cid}/ledger/${year}/fx`),
  postFXRevaluation: (cid, year) => request('POST', `/api/companies/${cid}/ledger/${year}/fx`, {}),
  ledgerStatements: (cid, year) => request('GET', `/api/companies/${cid}/ledger/${year}/statements`),
  openingProposal: (cid, year) => request('GET', `/api/companies/${cid}/ledger/${year}/opening-proposal`),
  setLedgerOpening: (cid, year, lines) => request('PUT', `/api/companies/${cid}/ledger/${year}/opening`, { lines }),
  saveInternal: (cid, year, doc) =>
    doc.id ? request('PUT', `/api/companies/${cid}/ledger/${year}/internal/${doc.id}`, doc) : request('POST', `/api/companies/${cid}/ledger/${year}/internal`, doc),
  deleteInternal: (cid, year, id) => request('DELETE', `/api/companies/${cid}/ledger/${year}/internal/${id}`),
  saveTemplate: (cid, old, t) =>
    old ? request('PUT', `/api/companies/${cid}/chart/templates/${encodeURIComponent(old)}`, t) : request('POST', `/api/companies/${cid}/chart/templates`, t),
  deleteTemplate: (cid, code) => request('DELETE', `/api/companies/${cid}/chart/templates/${encodeURIComponent(code)}`),
  assets: (cid, year) => request('GET', `/api/companies/${cid}/assets` + query({ year })),
  vehicles: (cid) => request('GET', `/api/companies/${cid}/vehicles`),
  saveVehicle: (cid, v) => (v.id ? request('PUT', `/api/companies/${cid}/vehicles/${v.id}`, v) : request('POST', `/api/companies/${cid}/vehicles`, v)),
  deleteVehicle: (cid, id) => request('DELETE', `/api/companies/${cid}/vehicles/${id}`),
  trips: (cid, vid, year) => request('GET', `/api/companies/${cid}/vehicles/${vid}/trips` + query({ year })),
  saveTrip: (cid, t) => (t.id ? request('PUT', `/api/companies/${cid}/trips/${t.id}`, t) : request('POST', `/api/companies/${cid}/trips`, t)),
  deleteTrip: (cid, id) => request('DELETE', `/api/companies/${cid}/trips/${id}`),
  fuel: (cid, vid, year) => request('GET', `/api/companies/${cid}/vehicles/${vid}/fuel` + query({ year })),
  saveFuel: (cid, f) => (f.id ? request('PUT', `/api/companies/${cid}/fuel/${f.id}`, f) : request('POST', `/api/companies/${cid}/fuel`, f)),
  deleteFuel: (cid, id) => request('DELETE', `/api/companies/${cid}/fuel/${id}`),
  fuelDocs: (cid, year) => request('GET', `/api/companies/${cid}/fuel-docs` + query({ year })),
  vehicleReport: (cid, vid, year) => request('GET', `/api/companies/${cid}/vehicles/${vid}/report` + query({ year })),
  postLogbook: (cid, vid, year) => request('POST', `/api/companies/${cid}/vehicles/${vid}/report/${year}`),
  saveAsset: (cid, a) =>
    a.id ? request('PUT', `/api/companies/${cid}/assets/${a.id}`, a) : request('POST', `/api/companies/${cid}/assets`, a),
  deleteAsset: (cid, id) => request('DELETE', `/api/companies/${cid}/assets/${id}`),
  postDepreciation: (cid, year) => request('POST', `/api/companies/${cid}/assets/depreciation/${year}`),
  resetJournal: (cid, key) => request('DELETE', `/api/companies/${cid}/journal/${encodeURIComponent(key)}`),
  cashBook: (cid, year, register = 0) => request('GET', `/api/companies/${cid}/cash` + query({ year, register: register || undefined })),
  cashRegisters: (cid) => request('GET', `/api/companies/${cid}/cash-registers`),
  saveCashRegisters: (cid, list) => request('PUT', `/api/companies/${cid}/cash-registers`, list),
  cash: (cid, id) => request('GET', `/api/companies/${cid}/cash/${id}`),
  cashCandidates: (cid, kind, register = 0) => request('GET', `/api/companies/${cid}/cash/candidates` + query({ kind, register: register || undefined })),
  previewCash: (cid, d) => request('POST', `/api/companies/${cid}/cash/preview`, d),
  createCash: (cid, d) => request('POST', `/api/companies/${cid}/cash`, d),
  updateCash: (cid, id, d) => request('PUT', `/api/companies/${cid}/cash/${id}`, d),
  cancelCash: (cid, id) => request('POST', `/api/companies/${cid}/cash/${id}/cancel`),
  bank: (cid, { year, status } = {}) => request('GET', `/api/companies/${cid}/bank` + query({ year, status })),
  importBank: (cid, file) => request('POST', `/api/companies/${cid}/bank/import` + query({ name: file.name }), file),
  bankSuggestions: (cid, id) => request('GET', `/api/companies/${cid}/bank/${id}/suggestions`),
  bankMatch: (cid, id, kind, docId) => request('POST', `/api/companies/${cid}/bank/${id}/match`, { kind, id: docId }),
  bankUnmatch: (cid, id) => request('POST', `/api/companies/${cid}/bank/${id}/unmatch`),
  bankIgnore: (cid, id, ignore, note) => request('POST', `/api/companies/${cid}/bank/${id}/ignore`, { ignore, note }),

  vat: (cid, period, excess, opts = {}) => request('GET', `/api/companies/${cid}/vat` + query({ period, excess, ...opts })),
  vatFilings: (cid, period) => request('GET', `/api/companies/${cid}/vat/filings` + query({ period })),
  recordVatFiling: (cid, period, excess, opts = {}, podane) =>
    request('POST', `/api/companies/${cid}/vat/filings` + query({ period, excess, ...opts, podane })),
  /** XML pre eDane ako Blob (kind: 'dph' | 'kv'). */
  vatXML: (cid, kind, period, excess, opts = {}, fetchImpl = globalThis.fetch) =>
    download(`/api/companies/${cid}/vat/${kind}.xml` + query({ period, excess, ...opts }), `${kind}.xml`, fetchImpl),
  /** E-faktúra (UBL, Peppol BIS 3.0) ako Blob; chýbajúce údaje sú v ApiError.fields. */
  einvoice: (cid, id, fetchImpl = globalThis.fetch) => download(`/api/companies/${cid}/invoices/${id}/ubl`, 'e-faktura.xml', fetchImpl),

  codes: () => request('GET', '/api/codes'),
  companies: () => request('GET', '/api/companies'),
  createCompany: (c) => request('POST', '/api/companies', c),
  updateCompany: (c) => request('PUT', `/api/companies/${c.id}`, c),
  registry: (ico) => request('GET', `/api/registry/${encodeURIComponent(ico)}`),
  vies: (vatId) => request('GET', `/api/vies/${encodeURIComponent(vatId.replace(/\s/g, ''))}`),

  contacts: (cid, q) => request('GET', `/api/companies/${cid}/contacts` + (q ? `?q=${encodeURIComponent(q)}` : '')),
  contact: (cid, id) => request('GET', `/api/companies/${cid}/contacts/${id}`),
  saveContact: (cid, c) =>
    c.id
      ? request('PUT', `/api/companies/${cid}/contacts/${c.id}`, c)
      : request('POST', `/api/companies/${cid}/contacts`, c),
  deleteContact: (cid, id) => request('DELETE', `/api/companies/${cid}/contacts/${id}`),

  invoices(cid, { year, status, q } = {}) {
    const p = new URLSearchParams()
    if (year) p.set('year', year)
    if (status) p.set('status', status)
    if (q) p.set('q', q)
    const qs = p.toString()
    return request('GET', inv(cid) + (qs ? `?${qs}` : ''))
  },
  invoice: (cid, id) => request('GET', `${inv(cid)}/${id}`),
  saveInvoice: (cid, invoice) =>
    invoice.id
      ? request('PUT', `${inv(cid)}/${invoice.id}`, invoice)
      : request('POST', inv(cid), invoice),
  preview: (cid, invoice) => request('POST', `${inv(cid)}/preview`, invoice),
  issue: (cid, id) => request('POST', `${inv(cid)}/${id}/issue`),
  // amount nepovinné: čiastočná úhrada, inak celý zostatok
  pay: (cid, id, date, amount) =>
    request('POST', `${inv(cid)}/${id}/pay`, date || amount ? { ...(date ? { date } : {}), ...(amount ? { amount } : {}) } : undefined),
  cancel: (cid, id) => request('POST', `${inv(cid)}/${id}/cancel`),
  creditNote: (cid, id) => request('POST', `${inv(cid)}/${id}/credit-note`),
  settleAdvance: (cid, id) => request('POST', `${inv(cid)}/${id}/settle`),
  // dodatočné daňové doklady k platbám zálohy (dodanie do 15 dní sa nestihlo)
  issueReceipts: (cid, id) => request('POST', `${inv(cid)}/${id}/receipts`),
  advanceCandidates: (cid, id) => request('GET', `${inv(cid)}/${id}/advances`),
  addAdvance: (cid, id, pid) => request('POST', `${inv(cid)}/${id}/advances/${pid}`),
  removeAdvance: (cid, id, pid) => request('DELETE', `${inv(cid)}/${id}/advances/${pid}`),

  purchases(cid, { year, q } = {}) {
    const p = new URLSearchParams()
    if (year) p.set('year', year)
    if (q) p.set('q', q)
    const qs = p.toString()
    return request('GET', `/api/companies/${cid}/purchases` + (qs ? `?${qs}` : ''))
  },
  purchase: (cid, id) => request('GET', `/api/companies/${cid}/purchases/${id}`),
  savePurchase: (cid, p) =>
    p.id
      ? request('PUT', `/api/companies/${cid}/purchases/${p.id}`, p)
      : request('POST', `/api/companies/${cid}/purchases`, p),
  previewPurchase: (cid, p) => request('POST', `/api/companies/${cid}/purchases/preview`, p),
  quotes: (cid, year) => request('GET', `/api/companies/${cid}/quotes` + (year ? `?year=${year}` : '')),
  quote: (cid, id) => request('GET', `/api/companies/${cid}/quotes/${id}`),
  saveQuote: (cid, q) =>
    q.id
      ? request('PUT', `/api/companies/${cid}/quotes/${q.id}`, q)
      : request('POST', `/api/companies/${cid}/quotes`, q),
  previewQuote: (cid, q) => request('POST', `/api/companies/${cid}/quotes/preview`, q),
  quoteAction: (cid, id, action) => request('POST', `/api/companies/${cid}/quotes/${id}/${action}`),
  unshareQuote: (cid, id) => request('DELETE', `/api/companies/${cid}/quotes/${id}/share`),
  // ponuka pre zákazníka cez odkaz (bez prihlásenia)
  publicQuote: (token) => request('GET', `/api/public/quotes/${encodeURIComponent(token)}`),
  respondQuote: (token, body) => request('POST', `/api/public/quotes/${encodeURIComponent(token)}/respond`, body),
  publicQuotePDF: (token) => `/api/public/quotes/${encodeURIComponent(token)}/pdf`,
  recurringList: (cid) => request('GET', `/api/companies/${cid}/recurring`),
  recurringDrafts: (cid) => request('GET', `/api/companies/${cid}/recurring/drafts`),
  recurring: (cid, id) => request('GET', `/api/companies/${cid}/recurring/${id}`),
  saveRecurring: (cid, t) =>
    t.id
      ? request('PUT', `/api/companies/${cid}/recurring/${t.id}`, t)
      : request('POST', `/api/companies/${cid}/recurring`, t),
  previewRecurring: (cid, t) => request('POST', `/api/companies/${cid}/recurring/preview`, t),
  recurringAction: (cid, id, action) => request('POST', `/api/companies/${cid}/recurring/${id}/${action}`),
  deleteRecurring: (cid, id) => request('DELETE', `/api/companies/${cid}/recurring/${id}`),
  overdue: (cid) => request('GET', `/api/companies/${cid}/overdue`),
  items: (cid) => request('GET', `/api/companies/${cid}/items`),
  logoURL: (cid, v = 0) => `/api/companies/${cid}/logo?v=${v}`,
  // E-maily: kind dokladu v URL je invoices | quotes | reminders
  mailSettings: (cid) => request('GET', `/api/companies/${cid}/mail-settings`),
  saveMailSettings: (cid, s) => request('PUT', `/api/companies/${cid}/mail-settings`, s),
  testMail: (cid, to) => request('POST', `/api/companies/${cid}/mail-settings/test`, { to }),
  emails: (cid, year) => request('GET', `/api/companies/${cid}/emails${query({ year })}`),
  mailDraft: (cid, kind, id) => request('GET', `/api/companies/${cid}/${kind}/${id}/email`),
  sendMail: (cid, kind, id, msg) => request('POST', `/api/companies/${cid}/${kind}/${id}/email`, msg),
  // PDF dokladu zo servera; kind: invoices | quotes | reminders
  pdfURL: (cid, kind, id, inline = false) => `/api/companies/${cid}/${kind}/${id}/pdf${inline ? '?inline=1' : ''}`,
  uploadLogo: (cid, file) => request('PUT', `/api/companies/${cid}/logo`, file),
  deleteLogo: (cid) => request('DELETE', `/api/companies/${cid}/logo`),
  saveItem: (cid, it) =>
    it.id
      ? request('PUT', `/api/companies/${cid}/items/${it.id}`, it)
      : request('POST', `/api/companies/${cid}/items`, it),
  deleteItem: (cid, id) => request('DELETE', `/api/companies/${cid}/items/${id}`),
  // súbor CSV/XLSX; apply=false vráti iba náhľad
  importItems: (cid, file, apply = false) => request('POST', `/api/companies/${cid}/items/import${apply ? '?apply=1' : ''}`, file),
  importMigrate: (cid, source, file, { apply = false, paidUntil = '', kind = '' } = {}) => {
    const q = new URLSearchParams()
    if (apply) q.set('apply', '1')
    if (paidUntil) q.set('paidUntil', paidUntil)
    if (kind) q.set('kind', kind)
    const qs = q.toString()
    return request('POST', `/api/companies/${cid}/import/${source}${qs ? '?' + qs : ''}`, file)
  },
  rates: (f = {}) => request('GET', `/api/rates${query(f)}`),
  saveRate: (r) => request('PUT', '/api/rates', r),
  updateRates: () => request('POST', '/api/rates/update'),
  deleteRate: (currency, date) => request('DELETE', `/api/rates/${currency}/${date}`),
  aiSettings: () => request('GET', '/api/ai/settings'),
  saveAISettings: (s) => request('PUT', '/api/ai/settings', s),
  // PDF alebo fotka prijatej faktúry → koncept na kontrolu (Claude API)
  einvoiceSettings: (cid) => request('GET', `/api/companies/${cid}/einvoice-settings`),
  saveEInvoiceSettings: (cid, s) => request('PUT', `/api/companies/${cid}/einvoice-settings`, s),
  sendEInvoice: (cid, id) => request('POST', `/api/companies/${cid}/invoices/${id}/einvoice`),
  einvoices: (cid, filter = {}) => request('GET', `/api/companies/${cid}/einvoices` + query(filter)),
  einvoiceInbox: (cid) => request('GET', `/api/companies/${cid}/einvoice-inbox`),
  takeEInvoice: (cid, msg) => request('POST', `/api/companies/${cid}/einvoice-inbox/${encodeURIComponent(msg)}/take`),
  reopenEInvoice: (cid, id) => request('POST', `/api/companies/${cid}/einvoices/${id}/reopen`),
  importUBL: (cid, file) => request('POST', `/api/companies/${cid}/purchases/import-ubl${query({ name: file.name })}`, file),
  extractPurchase: (cid, file) => request('POST', `/api/companies/${cid}/purchases/extract${query({ name: file.name })}`, file),
  legal: () => request('GET', '/api/legal'),
  saveLegal: (p) => request('PUT', '/api/legal', p),
  resetLegal: () => request('POST', '/api/legal/reset'),
  updateECBRates: () => request('POST', '/api/legal/ecb'),
  reminders: (cid, year) => request('GET', `/api/companies/${cid}/reminders` + (year ? `?year=${year}` : '')),
  reminder: (cid, id) => request('GET', `/api/companies/${cid}/reminders/${id}`),
  previewReminder: (cid, req) => request('POST', `/api/companies/${cid}/reminders/preview`, req),
  createReminder: (cid, req) => request('POST', `/api/companies/${cid}/reminders`, req),
  deleteReminder: (cid, id) => request('DELETE', `/api/companies/${cid}/reminders/${id}`),
  paymentCandidates: (cid) => request('GET', `/api/companies/${cid}/purchases/payment-order`),
  paymentOrder: (cid, ids, date, fetchImpl = globalThis.fetch) =>
    download(`/api/companies/${cid}/purchases/payment-order`, 'prikaz.xml', fetchImpl, { ids, date }),
  payPurchase: (cid, id) => request('POST', `/api/companies/${cid}/purchases/${id}/pay`),
  unpayPurchase: (cid, id) => request('POST', `/api/companies/${cid}/purchases/${id}/unpay`),
  deletePurchase: (cid, id) => request('DELETE', `/api/companies/${cid}/purchases/${id}`),
}
