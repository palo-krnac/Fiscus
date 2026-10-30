<script setup>
// Vlastné termíny firmy: odvody, preddavky, STK, poistky… Opakovanie a posun
// na pracovný deň počíta server.
import { computed, ref, watch } from 'vue'
import Drawer from 'primevue/drawer'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import Checkbox from 'primevue/checkbox'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import FormField from './FormField.vue'
import ExportButton from './ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatDate, fromISODate, toISODate } from '../lib/format.js'
import { matches, searchTerms } from '../lib/search.js'
import { ownEventColumns } from '../lib/columns.js'
import { presets, repeatLabel, repeats } from '../lib/calendar.js'

const visible = defineModel('visible', { type: Boolean, default: false })
const props = defineProps({ canEdit: { type: Boolean, default: false }, editId: { type: Number, default: 0 } })
const emit = defineEmits(['changed'])
const toast = useToast()
const confirm = useConfirm()

const list = ref([])
const search = ref('')
const form = ref(null)
const errors = ref({})
const busy = ref(false)
const year = new Date().getFullYear()
const presetList = presets(year)
const preset = ref(null)

const rows = computed(() => {
  const terms = searchTerms(search.value)
  return list.value.filter((o) => matches(terms, [o.title, o.note, repeatLabel(o.repeat)]))
})

async function load() {
  try {
    list.value = await api.ownEvents(session.companyId)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Termíny sa nenačítali', detail: e.message, life: 6000 })
  }
}
watch(visible, async (v) => {
  if (!v) return
  await load()
  form.value = null
  if (props.editId) edit(list.value.find((o) => o.id === props.editId))
})

function edit(o) {
  errors.value = {}
  preset.value = null
  form.value = o
    ? { ...o, date: fromISODate(o.date), until: o.until ? fromISODate(o.until) : null, repeat: o.repeat || '' }
    : { id: 0, title: '', note: '', date: null, until: null, repeat: 'monthly', workday: true }
}

function usePreset(p) {
  if (!p || !form.value) return
  Object.assign(form.value, { title: p.title, note: p.note, repeat: p.repeat, workday: p.workday, date: p.date ? fromISODate(p.date) : form.value.date })
}

