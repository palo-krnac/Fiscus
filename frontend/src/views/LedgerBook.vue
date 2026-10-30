<script setup>
// Účtovný denník a hlavná kniha (podvojné účtovníctvo). Doklady zaúčtuje
// server podľa predkontácií; tu sa predkontácia mení, zápis zadáva ručne,
// pridávajú interné doklady a počiatočné stavy.
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import DatePicker from 'primevue/datepicker'
import Drawer from 'primevue/drawer'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import LedgerLines from '../components/LedgerLines.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { dimColumns, ledgerStatementColumns, postingColumns, turnoverColumns } from '../lib/columns.js'
import { matches, searchTerms } from '../lib/search.js'
import { accountDisplay, docLabels } from '../lib/chart.js'
import { accountLines, apiLines, editLines, filterPostings, filterTurnovers, openingLines, parseAmount, postingFilters, postingRows, sideOf } from '../lib/ledger.js'

const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))

const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
const year = ref(thisYear)
const tab = ref('journal')
const tabs = [
  { value: 'journal', label: 'Denník' },
  { value: 'book', label: 'Hlavná kniha' },
  { value: 'balance', label: 'Súvaha' },
  { value: 'income', label: 'Výsledovka' },
]
const heading = computed(() => ({ journal: 'Účtovný denník', book: 'Hlavná kniha', balance: 'Súvaha', income: 'Výkaz ziskov a strát' })[tab.value])
const statementTab = computed(() => tab.value === 'balance' || tab.value === 'income')
const st = ref(null)
async function loadStatements() {
  if (!statementTab.value) return
  try {
    st.value = await api.ledgerStatements(session.companyId, year.value)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Závierka sa nenačítala', detail: e.message, life: 6000 })
  }
}
// celé eurá: 1 234,00 → 1 234
const euro = (v) => formatAmount(v).replace(/,00$/, '')
// riadok bez sumy ostane prázdny ako vo formulári; súčtové riadky ukazujú aj nulu
const cell = (r, v) => (r.sum || Number(v) ? euro(v) : '')
const statementRows = () =>
  tab.value === 'balance'
    ? [...st.value.assets.map((r) => ({ ...r, statement: 'Súvaha – aktíva' })), ...st.value.liabilities.map((r) => ({ ...r, statement: 'Súvaha – pasíva' }))]
    : st.value.income.map((r) => ({ ...r, statement: 'Výkaz ziskov a strát' }))
const grandRows = ['01', '24']
const book = ref(null)
const accounts = ref([])
const centers = ref([])
const jobs = ref([])
const search = ref('')
const filter = ref('all')
const month = ref(0)
const months = [{ value: 0, label: 'Celý rok' }, ...['január', 'február', 'marec', 'apríl', 'máj', 'jún', 'júl', 'august', 'september', 'október', 'november', 'december'].map((label, i) => ({ value: i + 1, label }))]
const level = ref('synthetic')
const levels = [
  { value: 'synthetic', label: 'Syntetické' },
  { value: 'analytic', label: 'Analytické' },
  { value: 'dims', label: 'Strediská a zákazky' },
]
const dimRows = computed(() => {
  if (!book.value) return []
  const terms = searchTerms(search.value)
  return book.value.dimTurnovers.filter((d) => matches(terms, [d.code, d.name, d.kind === 'center' ? 'stredisko' : 'zákazka']))
})
const centerOptions = computed(() => [{ value: '', label: 'Bez strediska' }, ...centers.value.map((c) => ({ value: c.code, label: `${c.code} – ${c.name}` }))])
const jobOptions = computed(() => [{ value: '', label: 'Bez zákazky' }, ...jobs.value.map((j) => ({ value: j.code, label: `${j.code} – ${j.name}` }))])
const hasDims = computed(() => centers.value.length + jobs.value.length > 0)

async function load() {
  try {
    const [b, c] = await Promise.all([api.ledger(session.companyId, year.value), api.chart(session.companyId)])
    book.value = b
    accounts.value = c.accounts
    centers.value = c.centers.filter((x) => x.active)
    jobs.value = c.jobs.filter((x) => x.active)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Denník sa nenačítal', detail: e.message, life: 6000 })
  }
}
onMounted(load)
watch([year, () => session.companyId], load)
watch(tab, () => {
  search.value = ''
  loadStatements()
})
watch(book, loadStatements)

const postings = computed(() => (book.value ? filterPostings(book.value.postings, { search: search.value, filter: filter.value, month: month.value }) : []))
const exportPostings = () => postingRows(postings.value)
const turnovers = computed(() => (book.value ? filterTurnovers(level.value === 'synthetic' ? book.value.synthetic : book.value.accounts, search.value) : []))
const accountOptions = computed(() => accounts.value.filter((a) => a.active).map((a) => ({ value: a.number, label: `${accountDisplay(a.number)} ${a.name}` })))
const amt = (v) => (Number(v) ? formatAmount(v) : '')

