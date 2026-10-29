<script setup>
// E-maily: zoznam odoslaných správ (hľadanie, export) a nastavenie servera SMTP firmy.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Password from 'primevue/password'
import Textarea from 'primevue/textarea'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { emailColumns } from '../lib/columns.js'
import {
  docKindLabel, docRoute, emailStatus, emailViews, filterEmails, guessProvider, placeholders, portFor, securityOptions, templateKinds,
} from '../lib/emails.js'
import ExportButton from '../components/ExportButton.vue'
import FormField from '../components/FormField.vue'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const canSettings = computed(() => can('settings.write'))
const tab = ref(route.query.nastavenie && can('settings.write') ? 'settings' : 'list')
const tabs = [
  { label: 'Odoslané', value: 'list' },
  { label: 'Nastavenie', value: 'settings' },
]

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

// ---- nastavenie ----
const form = ref(null)
const errors = ref({})
const saving = ref(false)
const testing = ref(false)
const testTo = ref('')
const tplKind = ref('invoice')
const clearPassword = ref(false)
const providerNote = ref('')

async function loadSettings() {
  if (!canSettings.value) return
  try {
    const s = await api.mailSettings(session.companyId)
    form.value = { ...s, password: '', templates: { ...s.templates } }
    if (!s.host && !s.fromEmail) {
      form.value.fromEmail = session.company?.email ?? ''
      form.value.fromName = session.company?.name ?? ''
      suggest()
    }
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nastavenie sa nenačítalo', detail: e.message, life: 6000 })
  }
}
onMounted(loadSettings)

function suggest() {
  const p = guessProvider(form.value.fromEmail)
  providerNote.value = p?.note ?? ''
  if (p && !form.value.host) {
    Object.assign(form.value, { host: p.host, security: p.security, port: p.port })
    if (!form.value.username) form.value.username = form.value.fromEmail
  }
}
function onSecurity(v) {
  form.value.port = portFor(v)
}

async function saveSettings() {
  saving.value = true
  try {
    const s = await api.saveMailSettings(session.companyId, { ...form.value, clearPassword: clearPassword.value })
    form.value = { ...s, password: '', templates: { ...s.templates } }
    clearPassword.value = false
    errors.value = {}
    toast.add({ severity: 'success', summary: 'Nastavenie uložené', life: 2500 })
    return true
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) {
      errors.value = e.fieldMap()
      toast.add({ severity: 'error', summary: 'Skontrolujte vyznačené polia', life: 4000 })
    } else {
      toast.add({ severity: 'error', summary: 'Uloženie zlyhalo', detail: e.message, life: 6000 })
    }
    return false
  } finally {
    saving.value = false
  }
}

async function sendTest() {
  if (!(await saveSettings())) return
  testing.value = true
  try {
    const e = await api.testMail(session.companyId, testTo.value)
    toast.add({ severity: 'success', summary: 'Skúšobná správa odoslaná', detail: e.to, life: 4000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Skúšobná správa sa neodoslala', detail: e.message, life: 10000 })
  } finally {
    testing.value = false
    load()
  }
}

function onKey(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's' && tab.value === 'settings') {
    e.preventDefault()
    if (!saving.value) saveSettings()
    return
  }
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
      <SelectButton v-if="canSettings" v-model="tab" :options="tabs" option-label="label" option-value="value" :allow-empty="false" size="small" />
      <div class="spacer" />
      <Select v-if="tab === 'list'" v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <Button v-else label="Uložiť" icon="pi pi-check" size="small" :loading="saving" :disabled="!form" @click="saveSettings" v-tooltip.bottom="'Ctrl+S'" />
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

    <div v-else-if="form" class="body">
      <fieldset class="form-section">
        <legend>Odosielateľ</legend>
        <div class="form-grid">
          <FormField label="E-mail odosielateľa" :span="2" :error="errors.fromEmail" hint="Z tejto adresy dostane odberateľ doklad">
            <InputText v-model="form.fromEmail" size="small" inputmode="email" autocomplete="email" :invalid="!!errors.fromEmail" @blur="suggest" />
          </FormField>
          <FormField label="Meno odosielateľa" :span="2" :error="errors.fromName" hint="Prázdne = názov firmy">
            <InputText v-model="form.fromName" size="small" :placeholder="session.company?.name" />
          </FormField>
          <FormField label="Odpovede posielať na" :span="2" :error="errors.replyTo" hint="Nepovinné, ak sa líši od odosielateľa">
            <InputText v-model="form.replyTo" size="small" inputmode="email" />
          </FormField>
          <label class="switch span2">
            <ToggleSwitch v-model="form.bccSelf" />
            <span>
              <strong>Kópiu každej správy aj mne</strong>
              <small>Odoslaný doklad nájdete vo svojej pošte.</small>
            </span>
          </label>
        </div>
      </fieldset>

      <fieldset class="form-section">
        <legend>Server SMTP</legend>
        <p v-if="providerNote" class="note"><i class="pi pi-info-circle" /> {{ providerNote }}</p>
        <div class="form-grid">
          <FormField label="Server" :span="2" :error="errors.host" hint="Napr. smtp.gmail.com — nájdete v nastavení pošty">
            <InputText v-model="form.host" size="small" spellcheck="false" autocomplete="off" :invalid="!!errors.host" />
          </FormField>
          <FormField label="Zabezpečenie" :error="errors.security">
            <Select v-model="form.security" :options="securityOptions" option-label="label" option-value="value" size="small" @update:model-value="onSecurity" />
          </FormField>
          <FormField label="Port" :error="errors.port">
            <InputNumber v-model="form.port" :min="1" :max="65535" :use-grouping="false" size="small" :invalid="!!errors.port" />
          </FormField>
          <FormField label="Používateľ" :span="2" hint="Väčšinou celá e-mailová adresa">
            <InputText v-model="form.username" size="small" autocomplete="off" spellcheck="false" />
          </FormField>
          <FormField label="Heslo" :span="2" :hint="form.passwordSet ? 'Heslo je uložené — vyplňte iba pri zmene' : ''">
            <Password v-model="form.password" :feedback="false" toggle-mask size="small" input-class="pw" autocomplete="new-password"
              :placeholder="form.passwordSet ? '••••••••' : ''" fluid />
          </FormField>
          <label v-if="form.passwordSet" class="check span2">
            <input v-model="clearPassword" type="checkbox" /> Zmazať uložené heslo
          </label>
        </div>
        <div class="test-row">
          <InputText v-model="testTo" size="small" inputmode="email" :placeholder="form.fromEmail || 'komu poslať skúšku'" aria-label="Skúšobná správa na" />
          <Button label="Uložiť a poslať skúšobnú správu" icon="pi pi-send" severity="secondary" outlined size="small" :loading="testing" @click="sendTest" />
        </div>
      </fieldset>

      <fieldset class="form-section">
        <legend>Texty správ</legend>
        <SelectButton v-model="tplKind" :options="templateKinds" option-label="label" option-value="kind" :allow-empty="false" size="small" class="tpl-tabs" />
        <div class="tpl">
          <FormField label="Predmet">
            <InputText v-model="form.templates[tplKind].subject" size="small" />
          </FormField>
          <FormField label="Text">
            <Textarea v-model="form.templates[tplKind].body" rows="10" auto-resize />
          </FormField>
        </div>
        <p class="help">
          Zástupné znaky:
          <template v-for="([code, label], i) in placeholders" :key="code"><code>{{ code }}</code> {{ label }}<template v-if="i < placeholders.length - 1">, </template></template>.
          Prázdne pole = predvolený text.
        </p>
      </fieldset>
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
