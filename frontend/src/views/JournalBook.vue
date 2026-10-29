<script setup>
// Peňažný denník (jednoduché účtovníctvo): pohyby v pokladnici a na účtoch
// rozpísané do stĺpcov podľa opatrenia MF SR č. MF/27076/2007-74, § 4.
// Úhrady spárovaných faktúr rozpíše Fiškus, ostatné pohyby rozpíše používateľ.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { RouterLink } from 'vue-router'
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
import { journalExportColumns } from '../lib/columns.js'
import { absAmount, columnCode, columnLabel, columnsFor, entryState, linesText, presets, unallocated } from '../lib/journal.js'
import ExportButton from '../components/ExportButton.vue'

const toast = useToast()
const canWrite = computed(() => can('ledger.write'))

const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
const year = ref(thisYear)
const view = ref('all')
const views = [
  { label: 'Pohyby', value: 'all' },
  { label: 'Nerozpísané', value: 'open' },
  { label: 'Súčty stĺpcov', value: 'sums' },
]
const search = ref('')
const book = ref({ entries: [], columns: [], totals: { columns: {} } })
const loading = ref(true)
const loadError = ref('')

async function load() {
  loading.value = true
  try {
    book.value = await api.journal(session.companyId, year.value)
    loadError.value = ''
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
watch(year, load)
onMounted(load)

const cols = computed(() => book.value.columns ?? [])
const exportCols = computed(() => journalExportColumns(cols.value))

const visibleRows = computed(() => {
  const terms = searchTerms(search.value)
  return book.value.entries.filter((e) => {
    if (view.value === 'open' && e.lines?.length) return false
    return matches(terms, [e.number, e.partner, e.text, e.doc?.number, e.amount, e.note, linesText(cols.value, e.lines, (v) => v)])
  })
})

const sumRows = computed(() =>
  cols.value.map((c) => ({ ...c, short: columnCode(c.code), amount: book.value.totals.columns?.[c.code] ?? '0' })),
)

const columnDefs = [
  { field: 'date', headerName: 'Dátum', valueFormatter: (p) => formatDate(p.value), width: 100 },
  {
    colId: 'number',
    headerName: 'Doklad',
    valueGetter: (p) => p.data.number || (p.data.means === 'bank' ? 'výpis' : ''),
    width: 130,
    cellClass: 'cell-number',
  },
  {
    colId: 'means',
    headerName: '',
    width: 44,
    valueGetter: (p) => (p.data.means === 'cash' ? 'P' : 'B'),
    tooltipValueGetter: (p) => (p.data.means === 'cash' ? 'Pokladnica' : `Banka ${p.data.account}`),
    cellClass: 'cell-means',
  },
  { field: 'partner', headerName: 'Partner', flex: 1, minWidth: 120 },
  {
    colId: 'text',
    headerName: 'Text',
    valueGetter: (p) => (p.data.doc ? `${p.data.doc.number} · ${p.data.text}` : p.data.text),
    flex: 1.2,
    minWidth: 140,
  },
  {
    field: 'amount',
    headerName: 'Suma',
    type: 'rightAligned',
    width: 110,
    valueFormatter: (p) => formatAmount(p.value),
    cellClass: (p) => (Number(p.value) >= 0 ? 'ag-right-aligned-cell cell-in' : 'ag-right-aligned-cell'),
    comparator: (a, b) => Number(a) - Number(b),
  },
  {
    colId: 'lines',
    headerName: 'Rozpis',
    flex: 1.6,
    minWidth: 160,
    valueGetter: (p) => linesText(cols.value, p.data.lines, formatAmount),
    tooltipValueGetter: (p) => linesText(cols.value, p.data.lines, formatAmount),
  },
  {
    colId: 'state',
    headerName: 'Stav',
    width: 140,
    valueGetter: (p) => entryState(p.data).label,
    cellRenderer: (p) => {
      const s = entryState(p.data)
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>`
    },
  },
]

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  // na mobile stav ukáže farba riadku (nerozpísaný je červený)
  gridApi.value?.setColumnsVisible(['number', 'means', 'text', 'lines', 'state'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
narrow.addEventListener('change', applyResponsive)
onBeforeUnmount(() => narrow.removeEventListener('change', applyResponsive))

// ---- rozpis pohybu ----
const current = ref(null)
const draft = ref([])
const note = ref('')
const editError = ref('')
const busy = ref(false)
const editOpen = computed({
  get: () => !!current.value,
  set: (v) => {
    if (!v) current.value = null
  },
})
const choices = computed(() => (current.value ? columnsFor(cols.value, current.value.amount) : []))
const rest = computed(() => (current.value ? unallocated(current.value.amount, draft.value) : '0.00'))

function open(e) {
  current.value = e
  draft.value = (e.lines ?? []).map((l) => ({ column: l.column, amount: String(l.amount) }))
  if (!draft.value.length) draft.value = [{ column: null, amount: absAmount(e.amount) }]
  note.value = e.note ?? ''
  editError.value = ''
}

function preset(p) {
  draft.value = [{ column: p.column, amount: absAmount(current.value.amount) }]
}

function addLine() {
  draft.value.push({ column: null, amount: Number(rest.value) > 0 ? rest.value : '' })
}

async function save() {
  editError.value = ''
  if (draft.value.some((l) => !l.column)) {
    editError.value = 'Pri každom riadku vyberte stĺpec'
    return
  }
  busy.value = true
  try {
    const lines = draft.value.map((l) => ({ column: l.column, amount: String(l.amount).replace(',', '.').trim() }))
    await api.classifyJournal(session.companyId, current.value.key, lines, note.value)
    toast.add({ severity: 'success', summary: 'Pohyb rozpísaný', life: 2000 })
    current.value = null
    await load()
  } catch (e) {
    editError.value = e.fields?.[0]?.message ?? e.message
  } finally {
    busy.value = false
  }
}

async function reset() {
  busy.value = true
  try {
    await api.resetJournal(session.companyId, current.value.key)
    current.value = null
    await load()
  } catch (e) {
    editError.value = e.message
  } finally {
    busy.value = false
  }
}

const docLink = (d) => (d.kind === 'invoice' ? `/faktury/${d.id}` : `/prijate/${d.id}`)
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Peňažný denník</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
    </header>

    <section class="ledger" aria-label="Súhrn">
      <div class="figure">
        <span class="label">Príjmy do základu dane</span>
        <span class="value in">{{ formatMoney(book.totals.taxIncome) }}</span>
      </div>
      <div class="figure">
        <span class="label">Výdavky do základu dane</span>
        <span class="value">{{ formatMoney(book.totals.taxExpenses) }}</span>
      </div>
      <div class="figure">
        <span class="label">Rozdiel príjmov a výdavkov</span>
        <span class="value"><span class="grand">{{ formatMoney(book.totals.result) }}</span></span>
      </div>
      <div class="figure">
        <span class="label">Nerozpísané pohyby</span>
        <span class="value" :class="{ open: book.totals.unclassified > 0 }">{{ book.totals.unclassified ?? 0 }}</span>
      </div>
    </section>

    <div class="toolbar">
      <SelectButton v-model="view" :options="views" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <IconField v-if="view !== 'sums'" class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať partnera, doklad, text, sumu" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton :name="`Peňažný denník ${year}`" :columns="exportCols" :rows="() => gridRows(gridApi, visibleRows)" />
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <section v-if="view === 'sums'" class="sums" aria-label="Súčty stĺpcov">
      <table>
        <thead>
          <tr><th>Stĺpec</th><th>Popis</th><th class="num">Spolu {{ year }}</th></tr>
        </thead>
        <tbody>
          <tr v-for="r in sumRows" :key="r.code" :class="{ exp: !r.income }">
            <td class="code">{{ r.short }}</td>
            <td>{{ r.label }}</td>
            <td class="num">{{ formatAmount(r.amount) }}</td>
          </tr>
        </tbody>
        <tfoot>
          <tr><td /><td>Pokladnica: príjmy / výdavky</td><td class="num">{{ formatAmount(book.totals.cashIn) }} / {{ formatAmount(book.totals.cashOut) }}</td></tr>
          <tr><td /><td>Banka: príjmy / výdavky</td><td class="num">{{ formatAmount(book.totals.bankIn) }} / {{ formatAmount(book.totals.bankOut) }}</td></tr>
          <tr class="result"><td /><td>Rozdiel príjmov a výdavkov zahrnovaných do základu dane</td><td class="num"><span class="grand">{{ formatAmount(book.totals.result) }}</span></td></tr>
        </tfoot>
      </table>
      <p class="muted">
        Členenie podľa opatrenia MF SR č. MF/27076/2007-74, § 4 ods. 6: d – príjmy a e – výdavky zahrnované do základu dane,
        f a g – mimo základu dane, c – priebežné položky.
      </p>
    </section>

    <div v-else class="grid-wrap">
      <AgGridVue
        class="grid"
        :theme="fiskusGridTheme"
        :row-data="visibleRows"
        :column-defs="columnDefs"
        :default-col-def="{ sortable: true, resizable: true }"
        :locale-text="skGridLocale"
        :loading="loading"
        :get-row-id="(p) => p.data.key"
        :row-class-rules="{ 'row-open': (p) => !p.data.lines?.length }"
        :overlay-no-rows-template="'<span>Žiadne pohyby. Denník sa skladá z bankových výpisov a pokladničných dokladov.</span>'"
        @grid-ready="(e) => { gridApi = e.api; applyResponsive() }"
        @row-clicked="(e) => open(e.data)"
      />
    </div>

    <Drawer v-model:visible="editOpen" position="right" class="journal-panel" header="Rozpis pohybu">
      <div v-if="current" class="edit">
        <p class="amount" :class="{ in: Number(current.amount) >= 0 }">{{ formatMoney(current.amount) }}</p>
        <dl class="facts">
          <dt>Dátum</dt><dd>{{ formatDate(current.date) }}</dd>
          <dt>{{ current.means === 'cash' ? 'Pokladnica' : 'Banka' }}</dt><dd>{{ current.number || current.account }}</dd>
          <template v-if="current.partner"><dt>Partner</dt><dd>{{ current.partner }}</dd></template>
          <template v-if="current.text"><dt>Text</dt><dd>{{ current.text }}</dd></template>
          <template v-if="current.doc">
            <dt>Uhrádza</dt>
            <dd><RouterLink :to="docLink(current.doc)">{{ current.doc.number }}</RouterLink></dd>
          </template>
        </dl>

        <template v-if="canWrite">
          <div class="presets" role="group" aria-label="Rýchly rozpis">
            <button v-for="p in presets(current.amount)" :key="p.column" type="button" @click="preset(p)">{{ p.label }}</button>
          </div>

          <table class="split">
            <thead><tr><th>Stĺpec</th><th class="num">Suma €</th><th aria-label="Odobrať" /></tr></thead>
            <tbody>
              <tr v-for="(l, i) in draft" :key="i">
                <td>
                  <Select v-model="l.column" :options="choices" option-label="label" option-value="code" placeholder="Vyberte stĺpec" size="small" fluid
                    :aria-label="`Stĺpec riadku ${i + 1}`">
                    <template #option="{ option }"><span class="code">{{ columnCode(option.code) }}</span> {{ option.label }}</template>
                  </Select>
                </td>
                <td><InputText v-model="l.amount" size="small" class="num" inputmode="decimal" :aria-label="`Suma riadku ${i + 1}`" /></td>
                <td><Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="`Odobrať riadok ${i + 1}`" @click="draft.splice(i, 1)" /></td>
              </tr>
            </tbody>
          </table>
          <div class="split-foot">
            <button type="button" class="link" @click="addLine"><i class="pi pi-plus" /> Pridať riadok</button>
            <span :class="{ off: rest !== '0.00' }">Nerozpísané: {{ formatAmount(rest) }} €</span>
          </div>
          <label class="note"><span>Poznámka</span><InputText v-model="note" size="small" fluid /></label>
          <p v-if="editError" class="error">{{ editError }}</p>
          <div class="actions">
            <Button v-if="!current.auto && current.lines?.length" label="Vrátiť návrh Fiškusa" text size="small" severity="secondary" :disabled="busy" @click="reset" />
            <Button label="Uložiť rozpis" icon="pi pi-check" size="small" :loading="busy" @click="save" />
          </div>
        </template>
        <p v-else class="muted">{{ linesText(cols, current.lines, formatAmount) || 'Pohyb ešte nie je rozpísaný.' }}</p>
      </div>
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
  gap: 10px;
}
.head h1 {
  font-size: var(--fs-xl);
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
  border-right: 1px solid var(--rule);
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
.figure .value.open {
  color: var(--tone-overdue);
}
.grand {
  border-bottom: 3px double currentColor;
  line-height: 1.15;
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
.grid :deep(.cell-number) {
  color: var(--blue-deep);
  font-weight: 600;
}
.grid :deep(.cell-in) {
  color: var(--tone-paid);
  font-weight: 600;
}
.grid :deep(.cell-means) {
  color: var(--ink-faint);
  text-align: center;
}
.grid :deep(.row-open) {
  background: var(--tone-overdue-bg);
}
.sums {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 4px 14px 12px;
  overflow: auto;
}
.sums table {
  width: 100%;
  max-width: 720px;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.sums th {
  text-align: left;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  font-weight: 600;
  padding: 6px 8px;
  border-bottom: 1px solid var(--ink);
}
.sums td {
  padding: 5px 8px;
  border-bottom: 1px solid var(--rule-soft);
}
.sums tr.exp td:nth-child(2) {
  color: var(--ink-soft);
}
.sums tfoot td {
  border-bottom: 0;
  color: var(--ink-soft);
}
.sums tfoot tr:first-child td {
  border-top: 1px solid var(--ink);
}
.sums tr.result td {
  color: var(--ink);
  font-weight: 600;
}
.code {
  font-weight: 600;
  color: var(--blue-deep);
  width: 3em;
}
.num {
  text-align: right;
}
.num :deep(input),
input.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.error {
  color: var(--tone-overdue);
  margin: 0;
}
.edit {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.amount {
  margin: 0;
  font-size: 1.625rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.amount.in {
  color: var(--tone-paid);
}
.facts {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 3px 12px;
  margin: 0;
}
.facts dt {
  color: var(--ink-faint);
}
.facts dd {
  margin: 0;
}
.facts a {
  color: var(--blue-deep);
}
.presets {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.presets button {
  font: inherit;
  font-size: var(--fs-sm);
  padding: 2px 8px;
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  background: var(--sheet);
  color: var(--blue-deep);
  cursor: pointer;
}
.presets button:hover {
  background: var(--blue-wash);
}
.split {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
.split :deep(.p-inputtext) {
  width: 100%;
}
.split th {
  text-align: left;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  font-weight: 400;
  padding: 0 4px 3px;
}
.split td {
  padding: 2px 4px;
}
.split th:nth-child(2) {
  width: 7.5rem;
}
.split th:nth-child(3) {
  width: 2.5rem;
}
.split-foot {
  display: flex;
  justify-content: space-between;
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.split-foot .off {
  color: var(--tone-overdue);
  font-weight: 600;
}
.link {
  border: 0;
  background: none;
  padding: 0;
  font: inherit;
  color: var(--blue);
  cursor: pointer;
}
.note {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.note > span {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
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
  .search {
    order: 3;
    flex: 1 0 100%;
    width: auto;
    min-width: 0;
    margin-left: 0;
  }
}
</style>

<style>
.journal-panel.p-drawer {
  width: 480px;
  max-width: 100vw;
}
</style>
