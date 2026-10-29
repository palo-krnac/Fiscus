<script setup>
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import FormField from '../components/FormField.vue'
import IcoInput from '../components/IcoInput.vue'
import Select from 'primevue/select'
import { countries } from '../lib/codes.js'
import { fillFromRegistry } from '../lib/registry.js'
import { api, ApiError } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { contactColumns } from '../lib/columns.js'
import ExportButton from '../components/ExportButton.vue'
import { filterContacts } from '../lib/contacts.js'

const props = defineProps({ id: String })
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()

const contacts = shallowRef([])
const loading = ref(true)
const search = ref('')
const form = ref(null)
const errors = ref({})
const busy = ref(false)

const blank = () => ({
  name: '', street: '', city: '', zip: '', country: 'SK', ico: '', dic: '', icdph: '',
  email: '', phone: '', iban: '', dueDays: null, note: '',
})

async function load() {
  loading.value = true
  try {
    contacts.value = await api.contacts(session.companyId)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Adresár sa nenačítal', detail: e.message, life: 6000 })
  } finally {
    loading.value = false
  }
}

function openFromRoute() {
  errors.value = {}
  if (props.id === 'novy') form.value = blank()
  else if (props.id) {
    const c = contacts.value.find((x) => String(x.id) === props.id)
    form.value = c ? { ...c, dueDays: c.dueDays || null } : null
  } else form.value = null
}

onMounted(async () => {
  await load()
  openFromRoute()
})
watch(() => props.id, openFromRoute)

const rows = computed(() => filterContacts(contacts.value, search.value))

const columnDefs = [
  { field: 'name', headerName: 'Názov / meno', flex: 2, minWidth: 180, cellClass: 'cell-name' },
  { field: 'ico', headerName: 'IČO', width: 100 },
  { field: 'icdph', headerName: 'IČ DPH', width: 130 },
  { field: 'city', headerName: 'Obec', flex: 1, minWidth: 120 },
  { field: 'email', headerName: 'E-mail', flex: 1, minWidth: 160 },
  {
    field: 'dueDays', headerName: 'Splatnosť', width: 100, type: 'rightAligned',
    valueFormatter: (p) => (p.value ? `${p.value} dní` : '—'),
  },
]

