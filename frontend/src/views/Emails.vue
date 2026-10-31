<script setup>
// E-maily: zoznam odoslaných správ (hľadanie, export). Nastavenie servera
// SMTP a textov je v Nastaveniach → E-mail (components/EmailSettings.vue).
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { emailColumns } from '../lib/columns.js'
import {
  docKindLabel, docRoute, emailStatus, emailViews, filterEmails,
} from '../lib/emails.js'
import ExportButton from '../components/ExportButton.vue'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const canSettings = computed(() => can('settings.write'))
// nastavenie e-mailu je v Nastaveniach (staré odkazy ?nastavenie=1 tam presmerujú)
const tab = ref('list')
if (route.query.nastavenie) router.replace('/nastavenia/e-mail')

// ---- zoznam ----
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
    rows.value = await api.emails(session.companyId, year.value)
    loadError.value = null
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
watch(year, load)
onMounted(load)

const visibleRows = computed(() => filterEmails(rows.value, view.value, search.value))
const failedCount = computed(() => rows.value.filter((e) => e.error).length)

const fmt = new Intl.DateTimeFormat('sk-SK', { day: 'numeric', month: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
const columnDefs = [
  { field: 'sent', headerName: 'Odoslaný', valueFormatter: (p) => fmt.format(new Date(p.value)), width: 140, minWidth: 128, sort: 'desc' },
  { colId: 'kind', headerName: 'Doklad', valueGetter: (p) => docKindLabel(p.data.docKind), width: 130 },
  { field: 'docNumber', headerName: 'Číslo', width: 120, cellClass: 'cell-number' },
  { field: 'to', headerName: 'Komu', flex: 1, minWidth: 130 },
  { field: 'subject', headerName: 'Predmet', flex: 1.4, minWidth: 180 },
  { field: 'user', headerName: 'Poslal', width: 130 },
  {
    colId: 'status',
    headerName: 'Stav',
    width: 120,
    valueGetter: (p) => emailStatus(p.data).label,
    tooltipValueGetter: (p) => p.data.error || null,
    cellRenderer: (p) => {
      const s = emailStatus(p.data)
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>`
    },
  },
]
const defaultColDef = { sortable: true, resizable: true }
const rowClassRules = { 'row-overdue': (p) => !!p.data.error }

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['kind', 'docNumber', 'subject', 'user'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
function onGridReady(e) {
  gridApi.value = e.api
  applyResponsive()
}
narrow.addEventListener('change', applyResponsive)

function open(e) {
  const to = e.data && docRoute(e.data)
  if (to) router.push(to)
}

function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && (t.isContentEditable || /INPUT|TEXTAREA|SELECT/.test(t.tagName))
  if (!typing && e.key === '/' && tab.value === 'list') {
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
      <h1>E-maily</h1>
      <div class="spacer" />
      <Button v-if="canSettings" label="Nastavenie e-mailu" icon="pi pi-cog" size="small" text @click="router.push('/nastavenia/e-mail')" />
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
    </header>

    <template v-if="tab === 'list'">
      <div class="toolbar">
        <SelectButton v-model="view" :options="emailViews" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
        <span v-if="failedCount" class="failed">{{ failedCount }} neodoslaných</span>
        <IconField class="search">
          <InputIcon class="pi pi-search" />
          <InputText ref="searchEl" v-model="search" placeholder="Hľadať číslo, adresu, predmet" size="small" aria-label="Hľadať" />
        </IconField>
        <ExportButton :name="`E-maily ${year}`" :columns="emailColumns" :rows="() => gridRows(gridApi, visibleRows)" />
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
          :tooltip-show-delay="300"
          :get-row-id="(p) => String(p.data.id)"
          :overlay-no-rows-template="'<span class=&quot;empty&quot;>Zatiaľ žiadne e-maily. Doklad pošlete tlačidlom „Poslať e-mailom“.</span>'"
          @grid-ready="onGridReady"
          @row-clicked="open"
        />
      </div>
    </template>

  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 14px 16px 12px;
  gap: 10px;
  overflow: auto;
}
.head {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
}
.spacer {
  flex: 1;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.failed {
  font-size: var(--fs-sm);
  color: var(--tone-overdue);
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
.error {
  color: var(--tone-overdue);
  margin: 0;
}
.body {
  max-width: 880px;
}
.span2 {
  grid-column: span 2;
}
.switch {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  cursor: pointer;
  align-self: end;
}
.switch span {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.switch small {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.check {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.note {
  margin: 0 0 10px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.test-row {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  flex-wrap: wrap;
}
.test-row :deep(input) {
  width: 260px;
}
.tpl {
  display: grid;
  gap: 8px;
}
.tpl-tabs {
  margin-bottom: 10px;
}
.help {
  margin: 8px 0 0;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  line-height: 1.6;
}
.help code {
  font-size: var(--fs-xs);
  background: var(--rule-soft);
  padding: 1px 4px;
  border-radius: 3px;
}
@media (max-width: 760px) {
  .page {
    padding: 10px 10px 8px;
  }
  .head h1 {
    font-size: var(--fs-lg);
  }
  .search {
    flex: 1 1 100%;
    order: 3;
    width: auto;
    min-width: 0;
    margin-left: 0;
  }
  .span2 {
    grid-column: 1 / -1;
  }
  .test-row :deep(input) {
    width: 100%;
  }
}
</style>
