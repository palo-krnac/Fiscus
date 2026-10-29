<script setup>
// Kurzy mien ECB: prehľad (hľadanie, export), aktualizácia a ručný kurz (správca programu).
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import { api } from '../lib/api.js'
import { isSuperAdmin } from '../lib/session.js'
import { codes } from '../lib/codes.js'
import { formatDate, toISODate } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { rateColumns } from '../lib/columns.js'
import { filterRates, formatRate, sourceLabel } from '../lib/fx.js'
import ExportButton from '../components/ExportButton.vue'

const toast = useToast()
const admin = computed(() => isSuperAdmin())

const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
const year = ref(thisYear)
const currency = ref(null)
const currencyOptions = computed(() => [{ code: null, label: 'Všetky meny' }, ...codes.currencies.map((c) => ({ code: c.code, label: `${c.code} – ${c.name}` }))])
const search = ref('')
const searchEl = ref(null)
const rows = shallowRef([])
const loading = ref(true)
const loadError = ref(null)

async function load() {
  loading.value = true
  try {
    rows.value = await api.rates({ year: year.value, currency: currency.value })
    loadError.value = null
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
watch([year, currency], load)
onMounted(load)

const visibleRows = computed(() => filterRates(rows.value, search.value))
const latest = computed(() => rows.value.reduce((d, r) => (r.date > d ? r.date : d), ''))

const columnDefs = computed(() => [
  { field: 'date', headerName: 'Dátum', valueFormatter: (p) => formatDate(p.value), width: 120, sort: 'desc' },
  { field: 'currency', headerName: 'Mena', width: 90, cellClass: 'cell-number' },
  {
    colId: 'name',
    headerName: 'Názov',
    valueGetter: (p) => codes.currencies.find((c) => c.code === p.data.currency)?.name ?? '',
    flex: 1,
    minWidth: 120,
  },
  { field: 'rate', headerName: 'Kurz za 1 EUR', valueGetter: (p) => Number(p.data.rate), valueFormatter: (p) => formatRate(p.value), type: 'rightAligned', width: 140 },
  { colId: 'source', headerName: 'Zdroj', valueGetter: (p) => sourceLabel(p.data.source), width: 90 },
  ...(admin.value
    ? [{
        colId: 'act',
        headerName: '',
        width: 48,
        minWidth: 48,
        sortable: false,
        cellRenderer: () => '<button class="del" title="Zmazať kurz" aria-label="Zmazať kurz"><i class="pi pi-trash"></i></button>',
        onCellClicked: (p) => remove(p.data),
      }]
    : []),
])
const defaultColDef = { sortable: true, resizable: true }
const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['name', 'source'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
function onGridReady(e) {
  gridApi.value = e.api
  applyResponsive()
}
narrow.addEventListener('change', applyResponsive)

// ---- správca ----
const updating = ref(false)
async function update() {
  updating.value = true
  try {
    const res = await api.updateRates()
    toast.add({ severity: 'success', summary: 'Kurzy aktualizované', detail: `${res.count} kurzov, posledné z ${formatDate(res.latest)}`, life: 4000 })
    load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Aktualizácia zlyhala', detail: e.message, life: 8000 })
  } finally {
    updating.value = false
  }
}

const manual = ref({ currency: 'CZK', date: new Date(), rate: null })
const saving = ref(false)
async function saveManual() {
  saving.value = true
  try {
    await api.saveRate({ currency: manual.value.currency, date: toISODate(manual.value.date), rate: String(manual.value.rate ?? 0) })
    toast.add({ severity: 'success', summary: 'Kurz uložený', life: 2500 })
    manual.value.rate = null
    load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Kurz sa neuložil', detail: e.message, life: 6000 })
  } finally {
    saving.value = false
  }
}
async function remove(r) {
  try {
    await api.deleteRate(r.currency, r.date)
    load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Kurz sa nezmazal', detail: e.message, life: 6000 })
  }
}

function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && (t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName))
  if (!typing && e.key === '/') {
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
      <h1>Kurzy mien</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <Button v-if="admin" label="Aktualizovať z ECB" icon="pi pi-refresh" size="small" :loading="updating" @click="update" />
    </header>
    <p class="help">
      Referenčné kurzy ECB (koľko jednotiek meny je 1 euro). Faktúra v cudzej mene sa prepočíta kurzom vyhláseným deň pred dňom dodania;
      cez víkend platí posledný vyhlásený kurz (§ 26 zákona o DPH). Program kurzy sťahuje sám.
      <template v-if="latest"> Posledný kurz v zozname: <strong>{{ formatDate(latest) }}</strong>.</template>
    </p>

    <form v-if="admin" class="manual" @submit.prevent="saveManual">
      <span class="manual-label">Ručný kurz</span>
      <Select v-model="manual.currency" :options="codes.currencies" option-label="code" option-value="code" filter size="small" aria-label="Mena" />
      <DatePicker v-model="manual.date" date-format="d. m. yy" show-icon icon-display="input" size="small" aria-label="Dátum" />
      <InputNumber v-model="manual.rate" :min-fraction-digits="0" :max-fraction-digits="4" :use-grouping="false" locale="sk-SK" size="small" placeholder="kurz za 1 EUR" aria-label="Kurz" />
      <Button type="submit" label="Uložiť" severity="secondary" outlined size="small" :loading="saving" :disabled="!manual.rate" />
    </form>

    <div class="toolbar">
      <Select v-model="currency" :options="currencyOptions" option-label="label" option-value="code" filter size="small" class="cur" placeholder="Všetky meny" aria-label="Mena" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText ref="searchEl" v-model="search" placeholder="Hľadať menu, dátum, kurz" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton :name="`Kurzy ${currency || 'mien'} ${year}`" :columns="rateColumns" :rows="() => gridRows(gridApi, visibleRows)" />
    </div>
    <p v-if="loadError" class="error">{{ loadError }}</p>
    <div class="grid-wrap">
      <AgGridVue
        class="grid"
        :theme="fiskusGridTheme"
        :row-data="visibleRows"
        :column-defs="columnDefs"
        :default-col-def="defaultColDef"
        :locale-text="skGridLocale"
        :loading="loading"
        :get-row-id="(p) => p.data.currency + p.data.date"
        :overlay-no-rows-template="'<span class=&quot;empty&quot;>Žiadne kurzy za vybrané obdobie.</span>'"
        @grid-ready="onGridReady"
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
.help {
  margin: 0;
  max-width: 90ch;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.manual {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 8px 10px;
  background: var(--sheet);
  border: 1px solid var(--rule);
}
.manual-label {
  font-size: var(--fs-sm);
  font-weight: 600;
}
.manual :deep(.p-datepicker) {
  width: 150px;
}
.manual :deep(.p-inputnumber input) {
  width: 140px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.cur {
  width: 230px;
}
.search {
  margin-left: auto;
  width: 260px;
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
.grid :deep(.cell-number) {
  font-weight: 600;
  color: var(--blue-deep);
}
.grid :deep(.del) {
  border: 0;
  background: none;
  color: var(--tone-overdue);
  cursor: pointer;
  padding: 4px;
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
  .cur {
    flex: 1;
    width: auto;
  }
  .search {
    flex: 1 1 100%;
    order: 3;
    width: auto;
    margin-left: 0;
  }
}
</style>