const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['icdph', 'email', 'dueDays'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
narrow.addEventListener('change', applyResponsive)

const rowId = (p) => String(p.data.id)
const selectedId = computed(() => form.value?.id ?? null)
const rowClassRules = { 'row-selected': (p) => p.data.id === selectedId.value }
watch(selectedId, () => gridApi.value?.redrawRows())

const fields = [
  { key: 'name', label: 'Názov / meno', span: 4, autocomplete: 'organization' },
  { key: 'street', label: 'Ulica a číslo', span: 2 },
  { key: 'zip', label: 'PSČ' },
  { key: 'city', label: 'Obec' },
  { key: 'ico', label: 'IČO', inputmode: 'numeric' },
  { key: 'dic', label: 'DIČ', inputmode: 'numeric' },
  { key: 'icdph', label: 'IČ DPH' },
  { key: 'country', label: 'Krajina' },
  { key: 'email', label: 'E-mail', span: 2, autocomplete: 'email' },
  { key: 'phone', label: 'Telefón', span: 2, autocomplete: 'tel' },
  { key: 'iban', label: 'IBAN', span: 2 },
]

async function save() {
  busy.value = true
  try {
    const wasNew = !form.value.id
    const saved = await api.saveContact(session.companyId, { ...form.value, dueDays: form.value.dueDays || 0 })
    await load()
    errors.value = {}
    form.value = { ...saved, dueDays: saved.dueDays || null }
    toast.add({ severity: 'success', summary: wasNew ? 'Kontakt pridaný' : 'Kontakt uložený', life: 2500 })
    if (wasNew) router.replace(`/adresar/${saved.id}`)
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
    header: 'Zmazať kontakt',
    message: `${form.value.name} sa odstráni z adresára. Vystavené faktúry sa nezmenia.`,
    acceptProps: { label: 'Zmazať', severity: 'danger' },
    rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await api.deleteContact(session.companyId, form.value.id)
        toast.add({ severity: 'info', summary: 'Kontakt zmazaný', life: 2500 })
        router.push('/adresar')
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
  if ((e.ctrlKey || e.metaKey) && e.key === 's' && form.value && can('contacts.write')) {
    e.preventDefault()
    if (!busy.value) save()
  } else if (e.key === 'Escape' && form.value) {
    router.push('/adresar')
  } else if (!typing && !e.ctrlKey && !e.metaKey && e.key === 'n' && can('contacts.write')) {
    e.preventDefault()
    router.push('/adresar/novy')
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  narrow.removeEventListener('change', applyResponsive)
})
</script>

<template>
  <div class="contacts" :class="{ 'has-panel': !!form }">
    <section class="list">
      <header class="head">
        <h1>Adresár</h1>
        <span class="count">{{ contacts.length }}</span>
        <div class="spacer" />
        <ExportButton name="Adresár" :columns="contactColumns" :rows="() => gridRows(gridApi, rows)" />
        <Button v-if="can('contacts.write')" size="small" @click="router.push('/adresar/novy')">
          <i class="pi pi-plus" /><span>Nový kontakt</span><kbd class="hint">N</kbd>
        </Button>
      </header>
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať názov, IČO, obec, e-mail" size="small" aria-label="Hľadať v adresári" />
      </IconField>
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
          :overlay-no-rows-template="'<span>Adresár je prázdny. Nový kontakt pridáte klávesom N.</span>'"
          @grid-ready="(e) => { gridApi = e.api; applyResponsive() }"
          @grid-size-changed="(e) => e.api.sizeColumnsToFit()"
          @row-clicked="(e) => router.push(`/adresar/${e.data.id}`)"
        />
      </div>
    </section>

    <aside v-if="form" class="panel" aria-label="Detail kontaktu">
      <header class="panel-head">
        <button class="close" aria-label="Zavrieť" @click="router.push('/adresar')"><i class="pi pi-times" /></button>
        <h2>{{ form.id ? form.name || 'Kontakt' : 'Nový kontakt' }}</h2>
      </header>
      <div class="panel-body">
        <div class="form-grid">
          <FormField v-for="f in fields" :key="f.key" :label="f.label" :span="f.span" :hint="f.hint" :error="errors[f.key]">
            <IcoInput
              v-if="f.key === 'ico'"
              v-model="form.ico"
              :invalid="!!errors.ico"
              :auto="!form.id && !form.name"
              @found="(c) => (form = fillFromRegistry(form, c))"
            />
            <Select
              v-else-if="f.key === 'country'"
              v-model="form.country"
              :options="countries"
              option-label="label"
              option-value="code"
              filter
              filter-placeholder="Hľadať krajinu"
              size="small"
              :invalid="!!errors.country"
            />
            <InputText v-else v-model="form[f.key]" size="small" :invalid="!!errors[f.key]" :inputmode="f.inputmode" :autocomplete="f.autocomplete" />
          </FormField>
          <FormField label="Vlastná splatnosť" :span="2" :error="errors.dueDays" :hint="`Prázdne = predvolená (${session.company?.defaultDueDays || 14} dní)`">
            <InputNumber v-model="form.dueDays" :min="0" :max="365" suffix=" dní" size="small" :invalid="!!errors.dueDays" />
          </FormField>
          <FormField label="Poznámka" :span="4">
            <Textarea v-model="form.note" rows="2" auto-resize />
          </FormField>
        </div>
      </div>
      <footer class="panel-foot">
        <Button v-if="form.id && can('contacts.write')" label="Zmazať" severity="danger" text size="small" @click="remove" />
        <div class="spacer" />
        <Button v-if="form.id && can('invoices.write')" label="Nová faktúra" icon="pi pi-file-plus" severity="secondary" outlined size="small" @click="router.push(`/faktury/nova?kontakt=${form.id}`)" />
        <Button v-if="can('contacts.write')" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" @click="save" />
      </footer>
    </aside>
  </div>
</template>

<style scoped>
.contacts {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  height: 100%;
}
.contacts.has-panel {
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
  .contacts.has-panel {
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
