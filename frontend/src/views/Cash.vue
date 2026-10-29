<script setup>
// Pokladňa: príjmové a výdavkové pokladničné doklady, zostatok, úhrada faktúr v hotovosti.
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import DatePicker from 'primevue/datepicker'
import Drawer from 'primevue/drawer'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { api } from '../lib/api.js'
import { formatAmount, formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { session, can } from '../lib/session.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { matches, searchTerms } from '../lib/search.js'
import { cashColumns } from '../lib/columns.js'
import FormField from '../components/FormField.vue'
import Checkbox from 'primevue/checkbox'
import { codes, ratesOn } from '../lib/codes.js'
import { emptyTax, taxErrors, taxLabel, taxPayload } from '../lib/cashvat.js'
import ExportButton from '../components/ExportButton.vue'

const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('cash.write'))

const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
const year = ref(thisYear)
const view = ref('all')
const views = [
  { label: 'Všetky', value: 'all' },
  { label: 'Príjmy', value: 'in' },
  { label: 'Výdavky', value: 'out' },
]
const search = ref('')
const book = ref({ documents: [], opening: '0', balance: '0', in: '0', out: '0' })
const loading = ref(true)
const loadError = ref('')

async function load() {
  loading.value = true
  try {
    book.value = await api.cashBook(session.companyId, year.value)
    loadError.value = ''
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
  return book.value.documents.filter(
    (d) => (view.value === 'all' || d.kind === view.value) && matches(terms, [d.number, d.partner, d.purpose, d.linkNumber, d.amount, d.note]),
  )
})

