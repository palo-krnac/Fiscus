<script setup>
// Pravidelná fakturácia: šablóny, z ktorých Fiškus sám vytvára faktúry.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import SelectButton from 'primevue/selectbutton'
import { api } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { matches, searchTerms } from '../lib/search.js'
import { recurringColumns } from '../lib/columns.js'
import { intervalLabel, monthlyAmount, recurringStatusOf } from '../lib/recurring.js'
import ExportButton from '../components/ExportButton.vue'

const router = useRouter()
const view = ref('all')
const views = [
  { label: 'Všetky', value: 'all' },
  { label: 'Aktívne', value: 'active' },
  { label: 'Pozastavené a skončené', value: 'inactive' },
]
const search = ref('')
const searchEl = ref(null)
const rows = shallowRef([])
const loading = ref(true)
const loadError = ref(null)

async function load() {
  loading.value = true
  try {
    rows.value = await api.recurringList(session.companyId)
    loadError.value = null
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
onMounted(load)

const isActive = (r) => r.active && !!r.nextDate
const visibleRows = computed(() => {
  const terms = searchTerms(search.value)
  return rows.value.filter((r) => {
    if (view.value === 'active' && !isActive(r)) return false
    if (view.value === 'inactive' && isActive(r)) return false
    const c = r.customer ?? {}
    return matches(terms, [r.name, c.name, c.ico, c.city, intervalLabel(r.interval), recurringStatusOf(r).label, r.summary?.total,
      ...(r.lines ?? []).map((l) => l.description)])
  })
})

// Prehľad zo súčtov už vypočítaných súm šablón.
const ledger = computed(() => {
  const active = rows.value.filter(isActive)
  const next = [...active].sort((a, b) => a.nextDate.localeCompare(b.nextDate))[0]
  return {
    count: active.length,
    monthly: active.reduce((a, r) => a + monthlyAmount(r), 0),
    next,
  }
})

const columnDefs = [
  { field: 'name', headerName: 'Názov', flex: 1, minWidth: 150, cellClass: 'cell-number', valueFormatter: (p) => p.value || p.data.customer?.name },
  { colId: 'customer', headerName: 'Odberateľ', valueGetter: (p) => p.data.customer.name, flex: 1, minWidth: 130 },
  { colId: 'interval', headerName: 'Interval', valueGetter: (p) => intervalLabel(p.data.interval), width: 104 },
  { field: 'nextDate', headerName: 'Ďalšia faktúra', valueFormatter: (p) => (p.value ? formatDate(p.value) : '—'), width: 116 },
  {
    colId: 'total', headerName: 'Suma', type: 'rightAligned', width: 112, cellClass: 'ag-right-aligned-cell cell-total',
    valueGetter: (p) => Number(p.data.summary.total), valueFormatter: (p) => formatAmount(p.value),
  },
  { colId: 'auto', headerName: 'Vystaví', width: 104, valueGetter: (p) => (p.data.autoIssue ? 'automaticky' : 'koncept') },
  { field: 'generated', headerName: 'Vytvorených', type: 'rightAligned', width: 104 },
  {
    colId: 'status', headerName: 'Stav', width: 130,
    valueGetter: (p) => recurringStatusOf(p.data).label,
    cellRenderer: (p) => {
      const s = recurringStatusOf(p.data)
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>`
    },
  },
]
const defaultColDef = { sortable: true, resizable: true, suppressMovable: false }
const rowClassRules = { 'row-cancelled': (p) => !isActive(p.data) }

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['customer', 'interval', 'auto', 'generated', 'status'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
function onGridReady(e) {
  gridApi.value = e.api
  applyResponsive()
}
narrow.addEventListener('change', applyResponsive)

function open(e) {
  if (e.data?.id) router.push(`/pravidelne/${e.data.id}`)
}

function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && (t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName))
  if (typing || e.ctrlKey || e.metaKey || e.altKey) return
  if (e.key === 'n' && can('invoices.write')) {
    e.preventDefault()
    router.push('/pravidelne/nova')
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
      <h1>Pravidelná fakturácia</h1>
      <div class="spacer" />
      <Button v-if="can('invoices.write')" size="small" @click="router.push('/pravidelne/nova')">
        <template #default>
          <i class="pi pi-plus" /><span>Nová pravidelná faktúra</span><kbd class="hint">N</kbd>
        </template>
      </Button>
    </header>

    <section class="ledger" aria-label="Prehľad">
      <div class="figure">
        <span class="label">Aktívne šablóny</span>
        <span class="value">{{ ledger.count }}</span>
      </div>
      <div class="figure">
        <span class="label">Mesačne v priemere</span>
        <span class="value">{{ formatMoney(ledger.monthly) }}</span>
      </div>
      <div class="figure">
        <span class="label">Najbližšia faktúra</span>
        <span class="value">{{ ledger.next ? formatDate(ledger.next.nextDate) : '—' }}</span>
        <span v-if="ledger.next" class="sub">{{ ledger.next.name || ledger.next.customer.name }}</span>
      </div>
    </section>

    <p class="explain">
      Fiškus vytvorí faktúru sám v deň ďalšej faktúry (pri spustení programu a potom každú hodinu). Ak program v ten deň nebežal,
      faktúru dobehne neskôr. Dobehnuté faktúry z minulých mesiacov ostanú ako koncepty na kontrolu.
    </p>

    <div class="toolbar">
      <SelectButton v-model="view" :options="views" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText ref="searchEl" v-model="search" placeholder="Hľadať názov, odberateľa, položku" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton name="Pravidelná fakturácia" :columns="recurringColumns" :rows="() => gridRows(gridApi, visibleRows)" />
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <div class="grid-wrap">
      <AgGridVue
        class="grid"
        :theme="fiskusGridTheme"
        :row-data="visibleRows"
        :column-defs="columnDefs"
        :default-col-def="defaultColDef"
        :row-class-rules="rowClassRules"
        :locale-text="skGridLocale"
        :loading="loading"
        :get-row-id="(p) => String(p.data.id)"
        :overlay-no-rows-template="'<span class=&quot;empty&quot;>Žiadne pravidelné faktúry. Novú vytvoríte klávesom N.</span>'"
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

.explain {
  margin: 0;
  max-width: 900px;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
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
