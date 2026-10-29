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

export async function request(method, path, body, fetchImpl = globalThis.fetch) {
  const init = { method, headers: {} }
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }
  let res
  try {
    res = await fetchImpl(path, init)
  } catch {
    throw new ApiError(0, 'Server neodpovedá. Skontrolujte, či program Fiskus beží.')
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
    throw new ApiError(res.status, data?.error ?? `Chyba servera (${res.status})`, data?.fields ?? [])
  }
  return data
}

const inv = (cid) => `/api/companies/${cid}/invoices`

export const api = {
  capabilities: () => request('GET', '/api/capabilities'),
  companies: () => request('GET', '/api/companies'),
  createCompany: (c) => request('POST', '/api/companies', c),

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
  preview: (invoice) => request('POST', '/api/invoices/preview', invoice),
  issue: (cid, id) => request('POST', `${inv(cid)}/${id}/issue`),
  pay: (cid, id, date) => request('POST', `${inv(cid)}/${id}/pay`, date ? { date } : undefined),
  cancel: (cid, id) => request('POST', `${inv(cid)}/${id}/cancel`),
}
