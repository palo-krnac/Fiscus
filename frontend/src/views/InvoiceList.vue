<script setup>
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import { api } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, statusOf } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { matches, searchTerms } from '../lib/search.js'
import { invoiceColumns } from '../lib/columns.js'
import { kindBadge } from '../lib/advance.js'
import { eurSummary, isForeignCurrency } from '../lib/fx.js'
import ExportButton from '../components/ExportButton.vue'

const router = useRouter()

const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
const year = ref(thisYear)
const view = ref('all')
const views = [
  { label: 'Všetky', value: 'all' },
  { label: 'Koncepty', value: 'draft' },
  { label: 'Neuhradené', value: 'unpaid' },
  { label: 'Po splatnosti', value: 'overdue' },
  { label: 'Uhradené', value: 'paid' },
]
const search = ref('')
const searchEl = ref(null)

const rows = shallowRef([])
const loading = ref(true)
const loadError = ref(null)

async function load() {
  loading.value = true
  try {
    rows.value = await api.invoices(session.companyId, { year: year.value })
    loadError.value = null
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
watch(year, load)
onMounted(load)

const matchesView = (r) => {
  switch (view.value) {
    case 'draft': return r.status === 'draft'
    case 'unpaid': return r.status === 'issued'
    case 'overdue': return r.overdue
    case 'paid': return r.status === 'paid'
    default: return true
  }
}

const visibleRows = computed(() => {
  const terms = searchTerms(search.value)
  return rows.value.filter((r) => {
    if (!matchesView(r)) return false
    const c = r.customer ?? {}
    return matches(terms, [r.number, r.variableSymbol, c.name, c.ico, c.dic, c.icdph, c.city, r.note, r.summary?.total, r.currency, statusOf(r).label])
  })
})

// Súhrn za rok — nezávisle od filtra zobrazenia.
// sumy v eurách (faktúry v cudzej mene prepočítané kurzom zo servera)
const sum = (list) => list.reduce((acc, r) => acc + Number(eurSummary(r).total), 0)
const ledger = computed(() => {
  // zálohová faktúra nie je výnos — fakturované sú faktúry, dobropisy a doklady k prijatej platbe
  const counted = rows.value.filter((r) => (r.status === 'issued' || r.status === 'paid') && r.kind !== 'proforma')
  const unpaid = rows.value.filter((r) => r.status === 'issued')
  const overdue = unpaid.filter((r) => r.overdue)
  return {
    invoiced: sum(counted),
    unpaid: sum(unpaid),
    unpaidCount: unpaid.length,
    overdue: sum(overdue),
    overdueCount: overdue.length,
  }
})

const totalsRow = computed(() => {
  const list = visibleRows.value.filter((r) => r.status !== 'cancelled' && r.status !== 'draft' && r.kind !== 'proforma')
  const add = (k) => list.reduce((a, r) => a + Number(eurSummary(r)[k]), 0).toFixed(2)
  return [{
    number: `Spolu ${list.length}`,
    customer: { name: '' },
    summary: { base: add('base'), vat: add('vat'), total: add('total') },
    _totals: true,
  }]
})

const amountCol = (field, header, extra = {}) => ({
  colId: field,
  headerName: header,
  valueGetter: (p) => Number(eurSummary(p.data)[field]),
  valueFormatter: (p) => formatAmount(p.value),
  type: 'rightAligned',
  width: 116,
  ...extra,
})

const columnDefs = [
  {
    field: 'number',
    headerName: 'Číslo',
    minWidth: 104,
    cellRenderer: (p) => {
      if (p.data._totals) return p.value
      const num = p.value || 'koncept'
      const badge = kindBadge(p.data.kind)
      // cudzia mena: sumy v zozname sú v eurách, mena dokladu je pri čísle
      const cur = isForeignCurrency(p.data.currency)
        ? `<span class="kind-badge cur" title="${formatAmount(p.data.summary.total)} ${p.data.currency}">${p.data.currency}</span>`
        : ''
      return (badge ? `${num}<span class="kind-badge">${badge}</span>` : num) + cur
    },
    width: 178,
    cellClass: (p) => (p.data._totals ? '' : p.value ? 'cell-number' : 'cell-draft'),
  },
  { colId: 'customer', headerName: 'Odberateľ', valueGetter: (p) => p.data.customer.name, flex: 1, minWidth: 130 },
  { colId: 'ico', headerName: 'IČO', valueGetter: (p) => p.data.customer?.ico, width: 96 },
  { field: 'issueDate', headerName: 'Vystavená', valueFormatter: (p) => formatDate(p.value), width: 100 },
  { field: 'dueDate', headerName: 'Splatná', valueFormatter: (p) => formatDate(p.value), width: 100 },
  amountCol('base', 'Základ'),
  amountCol('vat', 'DPH', { width: 100 }),
  amountCol('total', 'Spolu', { cellClass: 'ag-right-aligned-cell cell-total', minWidth: 100 }),
  {
    colId: 'status',
    headerName: 'Stav',
    width: 170,
    valueGetter: (p) => (p.data._totals ? '' : statusOf(p.data).label),
    cellRenderer: (p) => {
      if (p.data._totals) return ''
      const s = statusOf(p.data)
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>`
    },
  },
]

const defaultColDef = { sortable: true, resizable: true, suppressMovable: false }

const rowClassRules = {
  'row-cancelled': (p) => p.data.status === 'cancelled',
  'row-overdue': (p) => p.data.overdue,
}

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['ico', 'issueDate', 'dueDate', 'base', 'vat', 'status'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
function onGridReady(e) {
  gridApi.value = e.api
  applyResponsive()
}
narrow.addEventListener('change', applyResponsive)

function open(e) {
  if (e.data?._totals || !e.data?.id) return
  router.push(`/faktury/${e.data.id}`)
}

function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && (t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName))
  if (typing || e.ctrlKey || e.metaKey || e.altKey) return
  if (e.key === 'n' && can('invoices.write')) {
    e.preventDefault()
    router.push('/faktury/nova')
  } else if (e.key === '/') {
    e.preventDefault()
    searchEl.value?.$el?.focus()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  narrow.removeEventListener('change', applyResponsive)
})
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Vydané faktúry</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <Button v-if="can('invoices.write')" label="Zálohová faktúra" icon="pi pi-plus" severity="secondary" outlined size="small"
        class="hide-narrow" @click="router.push('/faktury/nova?druh=zaloha')" />
      <Button v-if="can('invoices.write')" size="small" @click="router.push('/faktury/nova')">
        <template #default>
          <i class="pi pi-plus" /><span>Nová faktúra</span><kbd class="hint">N</kbd>
        </template>
      </Button>
    </header>

    <section class="ledger" aria-label="Súhrn za rok">
      <div class="figure">
        <span class="label">Fakturované v {{ year }}</span>
        <span class="value">{{ formatMoney(ledger.invoiced) }}</span>
      </div>
      <div class="figure">
        <span class="label">Neuhradené ({{ ledger.unpaidCount }})</span>
        <span class="value">{{ formatMoney(ledger.unpaid) }}</span>
      </div>
      <button
        class="figure figure-alert"
        :class="{ none: !ledger.overdueCount }"
        :disabled="!ledger.overdueCount"
        @click="view = 'overdue'"
      >
        <span class="label">Po splatnosti ({{ ledger.overdueCount }})</span>
        <span class="value">{{ formatMoney(ledger.overdue) }}</span>
      </button>
    </section>

    <div class="toolbar">
      <SelectButton v-model="view" :options="views" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText ref="searchEl" v-model="search" placeholder="Hľadať číslo, odberateľa, IČO, VS, sumu" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton :name="`Vydané faktúry ${year}`" :columns="invoiceColumns" :rows="() => gridRows(gridApi, visibleRows)" />
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <div class="grid-wrap">
      <AgGridVue
        class="grid"
        :theme="fiskusGridTheme"
        :row-data="visibleRows"
        :pinned-bottom-row-data="totalsRow"
        :column-defs="columnDefs"
        :default-col-def="defaultColDef"
        :row-class-rules="rowClassRules"
        :locale-text="skGridLocale"
        :loading="loading"
        :get-row-id="(p) => (p.data._totals ? 'totals' : String(p.data.id))"
        :overlay-no-rows-template="'<span class=&quot;empty&quot;>Žiadne faktúry. Novú vytvoríte klávesom N.</span>'"
        @grid-ready="onGridReady"
        @row-clicked="open"
      />
    </div>
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
.hint {
  margin-left: 6px;
  background: rgba(255, 255, 255, 0.16);
  border-color: rgba(255, 255, 255, 0.3);
  color: #fff;
}
.head :deep(.p-button) {
  gap: 6px;
}

/* Súhrn ako hlavička účtovnej knihy: čísla, nie karty */
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
  letter-spacing: -0.01em;
}
.figure-alert {
  cursor: pointer;
}
.figure-alert .value {
  color: var(--tone-overdue);
}
.figure-alert:hover:not(:disabled) {
  background: var(--tone-overdue-bg);
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
.grid :deep(.cell-number) {
  font-weight: 600;
  color: var(--blue-deep);
}
.grid :deep(.kind-badge) {
  margin-left: 6px;
  padding: 0 5px;
  font-size: var(--fs-xs);
  font-weight: 600;
  font-style: normal;
  color: var(--tone-draft);
  border: 1px solid currentColor;
  border-radius: 3px;
}
.grid :deep(.cell-draft) {
  color: var(--tone-draft);
  font-style: italic;
}
.grid :deep(.kind-badge.cur) {
  color: var(--blue);
  border-color: var(--blue);
}
.grid :deep(.cell-total) {
  font-weight: 600;
}
.grid :deep(.ag-floating-bottom .ag-row) {
  cursor: default;
}

.error {
  color: var(--tone-overdue);
  margin: 0;
}

@media (max-width: 760px) {
  .page {
    padding: 10px 10px 8px;
  }
  .head h1 {
    font-size: var(--fs-lg);
  }
  .hint {
    display: none;
  }
  .hide-narrow {
    display: none;
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
}
</style>
