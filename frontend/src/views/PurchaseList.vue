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
import { formatAmount, formatDate, formatMoney, purchaseStatusOf } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { matches, searchTerms } from '../lib/search.js'
import { purchaseColumns } from '../lib/columns.js'
import ExportButton from '../components/ExportButton.vue'
import { useToast } from 'primevue/usetoast'
import { setPurchaseDraft } from '../lib/drafts.js'

const router = useRouter()
const toast = useToast()

// ---- doklad z PDF alebo fotky (Claude API) ----
const aiReady = ref(false)
const reading = ref(false)
const pdfInput = ref(null)
onMounted(async () => {
  try {
    aiReady.value = (await api.aiSettings()).keySet
  } catch {
    aiReady.value = false
  }
})
async function readDocument(e) {
  const file = e.target.files?.[0]
  e.target.value = ''
  if (!file) return
  reading.value = true
  try {
    setPurchaseDraft(await api.extractPurchase(session.companyId, file))
    router.push('/prijate/nova')
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Doklad sa nepodarilo prečítať', detail: err.message, life: 8000 })
  } finally {
    reading.value = false
  }
}
const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
const year = ref(thisYear)
const view = ref('all')
const views = [
  { label: 'Všetky', value: 'all' },
  { label: 'Neuhradené', value: 'unpaid' },
  { label: 'Po splatnosti', value: 'overdue' },
  { label: 'Uhradené', value: 'paid' },
]
const search = ref('')
const rows = shallowRef([])
const loading = ref(true)
const loadError = ref(null)

async function load() {
  loading.value = true
  try {
    rows.value = await api.purchases(session.companyId, { year: year.value })
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
    if (view.value === 'unpaid' && r.status !== 'booked') return false
    if (view.value === 'overdue' && !r.overdue) return false
    if (view.value === 'paid' && r.status !== 'paid') return false
    const s = r.supplier ?? {}
    return matches(terms, [r.number, r.supplierNumber, r.variableSymbol, s.name, s.ico, s.icdph, s.city, r.description, r.note, r.summary?.total])
  })
})

const sum = (list, k) => list.reduce((a, r) => a + Number(r.summary[k]), 0)
const ledger = computed(() => {
  const unpaid = rows.value.filter((r) => r.status === 'booked')
  const overdue = unpaid.filter((r) => r.overdue)
  const deductible = rows.value.filter((r) => !r.noDeduction)
  return {
    total: sum(rows.value, 'total'),
    unpaid: sum(unpaid, 'total'),
    unpaidCount: unpaid.length,
    overdue: sum(overdue, 'total'),
    overdueCount: overdue.length,
    vat: sum(deductible, 'vat'),
  }
})

const totalsRow = computed(() => {
  const add = (k) => sum(visibleRows.value, k).toFixed(2)
  return [{ number: `Spolu ${visibleRows.value.length}`, supplier: { name: '' }, summary: { base: add('base'), vat: add('vat'), total: add('total') }, _totals: true }]
})

const amountCol = (field, header, extra = {}) => ({
  colId: field,
  headerName: header,
  valueGetter: (p) => Number(p.data.summary[field]),
  valueFormatter: (p) => formatAmount(p.value),
  type: 'rightAligned',
  width: 112,
  ...extra,
})

