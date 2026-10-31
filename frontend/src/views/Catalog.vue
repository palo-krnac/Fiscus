<script setup>
// Cenník: opakované položky faktúr. Ceny bez DPH a s DPH prepočítava backend.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Textarea from 'primevue/textarea'
import Dialog from 'primevue/dialog'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { codes, ratesOn } from '../lib/codes.js'
import { formatAmount, formatQuantity } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { itemColumns } from '../lib/columns.js'
import { filterItems, importActions, importErrors, importLabel } from '../lib/catalog.js'

const props = defineProps({ id: String })
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('invoices.write'))

const items = shallowRef([])
const loading = ref(true)
const search = ref('')
const showArchived = ref(false)
const form = ref(null)
const errors = ref({})
const busy = ref(false)

const units = computed(() => (session.company?.units?.length ? session.company.units : codes.defaultUnits))
const nonVat = computed(() => !session.company?.vatPayer)
const rates = computed(() => ratesOn(new Date().toISOString().slice(0, 10)))
const blank = () => ({ code: '', name: '', unit: units.value[0] ?? 'ks', price: null, priceMode: 'net', vatRate: nonVat.value ? 0 : rates.value[0], note: '', archived: false })
const priceModes = [
  { label: 'bez DPH', value: 'net' },
  { label: 's DPH', value: 'gross' },
]

// ---- import zo súboru CSV / Excel ----
const fileInput = ref(null)
const importFile = ref(null)
const importPreview = ref(null)
const importing = ref(false)
async function pickImport(e) {
  const file = e.target.files?.[0]
  e.target.value = ''
  if (!file) return
  if (file.size > 5 * 1024 * 1024) {
    toast.add({ severity: 'error', summary: 'Súbor je príliš veľký', detail: 'Najviac 5 MB.', life: 5000 })
    return
  }
  importing.value = true
  try {
    importPreview.value = await api.importItems(session.companyId, file)
    importFile.value = file
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Súbor sa nedá načítať', detail: err.message, life: 8000 })
  } finally {
    importing.value = false
  }
}
async function runImport() {
  importing.value = true
  try {
    const res = await api.importItems(session.companyId, importFile.value, true)
    toast.add({ severity: 'success', summary: 'Cenník importovaný', detail: `${res.created} nových, ${res.updated} zmenených`, life: 4000 })
    importPreview.value = null
    load()
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Import zlyhal', detail: err.message, life: 8000 })
  } finally {
    importing.value = false
  }
}

async function load() {
  loading.value = true
  try {
    items.value = await api.items(session.companyId)
    applyResponsive()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Cenník sa nenačítal', detail: e.message, life: 6000 })
  } finally {
    loading.value = false
  }
}

function openFromRoute() {
  errors.value = {}
  if (props.id === 'nova') form.value = blank()
  else if (props.id) {
    const it = items.value.find((x) => String(x.id) === props.id)
    form.value = it ? { ...it, price: Number(it.price) } : null
  } else form.value = null
}
onMounted(async () => {
  await load()
  openFromRoute()
})
watch(() => props.id, openFromRoute)

const rows = computed(() => filterItems(items.value, search.value, { archived: showArchived.value }))

const money = (field, header, extra = {}) => ({
  field, headerName: header, width: 116, type: 'rightAligned',
  valueGetter: (p) => Number(p.data[field]), valueFormatter: (p) => formatAmount(p.value), ...extra,
})
// náhľad hlavného obrázka (id je číslo z API, do HTML ide bezpečne)
const thumb = (p) =>
  p.data.imageId ? `<img class="thumb" src="${api.itemImageURL(session.companyId, p.data.id, Number(p.data.imageId))}" alt="" loading="lazy">` : ''
