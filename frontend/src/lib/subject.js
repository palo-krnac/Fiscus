// Predmet dokladu v zoznamoch: na širokej obrazovke má vlastný stĺpec,
// na užšej je za menom odberateľa a na mobile v druhom riadku pod ním.
// Rozmery sú zladené so styles.css (.party-cell).

export const wideQuery = '(min-width: 1360px)'
export const stackedRowHeight = 44

// rowHeight: na mobile majú riadky dva riadky textu (súčtový riadok nie).
export function rowHeight(narrow) {
  return (p) => (narrow.matches && !p.data?._totals ? stackedRowHeight : undefined)
}

// partyCell vytvorí obsah bunky Odberateľ — text, nie HTML, aby sa mená
// s „<“ alebo „&“ nezobrazili zle.
export function partyCell(name, subject) {
  const box = document.createElement('span')
  box.className = 'party-cell'
  const n = document.createElement('span')
  n.className = 'party-name'
  n.textContent = name || ''
  box.append(n)
  if (subject) {
    const s = document.createElement('span')
    s.className = 'party-subject'
    s.textContent = subject
    box.append(s)
    box.title = `${name || ''} — ${subject}` // celý text pri skrátení
  }
  return box
}

// subjectColumn je stĺpec Predmet pre AG Grid.
export const subjectColumn = {
  colId: 'subject',
  headerName: 'Predmet',
  valueGetter: (p) => p.data.subject || '',
  tooltipValueGetter: (p) => p.data.subject || '',
  flex: 1.2,
  minWidth: 140,
  cellClass: 'cell-subject',
}
