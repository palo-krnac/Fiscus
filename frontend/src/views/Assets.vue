<script setup>
// Register dlhodobého majetku s daňovými odpismi. Odpisový plán, zostatkové
// ceny a súčty počíta backend (internal/asset); tu sa iba zobrazujú.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import DatePicker from 'primevue/datepicker'
import Textarea from 'primevue/textarea'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { formatAmount, formatDate, fromISODate, toISODate } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { assetColumns } from '../lib/columns.js'
import {
  assetStatus, blankAsset, canAccelerate, disposalOptions, filterAssets, fixMethod, groupOptions, kindOptions,
  methodOptions, periodLabel, postingState,
} from '../lib/assets.js'

const props = defineProps({ id: String })
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))

const thisYear = new Date().getFullYear()
const year = ref(thisYear)
const reg = shallowRef(null)
const loading = ref(true)
const search = ref('')
const show = ref('active')
const showOptions = [
  { value: 'active', label: 'v roku' },
  { value: 'all', label: 'všetky' },
]
const form = ref(null)
const errors = ref({})
const busy = ref(false)
const posting = ref(false)
const purchases = shallowRef(null)

const items = computed(() => reg.value?.items ?? [])
const years = computed(() => {
  let first = thisYear - 2
  for (const it of items.value) first = Math.min(first, Number(it.acquired.slice(0, 4)))
  const out = []
  for (let y = thisYear + 1; y >= first; y--) out.push({ value: y, label: String(y) })
  return out
})
const rows = computed(() => filterAssets(items.value, search.value, { show: show.value, year: year.value }))
const post = computed(() => postingState(reg.value))
const current = computed(() => items.value.find((x) => x.id === form.value?.id) ?? null)

async function load() {
  loading.value = true
  try {
    reg.value = await api.assets(session.companyId, year.value)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Register majetku sa nenačítal', detail: e.message, life: 6000 })
  } finally {
    loading.value = false
  }
}
watch(year, async () => {
  await load()
  gridApi.value?.redrawRows()
})

// formulár: dátumy ako Date pre DatePicker, sumy ako čísla pre InputNumber
function toForm(a) {
  return { ...a, acquired: fromISODate(a.acquired), disposed: a.disposed ? fromISODate(a.disposed) : null,
    cost: a.cost == null || a.cost === '' ? null : Number(a.cost), purchaseId: a.purchaseId || null }
}
function fromForm(f) {
  return { ...f, acquired: f.acquired ? toISODate(f.acquired) : '', disposed: f.disposed ? toISODate(f.disposed) : '',
    disposal: f.disposed ? f.disposal : '', cost: f.cost == null ? '' : String(f.cost), years: f.years ?? 0,
    purchaseId: f.purchaseId ?? 0 }
}
function openFromRoute() {
  errors.value = {}
  if (props.id === 'novy') form.value = toForm(blankAsset(toISODate(new Date())))
  else if (props.id) {
    const it = items.value.find((x) => String(x.id) === props.id)
    form.value = it ? toForm(it) : null
  } else form.value = null
  if (form.value && canWrite.value && purchases.value === null) loadPurchases()
}
async function loadPurchases() {
  purchases.value = []
  try {
    const list = await api.purchases(session.companyId)
    purchases.value = list.map((p) => ({ value: p.id, label: `${p.number} – ${p.supplier?.name ?? ''} – ${formatAmount(p.summary?.total)} €` }))
  } catch {
    purchases.value = []
  }
}
onMounted(async () => {
  await load()
  openFromRoute()
})
watch(() => props.id, openFromRoute)
watch(() => [form.value?.kind, form.value?.group], () => form.value && fixMethod(form.value))

