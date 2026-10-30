// Cenové ponuky: stavy a povolené akcie. Ponuka nie je daňový doklad.

const STATES = {
  draft: { label: 'Koncept', tone: 'draft' },
  issued: { label: 'Odoslaná', tone: 'issued' },
  accepted: { label: 'Prijatá', tone: 'paid' },
  rejected: { label: 'Odmietnutá', tone: 'cancelled' },
  invoiced: { label: 'Vyfakturovaná', tone: 'paid' },
}

/** Stav ponuky vrátane uplynutej platnosti (backend posiela `expired`). */
export function quoteStatusOf(q) {
  if (q.expired) return { label: 'Po platnosti', tone: 'overdue' }
  return STATES[q.status] ?? { label: q.status, tone: 'draft' }
}

/** Akcie, ktoré dovoľuje stav ponuky (rovnaké pravidlá ako v backende). */
export function quoteActions(q) {
  const s = q?.status ?? 'draft'
  return {
    edit: s === 'draft',
    issue: s === 'draft',
    accept: s === 'issued' || s === 'rejected',
    reject: s === 'issued' || s === 'accepted',
    invoice: s === 'issued' || s === 'accepted',
    copy: !!q?.id,
  }
}

/** Filtre zoznamu ponúk. */
export const quoteViews = [
  { label: 'Všetky', value: 'all' },
  { label: 'Koncepty', value: 'draft' },
  { label: 'Odoslané', value: 'open' },
  { label: 'Prijaté', value: 'accepted' },
  { label: 'Vyfakturované', value: 'invoiced' },
  { label: 'Odmietnuté a po platnosti', value: 'lost' },
]

export function matchesQuoteView(q, view) {
  switch (view) {
    case 'draft': return q.status === 'draft'
    case 'open': return q.status === 'issued' && !q.expired
    case 'accepted': return q.status === 'accepted'
    case 'invoiced': return q.status === 'invoiced'
    case 'lost': return q.status === 'rejected' || !!q.expired
    default: return true
  }
}

/** Dni platnosti medzi dátumom ponuky a koncom platnosti (ISO dátumy). */
export function validDays(issueDate, validUntil) {
  if (!issueDate || !validUntil) return null
  return Math.round((Date.parse(validUntil) - Date.parse(issueDate)) / 86400000)
}

const dt = new Intl.DateTimeFormat('sk-SK', { day: 'numeric', month: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })

/** Stav odkazu pre zákazníka: otvorenie a odpoveď (texty do panela ponuky). */
export function shareInfo(share) {
  if (!share) return null
  const r = share.response
  return {
    viewed: share.viewed ? `Zákazník ponuku otvoril ${dt.format(new Date(share.viewed))}.` : 'Zákazník odkaz ešte neotvoril.',
    response: r ? `${r.accepted ? 'Prijal' : 'Odmietol'} ${r.name}, ${dt.format(new Date(r.at))}.` : '',
    order: r?.order ? `Objednávka ${r.order}` : '',
    note: r?.note ?? '',
  }
}

/** Zákazník môže na verejnej stránke odpovedať (server posiela canRespond). */
export const publicVerdict = (v) =>
  v.response ? (v.response.accepted ? 'accepted' : 'rejected') : v.status === 'accepted' ? 'accepted' : v.status === 'rejected' ? 'rejected' : v.status === 'invoiced' ? 'accepted' : v.expired ? 'expired' : 'open'