const columnDefs = [
  { colId: 'img', headerName: '', width: 46, sortable: false, resizable: false, cellRenderer: thumb, cellClass: 'cell-img' },
  { field: 'code', headerName: 'Kód', width: 90, cellClass: 'cell-code' },
  { field: 'name', headerName: 'Názov', flex: 2, minWidth: 180, cellClass: 'cell-name' },
  { field: 'category', headerName: 'Kategória', flex: 1, minWidth: 120, cellClass: 'cell-code' },
  { field: 'ean', headerName: 'EAN', width: 124, cellClass: 'cell-code' },
  { field: 'unit', headerName: 'MJ', width: 70 },
  {
    colId: 'stock', headerName: 'Sklad', width: 90, type: 'rightAligned',
    valueGetter: (p) => (p.data.stock === undefined ? null : Number(p.data.stock)),
    valueFormatter: (p) => (p.value === null ? '' : formatQuantity(p.value)),
    cellClassRules: { 'cell-neg': (p) => p.value !== null && p.value <= 0 },
  },
  money('priceNet', 'Bez DPH'),
  { field: 'vatRate', headerName: 'DPH', width: 74, type: 'rightAligned', valueFormatter: (p) => `${p.value} %` },
  money('priceGross', 'S DPH', { cellClass: 'ag-right-aligned-cell cell-name' }),
]

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['code', 'vatRate', 'priceNet', 'category', 'ean', 'unit'], !narrow.matches)
  // stĺpce, ktoré nikto nevyplnil, sa neukazujú
  gridApi.value?.setColumnsVisible(['img'], items.value.some((it) => it.imageId))
  if (!narrow.matches) {
    gridApi.value?.setColumnsVisible(['category'], items.value.some((it) => it.category))
    gridApi.value?.setColumnsVisible(['ean'], items.value.some((it) => it.ean))
  }
  gridApi.value?.setColumnsVisible(['stock'], items.value.some((it) => it.stock !== undefined))
  gridApi.value?.sizeColumnsToFit()
}
narrow.addEventListener('change', applyResponsive)

const rowId = (p) => String(p.data.id)
const selectedId = computed(() => form.value?.id ?? null)
const rowClassRules = { 'row-selected': (p) => p.data.id === selectedId.value, 'row-archived': (p) => p.data.archived }
watch(selectedId, () => gridApi.value?.redrawRows())

async function save() {
  busy.value = true
  try {
    const wasNew = !form.value.id
    const saved = await api.saveItem(session.companyId, { ...form.value, price: String(form.value.price ?? 0) })
    await load()
    errors.value = {}
    form.value = { ...saved, price: Number(saved.price) }
    toast.add({ severity: 'success', summary: wasNew ? 'Položka pridaná' : 'Položka uložená', life: 2500 })
    if (wasNew) router.replace(`/cennik/${saved.id}`)
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) {
      errors.value = e.fieldMap()
      toast.add({ severity: 'error', summary: 'Skontrolujte vyznačené polia', detail: e.fields[0].message, life: 4000 })
    } else {
      toast.add({ severity: 'error', summary: 'Uloženie zlyhalo', detail: e.message, life: 6000 })
    }
  } finally {
    busy.value = false
  }
}

function remove() {
  confirm.require({
    header: 'Zmazať položku',
    message: `${form.value.name} sa odstráni z cenníka. Faktúry, na ktorých je, sa nezmenia. Ak ju chcete iba skryť z ponuky, archivujte ju.`,
    acceptProps: { label: 'Zmazať', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await api.deleteItem(session.companyId, form.value.id)
        toast.add({ severity: 'info', summary: 'Položka zmazaná', life: 2500 })
        router.push('/cennik')
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Zmazanie zlyhalo', detail: e.message, life: 6000 })
      }
    },
  })
}

function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && /INPUT|TEXTAREA|SELECT/.test(t.tagName)
  if ((e.ctrlKey || e.metaKey) && e.key === 's' && form.value && canWrite.value) {
    e.preventDefault()
    if (!busy.value) save()
  } else if (e.key === 'Escape' && form.value) {
    router.push('/cennik')
  } else if (!typing && !e.ctrlKey && !e.metaKey && e.key === 'n' && canWrite.value) {
    e.preventDefault()
    router.push('/cennik/nova')
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  narrow.removeEventListener('change', applyResponsive)
})
</script>

