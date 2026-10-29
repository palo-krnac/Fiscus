<script setup>
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import DatePicker from 'primevue/datepicker'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import AutoComplete from 'primevue/autocomplete'
import FormField from '../components/FormField.vue'
import IcoInput from '../components/IcoInput.vue'
import VatIdInput from '../components/VatIdInput.vue'
import { api, ApiError } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { codes, ratesOn, supplyKinds, isForeign, supplyHint } from '../lib/codes.js'
import SelectButton from 'primevue/selectbutton'
import { filterContacts, customerFromContact } from '../lib/contacts.js'
import { fillFromRegistry, fillFromVIES } from '../lib/registry.js'
import { formatAmount, formatDate, formatMoney, fromISODate, parseNumber, purchaseStatusOf, toISODate } from '../lib/format.js'
import { takePurchaseDraft } from '../lib/drafts.js'
import { currencyOptions, isForeignCurrency, rateHint, rateText } from '../lib/fx.js'

const props = defineProps({ id: String })
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

const p = ref(null)
const loadError = ref(null)
const busy = ref(false)
const dirty = ref(false)
const errors = ref({})
const previewErrors = ref({})
const warnings = ref([])
const showErrors = ref(false)
const summary = ref({ base: '0', vat: '0', total: '0' })
const computedVat = ref([]) // DPH z náhľadu pre riadky bez vyplnenej DPH
const aiNotes = ref(null) // upozornenia, ak údaje prečítala AI

const canWrite = computed(() => can('purchases.write'))
const locked = computed(() => p.value?.status === 'paid' || !canWrite.value)
const title = computed(() => (p.value?.number ? `Prijatá faktúra ${p.value.number}` : 'Nová prijatá faktúra'))
const status = computed(() => (p.value?.id ? purchaseStatusOf(p.value) : null))

const blankRow = () => ({ vatRate: 23, base: '', vat: '' })

async function load() {
  loadError.value = null
  try {
    if (props.id) {
      const data = await api.purchase(session.companyId, props.id)
      // sumy z API ("100.00") → zobrazenie s čiarkou na úpravu
      data.rates = data.rates.map((r) => ({ vatRate: r.vatRate, base: formatAmount(r.base), vat: formatAmount(r.vat) }))
      p.value = data
      summary.value = data.summary
    } else if ((aiNotes.value = takePurchaseDraft())) {
      // koncept prečítaný z dokladu — sumy na úpravu s čiarkou
      const d = aiNotes.value.purchase
      d.rates = (d.rates?.length ? d.rates : [{ vatRate: 23, base: '0', vat: '0' }]).map((r) => ({ vatRate: r.vatRate, base: formatAmount(r.base), vat: formatAmount(r.vat) }))
      d.supplier = { street: '', city: '', zip: '', country: 'SK', ico: '', dic: '', icdph: '', email: '', ...d.supplier }
      p.value = { description: '', note: '', supplyKind: '', noDeduction: false, reverseCharge: false, ...d, id: undefined, number: '' }
      dirty.value = true
      return
    } else {
      const today = toISODate(new Date())
      p.value = {
        supplierNumber: '', variableSymbol: '', contactId: 0,
        supplier: { name: '', street: '', city: '', zip: '', country: 'SK', ico: '', dic: '', icdph: '', email: '' },
        supplierIban: '', issueDate: today, deliveryDate: today, receivedDate: today, dueDate: null,
        paymentMethod: 'transfer', reverseCharge: false, noDeduction: false, supplyKind: '', description: '', note: '',
        rates: [blankRow()],
      }
    }
    dirty.value = false
  } catch (e) {
    loadError.value = e.message
  }
}
onMounted(load)
watch(() => props.id, (id, prev) => {
  if (id !== prev && !(id && p.value && String(p.value.id) === id)) load()
})

// ---- náhľad (výpočet DPH a kontroly robí backend) ----
function payload() {
  return {
    ...p.value,
    rates: p.value.rates.map((r) => ({
      vatRate: Number(r.vatRate),
      base: parseNumber(r.base) ?? '0',
      vat: parseNumber(r.vat) ?? '0',
    })),
  }
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
    const out = await api.previewPurchase(session.companyId, payload())
    if (mine !== seq) return
    summary.value = out.summary
    // kurz ECB a sumy v eurách určuje server
    Object.assign(p.value, { fxRate: out.fxRate, fxDate: out.fxDate, fxManual: out.fxManual, local: out.local ?? null })
    computedVat.value = out.rates.map((r) => r.vat)
    warnings.value = out.warnings
    previewErrors.value = Object.fromEntries(out.errors.map((e) => [e.field, e.message]))
  } catch {
    /* chybný vstup sa ukáže pri uložení */
  }
}
onMounted(() => setTimeout(recalc, 50))