// ---- zaúčtovanie dokladu ----
const open = ref(false)
const mode = ref('') // posting | internal | opening | account
const current = ref(null)
const how = ref('template')
const hows = [
  { value: 'template', label: 'Predkontácia' },
  { value: 'manual', label: 'Ručne' },
]
const form = ref({})
const lines = ref([])
const errors = ref({})
const busy = ref(false)
const templateOptions = computed(() => {
  const doc = mode.value === 'internal' ? 'internal' : current.value?.doc
  return (book.value?.templates ?? []).filter((t) => t.doc === doc).map((t) => ({ value: t.code, label: `${t.code} – ${t.name}` }))
})
const drawerTitle = computed(() => {
  if (mode.value === 'opening') return `Počiatočné stavy k 1. 1. ${year.value}`
  if (mode.value === 'money') return 'Účty bánk a pokladníc'
  if (mode.value === 'fx') return `Kurzové rozdiely k 31. 12. ${year.value}`
  if (mode.value === 'account') return `Účet ${accountDisplay(current.value?.account)}`
  if (mode.value === 'internal') return current.value?.id ? `Interný doklad ${current.value.number}` : 'Nový interný doklad'
  return `Zaúčtovanie ${current.value?.number || 'dokladu'}`
})

function show(m, item) {
  mode.value = m
  errors.value = {}
  current.value = item
  open.value = true
}

function openPosting(p) {
  if (p.doc === 'internal') {
    const id = Number(p.key.split(':')[1])
    const doc = book.value.internal.find((d) => d.id === id)
    return openInternal(doc)
  }
  how.value = p.manual ? 'manual' : 'template'
  form.value = { template: p.template || null, note: p.note || '', center: p.center || '', job: p.job || '' }
  lines.value = editLines(p.manual ? p.lines : [])
  show('posting', p)
}

function openInternal(doc) {
  const d = doc ?? { id: 0, number: '', date: `${year.value}-12-31`, text: '', template: '', amount: '', lines: [] }
  how.value = d.template || !d.id ? 'template' : 'manual'
  form.value = { date: fromISODate(d.date), text: d.text, number: d.number, template: d.template || null, amount: Number(d.amount) ? String(d.amount) : '', center: d.center || '', job: d.job || '' }
  lines.value = editLines(d.lines)
  show('internal', d)
}

// kurzové rozdiely k 31. 12.
const fxr = ref(null)
async function openFX() {
  try {
    fxr.value = await api.fxRevaluation(session.companyId, year.value)
    show('fx', null)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  }
}

// analytické účty bánk a pokladníc
const moneyForm = ref([])
function openMoney() {
  moneyForm.value = book.value.moneySources.map((m) => ({ ...m }))
  show('money', null)
}
const moneyOptions = (key) => [
  { value: '', label: 'Podľa predkontácie (syntetický účet)' },
  ...accountOptions.value.filter((o) => o.value.startsWith(key.startsWith('bank:') ? '22' : '21') && o.value.length > 3),
]

function openOpening() {
  lines.value = openingLines(book.value.opening)
  show('opening', null)
}

function openAccount(t) {
  show('account', { ...t, lines: accountLines(book.value.postings, t.account) })
}

