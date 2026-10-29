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
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { codes, ratesOn } from '../lib/codes.js'
import { formatAmount } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { itemColumns } from '../lib/columns.js'
import { filterItems } from '../lib/catalog.js'

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

async function load() {
  loading.value = true
  try {
    items.value = await api.items(session.companyId)
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
const columnDefs = [
  { field: 'code', headerName: 'Kód', width: 90, cellClass: 'cell-code' },
  { field: 'name', headerName: 'Názov', flex: 2, minWidth: 180, cellClass: 'cell-name' },
  { field: 'unit', headerName: 'MJ', width: 70 },
  money('priceNet', 'Bez DPH'),
  { field: 'vatRate', headerName: 'DPH', width: 74, type: 'rightAligned', valueFormatter: (p) => `${p.value} %` },
  money('priceGross', 'S DPH', { cellClass: 'ag-right-aligned-cell cell-name' }),
]

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['code', 'vatRate', 'priceNet'], !narrow.matches)
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
        <ExportButton name="Cenník" :columns="itemColumns" :rows="() => gridRows(gridApi, rows)" />
        <Button v-if="canWrite" size="small" @click="router.push('/cennik/nova')">
          <i class="pi pi-plus" /><span>Nová položka</span><kbd class="hint">N</kbd>
        </Button>
      </header>
      <div class="toolbar">
        <IconField class="search">
          <InputIcon class="pi pi-search" />
          <InputText v-model="search" placeholder="Hľadať kód, názov, poznámku" size="small" aria-label="Hľadať v cenníku" />
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
        />
      </div>
    </section>

    <aside v-if="form" class="panel" aria-label="Detail položky">
      <header class="panel-head">
        <button class="close" aria-label="Zavrieť" @click="router.push('/cennik')"><i class="pi pi-times" /></button>
        <h2>{{ form.id ? form.name || 'Položka' : 'Nová položka' }}</h2>
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
  </div>
</template>

<style scoped>
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