const fieldError = (f) => errors.value[f] ?? (showErrors.value ? previewErrors.value[f] : undefined)

// ---- dátumy ----
const dateModel = (key) =>
  computed({
    get: () => fromISODate(p.value?.[key]),
    set: (d) => {
      p.value[key] = toISODate(d)
      changed()
    },
  })
const issueDate = dateModel('issueDate')
const deliveryDate = dateModel('deliveryDate')
const receivedDate = dateModel('receivedDate')
const dueDate = dateModel('dueDate')

const rateOptions = computed(() => ratesOn(p.value?.deliveryDate || p.value?.issueDate).map((r) => ({ label: `${r} %`, value: r })))

function addRow() {
  p.value.rates.push(blankRow())
  changed()
}
function removeRow(i) {
  p.value.rates.splice(i, 1)
  if (!p.value.rates.length) p.value.rates.push(blankRow())
  changed()
}

// ---- dodávateľ z adresára / registra ----
const contacts = shallowRef([])
const suggestions = ref([])
onMounted(async () => {
  contacts.value = await api.contacts(session.companyId).catch(() => [])
})
function searchContacts(e) {
  suggestions.value = filterContacts(contacts.value, e.query).slice(0, 12)
}
function onSupplierName(v) {
  if (v && typeof v === 'object') {
    p.value.supplier = customerFromContact(v)
    p.value.contactId = v.id
    if (v.iban && !p.value.supplierIban) p.value.supplierIban = v.iban
  } else {
    p.value.supplier.name = v ?? ''
  }
  changed()
}

const supplierFields = [
  { key: 'street', label: 'Ulica a číslo', span: 2 },
  { key: 'zip', label: 'PSČ' },
  { key: 'city', label: 'Obec' },
  { key: 'dic', label: 'DIČ' },
  { key: 'icdph', label: 'IČ DPH' },
]

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

async function save() {
  busy.value = true
  try {
    const wasNew = !p.value.id
    const out = await api.savePurchase(session.companyId, payload())
    errors.value = {}
    showErrors.value = false
    dirty.value = false
    toast.add({ severity: 'success', summary: wasNew ? `Zaevidovaná ${out.number}` : 'Uložené', life: 2500 })
    if (out.warnings.length) toast.add({ severity: 'warn', summary: 'Skontrolujte doklad', detail: out.warnings[0].message, life: 6000 })
    if (wasNew) router.replace(`/prijate/${out.id}`)
    else await load()
  } catch (e) {
    applyError(e)
  } finally {
    busy.value = false
  }
}

async function pay(undo = false) {
  busy.value = true
  try {
    if (undo) await api.unpayPurchase(session.companyId, p.value.id)
    else await api.payPurchase(session.companyId, p.value.id)
    toast.add({ severity: 'success', summary: undo ? 'Úhrada zrušená' : 'Označená ako uhradená', life: 2500 })
    await load()
  } catch (e) {
    applyError(e)
  } finally {
    busy.value = false
  }
}

function remove() {
  confirm.require({
    header: 'Zmazať prijatú faktúru',
    message: `${p.value.number} od ${p.value.supplier.name} sa zmaže z evidencie.`,
    acceptProps: { label: 'Zmazať', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await api.deletePurchase(session.companyId, p.value.id)
        toast.add({ severity: 'info', summary: 'Zmazaná', life: 2500 })
        router.push('/prijate')
      } catch (e) {
        applyError(e)
      }
    },
  })
}

function onKey(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    if (!locked.value && !busy.value) save()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  clearTimeout(timer)
})

