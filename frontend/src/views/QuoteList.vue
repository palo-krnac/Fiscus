<script setup>
// Zoznam cenových ponúk. Ponuka nie je daňový doklad — súčty sú iba prehľad.
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
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { matches, searchTerms } from '../lib/search.js'
import { quoteColumns } from '../lib/columns.js'
import { matchesQuoteView, quoteStatusOf, quoteViews } from '../lib/quotes.js'
import ExportButton from '../components/ExportButton.vue'

const router = useRouter()

const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
const year = ref(thisYear)
const view = ref('all')
const search = ref('')
const searchEl = ref(null)

const rows = shallowRef([])
const loading = ref(true)
const loadError = ref(null)

async function load() {
  loading.value = true
  try {
    rows.value = await api.quotes(session.companyId, year.value)
    loadError.value = null
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
  return rows.value.filter((r) => {
    if (!matchesQuoteView(r, view.value)) return false
    const c = r.customer ?? {}
    return matches(terms, [r.number, c.name, c.ico, c.icdph, c.city, r.note, r.summary?.total, r.invoiceNumber, quoteStatusOf(r).label,
      ...(r.lines ?? []).map((l) => l.description)])
  })
})

// Prehľad za rok — nezávisle od filtra zobrazenia (súčet už vypočítaných súm).
const sum = (list) => list.reduce((acc, r) => acc + Number(r.summary.total), 0)
const ledger = computed(() => {
  const open = rows.value.filter((r) => r.status === 'issued' && !r.expired)
  const waiting = rows.value.filter((r) => r.status === 'accepted')
  const won = rows.value.filter((r) => r.status === 'accepted' || r.status === 'invoiced')
  const decided = rows.value.filter((r) => r.status !== 'draft' && !(r.status === 'issued' && !r.expired))
  return {
    open: sum(open),
    openCount: open.length,
    waiting: sum(waiting),
    waitingCount: waiting.length,
    rate: decided.length ? Math.round((won.length / decided.length) * 100) : null,
    won: won.length,
    decided: decided.length,
  }
})

const totalsRow = computed(() => {
  const list = visibleRows.value.filter((r) => r.status !== 'draft')
  const add = (k) => list.reduce((a, r) => a + Number(r.summary[k]), 0).toFixed(2)
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
  valueGetter: (p) => Number(p.data.summary[field]),
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
    width: 130,
    valueFormatter: (p) => (p.data._totals ? p.value : p.value || 'koncept'),
    cellClass: (p) => (p.data._totals ? '' : p.value ? 'cell-number' : 'cell-draft'),
  },
  { colId: 'customer', headerName: 'Odberateľ', valueGetter: (p) => p.data.customer.name, flex: 1, minWidth: 130 },
  { field: 'issueDate', headerName: 'Dátum', valueFormatter: (p) => formatDate(p.value), width: 100 },
  { field: 'validUntil', headerName: 'Platná do', valueFormatter: (p) => formatDate(p.value), width: 100 },
  amountCol('base', 'Základ'),
  amountCol('total', 'Spolu', { cellClass: 'ag-right-aligned-cell cell-total', minWidth: 100 }),
  {
    colId: 'status',
    headerName: 'Stav',
    width: 190,
    valueGetter: (p) => (p.data._totals ? '' : quoteStatusOf(p.data).label),
    cellRenderer: (p) => {
      if (p.data._totals) return ''
      const s = quoteStatusOf(p.data)
      const inv = p.data.status === 'invoiced' ? `<span class="inv-ref">${p.data.invoiceNumber || 'koncept faktúry'}</span>` : ''
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>${inv}`
    },
  },
]

const defaultColDef = { sortable: true, resizable: true, suppressMovable: false }
const rowClassRules = {
  'row-cancelled': (p) => p.data.status === 'rejected',
  'row-overdue': (p) => !!p.data.expired,
}

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['issueDate', 'validUntil', 'base', 'status'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
function onGridReady(e) {
  gridApi.value = e.api
  applyResponsive()
}
narrow.addEventListener('change', applyResponsive)

function open(e) {
  if (e.data?._totals || !e.data?.id) return
  router.push(`/ponuky/${e.data.id}`)
}

function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && (t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName))
  if (typing || e.ctrlKey || e.metaKey || e.altKey) return
  if (e.key === 'n' && can('invoices.write')) {
    e.preventDefault()
    router.push('/ponuky/nova')
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
      <h1>Cenové ponuky</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <Button v-if="can('invoices.write')" size="small" @click="router.push('/ponuky/nova')">
        <template #default>
          <i class="pi pi-plus" /><span>Nová ponuka</span><kbd class="hint">N</kbd>
        </template>
      </Button>
    </header>

    <section class="ledger" aria-label="Prehľad za rok">
      <button class="figure figure-link" :disabled="!ledger.openCount" @click="view = 'open'">
        <span class="label">Odoslané, čakajú na odpoveď ({{ ledger.openCount }})</span>
        <span class="value">{{ formatMoney(ledger.open) }}</span>
      </button>
      <button class="figure figure-link" :disabled="!ledger.waitingCount" @click="view = 'accepted'">
        <span class="label">Prijaté, na vyfakturovanie ({{ ledger.waitingCount }})</span>
        <span class="value">{{ formatMoney(ledger.waiting) }}</span>
      </button>
      <div class="figure">
        <span class="label">Úspešnosť v {{ year }}</span>
        <span class="value">{{ ledger.rate === null ? '—' : `${ledger.rate} %` }}</span>
        <span class="sub" v-if="ledger.decided">{{ ledger.won }} z {{ ledger.decided }} rozhodnutých</span>
      </div>
    </section>

    <div class="toolbar">
      <SelectButton v-model="view" :options="quoteViews" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText ref="searchEl" v-model="search" placeholder="Hľadať číslo, odberateľa, IČO, položku" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton :name="`Cenové ponuky ${year}`" :columns="quoteColumns" :rows="() => gridRows(gridApi, visibleRows)" />
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
        :overlay-no-rows-template="'<span class=&quot;empty&quot;>Žiadne ponuky. Novú vytvoríte klávesom N.</span>'"
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
.figure-link {
  cursor: pointer;
}
.figure-link:hover:not(:disabled) {
  background: var(--blue-wash);
}
.figure-link:disabled {
  cursor: default;
}
.figure-link:disabled .value {
  color: var(--ink-faint);
}
.figure .sub {
  font-size: var(--fs-xs);
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
.grid :deep(.inv-ref) {
  margin-left: 6px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.grid :deep(.cell-draft) {
  color: var(--tone-draft);
  font-style: italic;
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
