<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import DatePicker from 'primevue/datepicker'
import SelectButton from 'primevue/selectbutton'
import Checkbox from 'primevue/checkbox'
import { api, ApiError } from '../lib/api.js'
import { session } from '../lib/session.js'
import {
  formatAmount, formatDate, formatMoney, formatQuantity,
  fromISODate, parseNumber, statusOf, toISODate,
} from '../lib/format.js'
import { fiskusGridTheme, skGridLocale } from '../lib/grid.js'

const props = defineProps({ id: String })
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

let keySeq = 0
const withKey = (l) => ({ ...l, _key: ++keySeq })
const blankLine = () => withKey({ description: '', quantity: '1', unit: 'ks', unitPrice: '0', vatRate: 23, amount: '0' })

const inv = ref(null)
const lines = shallowRef([])
const loadError = ref(null)
const busy = ref(false)
const dirty = ref(false)
const showErrors = ref(false)
const errors = ref({}) // pole → správa
const previewErrors = ref({})

const editable = computed(() => !inv.value?.status || inv.value.status === 'draft')
const status = computed(() => (inv.value ? statusOf(inv.value) : null))
const title = computed(() => {
  if (!inv.value) return ''
  if (inv.value.number) return `Faktúra ${inv.value.number}`
  return inv.value.id ? 'Koncept faktúry' : 'Nová faktúra'
})

async function load() {
  try {
    if (props.id) {
      const data = await api.invoice(session.companyId, props.id)
      inv.value = data
      lines.value = data.lines.map(withKey)
    } else {
      const today = toISODate(new Date())
      const due = new Date()
      due.setDate(due.getDate() + 14)
      inv.value = {
        kind: 'invoice',
        status: 'draft',
        customer: { name: '', street: '', city: '', zip: '', country: 'SK', ico: '', dic: '', icdph: '', email: '' },
        issueDate: today,
        deliveryDate: today,
        dueDate: toISODate(due),
        priceMode: 'net',
        reverseCharge: false,
        note: '',
        summary: { rates: [], base: '0', vat: '0', total: '0' },
      }
      lines.value = [blankLine()]
    }
  } catch (e) {
    loadError.value = e.message
  }
}
onMounted(load)

// ---- živý prepočet cez API (jediný zdroj pravdy pre výpočet DPH) ----
function payload() {
  return { ...inv.value, lines: lines.value }
}

let timer = null
let seq = 0
function changed() {
  dirty.value = true
  clearTimeout(timer)
  timer = setTimeout(recalc, 250)
}

async function recalc() {
  const mine = ++seq
  try {
    const out = await api.preview(payload())
    if (mine !== seq) return // medzitým prišla novšia zmena
    inv.value.summary = out.summary
    out.lines.forEach((l, i) => {
      if (lines.value[i]) lines.value[i].amount = l.amount
    })
    gridApi.value?.refreshCells({ columns: ['amount'] })
    previewErrors.value = Object.fromEntries(out.errors.map((e) => [e.field, e.message]))
  } catch {
    /* chybný vstup sa ukáže pri uložení */
  }
}

const fieldError = (f) => (showErrors.value ? errors.value[f] ?? previewErrors.value[f] : errors.value[f])

// ---- dátumy (DatePicker pracuje s Date, API s "YYYY-MM-DD") ----
const dateModel = (key) =>
  computed({
    get: () => fromISODate(inv.value?.[key]),
    set: (d) => {
      inv.value[key] = toISODate(d)
      changed()
    },
  })
const issueDate = dateModel('issueDate')
const deliveryDate = dateModel('deliveryDate')
const dueDate = dateModel('dueDate')
const dueDays = computed(() => {
  const a = fromISODate(inv.value?.issueDate)
  const b = fromISODate(inv.value?.dueDate)
  return a && b ? Math.round((b - a) / 86400000) : null
})
function setDueDays(n) {
  const d = fromISODate(inv.value.issueDate)
  d.setDate(d.getDate() + n)
  dueDate.value = d
}

const priceModes = [
  { label: 'Ceny bez DPH', value: 'net' },
  { label: 'Ceny s DPH', value: 'gross' },
]

// ---- položky ----
const gridApi = shallowRef(null)
const lineRowId = (p) => String(p.data._key)
const numParser = (p) => parseNumber(p.newValue) ?? p.oldValue