const amount = (field, header, get, extra = {}) => ({
  colId: field, headerName: header, width: 120, type: 'rightAligned',
  valueGetter: (p) => Number(get(p.data) ?? 0), valueFormatter: (p) => formatAmount(p.value), ...extra,
})
const columnDefs = [
  { field: 'number', headerName: 'Inv. č.', width: 84, cellClass: 'cell-code' },
  { field: 'name', headerName: 'Názov', flex: 2, minWidth: 170, cellClass: 'cell-name' },
  { colId: 'period', headerName: 'Skupina', width: 130, valueGetter: (p) => periodLabel(p.data) },
  { field: 'acquired', headerName: 'Zaradený', width: 100, valueFormatter: (p) => formatDate(p.value) },
  amount('cost', 'Vstupná cena', (r) => r.cost),
  amount('claimed', 'Odpis v roku', (r) => Number(r.thisYear?.claimed ?? 0) + Number(r.thisYear?.disposal ?? 0), {
    cellClass: 'ag-right-aligned-cell cell-name',
    tooltipValueGetter: (p) => (Number(p.data.thisYear?.disposal) ? 'zostatková cena pri vyradení' : ''),
  }),
  amount('residual', 'ZC na konci', (r) => r.residual),
  {
    colId: 'status', headerName: 'Stav', width: 106,
    valueGetter: (p) => assetStatus(p.data, year.value).label,
    cellRenderer: (p) => {
      const s = assetStatus(p.data, year.value)
      return `<span class="tag" data-tone="${s.tone}"><span class="tag-text">${s.label}</span></span>`
    },
  },
]

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  // úzky displej alebo otvorený detail: iba názov, odpis a zostatková cena
  const compact = narrow.matches || !!form.value
  gridApi.value?.setColumnsVisible(['number', 'period', 'acquired', 'cost'], !compact)
  gridApi.value?.setColumnsVisible(['status'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
narrow.addEventListener('change', applyResponsive)
watch(() => !!form.value, () => setTimeout(applyResponsive))

const rowId = (p) => String(p.data.id)
const selectedId = computed(() => form.value?.id ?? null)
const rowClassRules = { 'row-selected': (p) => p.data.id === selectedId.value, 'row-gone': (p) => !!p.data.disposed }
watch(selectedId, () => gridApi.value?.redrawRows())

async function save() {
  busy.value = true
  try {
    const wasNew = !form.value.id
    const saved = await api.saveAsset(session.companyId, fromForm(form.value))
    await load()
    errors.value = {}
    form.value = toForm(items.value.find((x) => x.id === saved.id) ?? saved)
    toast.add({ severity: 'success', summary: wasNew ? `Majetok ${saved.number} zaradený` : 'Majetok uložený', life: 2500 })
    if (wasNew) router.replace(`/majetok/${saved.id}`)
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) {
      errors.value = e.fieldMap()
      toast.add({ severity: 'error', summary: 'Skontrolujte vyznačené polia', detail: e.fields[0].message, life: 5000 })
    } else {
      toast.add({ severity: 'error', summary: 'Uloženie zlyhalo', detail: e.message, life: 6000 })
    }
  } finally {
    busy.value = false
  }
}

function remove() {
  confirm.require({
    header: 'Zmazať majetok',
    message: `${form.value.number} ${form.value.name} sa odstráni z registra. Predaný alebo zlikvidovaný majetok nezmažte — zadajte dátum vyradenia, aby sa jeho zostatková cena dostala do výdavkov.`,
    acceptProps: { label: 'Zmazať', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await api.deleteAsset(session.companyId, form.value.id)
        toast.add({ severity: 'info', summary: 'Majetok zmazaný', life: 2500 })
        router.push('/majetok')
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Zmazanie zlyhalo', detail: e.message, life: 6000 })
      }
    },
  })
}

