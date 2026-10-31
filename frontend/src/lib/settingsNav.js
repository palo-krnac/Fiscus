// Nastavenia po sekciách: menu vľavo (na mobile hamburger). Sekcie firmy
// sú časti stránky Nastavenia (/nastavenia/…), ostatné sú samostatné
// obrazovky správy, ktoré sa v menu programu zobrazujú pod Nastaveniami.

export const settingsGroups = [
  {
    group: 'Firma',
    items: [
      { path: '/nastavenia/firma', label: 'Údaje a logo', icon: 'pi-id-card', perm: 'settings.write' },
      { path: '/nastavenia/dph', label: 'DPH', icon: 'pi-percentage', perm: 'settings.write' },
      { path: '/nastavenia/doklady', label: 'Doklady a platby', icon: 'pi-file', perm: 'settings.write' },
      { path: '/nastavenia/e-faktury', label: 'E-faktúry', icon: 'pi-send', perm: 'settings.write' },
      { path: '/nastavenia/e-mail', label: 'E-mail (SMTP, texty)', icon: 'pi-envelope', perm: 'settings.write' },
      { path: '/nastavenia/kalendar', label: 'Kalendár', icon: 'pi-calendar', perm: 'settings.write' },
    ],
  },
  {
    group: 'Prístup',
    items: [
      { path: '/nastavenia/bezpecnost', label: 'Bezpečnosť', icon: 'pi-lock', perm: 'settings.write' },
      { path: '/pouzivatelia', label: 'Používatelia', icon: 'pi-users', perm: 'users.manage', serverOnly: true },
      { path: '/ai', label: 'Prístup pre AI', icon: 'pi-sparkles' },
      { path: '/firmy', label: 'Firmy a licencia', icon: 'pi-building', superAdmin: true, serverOnly: true },
    ],
  },
  {
    group: 'Dáta',
    items: [
      { path: '/prenos', label: 'Prenos dát', icon: 'pi-file-import', perm: 'invoices.write' },
      { path: '/nastavenia/e-shopy', label: 'E-shopy', icon: 'pi-shopping-cart', perm: 'settings.write' },
      { path: '/zalohy', label: 'Zálohy', icon: 'pi-history', superAdmin: true },
    ],
  },
  {
    group: 'Číselníky',
    items: [
      { path: '/kurzy', label: 'Kurzy mien', icon: 'pi-euro', perm: 'invoices.read' },
      { path: '/legislativa', label: 'Legislatíva', icon: 'pi-verified', superAdmin: true },
    ],
  },
]

/** Sekcie stránky Nastavenia (časť adresy za /nastavenia/). */
export const settingsSections = settingsGroups
  .flatMap((g) => g.items)
  .filter((i) => i.path.startsWith('/nastavenia/'))
  .map((i) => i.path.slice('/nastavenia/'.length))

/** Cesty obrazoviek, ktoré patria pod Nastavenia (pre menu programu). */
export const settingsPaths = settingsGroups.flatMap((g) => g.items).filter((i) => !i.external).map((i) => i.path)

/** Viditeľné skupiny podľa práv (allowed(item) → bool); prázdne skupiny vynechá. */
export function visibleSettings(allowed) {
  return settingsGroups.map((g) => ({ ...g, items: g.items.filter(allowed) })).filter((g) => g.items.length)
}

/** Patrí cesta pod Nastavenia? */
export function isSettingsPath(path) {
  return path === '/nastavenia' || settingsPaths.some((p) => path === p || path.startsWith(p + '/'))
}

/** Aktuálna položka menu Nastavení pre cestu (null mimo nastavení). */
export function currentSetting(path) {
  return settingsGroups.flatMap((g) => g.items).find((i) => !i.external && (path === i.path || path.startsWith(i.path + '/'))) ?? null
}

/** Sekcia formulára firmy, v ktorej je pole (pre chyby z uloženia). */
export function sectionOfField(key) {
  if (/^(vat|icdph|taxOffice)/.test(key)) return 'dph'
  if (key === 'bookkeeping') return 'firma'
  if (/^(numberFormats|units|iban|defaultDueDays)/.test(key)) return 'doklady'
  return 'firma'
}

/** Kam prejsť po chybe uloženia: aktuálna sekcia, ak má chybu, inak prvá s chybou. */
export function errorSection(current, keys) {
  const secs = keys.map(sectionOfField)
  return secs.length === 0 || secs.includes(current) ? current : secs[0]
}