const lineCols = computed(() => [
  { headerName: '', valueGetter: 'node.rowIndex + 1', width: 44, editable: false, cellClass: 'cell-pos', sortable: false },
  { field: 'description', headerName: 'Popis položky', flex: 1, minWidth: 200 },
  {
    field: 'quantity', headerName: 'Množstvo', width: 96, type: 'rightAligned',
    valueFormatter: (p) => formatQuantity(p.value), valueParser: numParser,
  },
  { field: 'unit', headerName: 'MJ', width: 64 },
  {
    field: 'unitPrice', headerName: 'Cena za MJ', width: 112, type: 'rightAligned',
    valueFormatter: (p) => formatAmount(p.value), valueParser: numParser,
  },
  {
    field: 'vatRate', headerName: 'DPH', width: 74, type: 'rightAligned',
    cellEditor: 'agSelectCellEditor', cellEditorParams: { values: [23, 19, 5, 0] },
    valueFormatter: (p) => `${p.value} %`,
    valueParser: (p) => Number(p.newValue),
  },
  {
    field: 'amount', headerName: 'Suma', width: 116, type: 'rightAligned', editable: false,
    valueFormatter: (p) => formatAmount(p.value), cellClass: 'ag-right-aligned-cell cell-amount',
  },
  {
    colId: 'del', headerName: '', width: 40, editable: false, sortable: false, hide: !editable.value,
    cellRenderer: () => '<i class="pi pi-times" title="Odstrániť položku" aria-label="Odstrániť položku"></i>',
    cellClass: 'cell-del',
  },
])

const lineDefaults = computed(() => ({ editable: editable.value, sortable: false, resizable: true, suppressMovable: true }))

function onLineCell(e) {
  if (e.colDef.colId === 'del' && editable.value) {
    lines.value = lines.value.filter((_, i) => i !== e.rowIndex)
    if (!lines.value.length) lines.value = [blankLine()]
    changed()
  }
}

function addLine() {
  lines.value = [...lines.value, blankLine()]
  changed()
  setTimeout(() => {
    const rowIndex = lines.value.length - 1
    // najprv presunúť fokus, inak by Tab pokračoval z predchádzajúcej bunky
    gridApi.value?.setFocusedCell(rowIndex, 'description')
    gridApi.value?.startEditingCell({ rowIndex, colKey: 'description' })
  }, 30)
}

// ---- akcie ----
function applyError(e) {
  if (e instanceof ApiError && e.fields.length) {
    errors.value = e.fieldMap()
    showErrors.value = true
    toast.add({ severity: 'error', summary: e.message, detail: e.fields[0].message, life: 5000 })
  } else {
    toast.add({ severity: 'error', summary: 'Akcia sa nepodarila', detail: e.message, life: 6000 })
  }
}

async function save({ quiet = false } = {}) {
  gridApi.value?.stopEditing()
  busy.value = true
  try {
    const saved = await api.saveInvoice(session.companyId, payload())
    const wasNew = !inv.value.id
    inv.value = saved
    lines.value = saved.lines.length ? saved.lines.map(withKey) : [blankLine()]
    dirty.value = false
    errors.value = {}
    if (!quiet) toast.add({ severity: 'success', summary: 'Koncept uložený', life: 2500 })
    if (wasNew) router.replace(`/faktury/${saved.id}`)
    return saved
  } catch (e) {
    applyError(e)
    return null
  } finally {
    busy.value = false
  }
}

function issue() {
  confirm.require({
    header: 'Vystaviť faktúru',
    message: 'Faktúra dostane číslo z radu a už sa nebude dať upravovať. Opraviť ju bude možné iba dobropisom.',
    acceptProps: { label: 'Vystaviť' },
    rejectProps: { label: 'Zrušiť', severity: 'secondary', outlined: true },
    accept: async () => {
      const saved = await save({ quiet: true })
      if (!saved) return
      busy.value = true
      try {
        inv.value = await api.issue(session.companyId, saved.id)
        showErrors.value = false
        toast.add({ severity: 'success', summary: `Vystavená ${inv.value.number}`, life: 3000 })
      } catch (e) {
        applyError(e)
      } finally {
        busy.value = false
      }
    },
  })
}

async function markPaid() {
  busy.value = true
  try {
    inv.value = await api.pay(session.companyId, inv.value.id)
    toast.add({ severity: 'success', summary: 'Označená ako uhradená', life: 2500 })
  } catch (e) {
    applyError(e)
  } finally {
    busy.value = false
  }
}

function cancelInvoice() {
  confirm.require({
    header: 'Stornovať faktúru',
    message: `Faktúra ${inv.value.number} sa označí ako stornovaná a nebude sa započítavať do súčtov.`,
    acceptProps: { label: 'Stornovať', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        inv.value = await api.cancel(session.companyId, inv.value.id)
        toast.add({ severity: 'info', summary: 'Faktúra stornovaná', life: 2500 })
      } catch (e) {
        applyError(e)
      }
    },
  })
}

