import { describe, expect, it } from 'vitest'
import { currentSetting, errorSection, isSettingsPath, sectionOfField, settingsSections, visibleSettings } from './settingsNav.js'

describe('nastavenia po sekciách', () => {
  it('sekcie stránky Nastavenia', () => {
    expect(settingsSections).toEqual(['firma', 'dph', 'doklady', 'e-faktury', 'e-mail', 'kalendar', 'bezpecnost', 'e-shopy'])
  })
  it.each([
    ['/nastavenia', true],
    ['/nastavenia/dph', true],
    ['/zalohy', true],
    ['/pouzivatelia', true],
    ['/emaily', false], // e-maily ostávajú v Predaji
    ['/faktury', false],
  ])('%s patrí pod nastavenia: %s', (p, want) => expect(isSettingsPath(p)).toBe(want))
  it('aktuálna položka', () => {
    expect(currentSetting('/nastavenia/dph').label).toBe('DPH')
    expect(currentSetting('/kurzy').label).toBe('Kurzy mien')
    expect(currentSetting('/faktury')).toBeNull()
  })
  it('podľa práv sa skryjú položky aj prázdne skupiny', () => {
    const g = visibleSettings((i) => !i.superAdmin && !i.serverOnly && (!i.perm || i.perm === 'invoices.read'))
    expect(g.map((x) => x.group)).toEqual(['Prístup', 'Číselníky'])
    expect(g.flatMap((x) => x.items.map((i) => i.label))).toEqual(['Prístup pre AI', 'Kurzy mien'])
  })
})

describe('chyby uloženia', () => {
  it.each([
    ['icdph', 'dph'],
    ['vatPeriod', 'dph'],
    ['numberFormats.invoice', 'doklady'],
    ['iban', 'doklady'],
    ['name', 'firma'],
  ])('%s je v sekcii %s', (key, sec) => expect(sectionOfField(key)).toBe(sec))

  it.each([
    ['firma', ['name', 'iban'], 'firma'],
    ['firma', ['iban'], 'doklady'],
    ['dph', [], 'dph'],
  ])('z %s pri %j → %s', (cur, keys, want) => expect(errorSection(cur, keys)).toBe(want))
})