const columnDefs = [
  { field: 'number', headerName: 'Číslo', width: 130, cellClass: 'cell-number' },
  { field: 'date', headerName: 'Dátum', valueFormatter: (p) => formatDate(p.value), width: 100 },
  {
    colId: 'in',
    headerName: 'Príjem',
    valueGetter: (p) => (p.data.kind === 'in' ? Number(p.data.amount) : null),
    valueFormatter: (p) => (p.value == null ? '' : formatAmount(p.value)),
    cellClass: 'ag-right-aligned-cell cell-in',
    type: 'rightAligned',
    width: 110,
  },
  {
    colId: 'out',
    headerName: 'Výdavok',
    valueGetter: (p) => (p.data.kind === 'out' ? Number(p.data.amount) : null),
    valueFormatter: (p) => (p.value == null ? '' : formatAmount(p.value)),
    type: 'rightAligned',
    width: 110,
  },
  {
    colId: 'signed',
    headerName: 'Suma',
    hide: true,
    valueGetter: (p) => (p.data.kind === 'out' ? -1 : 1) * Number(p.data.amount),
    valueFormatter: (p) => (p.value > 0 ? '+' : '') + formatAmount(p.value),
    cellClass: (p) => ['ag-right-aligned-cell', p.value > 0 ? 'cell-in' : ''],
    type: 'rightAligned',
    width: 100,
  },
  { field: 'partner', headerName: 'Partner', width: 170 },
  { field: 'purpose', headerName: 'Účel', flex: 1, minWidth: 160 },
  {
    colId: 'link',
    headerName: 'Doklad',
    width: 140,
    valueGetter: (p) => (p.data.cancelled ? 'stornovaný' : p.data.linkNumber || taxLabel(p.data, formatAmount)),
    cellClass: (p) => (p.data.cancelled ? 'cell-cancelled' : 'cell-link'),
  },
]
const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['partner', 'link', 'number', 'in', 'out'], !narrow.matches)
  gridApi.value?.setColumnsVisible(['signed'], narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
narrow.addEventListener('change', applyResponsive)

// ---- nový doklad ----
const form = ref(null)
const errors = ref({})
const busy = ref(false)
const candidates = ref([])
const formOpen = computed({
  get: () => !!form.value,
  set: (v) => {
    if (!v) form.value = null
  },
})

async function openNew(kind) {
  form.value = { kind, date: fromISODate(toISODate(new Date())), amount: '', partner: '', purpose: '', note: '', link: null, tax: null }
  preview.value = null
  errors.value = {}
  candidates.value = []
  try {
    candidates.value = await api.cashCandidates(session.companyId, kind)
  } catch {
    /* bez kandidátov */
  }
}
const kindLabel = (k) => (k === 'purchase' ? 'prijatá' : 'vydaná')
const candidateOptions = computed(() =>
  candidates.value.map((c) => ({ ...c, label: `${c.number} · ${c.partner} · ${formatAmount(c.amount)} €` })),
)
function pickLink(c) {
  form.value.link = c
  if (!c) return
  form.value.amount = String(c.amount).replace('.', ',')
  form.value.partner = c.partner
  form.value.purpose = `Úhrada ${c.kind === 'purchase' ? 'prijatej faktúry' : 'faktúry'} ${c.number}`
}

async function save() {
  busy.value = true
  errors.value = {}
  const f = form.value
  try {
    const d = await api.createCash(session.companyId, {
      kind: f.kind,
      date: toISODate(f.date),
      amount: String(f.amount).replace(/\s/g, '').replace(',', '.'),
      partner: f.partner,
      purpose: f.purpose,
      note: f.note,
      linkKind: f.link?.kind ?? '',
      linkId: f.link?.id ?? 0,
      tax: f.link ? undefined : taxPayload(f.tax),
    })
    form.value = null
    toast.add({ severity: 'success', summary: `Doklad ${d.number} vystavený`, detail: d.linkNumber ? `${d.linkNumber} je uhradená` : '', life: 4000 })
    await load()
  } catch (e) {
    errors.value = e.fieldMap?.() ?? {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Doklad nemožno vystaviť', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

// ---- DPH (iba platiteľ, doklad bez väzby na faktúru) ----
const vatPayer = computed(() => !!session.company?.vatPayer)
const preview = ref(null) // doklad po prepočte na serveri (základ, daň)
const rateOptions = computed(() => ratesOn(toISODate(form.value?.date) ?? undefined).map((r) => ({ label: `${r} %`, value: r })))
function toggleTax(on) {
  form.value.tax = on ? emptyTax(form.value.amount) : null
  preview.value = null
  if (on) refreshPreview()
}
let previewTimer = null
function refreshPreview() {
  clearTimeout(previewTimer)
  previewTimer = setTimeout(async () => {
    const f = form.value
    if (!f?.tax) return
    try {
      preview.value = await api.previewCash(session.companyId, {
        kind: f.kind, date: toISODate(f.date), amount: String(f.amount).replace(/\s/g, '').replace(',', '.') || '0',
        purpose: f.purpose || '-', tax: taxPayload(f.tax),
      })
    } catch {
      preview.value = null
    }
  }, 250)
}
watch(() => form.value && [form.value.tax, form.value.amount, form.value.date], refreshPreview, { deep: true })
const pvErrors = computed(() => taxErrors(preview.value?.errors))
const rowPreview = (i) => preview.value?.tax?.rows?.find((r) => Number(r.vatRate) === Number(form.value.tax.rows[i]?.vatRate))

// ---- detail ----
const detail = ref(null)
const detailOpen = computed({
  get: () => !!detail.value,
  set: (v) => {
    if (!v) detail.value = null
  },
})
function openDetail(d) {
  detail.value = { ...d }
}
async function saveText() {
  busy.value = true
  try {
    await api.updateCash(session.companyId, detail.value.id, { partner: detail.value.partner, purpose: detail.value.purpose, note: detail.value.note })
    toast.add({ severity: 'success', summary: 'Uložené', life: 2000 })
    detail.value = null
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa uložiť', detail: e.fields?.[0]?.message ?? e.message, life: 5000 })
  } finally {
    busy.value = false
  }
}
function cancelDoc() {
  const d = detail.value
  confirm.require({
    header: 'Stornovať doklad',
    message: `Stornovať ${d.number}? Doklad zostane v knihe označený ako stornovaný${d.linkNumber ? ` a ${d.linkNumber} sa vráti medzi neuhradené` : ''}.`,
    acceptLabel: 'Stornovať',
    rejectLabel: 'Späť',
    acceptProps: { severity: 'danger', size: 'small' },
    rejectProps: { severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.cancelCash(session.companyId, d.id)
        toast.add({ severity: 'success', summary: `${d.number} stornovaný`, life: 3000 })
        detail.value = null
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Storno nie je možné', detail: e.message, life: 6000 })
      }
    },
  })
}
const docLink = (kind, id) => (kind === 'purchase' ? `/prijate/${id}` : `/faktury/${id}`)
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Pokladňa</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <template v-if="canWrite">
        <Button size="small" icon="pi pi-plus" label="Príjem" @click="openNew('in')" />
        <Button size="small" icon="pi pi-minus" label="Výdavok" severity="secondary" outlined @click="openNew('out')" />
      </template>
    </header>

    <section class="ledger" aria-label="Súhrn">
      <div class="figure">
        <span class="label">Počiatočný stav {{ year }}</span>
        <span class="value">{{ formatMoney(book.opening) }}</span>
      </div>
      <div class="figure">
        <span class="label">Príjmy</span>
        <span class="value in">{{ formatMoney(book.in) }}</span>
      </div>
      <div class="figure">
        <span class="label">Výdavky</span>
        <span class="value">{{ formatMoney(book.out) }}</span>
      </div>
      <div class="figure">
        <span class="label">Zostatok v pokladni</span>
        <span class="value"><span class="grand">{{ formatMoney(book.balance) }}</span></span>
      </div>
    </section>

    <div class="toolbar">
      <SelectButton v-model="view" :options="views" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať číslo, partnera, účel, sumu" size="small" aria-label="Hľadať" />
      </IconField>
      <ExportButton :name="`Pokladňa ${year}`" :columns="cashColumns" :rows="() => gridRows(gridApi, visibleRows)" />
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <div class="grid-wrap">
      <AgGridVue
        class="grid"
        :theme="fiskusGridTheme"
        :row-data="visibleRows"
        :column-defs="columnDefs"
        :default-col-def="{ sortable: true, resizable: true }"
        :locale-text="skGridLocale"
        :loading="loading"
        :get-row-id="(p) => String(p.data.id)"
        :row-class-rules="{ 'row-cancelled': (p) => p.data.cancelled }"
        :overlay-no-rows-template="'<span>Žiadne pokladničné doklady. Začnite vkladom hotovosti tlačidlom Príjem.</span>'"
        @grid-ready="(e) => { gridApi = e.api; applyResponsive() }"
        @row-clicked="(e) => openDetail(e.data)"
      />
    </div>

    <!-- nový doklad -->
    <Drawer v-model:visible="formOpen" position="right" class="cash-panel" :header="form?.kind === 'in' ? 'Príjmový doklad' : 'Výdavkový doklad'">
      <form v-if="form" class="form-grid cash-form" @submit.prevent="save">
        <FormField v-if="candidates.length" :label="form.kind === 'in' ? 'Uhrádza faktúru (nepovinné)' : 'Uhrádza doklad (nepovinné)'" :span="2" :error="errors.linkId">
          <Select
            :model-value="form.link"
            :options="candidateOptions"
            option-label="label"
            filter
            show-clear
            placeholder="Bez väzby na faktúru"
            size="small"
            fluid
            @update:model-value="pickLink"
          >
            <template #option="{ option }">
              <div class="opt">
                <strong>{{ option.number }}</strong> <span class="muted">{{ kindLabel(option.kind) }}</span>
                <span>{{ option.partner }}</span>
                <span class="amt">{{ formatAmount(option.amount) }} €</span>
              </div>
            </template>
          </Select>
        </FormField>
        <FormField label="Dátum" :error="errors.date">
          <DatePicker v-model="form.date" date-format="d. m. yy" size="small" fluid />
        </FormField>
        <FormField label="Suma €" :error="errors.amount">
          <InputText v-model="form.amount" inputmode="decimal" size="small" class="num" :disabled="!!form.link" />
        </FormField>
        <FormField :label="form.kind === 'in' ? 'Prijaté od' : 'Vyplatené komu'" :span="2" :error="errors.partner">
          <InputText v-model="form.partner" size="small" />
        </FormField>
        <FormField label="Účel" :span="2" :error="errors.purpose">
          <InputText v-model="form.purpose" size="small" :placeholder="form.kind === 'in' ? 'napr. Vklad do pokladne, Tržba' : 'napr. Kancelárske potreby, Poštovné'" />
        </FormField>
        <fieldset v-if="vatPayer && !form.link" class="vat span2">
          <label class="vat-toggle">
            <Checkbox :model-value="!!form.tax" binary @update:model-value="toggleTax" />
            <span>{{ form.kind === 'out' ? 'Doklad s DPH — pokladničný blok (zjednodušená faktúra)' : 'Tržba z e-kasy s DPH' }}</span>
          </label>
          <template v-if="form.tax">
            <table class="vat-rows">
              <thead><tr><th>Sadzba</th><th class="num">Suma s DPH</th><th class="num">Základ</th><th class="num">DPH</th><th /></tr></thead>
              <tbody>
                <tr v-for="(r, i) in form.tax.rows" :key="i">
                  <td><Select v-model="r.vatRate" :options="rateOptions" option-label="label" option-value="value" size="small" :invalid="!!pvErrors[`tax.rows.${i}.vatRate`] || !!errors[`tax.rows.${i}.vatRate`]" /></td>
                  <td class="num"><InputText v-model="r.gross" inputmode="decimal" size="small" class="num" :aria-label="`Suma s DPH ${r.vatRate} %`" /></td>
                  <td class="num calc">{{ rowPreview(i) ? formatAmount(rowPreview(i).base) : '' }}</td>
                  <td class="num calc">{{ rowPreview(i) ? formatAmount(rowPreview(i).vat) : '' }}</td>
                  <td><Button v-if="form.tax.rows.length > 1" icon="pi pi-times" text size="small" aria-label="Odobrať sadzbu" @click="form.tax.rows.splice(i, 1)" /></td>
                </tr>
              </tbody>
            </table>
            <Button label="Ďalšia sadzba" icon="pi pi-plus" text size="small" @click="form.tax.rows.push({ vatRate: 5, gross: '' })" />
            <p v-if="pvErrors['tax.rows'] || errors['tax.rows']" class="err">{{ errors['tax.rows'] || pvErrors['tax.rows'] }}</p>
            <div v-if="form.kind === 'out'" class="form-grid">
              <FormField label="IČ DPH predajcu" :span="2" :error="errors['tax.supplierIcdph']" hint="Z pokladničného bloku — bez neho nemožno DPH odpočítať">
                <InputText v-model="form.tax.supplierIcdph" size="small" spellcheck="false" placeholder="SK…" :disabled="form.tax.noDeduction" />
              </FormField>
              <FormField label="Druh výdavku" :span="2" hint="Stĺpec v peňažnom denníku">
                <Select v-model="form.tax.expense" :options="codes.expenseKinds" option-label="label" option-value="code" size="small" fluid />
              </FormField>
              <label class="vat-toggle full">
                <Checkbox v-model="form.tax.noDeduction" binary />
                <span>Bez odpočtu DPH (reprezentácia, súkromná spotreba)</span>
              </label>
            </div>
            <label v-else class="vat-toggle">
              <Checkbox v-model="form.tax.goods" binary />
              <span>Predaj tovaru (inak výrobky a služby)</span>
            </label>
            <p class="muted small">
              {{ form.kind === 'out' ? 'Odpočet pôjde do priznania k DPH a do kontrolného výkazu (časť B.3).' : 'Daň pôjde do priznania k DPH a do kontrolného výkazu (časť D.1).' }}
            </p>
          </template>
          <p v-if="errors.tax" class="err">{{ errors.tax }}</p>
        </fieldset>
        <FormField label="Poznámka" :span="2">
          <InputText v-model="form.note" size="small" />
        </FormField>
        <div class="actions">
          <Button label="Zrušiť" severity="secondary" text size="small" @click="form = null" />
          <Button type="submit" label="Vystaviť doklad" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Drawer>

    <!-- detail -->
    <Drawer v-model:visible="detailOpen" position="right" class="cash-panel" :header="detail?.number">
      <template v-if="detail">
        <div class="amount" :class="detail.kind">
          {{ detail.kind === 'in' ? '+' : '−' }}{{ formatAmount(detail.amount) }} €
          <span v-if="detail.cancelled" class="badge">stornovaný</span>
        </div>
        <dl class="facts">
          <dt>Druh</dt><dd>{{ detail.kind === 'in' ? 'Príjmový pokladničný doklad' : 'Výdavkový pokladničný doklad' }}</dd>
          <dt>Dátum</dt><dd>{{ formatDate(detail.date) }}</dd>
          <template v-if="detail.tax?.rows?.length">
            <dt>DPH</dt>
            <dd>
              <span v-for="r in detail.tax.rows" :key="r.vatRate" class="vat-line">{{ r.vatRate }} %: základ {{ formatAmount(r.base) }} €, DPH {{ formatAmount(r.vat) }} €</span>
              <span v-if="detail.tax.supplierIcdph" class="vat-line">IČ DPH predajcu {{ detail.tax.supplierIcdph }}</span>
              <span v-if="detail.tax.noDeduction" class="vat-line">bez odpočtu</span>
            </dd>
          </template>
          <dt v-if="detail.linkNumber">Uhrádza</dt>
          <dd v-if="detail.linkNumber"><RouterLink :to="docLink(detail.linkKind, detail.linkId)">{{ detail.linkNumber }}</RouterLink></dd>
        </dl>
        <div class="form-grid cash-form">
          <FormField :label="detail.kind === 'in' ? 'Prijaté od' : 'Vyplatené komu'" :span="2">
            <InputText v-model="detail.partner" size="small" :disabled="!canWrite" />
          </FormField>
          <FormField label="Účel" :span="2">
            <InputText v-model="detail.purpose" size="small" :disabled="!canWrite" />
          </FormField>
          <FormField label="Poznámka" :span="2">
            <InputText v-model="detail.note" size="small" :disabled="!canWrite" />
          </FormField>
        </div>
        <p class="muted">Sumu ani dátum vystaveného dokladu nemožno meniť — doklad stornujte a vystavte nový.</p>
        <div class="actions">
          <Button label="Tlač" icon="pi pi-print" severity="secondary" text size="small" @click="router.push(`/pokladna/${detail.id}/tlac`)" />
          <div class="spacer" />
          <template v-if="canWrite && !detail.cancelled">
            <Button label="Stornovať" severity="danger" text size="small" @click="cancelDoc" />
            <Button label="Uložiť" icon="pi pi-check" size="small" :loading="busy" @click="saveText" />
          </template>
        </div>
      </template>
    </Drawer>
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
}
.head h1 {
  font-size: var(--fs-xl);
}
.spacer {
  flex: 1;
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
.figure .value.in {
  color: var(--tone-paid);
}
.grand {
  border-bottom: 3px double currentColor;
  line-height: 1.15;
}
.vat {
  border: 0;
  border-top: 1px solid var(--rule);
  margin: 4px 0 0;
  padding: 8px 0 0;
  min-width: 0;
}
.span2 {
  grid-column: span 2;
}
.vat-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 2px 0 6px;
}
.vat-rows {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.vat-rows th {
  text-align: left;
  font-weight: 500;
  color: var(--ink-soft);
  padding: 2px 4px;
}
.vat-rows td {
  padding: 2px 4px;
}
.vat-rows .num {
  text-align: right;
}
.vat-rows td.num :deep(input) {
  width: 5.6rem;
  text-align: right;
}
.vat-rows :deep(.p-select) {
  width: 6.2rem;
}
.full {
  grid-column: 1 / -1;
}
.vat-rows .calc {
  color: var(--ink-soft);
  white-space: nowrap;
}
.vat-line {
  display: block;
}
.err {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
  margin: 4px 0;
}
.small {
  font-size: var(--fs-sm);
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
.grid :deep(.cell-in) {
  color: var(--tone-paid);
  font-weight: 600;
}
.grid :deep(.cell-link) {
  color: var(--blue-deep);
}
.grid :deep(.cell-cancelled) {
  color: var(--tone-cancelled);
}
.grid :deep(.row-cancelled) {
  color: var(--tone-cancelled);
  text-decoration: line-through;
}
.error {
  color: var(--tone-overdue);
  margin: 0;
}
.cash-form {
  grid-template-columns: 1fr 1fr;
}
.num :deep(input),
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}
.opt {
  display: flex;
  gap: 8px;
  align-items: baseline;
  width: 100%;
}
.opt .amt {
  margin-left: auto;
  font-variant-numeric: tabular-nums;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.amount {
  font-size: 1.625rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  margin-bottom: 10px;
}
.amount.in {
  color: var(--tone-paid);
}
.badge {
  font-size: var(--fs-xs);
  font-weight: 400;
  padding: 0 6px;
  border-radius: 8px;
  background: var(--tone-cancelled-bg);
  color: var(--tone-cancelled);
  vertical-align: middle;
}
.facts {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 3px 12px;
  margin: 0 0 12px;
}
.facts dt {
  color: var(--ink-faint);
}
.facts dd {
  margin: 0;
}
.facts a {
  color: var(--blue-deep);
  font-weight: 600;
}
@media (max-width: 760px) {
  .page {
    padding: 10px 10px 8px;
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
  .search {
    flex: 1;
    width: auto;
    min-width: 0;
    margin-left: 0;
  }
  .head :deep(.p-button-label) {
    display: none;
  }
}
</style>

<style>
.cash-panel.p-drawer {
  width: 440px;
  max-width: 100vw;
}
</style>
