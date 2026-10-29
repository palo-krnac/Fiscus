import { describe, expect, it, vi } from 'vitest'
import { startHeartbeat } from './desktop.js'

describe('startHeartbeat', () => {
  it('ozve sa hneď, pravidelne a pri návrate na kartu; zastaví sa', async () => {
    vi.useFakeTimers()
    const ping = vi.fn(() => Promise.resolve())
    const doc = { visibilityState: 'visible', listeners: {}, addEventListener(e, f) { this.listeners[e] = f }, removeEventListener(e) { delete this.listeners[e] } }
    const stop = startHeartbeat(ping, { every: 1000, win: globalThis, doc })
    expect(ping).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(3000)
    expect(ping).toHaveBeenCalledTimes(4)
    doc.listeners.visibilitychange()
    expect(ping).toHaveBeenCalledTimes(5)
    doc.visibilityState = 'hidden'
    doc.listeners.visibilitychange()
    expect(ping).toHaveBeenCalledTimes(5)
    stop()
    vi.advanceTimersByTime(5000)
    expect(ping).toHaveBeenCalledTimes(5)
    expect(doc.listeners.visibilitychange).toBeUndefined()
    vi.useRealTimers()
  })
  it('chyba spojenia nevadí', async () => {
    const stop = startHeartbeat(() => Promise.reject(new Error('x')), { every: 1e9 })
    stop()
  })
})