async function postToClosing() {
  posting.value = true
  try {
    const res = await api.postDepreciation(session.companyId, year.value)
    await load()
    toast.add({ severity: 'success', summary: `Odpisy ${year.value} zaúčtované`,
      detail: `${formatAmount(res.posted)} € v uzávierke peňažného denníka (stĺpec e6)`, life: 4000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Zaúčtovanie zlyhalo', detail: e.message, life: 6000 })
  } finally {
    posting.value = false
  }
}

function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && /INPUT|TEXTAREA|SELECT/.test(t.tagName)
  if ((e.ctrlKey || e.metaKey) && e.key === 's' && form.value && canWrite.value) {
    e.preventDefault()
    if (!busy.value) save()
  } else if (e.key === 'Escape' && form.value) {
    router.push('/majetok')
  } else if (!typing && !e.ctrlKey && !e.metaKey && e.key === 'n' && canWrite.value) {
    e.preventDefault()
    router.push('/majetok/novy')
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  narrow.removeEventListener('change', applyResponsive)
})
</script>

<template>
  <div class="assets" :class="{ 'has-panel': !!form }">
    <section class="list">
      <header class="head">
        <h1>Majetok</h1>
        <span class="count">{{ items.length }}</span>
        <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
        <div class="spacer" />
        <ExportButton :name="`Majetok ${year}`" :columns="assetColumns" :rows="() => gridRows(gridApi, rows)" />
        <Button v-if="canWrite" size="small" @click="router.push('/majetok/novy')">
          <i class="pi pi-plus" /><span>Zaradiť majetok</span><kbd class="hint">N</kbd>
        </Button>
      </header>

      <div v-if="reg" class="ledger" aria-label="Daňové odpisy za rok">
        <dl>
          <div><dt>Odpisy {{ year }}</dt><dd>{{ formatAmount(reg.totals.claimed) }} €</dd></div>
          <div v-if="Number(reg.totals.disposal)"><dt>ZC vyradeného</dt><dd>{{ formatAmount(reg.totals.disposal) }} €</dd></div>
          <div class="total"><dt>Daňový výdavok</dt><dd><span class="dbl">{{ formatAmount(reg.totals.expense) }} €</span></dd></div>
          <div class="zc"><dt>ZC hmotného k 31. 12.</dt><dd>{{ formatAmount(reg.totals.tangible) }} €</dd></div>
          <div v-if="Number(reg.totals.intangible)" class="zc"><dt>ZC nehmotného</dt><dd>{{ formatAmount(reg.totals.intangible) }} €</dd></div>
        </dl>
        <div class="post">
          <span class="tag" :data-tone="post.tone"><span class="tag-text">{{ post.label }}</span></span>
          <Button v-if="canWrite && post.action" :label="post.action" icon="pi pi-book" size="small"
            :outlined="post.tone === 'paid'" :severity="post.tone === 'paid' ? 'secondary' : undefined" :loading="posting" @click="postToClosing" />
        </div>
      </div>

      <div class="toolbar">
        <IconField class="search">
          <InputIcon class="pi pi-search" />
          <InputText v-model="search" placeholder="Hľadať číslo, názov, faktúru, poznámku" size="small" aria-label="Hľadať v majetku" />
        </IconField>
        <SelectButton v-model="show" :options="showOptions" option-label="label" option-value="value" :allow-empty="false" size="small"
          aria-label="Zobraziť" />
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
          :tooltip-show-delay="300"
          :overlay-no-rows-template="'<span>V registri nie je žiadny majetok pre tento rok. Nový zaradíte klávesom N.</span>'"
          @grid-ready="(e) => { gridApi = e.api; applyResponsive() }"
          @grid-size-changed="(e) => e.api.sizeColumnsToFit()"
          @row-clicked="(e) => router.push(`/majetok/${e.data.id}`)"
        />
      </div>
      <p class="foot-note">
        Daňové odpisy podľa § 26 až 28 zákona o dani z príjmov. Hmotný majetok nad 1 700 €, nehmotný nad 2 400 €; lacnejší je výdavok
        pri nákupe. Zostatkové ceny sa prenášajú do výkazu o majetku a záväzkoch (r. 01, 02) a do tabuľky 1a daňového priznania.
      </p>
    </section>

    <aside v-if="form" class="panel" aria-label="Detail majetku">
      <header class="panel-head">
        <button class="close" aria-label="Zavrieť" @click="router.push('/majetok')"><i class="pi pi-times" /></button>
        <h2>{{ form.id ? `${form.number} ${form.name}` : 'Zaradiť majetok' }}</h2>
      </header>
      <div class="panel-body">
        <div class="form-grid">
          <FormField label="Názov" :span="3" :error="errors.name">
            <InputText v-model="form.name" size="small" :invalid="!!errors.name" :disabled="!canWrite" />
          </FormField>
          <FormField label="Inventárne č." :error="errors.number" :hint="form.id ? '' : 'prázdne = ďalšie'">
            <InputText v-model="form.number" size="small" :invalid="!!errors.number" :disabled="!canWrite" />
          </FormField>
          <FormField label="Druh" :span="2" :error="errors.kind">
            <SelectButton v-model="form.kind" :options="kindOptions" option-label="label" option-value="value" :allow-empty="false"
              size="small" :disabled="!canWrite" />
          </FormField>
          <FormField v-if="form.kind === 'tangible'" label="Odpisová skupina" :span="2" :error="errors.group">
            <Select v-model="form.group" :options="groupOptions" option-label="label" option-value="value" size="small"
              :invalid="!!errors.group" :disabled="!canWrite" />
          </FormField>
          <FormField v-else label="Doba odpisovania" :span="2" :error="errors.years" hint="podľa odpisového plánu">
            <InputNumber v-model="form.years" :min="1" :max="40" suffix=" r." size="small" :invalid="!!errors.years" :disabled="!canWrite" />
          </FormField>
          <FormField v-if="canAccelerate(form)" label="Spôsob odpisovania" :span="4" :error="errors.method">
            <SelectButton v-model="form.method" :options="methodOptions" option-label="label" option-value="value" :allow-empty="false"
              size="small" :disabled="!canWrite" />
          </FormField>
          <FormField label="Zaradený do užívania" :span="2" :error="errors.acquired">
            <DatePicker v-model="form.acquired" date-format="d. m. yy" size="small" fluid :invalid="!!errors.acquired" :disabled="!canWrite" />
          </FormField>
          <FormField label="Vstupná cena" :span="2" :error="errors.cost">
            <InputNumber v-model="form.cost" :min-fraction-digits="2" :max-fraction-digits="2" locale="sk-SK" suffix=" €" size="small"
              :invalid="!!errors.cost" :disabled="!canWrite" />
          </FormField>
          <FormField label="Používanie na podnikanie" :span="2" :error="errors.share" hint="pri aute s jazdou aj súkromne">
            <InputNumber v-model="form.share" :min="1" :max="100" suffix=" %" size="small" :invalid="!!errors.share" :disabled="!canWrite" />
          </FormField>
          <FormField label="Prijatá faktúra" :span="2" :error="errors.purchaseId">
            <Select v-model="form.purchaseId" :options="purchases ?? []" option-label="label" option-value="value" filter show-clear
              size="small" placeholder="nepovinné" :disabled="!canWrite" empty-message="Žiadne prijaté faktúry"
              empty-filter-message="Nič sa nenašlo">
              <template #value="{ value, placeholder }">
                <span v-if="value">{{ form.purchaseNumber || purchases?.find((p) => p.value === value)?.label || value }}</span>
                <span v-else class="placeholder">{{ placeholder }}</span>
              </template>
            </Select>
          </FormField>
          <FormField label="Vyradený" :span="2" :error="errors.disposed" hint="predaj, likvidácia">
            <DatePicker v-model="form.disposed" date-format="d. m. yy" size="small" fluid show-clear :invalid="!!errors.disposed"
              :disabled="!canWrite" />
          </FormField>
          <FormField v-if="form.disposed" label="Dôvod vyradenia" :span="2" :error="errors.disposal">
            <Select v-model="form.disposal" :options="disposalOptions" option-label="label" option-value="value" size="small"
              :invalid="!!errors.disposal" :disabled="!canWrite" />
          </FormField>
          <FormField label="Poznámka" :span="4">
            <Textarea v-model="form.note" rows="2" auto-resize :disabled="!canWrite" />
          </FormField>
        </div>

        <section v-if="current?.plan?.length" class="plan" aria-label="Odpisový plán">
          <h3>Odpisový plán</h3>
          <table>
            <thead>
              <tr><th>Rok</th><th class="num">Ročný odpis</th><th class="num">Uplatnený</th><th class="num">ZC na konci</th></tr>
            </thead>
            <tbody>
              <tr v-for="y in current.plan" :key="y.year" :class="{ now: y.year === year }">
                <td>{{ y.year }}<small v-if="y.note"> {{ y.note }}</small></td>
                <td class="num">{{ Number(y.disposal) ? '' : formatAmount(y.annual) }}</td>
                <td class="num">{{ formatAmount(Number(y.disposal) ? y.disposal : y.claimed) }}</td>
                <td class="num">{{ Number(y.disposal) ? '—' : formatAmount(y.residual) }}</td>
              </tr>
            </tbody>
          </table>
          <p v-if="current.share < 100" class="plan-note">Uplatnený odpis je {{ current.share }} % ročného — zvyšok pripadá na súkromné používanie.</p>
        </section>
      </div>
      <footer class="panel-foot">
        <Button v-if="form.id && canWrite" label="Zmazať" severity="danger" text size="small" @click="remove" />
        <div class="spacer" />
        <Button v-if="canWrite" :label="form.id ? 'Uložiť' : 'Zaradiť'" icon="pi pi-check" size="small" :loading="busy" @click="save" />
      </footer>
    </aside>
  </div>
</template>

<style scoped>
.assets {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  height: 100%;
}
.assets.has-panel {
  grid-template-columns: minmax(0, 1fr) 480px;
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

/* súčty roka ako riadok účtovnej knihy */
.ledger {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding: 8px 12px;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-left: 3px solid var(--blue-deep);
}
.ledger dl {
  display: flex;
  gap: 22px;
  flex-wrap: wrap;
  margin: 0;
}
.ledger dt {
  font-size: var(--fs-xs);
  color: var(--ink-soft);
}
.ledger dd {
  margin: 0;
  font-size: var(--fs-md);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.ledger .total dd {
  font-weight: 700;
  font-size: var(--fs-lg);
  color: var(--blue-deep);
}
.ledger .dbl {
  border-bottom: 3px double currentColor;
}
.ledger .zc dd {
  color: var(--ink-soft);
}
.post {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-left: auto;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.search {
  max-width: 360px;
  flex: 1;
}
.search :deep(input) {
  width: 100%;
}
.grid-wrap {
  flex: 1;
  min-height: 280px;
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
.grid :deep(.cell-code) {
  color: var(--ink-soft);
  font-variant-numeric: tabular-nums;
}
.grid :deep(.row-gone) {
  color: var(--ink-faint);
}
.grid :deep(.row-selected) {
  background: var(--blue-wash);
  box-shadow: inset 3px 0 0 var(--blue);
}
.foot-note {
  margin: 0;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
  max-width: 80ch;
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
.placeholder {
  color: var(--ink-faint);
}
.plan {
  margin-top: 16px;
}
.plan h3 {
  font-size: var(--fs-md);
  margin: 0 0 6px;
}
.plan table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
}
.plan th {
  text-align: left;
  font-weight: 600;
  color: var(--ink-soft);
  padding: 4px 6px;
  border-bottom: 1px solid var(--rule);
}
.plan td {
  padding: 3px 6px;
  border-bottom: 1px solid var(--rule-soft);
}
.plan .num {
  text-align: right;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.plan small {
  margin-left: 6px;
  color: var(--ink-faint);
}
.plan tr.now td {
  background: var(--blue-wash);
  font-weight: 600;
}
.plan-note {
  margin: 6px 0 0;
  font-size: var(--fs-xs);
  color: var(--ink-soft);
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
  .assets.has-panel {
    grid-template-columns: minmax(0, 1fr);
  }
  .panel {
    position: fixed;
    inset: var(--topbar-h) 0 0 auto;
    width: min(480px, 100%);
    z-index: 15;
    box-shadow: -8px 0 24px rgba(18, 24, 32, 0.12);
  }
}
@media (max-width: 760px) {
  .list {
    padding: 10px 10px 8px;
  }
  .head {
    flex-wrap: wrap;
  }
  .head h1 {
    font-size: var(--fs-lg);
  }
  .hint {
    display: none;
  }
  .ledger dl {
    gap: 14px;
  }
  .post {
    margin-left: 0;
    width: 100%;
    justify-content: space-between;
  }
  .search {
    max-width: none;
  }
  .panel-body .form-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