async function save() {
  busy.value = true
  errors.value = {}
  try {
    const f = form.value
    await api.saveOwnEvent(session.companyId, {
      id: f.id || undefined, title: f.title, note: f.note, date: toISODate(f.date), until: toISODate(f.until),
      repeat: f.repeat, workday: f.workday, done: f.done ?? [],
    })
    toast.add({ severity: 'success', summary: f.id ? 'Termín upravený' : 'Termín pridaný', detail: f.title, life: 3000 })
    form.value = null
    await load()
    emit('changed')
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) errors.value = e.fieldMap()
    else toast.add({ severity: 'error', summary: 'Termín sa neuložil', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

function remove(o) {
  confirm.require({
    header: 'Zmazať termín',
    message: `Zmazať „${o.title}“ aj so všetkými opakovaniami?`,
    acceptLabel: 'Zmazať',
    rejectLabel: 'Späť',
    acceptProps: { severity: 'danger', size: 'small' },
    rejectProps: { severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.deleteOwnEvent(session.companyId, o.id)
        form.value = null
        await load()
        emit('changed')
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
      }
    },
  })
}
</script>

<template>
  <Drawer v-model:visible="visible" position="right" class="own-panel" :header="form ? (form.id ? 'Upraviť termín' : 'Nový termín') : 'Vlastné termíny'">
    <form v-if="form" class="form-grid own-form" @submit.prevent="save">
      <FormField v-if="!form.id" label="Rýchly výber" :span="2" hint="Doplní názov, opakovanie a lehotu — dátum skontrolujte">
        <Select v-model="preset" :options="presetList" option-label="title" size="small" fluid placeholder="Odvody, preddavky, STK…" show-clear
          @update:model-value="usePreset" />
      </FormField>
      <FormField label="Názov" :span="2" :error="errors.title">
        <InputText v-model="form.title" size="small" :invalid="!!errors.title" :disabled="!canEdit" />
      </FormField>
      <FormField label="Prvý termín" :error="errors.date">
        <DatePicker v-model="form.date" date-format="d. m. yy" size="small" fluid :invalid="!!errors.date" :disabled="!canEdit" />
      </FormField>
      <FormField label="Opakovanie" :error="errors.repeat">
        <Select v-model="form.repeat" :options="repeats" option-label="label" option-value="value" size="small" fluid :disabled="!canEdit" />
      </FormField>
      <FormField v-if="form.repeat" label="Opakovať do" :error="errors.until" hint="Nepovinné">
        <DatePicker v-model="form.until" date-format="d. m. yy" size="small" fluid show-clear :disabled="!canEdit" />
      </FormField>
      <label class="check" :class="{ span2: !form.repeat }">
        <Checkbox v-model="form.workday" binary :disabled="!canEdit" />
        <span>Ak pripadne na víkend alebo sviatok, posunúť na najbližší pracovný deň</span>
      </label>
      <FormField label="Poznámka" :span="2">
        <Textarea v-model="form.note" rows="2" auto-resize :disabled="!canEdit" />
      </FormField>
      <div class="actions span2">
        <Button v-if="form.id && canEdit" label="Zmazať" severity="danger" text size="small" @click="remove(form)" />
        <span class="spacer" />
        <Button label="Späť" severity="secondary" text size="small" @click="form = null" />
        <Button v-if="canEdit" type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
      </div>
    </form>

    <template v-else>
      <div class="toolbar">
        <IconField class="search">
          <InputIcon class="pi pi-search" />
          <InputText v-model="search" placeholder="Hľadať termín" size="small" aria-label="Hľadať termín" />
        </IconField>
        <ExportButton name="Vlastné termíny" :columns="ownEventColumns" :rows="rows" />
      </div>
      <ul class="own-list">
        <li v-if="!rows.length" class="empty">
          {{ list.length ? 'Nič sa nenašlo.' : 'Zatiaľ žiadne vlastné termíny. Pridajte odvody, preddavky na daň alebo STK.' }}
        </li>
        <li v-for="o in rows" :key="o.id">
          <button type="button" class="own-item" @click="edit(o)">
            <strong>{{ o.title }}</strong>
            <span class="meta">{{ repeatLabel(o.repeat) }} · od {{ formatDate(o.date) }}<template v-if="o.until"> do {{ formatDate(o.until) }}</template><template v-if="o.workday"> · posun na pracovný deň</template></span>
          </button>
        </li>
      </ul>
      <Button v-if="canEdit" label="Pridať termín" icon="pi pi-plus" size="small" class="add" @click="edit(null)" />
    </template>
  </Drawer>
</template>

<style scoped>
.own-form {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.span2 {
  grid-column: 1 / -1;
}
.check {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  align-self: end;
  padding-bottom: 6px;
}
.actions {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
}
.spacer {
  flex: 1;
}
.toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}
.search {
  flex: 1;
}
.search :deep(input) {
  width: 100%;
}
.own-list {
  list-style: none;
  margin: 0 0 12px;
  padding: 0;
  border-top: 1px solid var(--rule);
}
.own-list li {
  border-bottom: 1px solid var(--rule-soft);
}
.own-item {
  all: unset;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 1px;
  width: 100%;
  padding: 7px 4px;
  cursor: pointer;
}
.own-item:hover {
  background: var(--blue-wash);
}
.own-item:focus-visible {
  outline: 2px solid var(--blue);
  outline-offset: -2px;
}
.meta {
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.empty {
  padding: 10px 4px;
  color: var(--ink-faint);
}
</style>

<style>
.p-drawer.p-component.own-panel {
  width: min(480px, 100vw);
}
</style>