function onKey(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    if (editable.value && !busy.value) save()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  clearTimeout(timer)
})

const c = computed(() => inv.value?.customer ?? {})
const upd = (obj, key) => (v) => {
  obj[key] = v
  changed()
}
const customerFields = reactive([
  { key: 'name', label: 'Názov / meno', wide: true, autocomplete: 'organization' },
  { key: 'street', label: 'Ulica a číslo', wide: true, autocomplete: 'street-address' },
  { key: 'zip', label: 'PSČ', autocomplete: 'postal-code' },
  { key: 'city', label: 'Obec', autocomplete: 'address-level2' },
  { key: 'ico', label: 'IČO', inputmode: 'numeric' },
  { key: 'dic', label: 'DIČ', inputmode: 'numeric' },
  { key: 'icdph', label: 'IČ DPH' },
  { key: 'email', label: 'E-mail', autocomplete: 'email' },
])
</script>

<template>
  <div v-if="loadError" class="state">
    <p>{{ loadError }}</p>
    <RouterLink to="/faktury">Späť na zoznam</RouterLink>
  </div>

  <div v-else-if="inv" class="editor" :class="{ locked: !editable }">
    <header class="bar">
      <RouterLink to="/faktury" class="back" aria-label="Späť na zoznam faktúr"><i class="pi pi-arrow-left" /></RouterLink>
      <h1>{{ title }}</h1>
      <span v-if="inv.id" class="tag" :data-tone="status.tone"><span class="tag-text">{{ status.label }}</span></span>
      <span v-if="dirty && editable" class="unsaved">neuložené zmeny</span>
      <div class="spacer" />
      <template v-if="editable">
        <Button label="Uložiť koncept" severity="secondary" outlined size="small" :loading="busy" @click="save()" v-tooltip.bottom="'Ctrl+S'" />
        <Button label="Vystaviť" icon="pi pi-check" size="small" :disabled="busy" @click="issue" />
      </template>
      <template v-else-if="inv.status === 'issued'">
        <Button label="Stornovať" severity="secondary" text size="small" @click="cancelInvoice" />
        <Button label="Označiť ako uhradenú" icon="pi pi-wallet" size="small" :loading="busy" @click="markPaid" />
      </template>
    </header>

    <div class="body">
      <div class="form">
        <fieldset class="block">
          <legend>Odberateľ</legend>
          <div class="fields customer">
            <label v-for="f in customerFields.filter((f) => editable || c[f.key])" :key="f.key" :class="{ wide: f.wide }">
              <span>{{ f.label }}</span>
              <InputText
                :model-value="c[f.key]"
                :disabled="!editable"
                :invalid="!!fieldError(`customer.${f.key}`)"
                :inputmode="f.inputmode"
                :autocomplete="f.autocomplete"
                size="small"
                @update:model-value="upd(inv.customer, f.key)($event)"
              />
              <small v-if="fieldError(`customer.${f.key}`)" class="err">{{ fieldError(`customer.${f.key}`) }}</small>
            </label>
          </div>
        </fieldset>

        <fieldset class="block">
          <legend>Doklad</legend>
          <div class="fields dates">
            <label>
              <span>Dátum vystavenia</span>
              <DatePicker v-model="issueDate" :disabled="!editable" date-format="d. m. yy" show-icon icon-display="input" size="small" :invalid="!!fieldError('issueDate')" />
            </label>
            <label>
              <span>Dátum dodania</span>
              <DatePicker v-model="deliveryDate" :disabled="!editable" date-format="d. m. yy" show-icon icon-display="input" size="small" :invalid="!!fieldError('deliveryDate')" />
              <small v-if="fieldError('deliveryDate')" class="err">{{ fieldError('deliveryDate') }}</small>
            </label>
            <label>
              <span>Splatnosť</span>
              <DatePicker v-model="dueDate" :disabled="!editable" date-format="d. m. yy" show-icon icon-display="input" size="small" :invalid="!!fieldError('dueDate')" />
              <small v-if="fieldError('dueDate')" class="err">{{ fieldError('dueDate') }}</small>
            </label>
            <div v-if="editable" class="due-quick" role="group" aria-label="Splatnosť v dňoch">
              <button v-for="n in [7, 14, 30]" :key="n" type="button" :class="{ on: dueDays === n }" @click="setDueDays(n)">{{ n }} dní</button>
            </div>
          </div>
          <div class="fields vat-opts">
            <SelectButton
              :model-value="inv.priceMode"
              :options="priceModes"
              option-label="label"
              option-value="value"
              :allow-empty="false"
              :disabled="!editable"
              size="small"
              @update:model-value="upd(inv, 'priceMode')($event)"
            />
            <label class="check">
              <Checkbox :model-value="inv.reverseCharge" binary :disabled="!editable" input-id="rc" @update:model-value="upd(inv, 'reverseCharge')($event)" />
              <span>Prenesenie daňovej povinnosti</span>
            </label>
          </div>
          <small v-if="fieldError('customer.icdph') && inv.reverseCharge" class="err">{{ fieldError('customer.icdph') }}</small>
        </fieldset>

        <fieldset class="block lines-block">
          <legend>Položky</legend>
          <div class="lines" :class="{ invalid: !!fieldError('lines') }">
            <AgGridVue
              :theme="fiskusGridTheme"
              :row-data="lines"
              :column-defs="lineCols"
              :default-col-def="lineDefaults"
              :locale-text="skGridLocale"
              dom-layout="autoHeight"
              :get-row-id="lineRowId"
              :single-click-edit="true"
              :stop-editing-when-cells-lose-focus="true"
              :enter-navigates-vertically-after-edit="true"
              @grid-ready="(e) => (gridApi = e.api)"
              @cell-value-changed="changed"
              @cell-clicked="onLineCell"
            />
          </div>
          <small v-if="fieldError('lines')" class="err">{{ fieldError('lines') }}</small>
          <Button v-if="editable" label="Pridať položku" icon="pi pi-plus" text size="small" class="add" @click="addLine" />
        </fieldset>

        <fieldset class="block">
          <legend>Poznámka na faktúre</legend>
          <Textarea :model-value="inv.note" :disabled="!editable" rows="2" auto-resize class="note" @update:model-value="upd(inv, 'note')($event)" />
        </fieldset>
      </div>

      <aside class="recap" aria-label="Rekapitulácia DPH">
        <h2>Rekapitulácia DPH</h2>
        <table>
          <thead>
            <tr>
              <th>Sadzba</th>
              <th class="num">Základ</th>
              <th class="num">DPH</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in inv.summary.rates" :key="r.rate">
              <td>{{ r.rate }} %</td>
              <td class="num">{{ formatAmount(r.base) }}</td>
              <td class="num">{{ formatAmount(r.vat) }}</td>
            </tr>
            <tr v-if="!inv.summary.rates?.length">
              <td colspan="3" class="empty">Pridajte položky</td>
            </tr>
          </tbody>
          <tfoot>
            <tr>
              <td>Spolu</td>
              <td class="num">{{ formatAmount(inv.summary.base) }}</td>
              <td class="num">{{ formatAmount(inv.summary.vat) }}</td>
            </tr>
          </tfoot>
        </table>
        <div class="payable">
          <span>K úhrade</span>
          <strong class="num">{{ formatMoney(inv.summary.total, inv.currency) }}</strong>
        </div>
        <p v-if="inv.reverseCharge" class="note-rc">Prenesenie daňovej povinnosti — DPH odvedie odberateľ.</p>
        <dl v-if="inv.number" class="pay-info">
          <dt>Variabilný symbol</dt>
          <dd>{{ inv.variableSymbol }}</dd>
          <template v-if="session.company?.iban">
            <dt>IBAN</dt>
            <dd>{{ session.company.iban }}</dd>
          </template>
          <template v-if="inv.paidDate">
            <dt>Uhradená</dt>
            <dd>{{ formatDate(inv.paidDate) }}</dd>
          </template>
        </dl>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.state {
  padding: 48px 16px;
  text-align: center;
}

