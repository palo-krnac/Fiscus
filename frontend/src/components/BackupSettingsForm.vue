<script setup>
// Nastavenie priečinka a hesla záloh (iba správca inštalácie).
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import { useToast } from 'primevue/usetoast'
import { api, ApiError } from '../lib/api.js'
import { changeNotes, cloudLabel, confirmError } from '../lib/backups.js'
import FormField from './FormField.vue'

const props = defineProps({ status: { type: Object, required: true } })
const emit = defineEmits(['saved'])
const toast = useToast()

const s = computed(() => props.status.settings)
const dir = ref('')
const password = ref('')
const confirm = ref('')
const errors = ref({})
const saving = ref(false)
watch(
  () => s.value?.dir,
  (d) => (dir.value = d || s.value?.defaultDir || ''),
  { immediate: true },
)

const mismatch = computed(() => confirmError(password.value, confirm.value))
const dirChanged = computed(() => dir.value.trim() !== (s.value?.dir ?? ''))
const notes = computed(() =>
  changeNotes({ count: props.status.backups.length, passwordSet: s.value?.passwordSet, dirChanged: dirChanged.value, passwordChanged: !!password.value }),
)
const canSave = computed(() => !mismatch.value && (dirChanged.value || !!password.value) && !saving.value)

async function save() {
  saving.value = true
  errors.value = {}
  try {
    const st = await api.setBackupSettings({ dir: dir.value.trim(), password: password.value })
    password.value = confirm.value = ''
    toast.add({ severity: 'success', summary: 'Zálohy nastavené', detail: 'Prvá záloha je vytvorená a overená.', life: 5000 })
    emit('saved', st)
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) errors.value = e.fieldMap()
    else toast.add({ severity: 'error', summary: 'Nastavenie sa neuložilo', detail: e.message, life: 8000 })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="card settings" aria-labelledby="bs-title">
    <h2 id="bs-title">Kam a s akým heslom zálohovať</h2>
    <div class="form-grid">
      <FormField label="Priečinok záloh" :span="4" :error="errors.dir"
        :hint="s.dirFixed ? 'Určený parametrom -backup-dir pri spustení programu.' : 'Najlepšie priečinok, ktorý sa synchronizuje do cloudu alebo na iný disk.'">
        <InputText v-model="dir" size="small" spellcheck="false" :disabled="s.dirFixed" :invalid="!!errors.dir" />
      </FormField>
      <div v-if="!s.dirFixed && (s.suggestions.length || dir !== s.defaultDir)" class="picks span4">
        <span class="muted">Použiť:</span>
        <Button v-for="p in s.suggestions" :key="p" :label="cloudLabel(p)" icon="pi pi-cloud" severity="secondary" outlined size="small"
          :class="{ on: dir === p }" @click="dir = p" />
        <Button v-if="dir !== s.defaultDir" label="Predvolený priečinok" severity="secondary" text size="small" @click="dir = s.defaultDir" />
      </div>
      <template v-if="s.passwordFixed">
        <p class="fixed span4"><i class="pi pi-lock" /> Heslo je určené premennou FISKUS_BACKUP_PASSWORD pri spustení programu.</p>
      </template>
      <template v-else>
        <FormField :label="s.passwordSet ? 'Nové heslo záloh' : 'Heslo záloh'" :span="2" :error="errors.password"
          :hint="s.passwordSet ? 'Vyplňte iba pri zmene' : 'Aspoň 8 znakov'">
          <Password v-model="password" :feedback="false" toggle-mask size="small" autocomplete="new-password" :invalid="!!errors.password" fluid />
        </FormField>
        <FormField label="Zopakovať heslo" :span="2" :error="mismatch">
          <Password v-model="confirm" :feedback="false" toggle-mask size="small" autocomplete="new-password" :invalid="!!mismatch" fluid />
        </FormField>
      </template>
    </div>
    <ul v-if="notes.length" class="notes">
      <li v-for="n in notes" :key="n"><i class="pi pi-info-circle" /> {{ n }}</li>
    </ul>
    <p class="muted">
      Heslo si dobre uschovajte — bez neho sa zo zálohy nedá obnoviť. Program ho drží v súbore vedľa databázy, aby mohol zálohovať sám;
      chráni zálohy, ktoré sa dostanú mimo počítača.
    </p>
    <div class="actions">
      <Button label="Uložiť a zálohovať" icon="pi pi-check" size="small" :loading="saving" :disabled="!canSave" @click="save" />
    </div>
  </section>
</template>

<style scoped>
.card {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 12px 14px;
}
.card h2 {
  font-size: var(--fs-md);
  margin: 0 0 10px;
}
.span4 {
  grid-column: span 4;
}
.picks {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: -4px;
}
.picks .on {
  border-color: var(--blue);
  color: var(--blue-deep);
}
.fixed {
  margin: 0;
  color: var(--ink-soft);
}
.notes {
  list-style: none;
  margin: 10px 0 0;
  padding: 0;
  color: var(--tone-draft);
  font-size: var(--fs-sm);
}
.muted {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
  margin: 10px 0 0;
}
.actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 10px;
}
@media (max-width: 760px) {
  .span4 {
    grid-column: 1 / -1;
  }
}
</style>