async function save() {
  busy.value = true
  errors.value = {}
  const cid = session.companyId
  try {
    if (mode.value === 'posting') {
      const dims = { center: form.value.center || '', job: form.value.job || '' }
      const body = how.value === 'manual' ? { lines: apiLines(lines.value), note: form.value.note, ...dims } : { template: form.value.template ?? '', ...dims }
      if (how.value === 'template' && !body.template) {
        errors.value = { template: 'Vyberte predkontáciu' }
        return
      }
      await api.setPosting(cid, year.value, current.value.key, body)
    } else if (mode.value === 'internal') {
      const f = form.value
      const doc = { id: current.value.id, number: f.number, date: toISODate(f.date), text: f.text, center: f.center || '', job: f.job || '' }
      if (how.value === 'template') Object.assign(doc, { template: f.template ?? '', amount: parseAmount(f.amount) || '0' })
      else doc.lines = apiLines(lines.value)
      if (how.value === 'template' && !doc.template) {
        errors.value = { template: 'Vyberte predkontáciu' }
        return
      }
      await api.saveInternal(cid, year.value, doc)
    } else if (mode.value === 'fx') {
      const doc = await api.postFXRevaluation(cid, year.value)
      toast.add({ severity: 'success', summary: 'Kurzové rozdiely zaúčtované', detail: doc.number, life: 4000 })
    } else if (mode.value === 'money') {
      await api.setMoneyAccounts(cid, Object.fromEntries(moneyForm.value.map((m) => [m.key, m.account || ''])))
    } else if (mode.value === 'opening') {
      await api.setLedgerOpening(cid, year.value, apiLines(lines.value))
    }
    open.value = false
    await load()
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) errors.value = e.fieldMap()
    else toast.add({ severity: 'error', summary: 'Neuložilo sa', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

// počiatočné stavy z konečných stavov minulého roka (výsledok na 431)
async function takeOpening() {
  busy.value = true
  try {
    const ls = await api.openingProposal(session.companyId, year.value)
    if (!ls.length) toast.add({ severity: 'info', summary: `Rok ${year.value - 1} nemá v programe zostatky`, life: 4000 })
    else lines.value = editLines(ls)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

async function resetPosting() {
  try {
    await api.setPosting(session.companyId, year.value, current.value.key, { center: form.value.center || '', job: form.value.job || '' })
    open.value = false
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  }
}

function removeInternal() {
  confirm.require({
    header: 'Zmazať interný doklad',
    message: `Zmazať doklad ${current.value.number}?`,
    acceptLabel: 'Zmazať',
    rejectLabel: 'Späť',
    acceptProps: { severity: 'danger', size: 'small' },
    rejectProps: { severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.deleteInternal(session.companyId, year.value, current.value.id)
        open.value = false
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
      }
    },
  })
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>{{ heading }}</h1>
      <SelectButton v-model="tab" :options="tabs" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobrazenie" />
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <template v-if="canWrite && book">
        <Button label="Banky a pokladnice" icon="pi pi-wallet" size="small" severity="secondary" outlined @click="openMoney" />
        <Button label="Počiatočné stavy" icon="pi pi-flag" size="small" severity="secondary" outlined @click="openOpening" />
        <Button label="Interný doklad" icon="pi pi-plus" size="small" @click="openInternal(null)" />
      </template>
    </header>

    <template v-if="book">
      <section class="summary" aria-label="Obratová predvaha">
        <div class="cell">
          <span class="label">Obraty MD</span>
          <span class="value">{{ formatMoney(book.totals.debit) }}</span>
        </div>
        <div class="cell">
          <span class="label">Obraty D</span>
          <span class="value">{{ formatMoney(book.totals.credit) }}</span>
        </div>
        <div class="cell">
          <span class="label">Predvaha</span>
          <span class="value" :class="book.totals.balanced ? 'ok' : 'bad'">{{ book.totals.balanced ? 'vyrovnaná' : 'nevyrovnaná' }}</span>
        </div>
        <div class="cell">
          <span class="label">Nezaúčtované doklady</span>
          <span class="value" :class="{ bad: book.unposted }">
            {{ book.unposted }}
            <Button v-if="book.unposted && !(tab === 'journal' && filter === 'problem')" label="Zobraziť" text size="small" class="show"
              @click="((tab = 'journal'), (filter = 'problem'))" />
          </span>
        </div>
      </section>

      <div v-if="!statementTab" class="toolbar">
        <IconField class="search">
          <InputIcon class="pi pi-search" />
          <InputText v-model="search" :placeholder="tab === 'journal' ? 'Hľadať doklad, partnera, účet' : 'Hľadať účet'" size="small" aria-label="Hľadať" />
        </IconField>
        <template v-if="tab === 'journal'">
          <Select v-model="filter" :options="postingFilters" option-label="label" option-value="value" size="small" class="sel" aria-label="Filter" />
          <Select v-model="month" :options="months" option-label="label" option-value="value" size="small" class="sel-month" aria-label="Mesiac" />
        </template>
        <SelectButton v-else v-model="level" :options="levels" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Úroveň účtov" />
        <div class="spacer" />
        <ExportButton v-if="tab === 'journal'" :name="`Účtovný denník ${year}`" :columns="postingColumns" :rows="exportPostings" />
        <ExportButton v-else-if="level === 'dims'" :name="`Strediská a zákazky ${year}`" :columns="dimColumns" :rows="dimRows" />
        <ExportButton v-else :name="`Hlavná kniha ${year}`" :columns="turnoverColumns" :rows="turnovers" />
      </div>

      <template v-if="statementTab">
        <div v-if="st" class="st-notes">
          <p v-if="st.unposted" class="warn">{{ st.unposted }} dokladov nie je zaúčtovaných — závierka z nich nevychádza.</p>
          <p v-if="!st.balanced" class="warn">Hlavná kniha nie je vyrovnaná (MD ≠ D alebo počiatočné stavy).</p>
          <p v-if="tab === 'balance' && Number(st.difference)" class="warn">Aktíva a pasíva sa líšia o {{ euro(st.difference) }} € (zaokrúhlenie alebo nevyrovnané počiatočné stavy).</p>
          <p v-if="st.unmapped.length" class="warn">
            Účty so zostatkom, ktoré nepatria do žiadneho riadku:
            {{ st.unmapped.map((u) => `${accountDisplay(u.account)} ${u.name} (${formatAmount(u.closing)})`).join(', ') }}
          </p>
          <p class="hint">
            Mikro účtovná jednotka, vzory Úč MÚJ 1-01 a 2-01, v celých eurách. Väzbu riadkov na účty nechajte overiť účtovníkovi.
            <template v-if="st.previousEmpty">Predchádzajúce obdobie nemá v programe zápisy.</template>
          </p>
          <div class="toolbar">
            <Button v-if="canWrite" :label="`Kurzové rozdiely k 31. 12. ${year}`" icon="pi pi-euro" size="small" severity="secondary" outlined @click="openFX" />
            <div class="spacer" />
            <ExportButton :name="`${tab === 'balance' ? 'Súvaha' : 'Výkaz ziskov a strát'} ${year}`" :columns="ledgerStatementColumns(tab === 'balance', st.previousEmpty)" :rows="statementRows" />
          </div>
        </div>
        <section v-if="st && tab === 'balance'" class="sheet scroll">
          <table class="list statement">
            <caption>Aktíva</caption>
            <thead><tr><th>r.</th><th>Položka</th><th class="num bk">Brutto</th><th class="num bk">Korekcia</th><th class="num">Netto</th><th v-if="!st.previousEmpty" class="num">Minulé obdobie</th></tr></thead>
            <tbody>
              <tr v-for="r in st.assets" :key="r.code" :class="{ sum: r.sum }" :title="r.accounts">
                <td class="code">{{ r.code }}</td>
                <td>{{ r.label }}</td>
                <td class="num bk">{{ cell(r, r.brutto) }}</td>
                <td class="num bk">{{ cell(r, r.korekcia) }}</td>
                <td class="num"><span :class="{ grand: grandRows.includes(r.code) }">{{ cell(r, r.netto) }}</span></td>
                <td v-if="!st.previousEmpty" class="num">{{ r.sum || Number(r.previous) ? euro(r.previous) : '' }}</td>
              </tr>
            </tbody>
          </table>
          <table class="list statement">
            <caption>Pasíva</caption>
            <thead><tr><th>r.</th><th>Položka</th><th class="num">Bežné obdobie</th><th v-if="!st.previousEmpty" class="num">Minulé obdobie</th></tr></thead>
            <tbody>
              <tr v-for="r in st.liabilities" :key="r.code" :class="{ sum: r.sum }" :title="r.accounts">
                <td class="code">{{ r.code }}</td>
                <td>{{ r.label }}</td>
                <td class="num"><span :class="{ grand: grandRows.includes(r.code) }">{{ cell(r, r.netto) }}</span></td>
                <td v-if="!st.previousEmpty" class="num">{{ r.sum || Number(r.previous) ? euro(r.previous) : '' }}</td>
              </tr>
            </tbody>
          </table>
        </section>
        <section v-else-if="st" class="sheet scroll">
          <table class="list statement">
            <thead><tr><th>r.</th><th>Položka</th><th class="num">Bežné obdobie</th><th v-if="!st.previousEmpty" class="num">Minulé obdobie</th></tr></thead>
            <tbody>
              <tr v-for="r in st.income" :key="r.code" :class="{ sum: r.sum }" :title="r.accounts">
                <td class="code">{{ r.code }}</td>
                <td>{{ r.label }}</td>
                <td class="num"><span :class="{ grand: r.code === '38' }">{{ cell(r, r.netto) }}</span></td>
                <td v-if="!st.previousEmpty" class="num">{{ r.sum || Number(r.previous) ? euro(r.previous) : '' }}</td>
              </tr>
            </tbody>
          </table>
        </section>
      </template>

      <section v-else-if="tab === 'journal'" class="sheet journal">
        <div class="jhead" aria-hidden="true"><span>Účet</span><span class="num">MD</span><span class="num">D</span></div>
        <p v-if="!postings.length" class="empty">{{ book.postings.length ? 'Nič sa nenašlo.' : `V roku ${year} zatiaľ nie sú žiadne doklady.` }}</p>
        <article v-for="p in postings" :key="p.key" class="post" :class="{ problem: p.problem }" tabindex="0"
          @click="canWrite && openPosting(p)" @keydown.enter="canWrite && openPosting(p)">
          <div class="post-head">
            <span class="date">{{ formatDate(p.date) }}</span>
            <span class="doc"><strong>{{ p.number || '—' }}</strong> <span class="kind">{{ docLabels[p.doc] }}</span></span>
            <span class="text">{{ [p.partner, p.text].filter(Boolean).join(' · ') }}</span>
            <span class="tags">
              <span v-if="p.center" class="tag dim" title="Stredisko">{{ p.center }}</span>
              <span v-if="p.job" class="tag dim" title="Zákazka">{{ p.job }}</span>
              <span class="tag" :class="{ muted: !p.manual }">{{ p.manual ? 'ručne' : p.template }}</span>
            </span>
          </div>
          <p v-if="p.problem" class="post-problem">{{ p.problem }} <span class="amount">{{ formatMoney(p.amount) }}</span></p>
          <div v-for="(l, i) in p.lines" :key="i" class="post-line" :class="{ credit: Number(l.credit) }">
            <span class="acc"><b>{{ accountDisplay(l.account) }}</b> {{ l.name }}</span>
            <span class="num">{{ amt(l.debit) }}</span>
            <span class="num">{{ amt(l.credit) }}</span>
          </div>
        </article>
      </section>

      <section v-else-if="level === 'dims'" class="sheet scroll">
        <table class="list">
          <thead><tr><th>Stredisko / zákazka</th><th class="num">Náklady</th><th class="num">Výnosy</th><th class="num">Výsledok</th></tr></thead>
          <tbody>
            <tr v-if="!dimRows.length">
              <td colspan="4" class="empty">
                {{ book.dimTurnovers.length ? 'Nič sa nenašlo.' : 'Zatiaľ žiadne zápisy so strediskom alebo zákazkou — priraďte ich pri zaúčtovaní dokladu.' }}
              </td>
            </tr>
            <tr v-for="d in dimRows" :key="d.kind + d.code" class="static">
              <td class="acc"><b>{{ d.code }}</b> <span class="name">{{ d.name }} · {{ d.kind === 'center' ? 'stredisko' : 'zákazka' }}</span></td>
              <td class="num">{{ amt(d.costs) }}</td>
              <td class="num">{{ amt(d.revenues) }}</td>
              <td class="num" :class="{ loss: Number(d.result) < 0 }">{{ formatAmount(d.result) }}</td>
            </tr>
          </tbody>
        </table>
      </section>

      <section v-else class="sheet scroll">
        <table class="list book">
          <thead>
            <tr><th>Účet</th><th class="num ps">PS MD</th><th class="num ps">PS D</th><th class="num">Obrat MD</th><th class="num">Obrat D</th><th class="num">KS MD</th><th class="num">KS D</th></tr>
          </thead>
          <tbody>
            <tr v-if="!turnovers.length"><td colspan="7" class="empty">{{ book.accounts.length ? 'Nič sa nenašlo.' : 'Zatiaľ žiadne zápisy.' }}</td></tr>
            <tr v-for="t in turnovers" :key="t.account" tabindex="0" @click="openAccount(t)" @keydown.enter="openAccount(t)">
              <td class="acc"><b>{{ accountDisplay(t.account) }}</b> <span class="name">{{ t.name }}</span></td>
              <td class="num ps">{{ amt(sideOf(t.opening).debit) }}</td>
              <td class="num ps">{{ amt(sideOf(t.opening).credit) }}</td>
              <td class="num">{{ amt(t.debit) }}</td>
              <td class="num">{{ amt(t.credit) }}</td>
              <td class="num">{{ amt(sideOf(t.closing).debit) }}</td>
              <td class="num">{{ amt(sideOf(t.closing).credit) }}</td>
            </tr>
          </tbody>
          <tfoot v-if="!search">
            <tr>
              <td>Spolu</td>
              <td class="num ps">{{ amt(book.totals.openingDebit) }}</td>
              <td class="num ps">{{ amt(book.totals.openingCredit) }}</td>
              <td class="num"><span class="grand">{{ formatAmount(book.totals.debit) }}</span></td>
              <td class="num"><span class="grand">{{ formatAmount(book.totals.credit) }}</span></td>
              <td class="num">{{ amt(book.totals.closingDebit) }}</td>
              <td class="num">{{ amt(book.totals.closingCredit) }}</td>
            </tr>
          </tfoot>
        </table>
      </section>
    </template>

    <Drawer v-model:visible="open" position="right" class="ledger-panel" :header="drawerTitle">
      <!-- zaúčtovanie dokladu -->
      <form v-if="mode === 'posting' && current" class="dform" @submit.prevent="save">
        <dl class="facts">
          <dt>Doklad</dt><dd>{{ docLabels[current.doc] }} {{ current.number }} · {{ formatDate(current.date) }}</dd>
          <dt v-if="current.partner">Partner</dt><dd v-if="current.partner">{{ current.partner }}</dd>
          <dt>Suma</dt><dd class="num-l">{{ formatMoney(current.amount) }}</dd>
        </dl>
        <RouterLink v-if="current.link" :to="current.link" class="doclink"><i class="pi pi-external-link" /> Otvoriť doklad</RouterLink>
        <p v-if="current.problem" class="post-problem">{{ current.problem }}</p>
        <SelectButton v-model="how" :options="hows" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Spôsob zaúčtovania" />
        <template v-if="how === 'template'">
          <FormField label="Predkontácia" :error="errors.template">
            <Select v-model="form.template" :options="templateOptions" option-label="label" option-value="value" size="small" fluid filter :invalid="!!errors.template" />
          </FormField>
          <div v-if="!current.manual && current.lines.length && form.template === current.template" class="preview">
            <div v-for="(l, i) in current.lines" :key="i" class="post-line">
              <span class="acc"><b>{{ accountDisplay(l.account) }}</b> {{ l.name }}</span>
              <span class="num">{{ amt(l.debit) }}</span>
              <span class="num">{{ amt(l.credit) }}</span>
            </div>
          </div>
          <p v-else class="hint">Riadky sa prepočítajú po uložení.</p>
        </template>
        <template v-else>
          <LedgerLines v-model="lines" :accounts="accountOptions" />
          <small v-if="errors.lines" class="err">{{ errors.lines }}</small>
          <FormField label="Poznámka">
            <Textarea v-model="form.note" rows="2" auto-resize />
          </FormField>
        </template>
        <div v-if="hasDims" class="grid2">
          <FormField label="Stredisko" :error="errors.center">
            <Select v-model="form.center" :options="centerOptions" option-label="label" option-value="value" size="small" fluid placeholder="Bez strediska" />
          </FormField>
          <FormField label="Zákazka" :error="errors.job">
            <Select v-model="form.job" :options="jobOptions" option-label="label" option-value="value" size="small" fluid filter placeholder="Bez zákazky" />
          </FormField>
        </div>
        <div class="actions">
          <Button label="Návrh Fiškusa" severity="secondary" text size="small" icon="pi pi-replay" @click="resetPosting" />
          <span class="spacer" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>

      <!-- interný doklad -->
      <form v-else-if="mode === 'internal'" class="dform" @submit.prevent="save">
        <div class="grid2">
          <FormField label="Dátum" :error="errors.date">
            <DatePicker v-model="form.date" date-format="d. m. yy" size="small" fluid :invalid="!!errors.date" />
          </FormField>
          <FormField label="Číslo" hint="Prázdne = pridelí Fiškus">
            <InputText v-model="form.number" size="small" />
          </FormField>
        </div>
        <FormField label="Popis" :error="errors.text">
          <InputText v-model="form.text" size="small" :invalid="!!errors.text" placeholder="Napr. Odpisy za rok" />
        </FormField>
        <SelectButton v-model="how" :options="hows" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Spôsob zaúčtovania" />
        <div v-if="how === 'template'" class="grid2">
          <FormField label="Predkontácia" :error="errors.template">
            <Select v-model="form.template" :options="templateOptions" option-label="label" option-value="value" size="small" fluid :invalid="!!errors.template" />
          </FormField>
          <FormField label="Suma" :error="errors.amount">
            <InputText v-model="form.amount" size="small" inputmode="decimal" class="amt" :invalid="!!errors.amount" />
          </FormField>
        </div>
        <template v-else>
          <LedgerLines v-model="lines" :accounts="accountOptions" />
          <small v-if="errors.lines" class="err">{{ errors.lines }}</small>
        </template>
        <div v-if="hasDims" class="grid2">
          <FormField label="Stredisko" :error="errors.center">
            <Select v-model="form.center" :options="centerOptions" option-label="label" option-value="value" size="small" fluid placeholder="Bez strediska" />
          </FormField>
          <FormField label="Zákazka" :error="errors.job">
            <Select v-model="form.job" :options="jobOptions" option-label="label" option-value="value" size="small" fluid filter placeholder="Bez zákazky" />
          </FormField>
        </div>
        <div class="actions">
          <Button v-if="current?.id" label="Zmazať" severity="danger" text size="small" @click="removeInternal" />
          <span class="spacer" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>

      <!-- kurzové rozdiely k 31. 12. -->
      <form v-else-if="mode === 'fx' && fxr" class="dform" @submit.prevent="save">
        <p class="hint">
          Neuhradené pohľadávky a záväzky v cudzej mene sa k 31. 12. prepočítajú kurzom ECB. Rozdiel je kurzový zisk (663) alebo strata (563);
          prepočítaná suma platí aj pri úhrade v ďalšom roku.
        </p>
        <p v-if="fxr.missing.length" class="warn">Chýba kurz k 31. 12. pre {{ fxr.missing.join(', ') }} — aktualizujte kurzy mien.</p>
        <p v-if="fxr.items.some((it) => it.rateDate < `${year}-12-24`)" class="warn">
          Kurz k 31. 12. {{ year }} ešte nie je vyhlásený — návrh používa posledný známy kurz. Zaúčtujte ho až po skončení roka.
        </p>
        <p v-if="fxr.posted" class="hint">Zaúčtované dokladom {{ fxr.posted }}. Ďalšie zaúčtovanie ho nahradí.</p>
        <p v-if="!fxr.items.length && !fxr.missing.length" class="hint">K 31. 12. {{ year }} nie sú žiadne kurzové rozdiely.</p>
        <div v-for="it in fxr.items" :key="it.key" class="fx-item">
          <span class="text"><b>{{ it.number }}</b> {{ it.partner }}</span>
          <span class="num" :class="{ loss: Number(it.diff) < 0 }">{{ Number(it.diff) > 0 ? '+' : '' }}{{ formatAmount(it.diff) }}</span>
          <span class="meta">
            {{ it.kind === 'receivable' ? 'pohľadávka' : 'záväzok' }} {{ formatAmount(it.remaining) }} {{ it.currency }} · kurz {{ it.rate }} z {{ formatDate(it.rateDate) }}
            · {{ formatAmount(it.bookEur) }} → {{ formatAmount(it.newEur) }} €
          </span>
        </div>
        <small v-if="errors.rates || errors.items" class="err">{{ errors.rates || errors.items }}</small>
        <div v-if="fxr.lines.length" class="preview">
          <div v-for="(l, i) in fxr.lines" :key="i" class="post-line">
            <span class="acc"><b>{{ accountDisplay(l.account) }}</b></span>
            <span class="num">{{ amt(l.debit) }}</span>
            <span class="num">{{ amt(l.credit) }}</span>
          </div>
        </div>
        <div class="actions">
          <RouterLink to="/kurzy" class="doclink"><i class="pi pi-euro" /> Kurzy mien</RouterLink>
          <span class="spacer" />
          <Button v-if="canWrite" type="submit" :label="fxr.posted ? 'Zaúčtovať znova' : 'Zaúčtovať'" icon="pi pi-check" size="small" :loading="busy"
            :disabled="!fxr.items.length || fxr.missing.length > 0" />
        </div>
      </form>

      <!-- účty bánk a pokladníc -->
      <form v-else-if="mode === 'money'" class="dform" @submit.prevent="save">
        <p class="hint">
          Každému bankovému účtu a pokladnici môžete priradiť analytický účet (napr. 221.001 Tatra banka, 211.001 Pokladnica EUR).
          Analytické účty pridáte v účtovom rozvrhu. Bez priradenia sa účtuje na 221 alebo 211 podľa predkontácie.
        </p>
        <p v-if="!moneyForm.length" class="hint">Zatiaľ žiadne bankové účty ani pokladnice.</p>
        <FormField v-for="m in moneyForm" :key="m.key" :label="m.label" :error="errors.money">
          <Select v-model="m.account" :options="moneyOptions(m.key)" option-label="label" option-value="value" size="small" fluid />
        </FormField>
        <RouterLink to="/uctovy-rozvrh" class="doclink"><i class="pi pi-sitemap" /> Účtový rozvrh</RouterLink>
        <div class="actions">
          <span class="spacer" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>

      <!-- počiatočné stavy -->
      <form v-else-if="mode === 'opening'" class="dform" @submit.prevent="save">
        <p class="hint">Zostatky súvahových účtov k 1. 1. {{ year }} (z konečnej súvahy minulého roka). Strana MD sa musí rovnať strane D.</p>
        <Button :label="`Prevziať konečné stavy ${year - 1}`" icon="pi pi-download" size="small" severity="secondary" outlined class="take"
          :loading="busy" @click="takeOpening" />
        <LedgerLines v-model="lines" :accounts="accountOptions" />
        <small v-if="errors.lines" class="err">{{ errors.lines }}</small>
        <div class="actions">
          <span class="spacer" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>

      <!-- zápisy na účte -->
      <div v-else-if="mode === 'account' && current" class="dform">
        <p class="acc-name">{{ current.name }}</p>
        <dl class="facts">
          <dt>Počiatočný stav</dt><dd class="num-l">{{ formatMoney(current.opening) }}</dd>
          <dt>Obrat MD / D</dt><dd class="num-l">{{ formatAmount(current.debit) }} / {{ formatAmount(current.credit) }}</dd>
          <dt>Konečný stav</dt><dd class="num-l"><span class="grand">{{ formatMoney(current.closing) }}</span></dd>
        </dl>
        <p v-if="!current.lines.length" class="hint">Na účte nie sú v roku {{ year }} žiadne zápisy.</p>
        <div v-for="(r, i) in current.lines" :key="i" class="acc-line" tabindex="0" @click="openPosting(r)" @keydown.enter="openPosting(r)">
          <span class="date">{{ formatDate(r.date) }}</span>
          <span class="text"><b>{{ r.number }}</b> {{ r.partner || r.text }}<template v-if="r.line.account !== current.account"> · {{ accountDisplay(r.line.account) }}</template></span>
          <span class="num">{{ amt(r.line.debit) }}</span>
          <span class="num">{{ amt(r.line.credit) }}</span>
        </div>
      </div>
    </Drawer>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  max-width: 1080px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
  margin: 0;
}
.spacer {
  flex: 1;
}
.summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  background: var(--sheet);
  border: 1px solid var(--rule);
}
.cell {
  display: flex;
  flex-direction: column;
  padding: 8px 12px;
  border-right: 1px solid var(--rule-soft);
}
.cell:last-child {
  border-right: 0;
}
.label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.value {
  font-size: var(--fs-lg);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  display: flex;
  align-items: center;
  gap: 4px;
}
.value.ok {
  color: var(--tone-paid);
}
.value.bad {
  color: var(--tone-overdue);
}
.show {
  padding: 0 4px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.search {
  width: 280px;
}
.search :deep(input) {
  width: 100%;
}
.sel {
  width: 190px;
}
.sel-month {
  width: 140px;
}
.sheet {
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 2px solid var(--ink);
}
.scroll {
  overflow-x: auto;
}
.empty {
  color: var(--ink-faint);
  padding: 10px;
  margin: 0;
}
/* denník */
.jhead,
.post-line {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 120px 120px;
  gap: 10px;
}
.jhead {
  padding: 5px 10px 5px 102px;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  border-bottom: 1px solid var(--rule);
}
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.post {
  padding: 5px 10px 6px;
  border-bottom: 1px solid var(--rule);
  cursor: pointer;
}
.post:hover,
.post:focus-visible {
  background: var(--blue-wash);
  outline: none;
}
.post-head {
  display: grid;
  grid-template-columns: 82px 190px minmax(0, 1fr) auto;
  gap: 10px;
  align-items: baseline;
}
.date {
  font-variant-numeric: tabular-nums;
  color: var(--ink-soft);
  white-space: nowrap;
}
.kind {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.text {
  color: var(--ink-soft);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.post-line {
  padding-left: 92px;
  font-size: var(--fs-md);
}
.post-line.credit .acc {
  padding-left: 18px; /* strana D odsadená ako v účtovnej knihe */
}
.acc {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.acc b {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}
.post-problem {
  margin: 2px 0 0 92px;
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.post-problem .amount {
  color: var(--ink-soft);
  margin-left: 6px;
  font-variant-numeric: tabular-nums;
}
.tag {
  font-size: var(--fs-xs);
  border: 1px solid var(--blue);
  color: var(--blue);
  padding: 0 4px;
  border-radius: 2px;
  white-space: nowrap;
}
.fx-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 2px 8px;
  padding: 5px 0;
  border-bottom: 1px solid var(--rule-soft);
}
.fx-item .meta {
  grid-column: 1 / -1;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.tags {
  display: inline-flex;
  gap: 4px;
}
.tag.dim {
  border-color: var(--tone-draft);
  color: var(--tone-draft);
}
.list tr.static {
  cursor: default;
}
.loss {
  color: var(--tone-overdue);
}
.tag.muted {
  border-color: var(--rule);
  color: var(--ink-faint);
}
.tag:empty {
  display: none;
}
/* hlavná kniha */
.list {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.list th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 5px 8px;
  border-bottom: 1px solid var(--rule);
  white-space: nowrap;
}
.list th.num {
  text-align: right;
}
.list td {
  padding: 3px 8px;
  border-bottom: 1px solid var(--rule-soft);
}
.list tbody tr {
  cursor: pointer;
}
.list tbody tr:hover,
.list tbody tr:focus-visible {
  background: var(--blue-wash);
  outline: none;
}
.list .name {
  color: var(--ink-soft);
}
.list tfoot td {
  font-weight: 600;
  border-top: 1px solid var(--ink);
  border-bottom: 0;
  padding-top: 6px;
}
.st-notes {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.st-notes p {
  margin: 0;
}
.warn {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.statement caption {
  text-align: left;
  font-weight: 600;
  padding: 8px 8px 4px;
  background: var(--paper);
  border-bottom: 1px solid var(--rule);
}
.statement + .statement {
  border-top: 2px solid var(--ink);
}
.statement tbody tr {
  cursor: default;
}
.statement .code {
  width: 34px;
  color: var(--ink-faint);
  font-variant-numeric: tabular-nums;
}
.statement tr.sum td {
  font-weight: 600;
}
.statement td.num {
  white-space: nowrap;
  width: 110px;
}
.take {
  align-self: flex-start;
}
.grand {
  border-bottom: 3px double currentColor;
  line-height: 1.15;
}
/* panel */
.dform {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.grid2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}
.facts {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 2px 12px;
  margin: 0;
  font-size: var(--fs-md);
}
.facts dt {
  color: var(--ink-soft);
}
.facts dd {
  margin: 0;
}
.num-l {
  font-variant-numeric: tabular-nums;
}
.doclink {
  font-size: var(--fs-sm);
  color: var(--blue);
  align-self: flex-start;
  text-decoration: none;
}
.doclink:hover {
  text-decoration: underline;
}
.dform .post-problem {
  margin: 0;
}
.preview {
  border-top: 1px solid var(--rule);
}
.preview .post-line {
  padding-left: 0;
  grid-template-columns: minmax(0, 1fr) 90px 90px;
  border-bottom: 1px solid var(--rule-soft);
  padding-top: 3px;
  padding-bottom: 3px;
}
.hint {
  margin: 0;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.err {
  color: var(--tone-overdue);
}
.amt :deep(input),
.amt {
  text-align: right;
}
.actions {
  display: flex;
  align-items: center;
  margin-top: 4px;
}
.acc-name {
  margin: 0;
  color: var(--ink-soft);
}
.acc-line {
  display: grid;
  grid-template-columns: 78px minmax(0, 1fr) 82px 82px;
  gap: 6px;
  padding: 4px 2px;
  border-bottom: 1px solid var(--rule-soft);
  font-size: var(--fs-sm);
  cursor: pointer;
}
.acc-line:hover {
  background: var(--blue-wash);
}
@media (max-width: 640px) {
  .page {
    padding: 10px 10px 20px;
  }
  .summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .cell:nth-child(2) {
    border-right: 0;
  }
  .cell:nth-child(-n + 2) {
    border-bottom: 1px solid var(--rule-soft);
  }
  .search,
  .sel,
  .sel-month {
    flex: 1 1 100%;
    width: auto;
  }
  .post-head {
    grid-template-columns: auto minmax(0, 1fr) auto;
  }
  .post-head .text {
    grid-column: 1 / -1;
    grid-row: 2;
  }
  .jhead,
  .post-line {
    grid-template-columns: minmax(0, 1fr) 80px 80px;
    gap: 6px;
  }
  .post-line {
    padding-left: 0;
  }
  .jhead {
    padding-left: 10px;
  }
  .acc-line {
    grid-template-columns: minmax(0, 1fr) 90px 90px;
  }
  .acc-line .text {
    grid-column: 1 / -1;
    grid-row: 2;
    white-space: normal;
  }
  .post-problem {
    margin-left: 0;
  }
  .ps,
  .bk {
    display: none;
  }
  .statement td.num {
    width: 76px;
  }
  .list {
    font-size: var(--fs-sm);
  }
  .list .name {
    display: block;
    font-size: var(--fs-xs);
  }
  .list td,
  .list th {
    padding: 3px 5px;
  }
}
</style>

<style>
.p-drawer.p-component.ledger-panel {
  width: min(560px, 100vw);
}
</style>
