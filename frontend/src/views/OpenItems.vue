<script setup>
// Kniha pohľadávok a záväzkov (jednoduché účtovníctvo, § 5 postupov účtovania):
// neuhradené vydané a prijaté doklady k zvolenému dňu, napr. k 31. 12.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { AgGridVue } from 'ag-grid-vue3'
import DatePicker from 'primevue/datepicker'
import SelectButton from 'primevue/selectbutton'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { api } from '../lib/api.js'
import { formatAmount, formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { session } from '../lib/session.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { matches, searchTerms } from '../lib/search.js'
import { openItemColumns } from '../lib/columns.js'
import { kindLabel } from '../lib/advance.js'
import ExportButton from '../components/ExportButton.vue'

const router = useRouter()
const today = new Date()
const lastYearEnd = new Date(today.getFullYear() - 1, 11, 31)
const date = ref(today)
const view = ref('receivables')
const views = [
  { label: 'Pohľadávky', value: 'receivables' },
  { label: 'Záväzky', value: 'payables' },
]
const search = ref('')
const data = ref({ receivables: [], payables: [], receivablesTotal: '0', payablesTotal: '0' })
const loading = ref(true)
const loadError = ref('')

async function load() {
  loading.value = true
  try {
    data.value = await api.openItems(session.companyId, toISODate(date.value))
    loadError.value = ''
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
watch(date, load)
onMounted(load)

const setDate = (d) => {
  date.value = d
}
const isDay = (d) => toISODate(date.value) === toISODate(d)

const kindText = (k) => (k === 'purchase' ? 'prijatá faktúra' : kindLabel(k))
const rows = computed(() => {
  const terms = searchTerms(search.value)
  return (data.value[view.value] ?? []).filter((r) => matches(terms, [r.number, r.partner, r.ico, r.remaining, kindText(r.kind)]))
})
const exportCols = computed(() => openItemColumns(view.value === 'payables'))

const columnDefs = computed(() => [
  { field: 'number', headerName: 'Číslo', width: 130, cellClass: 'cell-number' },
  { colId: 'kind', headerName: 'Druh', valueGetter: (p) => kindText(p.data.kind), width: 150 },
  { field: 'partner', headerName: view.value === 'payables' ? 'Dodávateľ' : 'Odberateľ', flex: 1, minWidth: 130 },
  { field: 'issueDate', headerName: view.value === 'payables' ? 'Doručená' : 'Vystavená', valueFormatter: (p) => formatDate(p.value), width: 104 },
  { field: 'dueDate', headerName: 'Splatná', valueFormatter: (p) => formatDate(p.value), width: 100 },
  {
    field: 'total', headerName: 'Suma', type: 'rightAligned', width: 112,
    valueFormatter: (p) => formatAmount(p.value) + (p.data.currency !== 'EUR' ? ` ${p.data.currency}` : ''),
  },
  { field: 'paid', headerName: 'Uhradené', type: 'rightAligned', width: 104, valueFormatter: (p) => formatAmount(p.value) },
  {
    field: 'remainingEur', headerName: 'Zostáva €', type: 'rightAligned', width: 110, valueFormatter: (p) => formatAmount(p.value),
    cellClass: 'ag-right-aligned-cell cell-total', comparator: (a, b) => Number(a) - Number(b),
  },
  {
    field: 'daysOverdue', headerName: 'Po splatnosti', type: 'rightAligned', width: 116,
    valueFormatter: (p) => (p.value ? `${p.value} d` : ''), cellClass: (p) => (p.value ? 'ag-right-aligned-cell cell-late' : 'ag-right-aligned-cell'),
  },
])

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['kind', 'issueDate', 'dueDate', 'total', 'paid', 'daysOverdue'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
narrow.addEventListener('change', applyResponsive)
onBeforeUnmount(() => narrow.removeEventListener('change', applyResponsive))

function open(r) {
  router.push(r.kind === 'purchase' ? `/prijate/${r.id}` : `/faktury/${r.id}`)
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Pohľadávky a záväzky</h1>
      <span class="at">k</span>
      <DatePicker :model-value="date" date-format="d. m. yy" show-icon icon-display="input" size="small" class="date" aria-label="Ku dňu"
        @update:model-value="(d) => d && setDate(d)" />
      <div class="quick" role="group" aria-label="Rýchly výber dňa">
        <button type="button" :class="{ on: isDay(today) }" @click="setDate(today)">dnes</button>
        <button type="button" :class="{ on: isDay(lastYearEnd) }" @click="setDate(lastYearEnd)">31. 12. {{ lastYearEnd.getFullYear() }}</button>
      </div>
    </header>

    <section class="ledger" aria-label="Súhrn">
      <div class="figure">
        <span class="label">Pohľadávky ({{ data.receivables.length }})</span>
        <span class="value">{{ formatMoney(data.receivablesTotal) }}</span>
      </div>
      <div class="figure">
        <span class="label">Záväzky ({{ data.payables.length }})</span>
        <span class="value">{{ formatMoney(data.payablesTotal) }}</span>
      </div>
    </section>

    <div class="toolbar">
      <SelectButton v-model="view" :options="views" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Kniha" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať číslo, partnera, IČO, sumu" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton :name="`${view === 'payables' ? 'Záväzky' : 'Pohľadávky'} k ${formatDate(toISODate(date))}`" :columns="exportCols"
        :rows="() => gridRows(gridApi, rows)" />
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <div class="grid-wrap">
      <AgGridVue
        class="grid"
        :theme="fiskusGridTheme"
        :row-data="rows"
        :column-defs="columnDefs"
        :default-col-def="{ sortable: true, resizable: true }"
        :locale-text="skGridLocale"
        :loading="loading"
        :get-row-id="(p) => p.data.kind + p.data.id"
        :overlay-no-rows-template="'<span>K tomuto dňu nie je nič neuhradené.</span>'"
        @grid-ready="(e) => { gridApi = e.api; applyResponsive() }"
        @row-clicked="(e) => open(e.data)"
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
  gap: 10px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
}
.at {
  color: var(--ink-soft);
}
.date {
  width: 150px;
}
.quick {
  display: flex;
  gap: 4px;
}
.quick button {
  font: inherit;
  font-size: var(--fs-sm);
  padding: 3px 8px;
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  background: var(--sheet);
  color: var(--ink-soft);
  cursor: pointer;
}
.quick button.on {
  border-color: var(--blue);
  color: var(--blue-deep);
  background: var(--blue-wash);
  font-weight: 600;
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
.grid :deep(.cell-late) {
  color: var(--tone-overdue);
  font-weight: 600;
}
.error {
  color: var(--tone-overdue);
  margin: 0;
}
@media (max-width: 760px) {
  .page {
    padding: 10px 10px 8px;
  }
  .figure {
    padding: 6px 12px 7px 10px;
    flex: 1;
  }
  .figure .value {
    font-size: var(--fs-lg);
  }
  .at {
    display: none;
  }
  .search {
    order: 3;
    flex: 1 0 100%;
    width: auto;
    margin-left: 0;
  }
}
</style>
