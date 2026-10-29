<script setup>
// Pripojenie k digitálnemu poštárovi (Peppol): priečinky alebo API adaptéra.
import { computed, onMounted, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import SelectButton from 'primevue/selectbutton'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { session, isSuperAdmin } from '../lib/session.js'
import { postmanModes } from '../lib/einvoice.js'

const toast = useToast()
const form = ref({ mode: '', outDir: '', inDir: '', url: '', token: '', provider: '' })
const tokenSet = ref(false)
const errors = ref({})
const busy = ref(false)
const loaded = ref(false)
// priečinky servera smie nastaviť iba správca inštalácie
const modes = computed(() => postmanModes(session.caps.mode, isSuperAdmin()))

onMounted(async () => {
  try {
    const s = await api.einvoiceSettings(session.companyId)
    tokenSet.value = s.tokenSet
    form.value = { mode: s.mode ?? '', outDir: s.outDir ?? '', inDir: s.inDir ?? '', url: s.url ?? '', token: '', provider: s.provider ?? '' }
  } finally {
    loaded.value = true
  }
})

async function save() {
  busy.value = true
  try {
    const s = await api.saveEInvoiceSettings(session.companyId, form.value)
    tokenSet.value = s.tokenSet
    form.value.token = ''
    errors.value = {}
    toast.add({ severity: 'success', summary: 'Pripojenie e-faktúr uložené', life: 2500 })
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    toast.add({ severity: 'error', summary: 'Nastavenie sa neuložilo', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <fieldset v-if="loaded" class="form-section einv">
    <legend>E-faktúry (digitálny poštár)</legend>
    <p class="help">
      Od 1. 1. 2027 si platitelia DPH posielajú faktúry ako e-faktúry cez sieť Peppol. Fiškus e-faktúru vytvorí; odošle a prijme ju
      váš digitálny poštár (certifikovaný poskytovateľ). Vaša adresa v sieti je <strong>0245:{{ session.company?.dic || 'DIČ' }}</strong>.
    </p>
    <SelectButton v-model="form.mode" :options="modes" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Pripojenie" />
    <div v-if="form.mode" class="form-grid">
      <FormField label="Poskytovateľ" :span="2" hint="Názov poštára (iba pre vás)">
        <InputText v-model="form.provider" size="small" />
      </FormField>
      <template v-if="form.mode === 'folder'">
        <FormField label="Priečinok na odosielanie" :span="2" :error="errors.outDir" hint="Sem Fiškus uloží e-faktúru; aplikácia poštára ju odošle">
          <InputText v-model="form.outDir" size="small" :invalid="!!errors.outDir" placeholder="napr. C:\Users\…\Postar\Odoslat" />
        </FormField>
        <FormField label="Priečinok prijatých" :span="2" :error="errors.inDir" hint="Odtiaľto Fiškus číta prijaté e-faktúry; prevzaté presunie do „prevzate“">
          <InputText v-model="form.inDir" size="small" :invalid="!!errors.inDir" placeholder="napr. C:\Users\…\Postar\Prijate" />
        </FormField>
      </template>
      <template v-else>
        <FormField label="Adresa API" :span="2" :error="errors.url" hint="Adaptér poskytovateľa (POST /documents, GET /inbox)">
          <InputText v-model="form.url" size="small" :invalid="!!errors.url" placeholder="https://…" />
        </FormField>
        <FormField label="Prístupový token" :span="2" :error="errors.token" :hint="tokenSet ? 'Uložený — nový zadajte iba pri zmene' : ''">
          <Password v-model="form.token" :feedback="false" toggle-mask size="small" fluid :invalid="!!errors.token" autocomplete="off" />
        </FormField>
      </template>
    </div>
    <Button label="Uložiť pripojenie" icon="pi pi-check" severity="secondary" outlined size="small" :loading="busy" @click="save" />
  </fieldset>
</template>

<style scoped>
.einv {
  display: grid;
  gap: 10px;
  justify-items: start;
}
.einv .form-grid {
  width: 100%;
}
.help {
  margin: 0;
  max-width: 70ch;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
</style>
