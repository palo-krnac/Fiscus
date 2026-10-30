<script setup>
// Účtový rozvrh (podvojné účtovníctvo): syntetické účty z rámcovej účtovej
// osnovy, analytické účty firmy, strediská a zákazky.
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import Drawer from 'primevue/drawer'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatDate, fromISODate, toISODate } from '../lib/format.js'
import { matches, searchTerms } from '../lib/search.js'
import { accountColumns, centerColumns, jobColumns } from '../lib/columns.js'
import { accountDisplay, blankAccount, groupAccounts, typeLabel } from '../lib/chart.js'

const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))

const v = ref(null)
const tab = ref('accounts')
const tabs = [
  { value: 'accounts', label: 'Účty' },
  { value: 'centers', label: 'Strediská' },
  { value: 'jobs', label: 'Zákazky' },
]
const search = ref('')
const cls = ref(-1)
const showHidden = ref(false)

async function load() {
  try {
    v.value = await api.chart(session.companyId)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Účtový rozvrh sa nenačítal', detail: e.message, life: 6000 })
  }
}
onMounted(load)
watch(() => session.companyId, load)
watch(tab, () => (search.value = ''))

const classOptions = computed(() => [{ value: -1, label: 'Všetky triedy' }, ...(v.value?.classes ?? []).map((n, i) => ({ value: i, label: `${i} – ${n}` }))])
const groups = computed(() => (v.value ? groupAccounts(v.value.accounts, v.value.classes, { search: search.value, cls: cls.value < 0 ? null : cls.value, hidden: showHidden.value }) : []))
const accountRows = computed(() => groups.value.flatMap((g) => g.accounts))
const framework = computed(() => new Set(v.value?.framework ?? []))
const centers = computed(() => {
  const terms = searchTerms(search.value)
  return (v.value?.centers ?? []).filter((c) => matches(terms, [c.code, c.name, c.note]))
})
const jobs = computed(() => {
  const terms = searchTerms(search.value)
  return (v.value?.jobs ?? []).filter((j) => matches(terms, [j.code, j.name, j.customer, j.note]))
})

// ---- formulár ----
const open = ref(false)
const kind = ref('account')
const form = ref(null)
const old = ref('')
const errors = ref({})
const busy = ref(false)
const title = computed(() => {
  const n = { account: 'účet', center: 'stredisko', job: 'zákazka' }[kind.value]
  return old.value ? `Upraviť ${n}` : kind.value === 'job' ? 'Nová zákazka' : kind.value === 'center' ? 'Nové stredisko' : 'Nový analytický účet'
})
const typeOptions = computed(() => (v.value?.types ?? []).map((t) => ({ value: t.code, label: t.label })))
const isSynthetic = computed(() => kind.value === 'account' && old.value && old.value.length === 3)

function edit(k, item, syntheticFor = '') {
  kind.value = k
  errors.value = {}
  old.value = item ? (k === 'account' ? item.number : item.code) : ''
  if (k === 'account') form.value = item ? { ...item, number: accountDisplay(item.number) } : { ...blankAccount(syntheticFor ? syntheticFor + '.' : ''), type: v.value.accounts.find((a) => a.number === syntheticFor)?.type ?? 'A' }
  else if (k === 'center') form.value = item ? { ...item } : { code: '', name: '', active: true, note: '' }
  else form.value = item ? { ...item, from: fromISODate(item.from), to: fromISODate(item.to) } : { code: '', name: '', customer: '', active: true, note: '', from: null, to: null }
  open.value = true
}

