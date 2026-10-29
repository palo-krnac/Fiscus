import { describe, expect, it, vi } from 'vitest'
import { ApiError, api, query, request, setUnauthorizedHandler } from './api.js'

const respond = (status, body) =>
  vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(body === undefined ? '' : JSON.stringify(body)),
  })

describe('request', () => {
  it('pošle JSON a vráti dáta', async () => {
    const f = respond(200, { id: 1 })
    const out = await request('POST', '/api/x', { a: 1 }, f)
    expect(out).toEqual({ id: 1 })
    const [, init] = f.mock.calls[0]
    expect(init.headers['Content-Type']).toBe('application/json')
    expect(JSON.parse(init.body)).toEqual({ a: 1 })
  })

  it('chyby polí prevedie na ApiError', async () => {
    const f = respond(422, { error: 'Doklad obsahuje chyby', fields: [{ field: 'customer.name', message: 'Zadajte odberateľa' }] })
    const err = await request('POST', '/api/x', {}, f).catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(422)
    expect(err.fieldMap()).toEqual({ 'customer.name': 'Zadajte odberateľa' })
  })

  it('výpadok servera má zrozumiteľnú správu', async () => {
    const f = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    const err = await request('GET', '/api/x', undefined, f).catch((e) => e)
    expect(err.status).toBe(0)
    expect(err.message).toMatch(/Server neodpovedá/)
  })

  it('prázdna odpoveď nie je chyba', async () => {
    expect(await request('POST', '/api/x', undefined, respond(204))).toBeNull()
  })

  it('zmeny posielajú hlavičku proti CSRF, čítanie nie', async () => {
    const f = respond(200, {})
    await request('POST', '/api/x', {}, f)
    await request('GET', '/api/x', undefined, f)
    expect(f.mock.calls[0][1].headers['X-Fiskus']).toBe('1')
    expect(f.mock.calls[1][1].headers['X-Fiskus']).toBeUndefined()
  })

  it('401 zavolá obsluhu prihlásenia, okrem samotného prihlasovania', async () => {
    const h = vi.fn()
    setUnauthorizedHandler(h)
    await request('GET', '/api/companies', undefined, respond(401, { error: 'x' })).catch(() => {})
    await request('POST', '/api/auth/login', {}, respond(401, { error: 'x' })).catch(() => {})
    expect(h).toHaveBeenCalledTimes(1)
    setUnauthorizedHandler(null)
  })
})

describe('súbor', () => {
  it('Blob sa posiela bez prevodu na JSON', async () => {
    const f = respond(200, {})
    const file = new Blob(['<Document/>'], { type: 'text/xml' })
    await request('POST', '/api/x', file, f)
    const [, init] = f.mock.calls[0]
    expect(init.body).toBe(file)
    expect(init.headers['Content-Type']).toBe('text/xml')
    expect(init.headers['X-Fiskus']).toBe('1')
  })
})

describe('query', () => {
  it('vynechá prázdne hodnoty', () => {
    expect(query({ category: 'invoices', q: '', user: 0, from: undefined })).toBe('?category=invoices&user=0')
    expect(query({})).toBe('')
  })
})

describe('pdfURL', () => {
  it.each([
    [[3, 'invoices', 7], '/api/companies/3/invoices/7/pdf'],
    [[3, 'quotes', 2, true], '/api/companies/3/quotes/2/pdf?inline=1'],
    [[1, 'reminders', 5], '/api/companies/1/reminders/5/pdf'],
  ])('%j → %s', (args, want) => {
    expect(api.pdfURL(...args)).toBe(want)
  })
})