// ---- mena a kurz ----
const currencies = computed(() => currencyOptions(codes.currencies))
const foreign = computed(() => isForeignCurrency(p.value?.currency))
const fxNumber = computed(() => Number(p.value?.fxRate) || null)
function setCurrency(v) {
  Object.assign(p.value, { currency: v === 'EUR' ? '' : v, fxRate: '0', fxManual: false, local: null })
  changed()
}
function setRate(v) {
  Object.assign(p.value, { fxRate: v ? String(v) : '0', fxManual: !!v })
  changed()
}
function resetRate() {
  Object.assign(p.value, { fxRate: '0', fxManual: false })
  changed()
}

const set = (obj, key) => (v) => {
  obj[key] = v
  changed()
}
</script>

<template>
  <div v-if="loadError" class="state">
    <p>{{ loadError }}</p>
    <RouterLink to="/prijate">Späť na zoznam</RouterLink>
  </div>

  <div v-else-if="p" class="editor" :class="{ locked }">
    <header class="bar">
      <RouterLink to="/prijate" class="back" aria-label="Späť na prijaté faktúry"><i class="pi pi-arrow-left" /></RouterLink>
      <h1>{{ title }}</h1>
      <span v-if="status" class="tag" :data-tone="status.tone"><span class="tag-text">{{ status.label }}</span></span>
      <span v-if="dirty && !locked" class="unsaved">neuložené zmeny</span>
      <div class="spacer" />
      <Button v-if="p.id && !locked" label="Zmazať" severity="danger" text size="small" @click="remove" />
      <template v-if="!canWrite" />
      <template v-else-if="locked">
        <span class="paid-on">Uhradená {{ formatDate(p.paidDate) }}</span>
        <Button label="Zrušiť úhradu" severity="secondary" text size="small" :loading="busy" @click="pay(true)" />
      </template>
      <template v-else>
        <Button v-if="p.id && !dirty" label="Označiť ako uhradenú" icon="pi pi-wallet" severity="secondary" outlined size="small" :loading="busy" @click="pay()" />
        <Button :label="p.id ? 'Uložiť' : 'Zaevidovať'" icon="pi pi-check" size="small" :loading="busy" @click="save" v-tooltip.bottom="'Ctrl+S'" />
      </template>
    </header>

    <div class="body">
      <div class="form">
        <div v-if="aiNotes" class="ai-note">
          <i class="pi" :class="aiNotes.source === 'ubl' ? 'pi-file-import' : 'pi-sparkles'" />
          <div>
            <strong v-if="aiNotes.source === 'ubl'">Údaje sú z e-faktúry — pred zaevidovaním ich skontrolujte.</strong>
            <strong v-else>Údaje prečítala AI z dokladu — pred zaevidovaním ich skontrolujte.</strong>
            <ul v-if="aiNotes.warnings?.length">
              <li v-for="w in aiNotes.warnings" :key="w">{{ w }}</li>
            </ul>
          </div>
        </div>
        <fieldset class="form-section">
          <legend>Dodávateľ</legend>
          <div class="form-grid">
            <FormField label="Názov / meno" :span="2" :error="fieldError('supplier.name')">
              <AutoComplete
                v-if="!locked"
                :model-value="p.supplier.name"
                :suggestions="suggestions"
                option-label="name"
                :delay="80"
                :show-empty-message="false"
                :invalid="!!fieldError('supplier.name')"
                placeholder="Začnite písať názov alebo IČO z adresára"
                size="small"
                fluid
                @complete="searchContacts"
                @update:model-value="onSupplierName"
              />
              <InputText v-else :model-value="p.supplier.name" disabled size="small" />
            </FormField>
            <FormField label="IČO" :error="fieldError('supplier.ico')">
              <IcoInput
                v-if="!locked"
                :model-value="p.supplier.ico"
                :invalid="!!fieldError('supplier.ico')"
                :auto="!p.supplier.name"
                @update:model-value="set(p.supplier, 'ico')($event)"
                @found="(r) => { p.supplier = fillFromRegistry(p.supplier, r); changed() }"
              />
              <InputText v-else :model-value="p.supplier.ico" disabled size="small" />
            </FormField>
            <FormField v-for="f in supplierFields" :key="f.key" :label="f.label" :span="f.span" :error="fieldError(`supplier.${f.key}`)">
              <VatIdInput v-if="f.key === 'icdph' && !locked" :model-value="p.supplier.icdph" :invalid="!!fieldError('supplier.icdph')"
                @update:model-value="set(p.supplier, 'icdph')($event)" @found="(r) => { p.supplier = fillFromVIES(p.supplier, r); changed() }" />
              <InputText v-else :model-value="p.supplier[f.key]" :disabled="locked" :invalid="!!fieldError(`supplier.${f.key}`)" size="small" @update:model-value="set(p.supplier, f.key)($event)" />
            </FormField>
          </div>
        </fieldset>

        <fieldset class="form-section">
          <legend>Doklad</legend>
          <div class="form-grid">
            <FormField label="Číslo faktúry dodávateľa" :span="2" :error="fieldError('supplierNumber')">
              <InputText :model-value="p.supplierNumber" :disabled="locked" :invalid="!!fieldError('supplierNumber')" size="small" @update:model-value="set(p, 'supplierNumber')($event)" />
            </FormField>
            <FormField label="Variabilný symbol" hint="Prázdne = z čísla faktúry">
              <InputText :model-value="p.variableSymbol" :disabled="locked" inputmode="numeric" size="small" @update:model-value="set(p, 'variableSymbol')($event)" />
            </FormField>
            <FormField label="Spôsob úhrady">
              <Select :model-value="p.paymentMethod" :options="codes.paymentMethods" option-label="label" option-value="code" :disabled="locked" size="small" @update:model-value="set(p, 'paymentMethod')($event)" />
            </FormField>
            <FormField label="Dátum vyhotovenia" :error="fieldError('issueDate')">
              <DatePicker v-model="issueDate" :disabled="locked" date-format="d. m. yy" show-icon icon-display="input" size="small" />
            </FormField>
            <FormField label="Dátum dodania" hint="Rozhoduje o sadzbe DPH">
              <DatePicker v-model="deliveryDate" :disabled="locked" date-format="d. m. yy" show-icon icon-display="input" size="small" />
            </FormField>
            <FormField label="Dátum doručenia" :error="fieldError('receivedDate')" hint="Obdobie odpočtu DPH">
              <DatePicker v-model="receivedDate" :disabled="locked" date-format="d. m. yy" show-icon icon-display="input" size="small" :invalid="!!fieldError('receivedDate')" />
            </FormField>
            <FormField label="Splatnosť" :error="fieldError('dueDate')">
              <DatePicker v-model="dueDate" :disabled="locked" date-format="d. m. yy" show-icon icon-display="input" size="small" />
            </FormField>
            <FormField v-if="p.paymentMethod === 'transfer'" label="IBAN dodávateľa" :span="2" :error="fieldError('supplierIban')">
              <InputText :model-value="p.supplierIban" :disabled="locked" :invalid="!!fieldError('supplierIban')" size="small" @update:model-value="set(p, 'supplierIban')($event)" />
            </FormField>
            <FormField label="Predmet" :span="p.paymentMethod === 'transfer' ? 2 : 4">
              <InputText :model-value="p.description" :disabled="locked" placeholder="Napr. pohonné hmoty, nájom, materiál" size="small" @update:model-value="set(p, 'description')($event)" />
            </FormField>
            <FormField label="Mena" :error="fieldError('currency')">
              <Select :model-value="p.currency || 'EUR'" :options="currencies" option-label="label" option-value="code" filter :disabled="locked" size="small" aria-label="Mena" @update:model-value="setCurrency" />
            </FormField>
            <FormField v-if="foreign" :label="`Kurz: 1 EUR = … ${p.currency}`" :error="fieldError('fxRate')">
              <InputNumber
                :model-value="fxNumber"
                :min-fraction-digits="0"
                :max-fraction-digits="4"
                :use-grouping="false"
                locale="sk-SK"
                :disabled="locked"
                :invalid="!!fieldError('fxRate')"
                size="small"
                fluid
                aria-label="Kurz"
                @update:model-value="setRate"
              />
              <small v-if="!fieldError('fxRate')" class="fx-hint">
                {{ rateHint(p) }}
                <button v-if="p.fxManual && !locked" type="button" class="link" @click="resetRate">podľa ECB</button>
              </small>
            </FormField>
          </div>
        </fieldset>

        <fieldset class="form-section">
          <legend>Rekapitulácia DPH z dokladu</legend>
          <table class="rates">
            <thead>
              <tr><th>Sadzba</th><th class="num">Základ dane</th><th class="num">DPH</th><th /></tr>
            </thead>
            <tbody>
              <tr v-for="(r, i) in p.rates" :key="i">
                <td>
                  <Select :model-value="r.vatRate" :options="rateOptions" option-label="label" option-value="value" :disabled="locked" size="small" :invalid="!!fieldError(`rates[${i}].vatRate`)" @update:model-value="set(r, 'vatRate')($event)" />
                </td>
                <td>
                  <InputText :model-value="r.base" :disabled="locked" inputmode="decimal" class="num-input" size="small" :invalid="!!fieldError(`rates[${i}].base`) || (r.base !== '' && parseNumber(r.base) === null)" placeholder="0,00" @update:model-value="set(r, 'base')($event)" />
                </td>
                <td>
                  <InputText
                    :model-value="r.vat"
                    :disabled="locked || Number(r.vatRate) === 0"
                    inputmode="decimal"
                    class="num-input"
                    size="small"
                    :placeholder="computedVat[i] ? formatAmount(computedVat[i]) : 'vypočíta sa'"
                    @update:model-value="set(r, 'vat')($event)"
                  />
                </td>
                <td>
                  <button v-if="!locked" type="button" class="icon-btn" aria-label="Odstrániť riadok" @click="removeRow(i)"><i class="pi pi-times" /></button>
                </td>
              </tr>
            </tbody>
          </table>
          <p class="help">DPH opíšte z dokladu. Ak ju necháte prázdnu, vypočíta sa zo základu.</p>
          <ul v-if="Object.keys(previewErrors).some((k) => k.startsWith('rates')) || warnings.length" class="msgs">
            <li v-for="(m, k) in Object.fromEntries(Object.entries(previewErrors).filter(([f]) => f.startsWith('rates[') && f.endsWith('vatRate')))" :key="k" class="err">{{ m }}</li>
            <li v-for="w in warnings" :key="w.field" class="warn">{{ w.message }}</li>
          </ul>
          <Button v-if="!locked" label="Pridať sadzbu" icon="pi pi-plus" text size="small" @click="addRow" />
          <div class="checks">
            <label class="check">
              <Checkbox :model-value="p.reverseCharge" binary :disabled="locked" @update:model-value="set(p, 'reverseCharge')($event)" />
              <span>Prenesenie daňovej povinnosti (dodávateľ fakturuje bez DPH, daň priznávam ja)</span>
            </label>
            <label class="check">
              <Checkbox :model-value="p.noDeduction" binary :disabled="locked" @update:model-value="set(p, 'noDeduction')($event)" />
              <span>Neuplatňovať odpočet DPH</span>
            </label>
          </div>
          <small v-if="fieldError('supplier.icdph')" class="err">{{ fieldError('supplier.icdph') }}</small>
          <div v-if="isForeign(p.supplier.country)" class="supply">
            <span class="supply-label">Druh plnenia</span>
            <SelectButton
              :model-value="p.supplyKind || null"
              :options="supplyKinds"
              option-label="label"
              option-value="value"
              :disabled="locked"
              size="small"
              aria-label="Druh plnenia"
              @update:model-value="set(p, 'supplyKind')($event || '')"
            />
            <small v-if="fieldError('supplyKind')" class="err">{{ fieldError('supplyKind') }}</small>
            <small v-else class="supply-hint">{{ supplyHint('purchase', p.supplier.country, p.supplyKind) }}</small>
          </div>
        </fieldset>

        <fieldset class="form-section">
          <legend>Poznámka</legend>
          <Textarea :model-value="p.note" :disabled="locked" rows="2" auto-resize class="note" @update:model-value="set(p, 'note')($event)" />
        </fieldset>
      </div>

      <aside class="recap" aria-label="Súhrn dokladu">
        <h2>Súhrn</h2>
        <dl>
          <dt>Základ dane</dt><dd class="num">{{ formatAmount(summary.base) }}</dd>
          <dt>{{ p.reverseCharge ? 'DPH (samozdanenie)' : 'DPH' }}</dt><dd class="num">{{ formatAmount(summary.vat) }}</dd>
          <template v-if="!foreign">
            <dt>Odpočet DPH</dt><dd class="num">{{ p.noDeduction ? 'neuplatňuje sa' : formatAmount(summary.vat) }}</dd>
          </template>
        </dl>
        <div class="payable">
          <span>Na úhradu</span>
          <strong class="num">{{ formatMoney(summary.total, p.currency || 'EUR') }}</strong>
        </div>
        <div v-if="foreign" class="local">
          <p class="rate-line">{{ p.local ? rateText(p) : 'Bez kurzu sa doklad neprepočíta na eurá a nejde do priznania DPH.' }}</p>
          <dl v-if="p.local">
            <dt>Základ v EUR</dt><dd>{{ formatMoney(p.local.summary.base) }}</dd>
            <dt>DPH v EUR</dt><dd>{{ formatMoney(p.local.summary.vat) }}</dd>
            <dt>Odpočet DPH</dt><dd>{{ p.noDeduction ? 'neuplatňuje sa' : formatMoney(p.local.summary.vat) }}</dd>
            <dt>Spolu v EUR</dt><dd>{{ formatMoney(p.local.summary.total) }}</dd>
          </dl>
        </div>
        <dl v-if="p.number" class="meta">
          <dt>Interné číslo</dt><dd>{{ p.number }}</dd>
          <dt>Variabilný symbol</dt><dd>{{ p.variableSymbol }}</dd>
        </dl>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.ai-note {
  display: flex;
  gap: 10px;
  margin-bottom: 12px;
  padding: 8px 12px;
  background: var(--blue-wash);
  border-left: 3px solid var(--blue);
  font-size: var(--fs-sm);
}
.ai-note .pi {
  color: var(--blue);
  margin-top: 2px;
}
.ai-note ul {
  margin: 4px 0 0;
  padding-left: 18px;
  color: var(--tone-overdue);
}
.state {
  padding: 48px 16px;
  text-align: center;
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
.paid-on {
  font-size: var(--fs-sm);
  color: var(--tone-paid);
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
.form :deep(.p-select),
.form :deep(.p-datepicker) {
  width: 100%;
}
.rates {
  border-collapse: collapse;
  width: 100%;
  max-width: 560px;
}
.rates th {
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--ink-soft);
  text-align: left;
  padding: 0 8px 4px 0;
}
.rates th.num {
  text-align: right;
}
.rates td {
  padding: 3px 8px 3px 0;
}
.rates td:first-child {
  width: 110px;
}
.num-input {
  width: 100%;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.icon-btn {
  border: 0;
  background: none;
  color: var(--ink-faint);
  cursor: pointer;
  padding: 4px;
}
.icon-btn:hover {
  color: var(--tone-overdue);
}
.help {
  margin: 6px 0;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.msgs {
  margin: 4px 0;
  padding-left: 18px;
  font-size: var(--fs-sm);
}
.msgs .err,
.err {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.msgs .warn {
  color: var(--tone-draft);
}
.checks {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 10px;
}
.supply {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 10px;
  margin-top: 8px;
}
.supply-label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.supply-hint,
.supply .err {
  flex-basis: 100%;
  font-size: var(--fs-sm);
}
.supply-hint {
  color: var(--ink-faint);
}
.check {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
.note {
  width: 100%;
}
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
.recap dl {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 4px 10px;
  margin: 0;
}
.recap dt {
  color: var(--ink-soft);
}
.recap dd {
  margin: 0;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.payable {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-top: 12px;
  padding-top: 8px;
  border-top: 1px solid var(--ink);
}
.payable span {
  color: var(--ink-soft);
}
.payable strong {
  font-size: 1.625rem;
  font-weight: 700;
  border-bottom: 3px double var(--ink);
  line-height: 1.15;
}
.local {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px dashed var(--rule);
  font-size: var(--fs-sm);
}
.local .rate-line {
  margin: 0 0 4px;
  color: var(--ink-soft);
}
.fx-hint {
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.link {
  font: inherit;
  border: 0;
  background: none;
  padding: 0;
  color: var(--blue);
  cursor: pointer;
}
.meta {
  margin-top: 14px !important;
  padding-top: 10px;
  border-top: 1px solid var(--rule);
  font-size: var(--fs-sm);
}
.locked :deep(.p-inputtext:disabled),
.locked :deep(.p-textarea:disabled) {
  background: transparent;
  border-color: transparent;
  color: var(--ink);
  opacity: 1;
  padding-left: 0;
}
.locked :deep(.p-select.p-disabled) {
  background: transparent;
  border-color: transparent;
  opacity: 1;
}
.locked :deep(.p-select-dropdown),
.locked :deep(.p-datepicker-input-icon-container) {
  display: none;
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
}
</style>