const columnDefs = [
  { field: 'number', headerName: 'Číslo', width: 118, cellClass: (p) => (p.data._totals ? '' : 'cell-number') },
  { field: 'supplierNumber', headerName: 'Č. dokladu', width: 130 },
  { colId: 'supplier', headerName: 'Dodávateľ', valueGetter: (p) => p.data.supplier.name, flex: 1, minWidth: 170 },
  { field: 'receivedDate', headerName: 'Doručená', valueFormatter: (p) => formatDate(p.value), width: 100 },
  { field: 'dueDate', headerName: 'Splatná', valueFormatter: (p) => formatDate(p.value), width: 100 },
  amountCol('base', 'Základ'),
  amountCol('vat', 'DPH', { width: 96 }),
  amountCol('total', 'Spolu', { cellClass: 'ag-right-aligned-cell cell-total' }),
  {
    colId: 'status',
    headerName: 'Stav',
    width: 160,
    cellRenderer: (p) => {
      if (p.data._totals) return ''
      const s = purchaseStatusOf(p.data)
      const flags = [p.data.reverseCharge && 'prenesenie', p.data.noDeduction && 'bez odpočtu'].filter(Boolean)
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>` +
        (flags.length ? `<span class="flag">${flags.join(', ')}</span>` : '')
    },
  },
]

const rowClassRules = { 'row-overdue': (p) => p.data.overdue }
const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['supplierNumber', 'receivedDate', 'dueDate', 'base', 'vat', 'status'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
narrow.addEventListener('change', applyResponsive)
const rowId = (p) => (p.data._totals ? 'totals' : String(p.data.id))

function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && /INPUT|TEXTAREA|SELECT/.test(t.tagName)
  if (!typing && !e.ctrlKey && !e.metaKey && !e.altKey && e.key === 'n' && can('purchases.write')) {
    e.preventDefault()
    router.push('/prijate/nova')
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
      <h1>Prijaté faktúry</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <template v-if="can('purchases.write')">
        <input ref="pdfInput" type="file" accept="application/pdf,image/png,image/jpeg,image/webp" hidden @change="readDocument" />
        <Button icon="pi pi-sparkles" label="Z PDF" aria-label="Načítať z PDF" class="ai-btn" severity="secondary" outlined size="small" :loading="reading" :disabled="!aiReady"
          v-tooltip.bottom="aiReady ? 'Prečítať PDF alebo fotku faktúry cez Claude (AI) — údaje skontrolujete pred uložením' : 'Čítanie dokladov zapne správca v časti Prístup pre AI'"
          @click="pdfInput.click()" />
      </template>
      <Button v-if="can('purchases.write')" size="small" @click="router.push('/prijate/nova')">
        <i class="pi pi-plus" /><span>Nová prijatá faktúra</span><kbd class="hint">N</kbd>
      </Button>
    </header>

    <section class="ledger" aria-label="Súhrn za rok">
      <div class="figure">
        <span class="label">Prijaté v {{ year }}</span>
        <span class="value">{{ formatMoney(ledger.total) }}</span>
      </div>
      <div class="figure">
        <span class="label">DPH na odpočet</span>
        <span class="value">{{ formatMoney(ledger.vat) }}</span>
      </div>
      <div class="figure">
        <span class="label">Na úhradu ({{ ledger.unpaidCount }})</span>
        <span class="value">{{ formatMoney(ledger.unpaid) }}</span>
      </div>
      <button class="figure figure-alert" :class="{ none: !ledger.overdueCount }" :disabled="!ledger.overdueCount" @click="view = 'overdue'">
        <span class="label">Po splatnosti ({{ ledger.overdueCount }})</span>
        <span class="value">{{ formatMoney(ledger.overdue) }}</span>
      </button>
    </section>

    <div class="toolbar">
      <SelectButton v-model="view" :options="views" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať číslo, dodávateľa, IČO, predmet, sumu" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton :name="`Prijaté faktúry ${year}`" :columns="purchaseColumns" :rows="() => gridRows(gridApi, visibleRows)" />
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <div class="grid-wrap">
      <AgGridVue
        class="grid"
        :theme="fiskusGridTheme"
        :row-data="visibleRows"
        :pinned-bottom-row-data="totalsRow"
        :column-defs="columnDefs"
        :default-col-def="{ sortable: true, resizable: true }"
        :row-class-rules="rowClassRules"
        :locale-text="skGridLocale"
        :loading="loading"
        :get-row-id="rowId"
        :overlay-no-rows-template="'<span>Žiadne prijaté faktúry. Novú zaevidujete klávesom N.</span>'"
        @grid-ready="(e) => { gridApi = e.api; applyResponsive() }"
        @row-clicked="(e) => !e.data._totals && router.push(`/prijate/${e.data.id}`)"
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
.head :deep(.p-button) {
  gap: 6px;
}
.hint {
  margin-left: 6px;
  background: rgba(255, 255, 255, 0.16);
  border-color: rgba(255, 255, 255, 0.3);
  color: #fff;
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
.figure-alert {
  cursor: pointer;
}
.figure-alert .value {
  color: var(--tone-overdue);
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
.grid :deep(.cell-total) {
  font-weight: 600;
}
.grid :deep(.flag) {
  margin-left: 8px;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.error {
  color: var(--tone-overdue);
  margin: 0;
}
@media (max-width: 760px) {
  .head :deep(.ai-btn .p-button-label) {
    display: none;
  }
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
