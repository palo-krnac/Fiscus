import { describe, expect, it, vi } from 'vitest'
import { ApiError, request } from './api.js'

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
})
