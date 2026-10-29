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
export async function download(path, fallbackName, fetchImpl = globalThis.fetch) {
  const res = await fetchImpl(path, { credentials: 'same-origin' })
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
  backupURL: (name) => `/api/backups/${encodeURIComponent(name)}`,
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
  setJournalOpening: (cid, year, bank) => request('PUT', `/api/companies/${cid}/journal/opening`, { year, bank }),
  openItems: (cid, date) => request('GET', `/api/companies/${cid}/open-items` + query({ date })),
  resetJournal: (cid, key) => request('DELETE', `/api/companies/${cid}/journal/${encodeURIComponent(key)}`),
  cashBook: (cid, year) => request('GET', `/api/companies/${cid}/cash` + query({ year })),
  cash: (cid, id) => request('GET', `/api/companies/${cid}/cash/${id}`),
  cashCandidates: (cid, kind) => request('GET', `/api/companies/${cid}/cash/candidates` + query({ kind })),
  createCash: (cid, d) => request('POST', `/api/companies/${cid}/cash`, d),
  updateCash: (cid, id, d) => request('PUT', `/api/companies/${cid}/cash/${id}`, d),
  cancelCash: (cid, id) => request('POST', `/api/companies/${cid}/cash/${id}/cancel`),
  bank: (cid, { year, status } = {}) => request('GET', `/api/companies/${cid}/bank` + query({ year, status })),
  importBank: (cid, file) => request('POST', `/api/companies/${cid}/bank/import` + query({ name: file.name }), file),
  bankSuggestions: (cid, id) => request('GET', `/api/companies/${cid}/bank/${id}/suggestions`),
  bankMatch: (cid, id, kind, docId) => request('POST', `/api/companies/${cid}/bank/${id}/match`, { kind, id: docId }),
  bankUnmatch: (cid, id) => request('POST', `/api/companies/${cid}/bank/${id}/unmatch`),
  bankIgnore: (cid, id, ignore, note) => request('POST', `/api/companies/${cid}/bank/${id}/ignore`, { ignore, note }),

  vat: (cid, period, excess) => request('GET', `/api/companies/${cid}/vat` + query({ period, excess })),
  /** XML pre eDane ako Blob (kind: 'dph' | 'kv'). */
  vatXML: (cid, kind, period, excess, fetchImpl = globalThis.fetch) =>
    download(`/api/companies/${cid}/vat/${kind}.xml` + query({ period, excess }), `${kind}.xml`, fetchImpl),
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
  recurringList: (cid) => request('GET', `/api/companies/${cid}/recurring`),
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
  reminders: (cid, year) => request('GET', `/api/companies/${cid}/reminders` + (year ? `?year=${year}` : '')),
  reminder: (cid, id) => request('GET', `/api/companies/${cid}/reminders/${id}`),
  previewReminder: (cid, req) => request('POST', `/api/companies/${cid}/reminders/preview`, req),
  createReminder: (cid, req) => request('POST', `/api/companies/${cid}/reminders`, req),
  deleteReminder: (cid, id) => request('DELETE', `/api/companies/${cid}/reminders/${id}`),
  payPurchase: (cid, id) => request('POST', `/api/companies/${cid}/purchases/${id}/pay`),
  unpayPurchase: (cid, id) => request('POST', `/api/companies/${cid}/purchases/${id}/unpay`),
  deletePurchase: (cid, id) => request('DELETE', `/api/companies/${cid}/purchases/${id}`),
}
