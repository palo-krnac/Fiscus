// Desktop z inštalátora: otvorené okno sa ozýva programu (inak sa program
// po zatvorení posledného okna sám ukončí) a dá sa ukončiť tlačidlom.

/** Ozýva sa hneď, potom pravidelne a pri návrate na kartu. Vráti funkciu na zastavenie. */
export function startHeartbeat(ping, { every = 30000, win = globalThis.window, doc = globalThis.document } = {}) {
  const beat = () => ping().catch(() => {})
  beat()
  const timer = win.setInterval(beat, every)
  const onVisible = () => doc.visibilityState === 'visible' && beat()
  doc.addEventListener('visibilitychange', onVisible)
  return () => {
    win.clearInterval(timer)
    doc.removeEventListener('visibilitychange', onVisible)
  }
}
