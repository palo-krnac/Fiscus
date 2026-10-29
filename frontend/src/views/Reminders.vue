<script setup>
// Upomienky: faktúry po splatnosti podľa odberateľov a vytvorené upomienky.
// Úroky a súčty počíta backend (náhľad), rozhranie ich iba zobrazuje.
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Textarea from 'primevue/textarea'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { api, ApiError } from '../lib/api.js'
import { can, isSuperAdmin, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { matches, searchTerms } from '../lib/search.js'
import { overdueColumns, reminderColumns } from '../lib/columns.js'
import { interestModes, levelShort } from '../lib/reminders.js'
import ExportButton from '../components/ExportButton.vue'
import FormField from '../components/FormField.vue'
import SendMailDialog from '../components/SendMailDialog.vue'

const router = useRouter()
const mailId = ref(null)
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('invoices.write'))

const tab = ref('overdue')
const search = ref('')
const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
const year = ref(thisYear)

const groups = ref([])
const ecbUpdated = ref('')
const flatFee = ref('40.00')
const margins = ref({ business: '9', consumer: '5' })
const reminders = ref([])
const selected = ref({}) // id faktúry → true
const loading = ref(true)
const loadError = ref('')

async function load() {
  loading.value = true
  try {
    const [ov, list] = await Promise.all([api.overdue(session.companyId), api.reminders(session.companyId, year.value)])
    groups.value = ov.groups
    ecbUpdated.value = ov.ecbUpdated
    flatFee.value = ov.flatFee
    margins.value = { business: ov.businessMargin, consumer: ov.consumerMargin }
    reminders.value = list
    selected.value = Object.fromEntries(ov.groups.flatMap((g) => g.invoices.map((i) => [i.id, true])))
    loadError.value = ''
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(year, async () => {
  reminders.value = await api.reminders(session.companyId, year.value).catch(() => [])
})

const visibleGroups = computed(() => {
  const terms = searchTerms(search.value)
  return groups.value
    .map((g) => ({
      ...g,
      invoices: g.invoices.filter((i) => matches(terms, [g.customer.name, g.customer.ico, g.customer.city, i.number, i.total])),
    }))
    .filter((g) => g.invoices.length)
})
const overdueRows = () => visibleGroups.value.flatMap((g) => g.invoices.map((i) => ({ ...i, customer: g.customer })))
const overdueTotal = computed(() => groups.value.reduce((a, g) => a + Number(g.total), 0))
const overdueCount = computed(() => groups.value.reduce((a, g) => a + g.invoices.length, 0))

const visibleReminders = computed(() => {
  const terms = searchTerms(search.value)
  return reminders.value.filter((r) =>
    matches(terms, [r.number, r.customer?.name, r.customer?.ico, levelShort(r.level), r.total, ...r.items.map((i) => i.number)]),
  )
})

const pickedIn = (g) => g.invoices.filter((i) => selected.value[i.id])
function toggleGroup(g, on) {
  for (const i of g.invoices) selected.value[i.id] = on
}

// ---- dialóg upomienky ----
const dlg = ref(null) // { group, form, preview, errors }
const today = () => toISODate(new Date())
const plusDays = (iso, n) => {
  const d = fromISODate(iso)
  d.setDate(d.getDate() + n)
  return toISODate(d)
}

function openReminder(g) {
  const ids = pickedIn(g).map((i) => i.id)
  if (!ids.length) {
    toast.add({ severity: 'warn', summary: 'Vyberte aspoň jednu faktúru', life: 3000 })
    return
  }
  const date = today()
  dlg.value = {
    group: g,
    form: { invoiceIds: ids, date, payBy: plusDays(date, 7), interestMode: 'statutory', customRate: '', chargeFee: false, note: '' },
    preview: null,
    errors: {},
    error: '',
  }
  refresh()
}

let seq = 0
async function refresh() {
  const mine = ++seq
  const d = dlg.value
  if (!d) return
  try {
    const p = await api.previewReminder(session.companyId, payload(d.form))
    if (mine !== seq || !dlg.value) return
    d.preview = p
    d.errors = {}
    d.error = ''
  } catch (e) {
    if (mine !== seq || !dlg.value) return
    d.preview = null
    d.errors = e instanceof ApiError ? e.fieldMap() : {}
    d.error = e instanceof ApiError && e.fields.length ? '' : e.message
  }
}
const payload = (f) => ({ ...f, customRate: f.interestMode === 'custom' ? String(f.customRate || 0) : '0' })
const setF = (k, v) => {
  dlg.value.form[k] = v
  refresh()
}
const dateF = (k) =>
  computed({
    get: () => fromISODate(dlg.value?.form[k]),
    set: (d) => setF(k, d ? toISODate(d) : ''),
  })
const dlgDate = dateF('date')
const dlgPayBy = dateF('payBy')

const creating = ref(false)
async function create() {
  creating.value = true
  try {
    const r = await api.createReminder(session.companyId, payload(dlg.value.form))
    dlg.value = null
    toast.add({ severity: 'success', summary: `Upomienka ${r.number} vytvorená`, life: 3000 })
    router.push(`/upomienky/${r.id}/tlac`)
  } catch (e) {
    dlg.value.errors = e instanceof ApiError ? e.fieldMap() : {}
    toast.add({ severity: 'error', summary: 'Upomienku nemožno vytvoriť', detail: e.message, life: 6000 })
  } finally {
    creating.value = false
  }
}

function remove(r) {
  confirm.require({
    header: 'Zmazať upomienku',
    message: `Zmazať upomienku ${r.number}? Použite iba pri upomienke vytvorenej omylom — záznam ostane v audite.`,
    acceptProps: { label: 'Zmazať', severity: 'danger', size: 'small' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.deleteReminder(session.companyId, r.id)
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 5000 })
      }
    },
  })
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Upomienky</h1>
      <div class="spacer" />
      <SelectButton
        v-model="tab"
        :options="[
          { label: `Po splatnosti (${overdueCount})`, value: 'overdue' },
          { label: `Vytvorené (${reminders.length})`, value: 'sent' },
        ]"
        option-label="label"
        option-value="value"
        :allow-empty="false"
        size="small"
        aria-label="Zobraziť"
      />
    </header>

    <section class="ledger" aria-label="Prehľad">
      <div class="figure">
        <span class="label">Po splatnosti ({{ overdueCount }} faktúr)</span>
        <span class="value alert">{{ formatMoney(overdueTotal) }}</span>
      </div>
      <div class="figure">
        <span class="label">Odberatelia s dlhom</span>
        <span class="value">{{ groups.length }}</span>
      </div>
      <div class="figure">
        <span class="label">Upomienky v {{ year }}</span>
        <span class="value">{{ reminders.length }}</span>
      </div>
    </section>

    <div class="toolbar">
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať odberateľa, IČO, číslo faktúry" size="small" aria-label="Hľadať" />
      </IconField>
      <Select v-if="tab === 'sent'" v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <ExportButton v-if="tab === 'overdue'" name="Faktúry po splatnosti" :columns="overdueColumns" :rows="overdueRows" />
      <ExportButton v-else :name="`Upomienky ${year}`" :columns="reminderColumns" :rows="visibleReminders" />
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <!-- faktúry po splatnosti podľa odberateľov -->
    <template v-if="tab === 'overdue' && !loading">
      <p v-if="!visibleGroups.length" class="empty">{{ search ? 'Nič sa nenašlo.' : 'Žiadne faktúry po splatnosti.' }}</p>
      <section v-for="g in visibleGroups" :key="g.key" class="group">
        <header class="g-head">
          <Checkbox
            :model-value="pickedIn(g).length === g.invoices.length"
            :indeterminate="pickedIn(g).length > 0 && pickedIn(g).length < g.invoices.length"
            binary
            :aria-label="`Vybrať všetky faktúry ${g.customer.name}`"
            @update:model-value="toggleGroup(g, $event)"
          />
          <div class="who">
            <strong>{{ g.customer.name }}</strong>
            <span>{{ [g.customer.ico && `IČO ${g.customer.ico}`, g.customer.city, g.business ? null : 'spotrebiteľ'].filter(Boolean).join(' · ') }}</span>
          </div>
          <span class="days">až {{ g.maxDays }} dní</span>
          <strong class="sum">{{ formatMoney(g.total) }}</strong>
          <Button
            v-if="canWrite"
            :label="g.nextLevel > 1 ? `${levelShort(g.nextLevel)}…` : 'Upomienka…'"
            icon="pi pi-bell"
            size="small"
            :severity="g.nextLevel === 3 ? 'danger' : undefined"
            :outlined="g.nextLevel === 1"
            @click="openReminder(g)"
          />
        </header>
        <table class="inv">
          <tbody>
            <tr v-for="i in g.invoices" :key="i.id">
              <td class="c"><Checkbox v-model="selected[i.id]" binary :aria-label="`Vybrať ${i.number}`" /></td>
              <td><RouterLink :to="`/faktury/${i.id}`">{{ i.number }}</RouterLink></td>
              <td class="hide-m">splatná {{ formatDate(i.dueDate) }}</td>
              <td class="late">{{ i.days }} dní</td>
              <td class="num">{{ formatAmount(i.total) }}</td>
              <td class="rem">
                <span v-if="i.reminded" class="tag" data-tone="draft"><span class="tag-text"><span class="hide-m">{{ levelShort(i.reminded) }} {{ formatDate(i.lastReminded) }}</span><span class="only-m">{{ i.reminded }}. up.</span></span></span>
              </td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>

    <!-- vytvorené upomienky -->
    <table v-if="tab === 'sent' && !loading" class="list">
      <thead>
        <tr>
          <th>Číslo</th><th>Dátum</th><th>Stupeň</th><th>Odberateľ</th><th class="hide-m">Faktúry</th>
          <th class="num hide-m">Úrok</th><th class="num">Spolu</th><th aria-label="Akcie" />
        </tr>
      </thead>
      <tbody>
        <tr v-if="!visibleReminders.length"><td colspan="8" class="empty">Žiadne upomienky.</td></tr>
        <tr v-for="r in visibleReminders" :key="r.id">
          <td class="number">{{ r.number }}</td>
          <td>{{ formatDate(r.date) }}</td>
          <td><span class="tag" :data-tone="r.level === 3 ? 'overdue' : 'draft'"><span class="tag-text">{{ levelShort(r.level) }}</span></span></td>
          <td>{{ r.customer.name }}</td>
          <td class="hide-m">{{ r.items.map((i) => i.number).join(', ') }}</td>
          <td class="num hide-m">{{ formatAmount(r.interestTotal) }}</td>
          <td class="num strong">{{ formatAmount(r.total) }}</td>
          <td class="act">
            <Button v-if="canWrite" icon="pi pi-envelope" text size="small" aria-label="Poslať e-mailom" v-tooltip.left="'Poslať e-mailom'" @click="mailId = r.id" />
            <Button icon="pi pi-print" text size="small" aria-label="Tlač" v-tooltip.left="'Tlač / PDF'" @click="router.push(`/upomienky/${r.id}/tlac`)" />
            <Button v-if="canWrite" icon="pi pi-trash" text severity="danger" size="small" aria-label="Zmazať" @click="remove(r)" />
          </td>
        </tr>
      </tbody>
    </table>

    <Dialog
      :visible="!!dlg"
      modal
      :header="dlg ? `Upomienka – ${dlg.group.customer.name}` : ''"
      :style="{ width: '46rem' }"
      :breakpoints="{ '760px': '96vw' }"
      @update:visible="(v) => !v && (dlg = null)"
    >
      <template v-if="dlg">
        <div class="form-grid">
          <FormField label="Dátum upomienky" :error="dlg.errors.date">
            <DatePicker v-model="dlgDate" date-format="d. m. yy" show-icon icon-display="input" size="small" fluid />
          </FormField>
          <FormField label="Uhradiť do" :error="dlg.errors.payBy">
            <DatePicker v-model="dlgPayBy" date-format="d. m. yy" show-icon icon-display="input" size="small" fluid />
          </FormField>
          <FormField label="Úrok z omeškania" :span="2" :error="dlg.errors.interestMode">
            <SelectButton :model-value="dlg.form.interestMode" :options="interestModes" option-label="label" option-value="value"
              :allow-empty="false" size="small" @update:model-value="setF('interestMode', $event)" />
          </FormField>
          <FormField v-if="dlg.form.interestMode === 'custom'" label="Sadzba % ročne" :error="dlg.errors.customRate">
            <InputNumber :model-value="dlg.form.customRate === '' ? null : Number(dlg.form.customRate)" :min-fraction-digits="0" :max-fraction-digits="2"
              locale="sk-SK" size="small" fluid @update:model-value="setF('customRate', $event ?? '')" />
          </FormField>
          <label class="fee" :class="{ off: !dlg.group.business }">
            <Checkbox :model-value="dlg.form.chargeFee && dlg.group.business" binary :disabled="!dlg.group.business" input-id="fee"
              @update:model-value="setF('chargeFee', $event)" />
            <span>
              Paušálna náhrada {{ formatAmount(flatFee) }} € za faktúru
              <small>{{ dlg.group.business ? '§ 369c Obchodného zákonníka' : 'iba voči podnikateľovi' }}</small>
            </span>
          </label>
          <FormField label="Poznámka na upomienke" :span="4">
            <Textarea :model-value="dlg.form.note" rows="2" auto-resize fluid @update:model-value="setF('note', $event)" />
          </FormField>
        </div>

        <p v-if="dlg.error" class="error">{{ dlg.error }}</p>
        <div v-if="dlg.preview" class="preview">
          <h3>{{ dlg.preview.level }}. upomienka</h3>
          <table class="pv">
            <thead>
              <tr><th>Faktúra</th><th class="num">Dní</th><th class="num hide-m">Sadzba</th><th class="num">Suma</th><th class="num">Úrok</th><th class="num" v-if="Number(dlg.preview.fees) > 0">Náhrada</th></tr>
            </thead>
            <tbody>
              <tr v-for="it in dlg.preview.items" :key="it.invoiceId">
                <td>{{ it.number }}</td>
                <td class="num">{{ it.days }}</td>
                <td class="num hide-m">{{ Number(it.rate) ? `${formatAmount(it.rate)} %` : '—' }}</td>
                <td class="num">{{ formatAmount(it.amount) }}</td>
                <td class="num">{{ formatAmount(it.interest) }}</td>
                <td class="num" v-if="Number(dlg.preview.fees) > 0">{{ formatAmount(it.fee) }}</td>
              </tr>
            </tbody>
          </table>
          <div class="pv-total">
            <span>Spolu na úhradu</span>
            <strong>{{ formatMoney(dlg.preview.total) }}</strong>
          </div>
          <p class="muted">
            Zákonná sadzba = sadzba ECB k prvému dňu omeškania + {{ String(Number(dlg.group.business ? margins.business : margins.consumer)).replace('.', ',') }} p. b.
            Posledná sadzba ECB v Legislatíve je od {{ formatDate(ecbUpdated) }}.
            <RouterLink v-if="isSuperAdmin()" to="/legislativa">Upraviť</RouterLink>
          </p>
        </div>
      </template>
      <template #footer>
        <Button label="Zrušiť" severity="secondary" text size="small" @click="dlg = null" />
        <Button label="Vytvoriť a tlačiť" icon="pi pi-print" size="small" :loading="creating" :disabled="!dlg?.preview" @click="create" />
      </template>
    </Dialog>
    <SendMailDialog :visible="!!mailId" kind="reminders" :id="mailId" @update:visible="(v) => !v && (mailId = null)" />
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-width: 1100px;
}
.head {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
  margin: 0;
}
.spacer {
  flex: 1;
}
.ledger {
  display: flex;
  border-top: 2px solid var(--ink);
  border-bottom: 1px solid var(--rule);
  background: var(--sheet);
  overflow-x: auto;
}
.figure {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 8px 20px 9px 14px;
  border-right: 1px solid var(--rule);
  flex: none;
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
.figure .value.alert {
  color: var(--tone-overdue);
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.search {
  width: 300px;
}
.search :deep(input) {
  width: 100%;
}
.error {
  color: var(--tone-overdue);
  margin: 0;
}
.empty {
  color: var(--ink-faint);
  margin: 8px 0;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  margin: 6px 0 0;
}
.group {
  background: var(--sheet);
  border: 1px solid var(--rule);
}
.g-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 7px 10px;
  border-bottom: 1px solid var(--rule);
  background: var(--paper);
}
.who {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  line-height: 1.25;
}
.who span {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.days {
  font-size: var(--fs-sm);
  color: var(--tone-overdue);
  white-space: nowrap;
}
.sum {
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.inv,
.list,
.pv {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.inv td {
  padding: 3px 10px;
  border-bottom: 1px solid var(--rule-soft);
  font-variant-numeric: tabular-nums;
}
.inv tr:last-child td {
  border-bottom: 0;
}
.inv {
  table-layout: fixed;
}
.inv .c {
  width: 34px;
}
.inv td:nth-child(2) {
  width: 130px;
}
.inv td:nth-child(3) {
  width: 170px;
}
.inv td:nth-child(4) {
  width: 90px;
}
.inv td:nth-child(5) {
  width: 120px;
}
.inv a,
.list a {
  color: var(--blue-deep);
  font-weight: 600;
}
.late {
  color: var(--tone-overdue);
  white-space: nowrap;
}
.num {
  text-align: right;
  white-space: nowrap;
}
.rem {
  text-align: right;
  width: 190px;
  white-space: nowrap;
}
.only-m {
  display: none;
}
.list {
  background: var(--sheet);
  border: 1px solid var(--rule);
}
.list th,
.pv th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 5px 10px;
  border-bottom: 1px solid var(--rule);
}
.list th.num,
.pv th.num {
  text-align: right;
}
.list td,
.pv td {
  padding: 3px 10px;
  border-bottom: 1px solid var(--rule-soft);
  font-variant-numeric: tabular-nums;
}
.list .number {
  font-weight: 600;
  color: var(--blue-deep);
}
.strong {
  font-weight: 600;
}
.act {
  text-align: right;
  white-space: nowrap;
}
.fee {
  grid-column: span 2;
  display: flex;
  align-items: center;
  gap: 8px;
  align-self: end;
  padding-bottom: 4px;
}
.fee span {
  display: flex;
  flex-direction: column;
  line-height: 1.25;
}
.fee small {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.fee.off {
  opacity: 0.6;
}
.preview {
  margin-top: 14px;
  border-top: 1px solid var(--rule);
  padding-top: 10px;
}
.preview h3 {
  margin: 0 0 6px;
  font-size: var(--fs-md);
}
.pv-total {
  display: flex;
  justify-content: flex-end;
  align-items: baseline;
  gap: 16px;
  margin-top: 10px;
}
.pv-total span {
  color: var(--ink-soft);
}
.pv-total strong {
  font-size: 1.4rem;
  font-variant-numeric: tabular-nums;
  border-bottom: 3px double var(--ink);
  line-height: 1.15;
}
@media (max-width: 640px) {
  .page {
    padding: 10px 10px 20px;
  }
  .search {
    width: auto;
    flex: 1;
  }
  .hide-m {
    display: none;
  }
  .only-m {
    display: inline;
  }
  .g-head {
    flex-wrap: wrap;
  }
  .who {
    flex-basis: calc(100% - 40px);
  }
  .days {
    margin-left: 34px;
  }
  .sum {
    margin-left: auto;
  }
  .g-head :deep(.p-button) {
    width: 100%;
  }
  .rem {
    width: auto;
  }
  .inv td:nth-child(2) {
    width: 104px;
  }
  .inv td:nth-child(4) {
    width: 62px;
  }
  .inv td:nth-child(5) {
    width: 82px;
  }
  .rem .tag {
    font-size: var(--fs-xs);
  }
  .fee {
    grid-column: span 2;
  }
}
</style>