async function save() {
  busy.value = true
  errors.value = {}
  try {
    const cid = session.companyId
    if (kind.value === 'account') await api.saveAccount(cid, old.value, form.value)
    else if (kind.value === 'center') await api.saveCenter(cid, old.value, form.value)
    else await api.saveJob(cid, old.value, { ...form.value, from: toISODate(form.value.from), to: toISODate(form.value.to) })
    open.value = false
    await load()
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) errors.value = e.fieldMap()
    else toast.add({ severity: 'error', summary: 'Neuložilo sa', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

function remove() {
  const k = kind.value
  const label = k === 'account' ? `účet ${accountDisplay(old.value)}` : k === 'center' ? `stredisko ${old.value}` : `zákazku ${old.value}`
  const hideOnly = k === 'account' && framework.value.has(old.value)
  confirm.require({
    header: hideOnly ? 'Skryť účet' : 'Zmazať',
    message: hideOnly ? `Účet ${old.value} je v rámcovej osnove — skryje sa a nebude sa ponúkať pri účtovaní.` : `Zmazať ${label}?`,
    acceptLabel: hideOnly ? 'Skryť' : 'Zmazať',
    rejectLabel: 'Späť',
    acceptProps: { severity: 'danger', size: 'small' },
    rejectProps: { severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      try {
        const cid = session.companyId
        if (k === 'account') await api.deleteAccount(cid, old.value)
        else if (k === 'center') await api.deleteCenter(cid, old.value)
        else await api.deleteJob(cid, old.value)
        open.value = false
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
      }
    },
  })
}

function addNew() {
  if (tab.value === 'accounts') edit('account', null)
  else edit(tab.value === 'centers' ? 'center' : 'job', null)
}
const addLabel = computed(() => ({ accounts: 'Analytický účet', centers: 'Stredisko', jobs: 'Zákazka' })[tab.value])
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Účtový rozvrh</h1>
      <SelectButton v-model="tab" :options="tabs" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Číselník" />
      <div class="spacer" />
      <Button v-if="canWrite" :label="addLabel" icon="pi pi-plus" size="small" @click="addNew" />
    </header>
    <p class="lead">
      Predvyplnený rámcovou účtovou osnovou pre podnikateľov (opatrenie MF SR č. 23054/2002-92). Pridajte analytické účty
      (napr. 221.001 pre každý bankový účet), skryte nepoužívané a označte nedaňové náklady.
    </p>

    <div class="toolbar">
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" :placeholder="tab === 'accounts' ? 'Hľadať účet alebo názov' : 'Hľadať'" size="small" aria-label="Hľadať" />
      </IconField>
      <template v-if="tab === 'accounts'">
        <Select v-model="cls" :options="classOptions" option-label="label" option-value="value" size="small" class="cls" aria-label="Účtová trieda" />
        <label class="check"><Checkbox v-model="showHidden" binary /> <span>aj skryté</span></label>
      </template>
      <div class="spacer" />
      <ExportButton v-if="tab === 'accounts'" name="Účtový rozvrh" :columns="accountColumns" :rows="accountRows" />
      <ExportButton v-else-if="tab === 'centers'" name="Strediská" :columns="centerColumns" :rows="centers" />
      <ExportButton v-else name="Zákazky" :columns="jobColumns" :rows="jobs" />
    </div>

    <template v-if="v">
      <section v-if="tab === 'accounts'" class="sheet">
        <p v-if="!groups.length" class="empty">Nič sa nenašlo.</p>
        <table v-for="g in groups" :key="g.cls" class="list">
          <caption><span class="cls-no">{{ g.cls }}</span> {{ g.name }}</caption>
          <tbody>
            <tr v-for="a in g.accounts" :key="a.number" :class="{ analytic: a.number.length > 3, hidden: !a.active }" tabindex="0"
              @click="edit('account', a)" @keydown.enter="edit('account', a)">
              <td class="num-acc">{{ accountDisplay(a.number) }}</td>
              <td class="name">
                {{ a.name }}
                <span v-if="a.nonTax" class="tag">nedaňový</span>
                <span v-if="!a.active" class="tag muted">skrytý</span>
              </td>
              <td class="type">{{ typeLabel(a.type) }}</td>
              <td class="act">
                <Button v-if="canWrite && a.number.length === 3" icon="pi pi-plus" text rounded size="small" :aria-label="`Analytický účet k ${a.number}`"
                  v-tooltip.left="'Pridať analytický účet'" @click.stop="edit('account', null, a.number)" />
              </td>
            </tr>
          </tbody>
        </table>
      </section>

      <section v-else-if="tab === 'centers'" class="sheet">
        <table class="list">
          <thead><tr><th>Kód</th><th>Stredisko</th><th>Poznámka</th></tr></thead>
          <tbody>
            <tr v-if="!centers.length"><td colspan="3" class="empty">{{ v.centers.length ? 'Nič sa nenašlo.' : 'Zatiaľ žiadne strediská — slúžia na sledovanie nákladov a výnosov po prevádzkach.' }}</td></tr>
            <tr v-for="c in centers" :key="c.code" :class="{ hidden: !c.active }" tabindex="0" @click="edit('center', c)" @keydown.enter="edit('center', c)">
              <td class="num-acc">{{ c.code }}</td>
              <td class="name">{{ c.name }} <span v-if="!c.active" class="tag muted">neaktívne</span></td>
              <td class="note">{{ c.note }}</td>
            </tr>
          </tbody>
        </table>
      </section>

      <section v-else class="sheet">
        <table class="list">
          <thead><tr><th>Kód</th><th>Zákazka</th><th>Odberateľ</th><th>Obdobie</th></tr></thead>
          <tbody>
            <tr v-if="!jobs.length"><td colspan="4" class="empty">{{ v.jobs.length ? 'Nič sa nenašlo.' : 'Zatiaľ žiadne zákazky — slúžia na sledovanie nákladov a výnosov po projektoch.' }}</td></tr>
            <tr v-for="j in jobs" :key="j.code" :class="{ hidden: !j.active }" tabindex="0" @click="edit('job', j)" @keydown.enter="edit('job', j)">
              <td class="num-acc">{{ j.code }}</td>
              <td class="name">{{ j.name }} <span v-if="!j.active" class="tag muted">ukončená</span></td>
              <td>{{ j.customer }}</td>
              <td class="period">{{ j.from ? formatDate(j.from) : '' }}<template v-if="j.from || j.to"> – </template>{{ j.to ? formatDate(j.to) : '' }}</td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>

    <Drawer v-model:visible="open" position="right" class="chart-panel" :header="title">
      <form v-if="form" class="form-grid chart-form" @submit.prevent="save">
        <template v-if="kind === 'account'">
          <FormField label="Číslo účtu" :error="errors.number" :hint="isSynthetic ? 'Syntetický účet osnovy' : 'Napr. 221.001'">
            <InputText v-model="form.number" size="small" inputmode="numeric" :disabled="!canWrite || isSynthetic" :invalid="!!errors.number" />
          </FormField>
          <FormField label="Druh" :error="errors.type" :hint="form.number.replace(/\D/g, '').length > 3 ? 'Podľa syntetického účtu' : ''">
            <Select v-model="form.type" :options="typeOptions" option-label="label" option-value="value" size="small" fluid
              :disabled="!canWrite || form.number.replace(/\D/g, '').length > 3" />
          </FormField>
          <FormField label="Názov" :span="2" :error="errors.name">
            <InputText v-model="form.name" size="small" :disabled="!canWrite" :invalid="!!errors.name" />
          </FormField>
          <label v-if="['5', '6'].includes(form.number.trim()[0])" class="check span2">
            <Checkbox v-model="form.nonTax" binary :disabled="!canWrite" />
            <span>Nedaňový {{ form.number.trim()[0] === '5' ? 'náklad' : 'výnos' }} (pre daňové priznanie)</span>
          </label>
          <label class="check span2">
            <Checkbox v-model="form.active" binary :disabled="!canWrite" />
            <span>Používa sa (ponúka sa pri účtovaní)</span>
          </label>
        </template>
        <template v-else>
          <FormField label="Kód" :error="errors.code" hint="Bez medzier, napr. BB alebo Z-2026-01">
            <InputText v-model="form.code" size="small" :disabled="!canWrite" :invalid="!!errors.code" />
          </FormField>
          <FormField :label="kind === 'center' ? 'Stredisko' : 'Zákazka'" :error="errors.name">
            <InputText v-model="form.name" size="small" :disabled="!canWrite" :invalid="!!errors.name" />
          </FormField>
          <template v-if="kind === 'job'">
            <FormField label="Odberateľ" :span="2">
              <InputText v-model="form.customer" size="small" :disabled="!canWrite" />
            </FormField>
            <FormField label="Od">
              <DatePicker v-model="form.from" date-format="d. m. yy" size="small" fluid show-clear :disabled="!canWrite" />
            </FormField>
            <FormField label="Do" :error="errors.to">
              <DatePicker v-model="form.to" date-format="d. m. yy" size="small" fluid show-clear :disabled="!canWrite" />
            </FormField>
          </template>
          <label class="check span2">
            <Checkbox v-model="form.active" binary :disabled="!canWrite" />
            <span>{{ kind === 'center' ? 'Aktívne' : 'Aktívna (ponúka sa pri účtovaní)' }}</span>
          </label>
        </template>
        <FormField label="Poznámka" :span="2">
          <Textarea v-model="form.note" rows="2" auto-resize :disabled="!canWrite" />
        </FormField>
        <div v-if="canWrite" class="actions span2">
          <Button v-if="old" :label="kind === 'account' && framework.has(old) ? 'Skryť' : 'Zmazať'" severity="danger" text size="small" @click="remove" />
          <span class="spacer" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Drawer>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  max-width: 980px;
  display: flex;
  flex-direction: column;
  gap: 10px;
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
.lead {
  margin: 0;
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.search {
  width: 260px;
}
.search :deep(input) {
  width: 100%;
}
.cls {
  width: 260px;
}
.check {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.sheet {
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 2px solid var(--ink);
}
.list {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.list caption {
  text-align: left;
  font-weight: 600;
  padding: 8px 10px 4px;
  background: var(--paper);
  border-bottom: 1px solid var(--rule);
}
.cls-no {
  display: inline-block;
  min-width: 18px;
  color: var(--ink-faint);
}
.list th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 5px 10px;
  border-bottom: 1px solid var(--rule);
}
.list td {
  padding: 3px 10px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: middle;
}
.list tbody tr {
  cursor: pointer;
}
.list tbody tr:hover,
.list tbody tr:focus-visible {
  background: var(--blue-wash);
  outline: none;
}
.num-acc {
  width: 90px;
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  white-space: nowrap;
}
tr.analytic .num-acc {
  padding-left: 24px;
  font-weight: 400;
}
tr.analytic .name {
  color: var(--ink-soft);
}
tr.hidden td {
  color: var(--ink-faint);
}
.type {
  width: 130px;
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.act {
  width: 40px;
  text-align: right;
  padding-top: 0;
  padding-bottom: 0;
}
.act :deep(.p-button) {
  width: 26px;
  height: 26px;
}
.tag {
  font-size: var(--fs-xs);
  border: 1px solid var(--tone-draft);
  color: var(--tone-draft);
  padding: 0 4px;
  margin-left: 4px;
  border-radius: 2px;
}
.tag.muted {
  border-color: var(--rule);
  color: var(--ink-faint);
}
.note,
.period {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.empty {
  color: var(--ink-faint);
  padding: 10px;
}
.chart-form {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.span2 {
  grid-column: 1 / -1;
}
.actions {
  display: flex;
  align-items: center;
  margin-top: 6px;
}
@media (max-width: 640px) {
  .page {
    padding: 10px 10px 20px;
  }
  .search,
  .cls {
    flex: 1 1 100%;
    width: auto;
  }
  .type {
    display: none;
  }
}
</style>

<style>
.p-drawer.p-component.chart-panel {
  width: min(460px, 100vw);
}
</style>