<template>
  <div class="catalog" :class="{ 'has-panel': !!form }">
    <section class="list">
      <header class="head">
        <h1>Cenník</h1>
        <span class="count">{{ items.length }}</span>
        <div class="spacer" />
        <input ref="fileInput" type="file" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" class="visually-hidden" @change="pickImport" />
        <Button v-if="canWrite" icon="pi pi-upload" label="Import" severity="secondary" outlined size="small" :loading="importing && !importPreview"
          v-tooltip.bottom="'Načítať cenník z CSV alebo Excelu'" @click="fileInput.click()" />
        <ExportButton name="Cenník" :columns="itemColumns" :rows="() => gridRows(gridApi, rows)" />
        <Button v-if="canWrite" size="small" @click="router.push('/cennik/nova')">
          <i class="pi pi-plus" /><span>Nová položka</span><kbd class="hint">N</kbd>
        </Button>
      </header>
      <div class="toolbar">
        <IconField class="search">
          <InputIcon class="pi pi-search" />
          <InputText v-model="search" placeholder="Hľadať kód, názov, EAN" size="small" aria-label="Hľadať v cenníku" />
        </IconField>
        <label class="archived-toggle">
          <Checkbox v-model="showArchived" binary input-id="arch" />
          <span>aj archivované</span>
        </label>
      </div>
      <div class="grid-wrap">
        <AgGridVue
          class="grid"
          :theme="fiskusGridTheme"
          :row-data="rows"
          :column-defs="columnDefs"
          :default-col-def="{ sortable: true, resizable: true }"
          :row-class-rules="rowClassRules"
          :locale-text="skGridLocale"
          :loading="loading"
          :get-row-id="rowId"
          :overlay-no-rows-template="'<span>Cenník je prázdny. Novú položku pridáte klávesom N.</span>'"
          @grid-ready="(e) => { gridApi = e.api; applyResponsive() }"
          @grid-size-changed="(e) => e.api.sizeColumnsToFit()"
          @row-clicked="(e) => router.push(`/cennik/${e.data.id}`)"
          @row-double-clicked="(e) => router.push(`/cennik/${e.data.id}/karta`)"
        />
      </div>
    </section>

    <aside v-if="form" class="panel" aria-label="Detail položky">
      <header class="panel-head">
        <button class="close" aria-label="Zavrieť" @click="router.push('/cennik')"><i class="pi pi-times" /></button>
        <h2>{{ form.id ? form.name || 'Položka' : 'Nová položka' }}</h2>
        <div class="spacer" />
        <Button v-if="form.id" label="Karta produktu" icon="pi pi-id-card" size="small" severity="secondary" outlined
          v-tooltip.bottom="'Obrázky, parametre, sklady a pohyby'" @click="router.push(`/cennik/${form.id}/karta`)" />
      </header>
      <div class="panel-body">
        <div class="form-grid">
          <FormField label="Názov (text na faktúre)" :span="4" :error="errors.name">
            <InputText v-model="form.name" size="small" :invalid="!!errors.name" :disabled="!canWrite" />
          </FormField>
          <FormField label="Kód" :error="errors.code" hint="nepovinný">
            <InputText v-model="form.code" size="small" :invalid="!!errors.code" :disabled="!canWrite" />
          </FormField>
          <FormField label="Merná jednotka" :error="errors.unit">
            <Select v-model="form.unit" :options="units" editable size="small" :invalid="!!errors.unit" :disabled="!canWrite" />
          </FormField>
          <FormField label="Cena za MJ" :span="2" :error="errors.price">
            <InputNumber v-model="form.price" :min-fraction-digits="2" :max-fraction-digits="4" locale="sk-SK" suffix=" €" size="small" :disabled="!canWrite" />
          </FormField>
          <FormField v-if="!nonVat" label="Cena je" :span="2" :error="errors.priceMode">
            <SelectButton v-model="form.priceMode" :options="priceModes" option-label="label" option-value="value" :allow-empty="false" size="small" :disabled="!canWrite" />
          </FormField>
          <FormField v-if="!nonVat" label="Sadzba DPH" :span="2" :error="errors.vatRate">
            <Select v-model="form.vatRate" :options="rates" size="small" :disabled="!canWrite">
              <template #value="{ value }">{{ value }} %</template>
              <template #option="{ option }">{{ option }} %</template>
            </Select>
          </FormField>
          <div v-if="form.id && !nonVat" class="prices">
            <span>Bez DPH <strong>{{ formatAmount(form.priceNet) }} €</strong></span>
            <span>S DPH <strong>{{ formatAmount(form.priceGross) }} €</strong></span>
          </div>
          <FormField label="Interná poznámka" :span="4" hint="na faktúru sa nedostane">
            <Textarea v-model="form.note" rows="2" auto-resize :disabled="!canWrite" />
          </FormField>
          <label v-if="form.id" class="check">
            <Checkbox v-model="form.archived" binary input-id="archived" :disabled="!canWrite" />
            <span>Archivovaná — neponúka sa vo faktúrach</span>
          </label>
        </div>
      </div>
      <footer class="panel-foot">
        <Button v-if="form.id && canWrite" label="Zmazať" severity="danger" text size="small" @click="remove" />
        <div class="spacer" />
        <Button v-if="canWrite" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" @click="save" />
      </footer>
    </aside>
    <Dialog :visible="!!importPreview" modal header="Import cenníka" :style="{ width: '52rem' }" :breakpoints="{ '760px': '96vw' }"
      @update:visible="(v) => !v && (importPreview = null)">
      <template v-if="importPreview">
        <p class="imp-sum">
          <span class="tag" data-tone="paid"><span class="tag-text">{{ importPreview.created }} nových</span></span>
          <span class="tag" data-tone="issued"><span class="tag-text">{{ importPreview.updated }} zmenených</span></span>
          <span v-if="importPreview.skipped" class="tag" data-tone="overdue"><span class="tag-text">{{ importPreview.skipped }} s chybou sa preskočí</span></span>
        </p>
        <p class="imp-help">Položka s rovnakým kódom (bez kódu s rovnakým názvom) sa prepíše, ostatné pribudnú. Nič sa nemaže.</p>
        <div class="imp-table">
          <table>
            <thead>
              <tr><th>Riadok</th><th></th><th>Kód</th><th>Názov</th><th>MJ</th><th class="num">Cena</th><th class="num">DPH</th></tr>
            </thead>
            <tbody>
              <template v-for="r in importPreview.rows.slice(0, 300)" :key="r.line">
                <tr :class="{ bad: r.action === 'skip' }">
                  <td>{{ r.line }}</td>
                  <td><span class="tag" :data-tone="importActions[r.action].tone"><span class="tag-text">{{ importActions[r.action].label }}</span></span></td>
                  <td>{{ r.item.code }}</td>
                  <td>{{ r.item.name }}</td>
                  <td>{{ r.item.unit }}</td>
                  <td class="num">{{ formatAmount(r.item.price) }}<small v-if="r.item.priceMode === 'gross'"> s DPH</small></td>
                  <td class="num">{{ r.item.vatRate }} %</td>
                </tr>
                <tr v-if="r.errors?.length" class="err-row"><td></td><td colspan="6">{{ importErrors(r) }}</td></tr>
              </template>
            </tbody>
          </table>
          <p v-if="importPreview.rows.length > 300" class="imp-help">… a ďalších {{ importPreview.rows.length - 300 }} riadkov</p>
        </div>
      </template>
      <template #footer>
        <Button label="Zrušiť" severity="secondary" text size="small" @click="importPreview = null" />
        <Button :label="importLabel((importPreview?.created ?? 0) + (importPreview?.updated ?? 0))" icon="pi pi-check" size="small"
          :loading="importing" :disabled="!importPreview || importPreview.created + importPreview.updated === 0" @click="runImport" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
}
.imp-sum {
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
  margin: 0 0 6px;
}
.imp-help {
  margin: 0 0 10px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.imp-table {
  max-height: 50vh;
  overflow: auto;
  border: 1px solid var(--rule);
}
.imp-table table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
}
.imp-table th {
  position: sticky;
  top: 0;
  background: var(--sheet);
  text-align: left;
  font-weight: 600;
  color: var(--ink-soft);
  padding: 4px 8px;
  border-bottom: 1px solid var(--rule);
}
.imp-table td {
  padding: 3px 8px;
  border-bottom: 1px solid var(--rule-soft);
}
.imp-table .num {
  text-align: right;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.imp-table tr.bad td {
  color: var(--ink-faint);
}
.imp-table .err-row td {
  color: var(--tone-overdue);
  border-bottom: 1px solid var(--rule);
}
.catalog {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  height: 100%;
}
.catalog.has-panel {
  grid-template-columns: minmax(0, 1fr) 460px;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 16px 12px;
  min-width: 0;
  min-height: 0;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.head h1 {
  font-size: var(--fs-xl);
}
.count {
  color: var(--ink-faint);
  font-variant-numeric: tabular-nums;
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
.search {
  max-width: 360px;
}
.search :deep(input) {
  width: 100%;
}
.grid-wrap {
  flex: 1;
  min-height: 300px;
  border: 1px solid var(--rule);
  background: var(--sheet);
}
.grid {
  height: 100%;
}
.grid :deep(.ag-row) {
  cursor: pointer;
}
.grid :deep(.cell-name) {
  font-weight: 600;
}
.grid :deep(.row-archived) {
  color: var(--ink-faint);
}
.grid :deep(.cell-img) {
  padding: 2px 6px;
}
.grid :deep(.thumb) {
  display: block;
  width: 30px;
  height: 30px;
  object-fit: contain;
  background: var(--paper);
  border: 1px solid var(--rule-soft);
}
.grid :deep(.cell-neg) {
  color: var(--tone-overdue);
}
.grid :deep(.cell-code) {
  color: var(--ink-soft);
  font-variant-numeric: tabular-nums;
}
.archived-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.prices {
  grid-column: span 4;
  display: flex;
  gap: 18px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  background: var(--paper);
  padding: 6px 8px;
  border: 1px solid var(--rule-soft);
}
.prices strong {
  color: var(--ink);
  font-variant-numeric: tabular-nums;
}
.check {
  grid-column: span 4;
  display: flex;
  align-items: center;
  gap: 8px;
}
.grid :deep(.row-selected) {
  background: var(--blue-wash);
  box-shadow: inset 3px 0 0 var(--blue);
}

.panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--sheet);
  border-left: 1px solid var(--rule);
}
.panel-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--rule);
  border-top: 3px solid var(--blue-deep);
}
.panel-head h2 {
  font-size: var(--fs-lg);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.close {
  border: 0;
  background: none;
  padding: 4px;
  color: var(--ink-soft);
  cursor: pointer;
}
.panel-body {
  flex: 1;
  overflow-y: auto;
  padding: 14px;
}
.panel-body .form-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}
.panel-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-top: 1px solid var(--rule);
  background: var(--paper);
}

@media (max-width: 1100px) {
  .catalog.has-panel {
    grid-template-columns: minmax(0, 1fr);
  }
  .panel {
    position: fixed;
    inset: var(--topbar-h) 0 0 auto;
    width: min(460px, 100%);
    z-index: 15;
    box-shadow: -8px 0 24px rgba(18, 24, 32, 0.12);
  }
}
@media (max-width: 760px) {
  .list {
    padding: 10px 10px 8px;
  }
  .head h1 {
    font-size: var(--fs-lg);
  }
  .hint {
    display: none;
  }
  .search {
    max-width: none;
  }
  .panel-body .form-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