.editor {
  min-height: 100%;
  display: flex;
  flex-direction: column;
}

.bar {
  position: sticky;
  top: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  background: var(--paper);
  border-bottom: 1px solid var(--rule);
}
.bar h1 {
  font-size: var(--fs-xl);
  white-space: nowrap;
}
.back {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: var(--radius);
  color: var(--ink-soft);
}
.back:hover {
  background: var(--rule-soft);
}
.unsaved {
  font-size: var(--fs-sm);
  color: var(--tone-draft);
}
.spacer {
  flex: 1;
}

.body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 300px;
  gap: 16px;
  padding: 14px 16px 32px;
  align-items: start;
}

.block {
  margin: 0 0 14px;
  padding: 0;
  border: 0;
  min-width: 0;
}
legend {
  padding: 0 0 6px;
  font-size: var(--fs-md);
  font-weight: 600;
  color: var(--ink);
  width: 100%;
  border-bottom: 1px solid var(--rule);
  margin-bottom: 8px;
}

.fields {
  display: grid;
  gap: 8px 12px;
}
.fields.customer {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}
.fields.dates {
  grid-template-columns: repeat(3, minmax(0, 170px)) auto;
  align-items: end;
}
.fields label {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}
.fields label > span {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.fields label.wide {
  grid-column: span 2;
}
.fields :deep(input),
.fields :deep(.p-datepicker) {
  width: 100%;
}

.due-quick {
  display: flex;
  gap: 4px;
  padding-bottom: 2px;
}
.due-quick button {
  font: inherit;
  font-size: var(--fs-sm);
  padding: 4px 8px;
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  background: var(--sheet);
  color: var(--ink-soft);
  cursor: pointer;
}
.due-quick button.on {
  border-color: var(--blue);
  color: var(--blue-deep);
  background: var(--blue-wash);
  font-weight: 600;
}

.vat-opts {
  margin-top: 10px;
  grid-template-columns: auto 1fr;
  align-items: center;
  gap: 16px;
}
.check {
  flex-direction: row !important;
  align-items: center;
  gap: 8px !important;
}
.check span {
  font-size: var(--fs-md) !important;
  color: var(--ink) !important;
}

.lines {
  border: 1px solid var(--rule);
  background: var(--sheet);
}
.lines :deep(.ag-center-cols-viewport) {
  min-height: 0 !important;
}
.lines.invalid {
  border-color: var(--tone-overdue);
}
.lines :deep(.cell-pos) {
  color: var(--ink-faint);
  text-align: right;
}
.lines :deep(.cell-amount) {
  font-weight: 600;
}
.lines :deep(.cell-del) {
  color: var(--ink-faint);
  cursor: pointer;
  text-align: center;
}
.lines :deep(.cell-del:hover) {
  color: var(--tone-overdue);
}
.add {
  margin-top: 4px;
}

.note {
  width: 100%;
}

.err {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}

/* Vystavená faktúra: polia ako vytlačený doklad, nie ako zakázané vstupy */
.locked .fields :deep(.p-inputtext:disabled),
.locked :deep(.p-textarea:disabled) {
  background: transparent;
  border-color: transparent;
  box-shadow: none;
  color: var(--ink);
  opacity: 1;
  padding-left: 0;
}
.locked :deep(.p-datepicker-input-icon-container),
.locked .vat-opts :deep(.p-togglebutton:not(.p-togglebutton-checked)) {
  display: none;
}
.locked .vat-opts :deep(.p-selectbutton) {
  opacity: 1;
}

/* Rekapitulácia: účtovný súčet s dvojitou čiarou pod konečnou sumou */
.recap {
  position: sticky;
  top: 62px;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 3px solid var(--blue-deep);
  padding: 12px 14px 14px;
}
.recap h2 {
  font-size: var(--fs-md);
  margin-bottom: 8px;
}
.recap table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.recap th {
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--ink-soft);
  text-align: left;
  padding: 3px 0;
  border-bottom: 1px solid var(--rule);
}
.recap th.num {
  text-align: right;
}
.recap td {
  padding: 4px 0;
  border-bottom: 1px solid var(--rule-soft);
}
.recap td.empty {
  color: var(--ink-faint);
}
.recap tfoot td {
  font-weight: 600;
  border-bottom: 0;
  border-top: 1px solid var(--ink);
}
.payable {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-top: 12px;
  padding-top: 8px;
}
.payable span {
  color: var(--ink-soft);
}
.payable strong {
  font-size: 1.625rem;
  font-weight: 700;
  letter-spacing: -0.015em;
  border-bottom: 3px double var(--ink);
  line-height: 1.15;
}
.note-rc {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  margin: 10px 0 0;
}
.pay-info {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 4px 10px;
  margin: 14px 0 0;
  padding-top: 10px;
  border-top: 1px solid var(--rule);
  font-size: var(--fs-sm);
}
.pay-info dt {
  color: var(--ink-soft);
}
.pay-info dd {
  margin: 0;
  text-align: right;
  font-variant-numeric: tabular-nums;
}

@media (max-width: 1100px) {
  .body {
    grid-template-columns: 1fr;
  }
  .recap {
    position: static;
    order: -1;
  }
}

@media (max-width: 760px) {
  .bar {
    flex-wrap: wrap;
    padding: 8px 10px;
  }
  .bar h1 {
    font-size: var(--fs-lg);
  }
  .body {
    padding: 10px 10px 24px;
  }
  .fields.customer {
    grid-template-columns: 1fr 1fr;
  }
  .fields.dates {
    grid-template-columns: 1fr 1fr;
  }
  .vat-opts {
    grid-template-columns: 1fr;
  }
}
</style>
