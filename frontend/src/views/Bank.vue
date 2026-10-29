<script setup>
// Banka: import výpisov camt.053 a párovanie platieb s faktúrami.
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Drawer from 'primevue/drawer'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { session, can } from '../lib/session.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { matches, searchTerms } from '../lib/search.js'
import { bankColumns } from '../lib/columns.js'
import ExportButton from '../components/ExportButton.vue'

const router = useRouter()
const toast = useToast()
const canWrite = computed(() => can('bank.write'))

const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
const year = ref(thisYear)
const view = ref('all')
const views = [
  { label: 'Všetky', value: 'all' },
  { label: 'Nespárované', value: 'open' },
  { label: 'Spárované', value: 'matched' },
  { label: 'Nepárované', value: 'ignored' },
]
const search = ref('')
const rows = shallowRef([])
const loading = ref(true)
const loadError = ref('')
const importing = ref(false)
const fileEl = ref(null)

async function load() {
  loading.value = true
  try {
    rows.value = await api.bank(session.companyId, { year: year.value })
    loadError.value = ''
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
watch(year, load)
onMounted(load)

const visibleRows = computed(() => {
  const terms = searchTerms(search.value)
  return rows.value.filter(
    (r) =>
      (view.value === 'all' || r.status === view.value) &&
      matches(terms, [r.counterName, r.counterIban, r.vs, r.ss, r.ks, r.message, r.amount, r.match?.number, r.note]),
  )
})

// súčty v zozname (súčet hotových súm z výpisu)
const ledger = computed(() => {
  let inSum = 0
  let outSum = 0
  let open = 0
  for (const r of rows.value) {
    if (r.direction === 'in') inSum += Number(r.amount)
    else outSum += Number(r.amount)
    if (r.status === 'open') open++
  }
  return { in: inSum, out: outSum, open }
})

const STATUS = {
  open: { label: 'Nespárované', tone: 'draft' },
  matched: { label: 'Spárované', tone: 'paid' },
  ignored: { label: 'Nepárované', tone: 'cancelled' },
}

const columnDefs = [
  { field: 'date', headerName: 'Dátum', valueFormatter: (p) => formatDate(p.value), width: 100 },
  {
    colId: 'amount',
    headerName: 'Suma',
    valueGetter: (p) => (p.data.direction === 'out' ? -1 : 1) * Number(p.data.amount),
    valueFormatter: (p) => (p.value > 0 ? '+' : '') + formatAmount(p.value),
    cellClass: (p) => ['ag-right-aligned-cell', p.value > 0 ? 'cell-in' : 'cell-out'],
    type: 'rightAligned',
    width: 120,
  },
  { field: 'counterName', headerName: 'Protistrana', flex: 1, minWidth: 150 },
  { field: 'vs', headerName: 'VS', width: 110 },
  { field: 'message', headerName: 'Správa', flex: 1, minWidth: 160 },
  {
    colId: 'status',
    headerName: 'Stav',
    width: 190,
    valueGetter: (p) => STATUS[p.data.status]?.label,
    cellRenderer: (p) => {
      const s = STATUS[p.data.status] ?? STATUS.open
      const doc = p.data.match ? `<span class="doc">${p.data.match.number}${p.data.match.auto ? ' · auto' : ''}</span>` : ''
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>${doc}`
    },
  },
]
const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['vs', 'message', 'status'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
narrow.addEventListener('change', applyResponsive)

// ---- import ----
async function onFile(e) {
  const file = e.target.files?.[0]
  e.target.value = ''
  if (!file) return
  importing.value = true
  try {
    const r = await api.importBank(session.companyId, file)
    const parts = [`${r.imported} nových pohybov`]
    if (r.autoMatched) parts.push(`${r.autoMatched} automaticky spárovaných`)
    if (r.open) parts.push(`${r.open} na spárovanie`)
    if (r.duplicates) parts.push(`${r.duplicates} už importovaných`)
    toast.add({ severity: 'success', summary: `Výpis ${file.name} načítaný`, detail: parts.join(', '), life: 6000 })
    for (const w of r.warnings) toast.add({ severity: 'warn', summary: 'Upozornenie', detail: w, life: 8000 })
    if (r.open) view.value = 'open'
    await load()
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Výpis nemožno načítať', detail: err.message, life: 8000 })
  } finally {
    importing.value = false
  }
}

// ---- detail a párovanie ----
const selected = ref(null)
const panelOpen = computed({
  get: () => !!selected.value,
  set: (v) => {
    if (!v) selected.value = null
  },
})
const suggestions = ref([])
const documents = ref([])
const docSearch = ref('')
const note = ref('')
const busy = ref(false)

async function openTx(tx) {
  selected.value = tx
  note.value = tx.note ?? ''
  docSearch.value = ''
  suggestions.value = []
  documents.value = []
  if (tx.status !== 'open') return
  try {
    const r = await api.bankSuggestions(session.companyId, tx.id)
    suggestions.value = r.suggestions
    documents.value = r.documents
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Návrhy', detail: e.message, life: 5000 })
  }
}

const otherDocs = computed(() => {
  const terms = searchTerms(docSearch.value)
  const sug = new Set(suggestions.value.map((s) => `${s.kind}:${s.id}`))
  return documents.value
    .filter((d) => !sug.has(`${d.kind}:${d.id}`) && matches(terms, [d.number, d.vs, d.partner, d.amount]))
    .slice(0, 30)
})

async function act(fn, ok) {
  busy.value = true
  try {
    const t = await fn()
    toast.add({ severity: 'success', summary: ok, life: 3000 })
    rows.value = rows.value.map((r) => (r.id === t.id ? t : r))
    selected.value = null
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
const pairWith = (d) => act(() => api.bankMatch(session.companyId, selected.value.id, d.kind, d.id), `Spárované s ${d.number} — doklad je uhradený`)
const unmatch = () => act(() => api.bankUnmatch(session.companyId, selected.value.id), 'Spárovanie zrušené, doklad je opäť neuhradený')
const ignore = (on) => act(() => api.bankIgnore(session.companyId, selected.value.id, on, note.value), on ? 'Označené ako nepárované' : 'Vrátené medzi nespárované')
const docLink = (kind, id) => (kind === 'purchase' ? `/prijate/${id}` : `/faktury/${id}`)
const kindLabel = (k) => (k === 'purchase' ? 'prijatá' : 'vydaná')
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Banka</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <template v-if="canWrite">
        <input ref="fileEl" type="file" accept=".xml,application/xml,text/xml" hidden @change="onFile" />
        <Button size="small" icon="pi pi-upload" label="Načítať výpis" :loading="importing" v-tooltip.bottom="'Výpis vo formáte XML camt.053 z internetbankingu'" @click="fileEl.click()" />
      </template>
    </header>

    <section class="ledger" aria-label="Súhrn">
      <div class="figure">
        <span class="label">Príjmy {{ year }}</span>
        <span class="value in">{{ formatMoney(ledger.in) }}</span>
      </div>
      <div class="figure">
        <span class="label">Výdavky {{ year }}</span>
        <span class="value">{{ formatMoney(ledger.out) }}</span>
      </div>
      <button class="figure figure-alert" :class="{ none: !ledger.open }" :disabled="!ledger.open" @click="view = 'open'">
        <span class="label">Na spárovanie</span>
        <span class="value">{{ ledger.open }}</span>
      </button>
    </section>

    <div class="toolbar">
      <SelectButton v-model="view" :options="views" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať protistranu, VS, správu, sumu" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton :name="`Banka ${year}`" :columns="bankColumns" :rows="() => gridRows(gridApi, visibleRows)" />
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <div class="grid-wrap">
      <AgGridVue
        class="grid"
        :theme="fiskusGridTheme"
        :row-data="visibleRows"
        :column-defs="columnDefs"
        :default-col-def="{ sortable: true, resizable: true }"
        :locale-text="skGridLocale"
        :loading="loading"
        :get-row-id="(p) => String(p.data.id)"
        :row-class-rules="{ 'row-open': (p) => p.data.status === 'open' }"
        :overlay-no-rows-template="'<span>Žiadne pohyby. Výpis z internetbankingu (XML camt.053) načítate tlačidlom Načítať výpis.</span>'"
        @grid-ready="(e) => { gridApi = e.api; applyResponsive() }"
        @row-clicked="(e) => openTx(e.data)"
      />
    </div>

    <Drawer v-model:visible="panelOpen" position="right" class="tx-panel" :header="selected ? (selected.direction === 'in' ? 'Príjem' : 'Výdavok') : ''">
      <template v-if="selected">
        <div class="amount" :class="selected.direction">
          {{ selected.direction === 'in' ? '+' : '−' }}{{ formatAmount(selected.amount) }} {{ selected.currency }}
        </div>
        <dl class="facts">
          <dt>Dátum</dt><dd>{{ formatDate(selected.date) }}</dd>
          <dt>Protistrana</dt><dd>{{ selected.counterName || '—' }}</dd>
          <dt v-if="selected.counterIban">IBAN</dt><dd v-if="selected.counterIban" class="mono">{{ selected.counterIban }}</dd>
          <dt>Symboly</dt><dd class="mono">VS {{ selected.vs || '—' }} · ŠS {{ selected.ss || '—' }} · KS {{ selected.ks || '—' }}</dd>
          <dt v-if="selected.message">Správa</dt><dd v-if="selected.message">{{ selected.message }}</dd>
          <dt>Účet</dt><dd class="mono">{{ selected.account }}</dd>
        </dl>

        <section v-if="selected.status === 'matched'" class="box matched">
          <p>
            Spárované s dokladom
            <RouterLink :to="docLink(selected.match.kind, selected.match.id)">{{ selected.match.number }}</RouterLink>
            ({{ selected.match.auto ? 'automaticky' : 'ručne' }}).
          </p>
          <Button v-if="canWrite" label="Zrušiť spárovanie" severity="secondary" outlined size="small" :loading="busy" @click="unmatch" />
        </section>

        <template v-else-if="selected.status === 'open'">
          <section class="box">
            <h3>Návrhy</h3>
            <p v-if="!suggestions.length" class="muted">Žiadny doklad nesedí podľa VS ani sumy.</p>
            <ul class="docs">
              <li v-for="s in suggestions" :key="s.kind + s.id">
                <div class="doc-main">
                  <RouterLink :to="docLink(s.kind, s.id)" class="num">{{ s.number }}</RouterLink>
                  <span class="muted">{{ kindLabel(s.kind) }} · VS {{ s.vs || '—' }}</span>
                  <span class="partner">{{ s.partner }}</span>
                  <span class="reason" :data-score="s.score >= 100 ? 'hi' : s.score >= 60 ? 'mid' : 'lo'">{{ s.reason }}</span>
                </div>
                <div class="doc-side">
                  <strong>{{ formatAmount(s.amount) }}</strong>
                  <Button v-if="canWrite" label="Spárovať" size="small" :loading="busy" @click="pairWith(s)" />
                </div>
              </li>
            </ul>
          </section>

          <section class="box">
            <h3>Iný doklad</h3>
            <IconField class="doc-search">
              <InputIcon class="pi pi-search" />
              <InputText v-model="docSearch" placeholder="Číslo, VS, partner, suma" size="small" aria-label="Hľadať doklad" />
            </IconField>
            <ul class="docs">
              <li v-for="d in otherDocs" :key="d.kind + d.id">
                <div class="doc-main">
                  <RouterLink :to="docLink(d.kind, d.id)" class="num">{{ d.number }}</RouterLink>
                  <span class="muted">{{ kindLabel(d.kind) }} · VS {{ d.vs || '—' }} · splatná {{ formatDate(d.due) }}</span>
                  <span class="partner">{{ d.partner }}</span>
                </div>
                <div class="doc-side">
                  <strong>{{ formatAmount(d.amount) }}</strong>
                  <Button v-if="canWrite" label="Spárovať" severity="secondary" outlined size="small" :loading="busy" @click="pairWith(d)" />
                </div>
              </li>
              <li v-if="!otherDocs.length" class="muted">Žiadne ďalšie neuhradené doklady.</li>
            </ul>
          </section>
        </template>

        <section v-if="canWrite && selected.status !== 'matched'" class="box">
          <h3>Nepárovať</h3>
          <p class="muted">Poplatky banke, prevody medzi vlastnými účtami, výbery hotovosti…</p>
          <InputText v-model="note" size="small" placeholder="Poznámka (napr. poplatok)" fluid />
          <div class="row-actions">
            <Button v-if="selected.status === 'open'" label="Označiť ako nepárované" severity="secondary" outlined size="small" :loading="busy" @click="ignore(true)" />
            <Button v-else label="Vrátiť medzi nespárované" severity="secondary" outlined size="small" :loading="busy" @click="ignore(false)" />
          </div>
        </section>
      </template>
    </Drawer>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 14px 16px 12px;
  gap: 10px;
}
.head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.head h1 {
  font-size: var(--fs-xl);
}
.spacer {
  flex: 1;
}
.ledger {
  display: flex;
  border-top: 2px solid var(--ink);
  border-bottom: 1px solid var(--rule);
  background: var(--sheet);
}
.figure {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 8px 20px 9px 14px;
  border: 0;
  border-right: 1px solid var(--rule);
  background: none;
  font: inherit;
  color: inherit;
  text-align: left;
}
.figure .label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.figure .value {
  font-size: var(--fs-xl);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.figure .value.in {
  color: var(--tone-paid);
}
.figure-alert {
  cursor: pointer;
}
.figure-alert .value {
  color: var(--tone-draft);
}
.figure-alert:hover:not(:disabled) {
  background: var(--tone-draft-bg);
}
.figure-alert.none {
  cursor: default;
}
.figure-alert.none .value {
  color: var(--ink-faint);
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.search {
  margin-left: auto;
  width: 280px;
}
.search :deep(input) {
  width: 100%;
}
.grid-wrap {
  flex: 1;
  min-height: 320px;
  border: 1px solid var(--rule);
  background: var(--sheet);
}
.grid {
  height: 100%;
}
.grid :deep(.ag-row) {
  cursor: pointer;
}
.grid :deep(.cell-in) {
  color: var(--tone-paid);
  font-weight: 600;
}
.grid :deep(.cell-out) {
  font-weight: 600;
}
/* nespárované: mosadzný okraj iba na mobile, kde chýba stĺpec Stav */
@media (max-width: 760px) {
  .grid :deep(.row-open) {
    box-shadow: inset 3px 0 0 var(--tone-draft);
  }
}
.grid :deep(.doc) {
  margin-left: 8px;
  font-size: var(--fs-sm);
  color: var(--blue-deep);
  font-weight: 600;
}
.error {
  color: var(--tone-overdue);
  margin: 0;
}
.amount {
  font-size: 1.625rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  margin-bottom: 10px;
}
.amount.in {
  color: var(--tone-paid);
}
.facts {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 3px 12px;
  margin: 0 0 14px;
  font-size: var(--fs-md);
}
.facts dt {
  color: var(--ink-faint);
}
.facts dd {
  margin: 0;
  word-break: break-word;
}
.mono {
  font-variant-numeric: tabular-nums;
}
.box {
  border-top: 1px solid var(--rule);
  padding: 10px 0;
}
.box h3 {
  font-size: var(--fs-md);
  margin: 0 0 6px;
}
.box.matched p {
  margin: 0 0 8px;
}
.box.matched a,
.num {
  color: var(--blue-deep);
  font-weight: 600;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.docs {
  list-style: none;
  margin: 6px 0 0;
  padding: 0;
}
.docs li {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 6px 0;
  border-bottom: 1px solid var(--rule-soft);
}
.doc-main {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.doc-side {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  font-variant-numeric: tabular-nums;
}
.partner {
  font-size: var(--fs-sm);
}
.reason {
  font-size: var(--fs-xs);
}
.reason[data-score='hi'] {
  color: var(--tone-paid);
}
.reason[data-score='mid'] {
  color: var(--tone-draft);
}
.reason[data-score='lo'] {
  color: var(--ink-faint);
}
.doc-search {
  width: 100%;
}
.doc-search :deep(input) {
  width: 100%;
}
.row-actions {
  margin-top: 8px;
}
@media (max-width: 760px) {
  .page {
    padding: 10px 10px 8px;
  }
  .ledger {
    overflow-x: auto;
  }
  .figure {
    padding: 6px 12px 7px 10px;
    flex: none;
  }
  .figure .value {
    font-size: var(--fs-lg);
  }
  .toolbar :deep(.p-selectbutton) {
    overflow-x: auto;
    max-width: 100%;
    flex-wrap: nowrap;
  }
  .toolbar :deep(.p-togglebutton) {
    flex: none;
    white-space: nowrap;
  }
  .search {
    flex: 1;
    width: auto;
    min-width: 0;
    margin-left: 0;
  }
  .head :deep(.p-button-label) {
    display: none;
  }
}
</style>

<style>
.tx-panel.p-drawer {
  width: 440px;
  max-width: 100vw;
}
</style>
