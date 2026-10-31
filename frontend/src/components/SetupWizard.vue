<script setup>
// Úvodné nastavenie novej firmy: údaje, DPH, druh účtovníctva a začiatok
// (nová firma alebo prechod z iného programu). Ukladá sa až na konci.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import ToggleSwitch from 'primevue/toggleswitch'
import SelectButton from 'primevue/selectbutton'
import FormField from './FormField.vue'
import IcoInput from './IcoInput.vue'
import { fillFromRegistry } from '../lib/registry.js'
import { api, ApiError } from '../lib/api.js'
import { session, replaceCompany } from '../lib/session.js'
import { afterSetup, bookkeepingOptions, setupSteps, stepErrors } from '../lib/setup.js'

const router = useRouter()
const toast = useToast()
const step = ref(0)
const form = ref({ ...session.company, bookkeeping: session.company.bookkeeping || '' })
const start = ref('new')
const errors = ref({})
const busy = ref(false)
const last = computed(() => step.value === setupSteps.length - 1)

const companyFields = [
  { key: 'street', label: 'Ulica a číslo', span: 2, autocomplete: 'street-address' },
  { key: 'zip', label: 'PSČ', autocomplete: 'postal-code' },
  { key: 'city', label: 'Obec', autocomplete: 'address-level2' },
  { key: 'dic', label: 'DIČ', inputmode: 'numeric' },
  { key: 'iban', label: 'IBAN', span: 2 },
  { key: 'email', label: 'E-mail', span: 2, autocomplete: 'email' },
  { key: 'phone', label: 'Telefón', span: 2, autocomplete: 'tel' },
]

function next() {
  errors.value = stepErrors(step.value, form.value)
  if (Object.keys(errors.value).length === 0) step.value++
}

async function finish(skip = false) {
  busy.value = true
  try {
    const saved = await api.updateCompany({ ...form.value, setupDone: true })
    replaceCompany(saved)
    if (!skip) {
      toast.add({ severity: 'success', summary: 'Firma je nastavená', life: 3000 })
      router.push(afterSetup(start.value))
    }
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) {
      errors.value = e.fieldMap()
      step.value = errors.value.bookkeeping ? 2 : errors.value.icdph || errors.value.vatPeriod ? 1 : 0
      toast.add({ severity: 'error', summary: 'Skontrolujte vyznačené polia', life: 4000 })
    } else {
      toast.add({ severity: 'error', summary: 'Uloženie zlyhalo', detail: e.message, life: 6000 })
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="wizard">
    <header class="wz-head">
      <h1>Úvodné nastavenie</h1>
      <p>Pár otázok, aby Fiškus ukazoval iba to, čo vaša firma potrebuje. Všetko sa dá neskôr zmeniť v Nastaveniach.</p>
      <ol class="steps" aria-label="Kroky">
        <li v-for="(s, i) in setupSteps" :key="s" :class="{ done: i < step, current: i === step }" :aria-current="i === step ? 'step' : undefined">
          <span class="num">{{ i + 1 }}</span>{{ s }}
        </li>
      </ol>
    </header>

    <section v-if="step === 0" class="wz-body">
      <h2>Údaje firmy</h2>
      <p class="hint">Po zadaní IČO sa názov a adresa doplnia z registra.</p>
      <div class="form-grid">
        <FormField label="IČO" :error="errors.ico">
          <IcoInput v-model="form.ico" :invalid="!!errors.ico" @found="(c) => (form = fillFromRegistry(form, c))" />
        </FormField>
        <FormField label="Obchodné meno" :span="3" :error="errors.name">
          <InputText v-model="form.name" size="small" :invalid="!!errors.name" autocomplete="organization" />
        </FormField>
        <FormField v-for="f in companyFields" :key="f.key" :label="f.label" :span="f.span" :error="errors[f.key]">
          <InputText v-model="form[f.key]" size="small" :invalid="!!errors[f.key]" :inputmode="f.inputmode" :autocomplete="f.autocomplete" />
        </FormField>
      </div>
    </section>

    <section v-else-if="step === 1" class="wz-body">
      <h2>DPH</h2>
      <label class="switch">
        <ToggleSwitch v-model="form.vatPayer" />
        <span><strong>Som platiteľ DPH</strong><small>Ak nie ste, faktúry budú bez DPH s poznámkou „Nie som platiteľ DPH“.</small></span>
      </label>
      <div v-if="form.vatPayer" class="form-grid">
        <FormField label="IČ DPH" :error="errors.icdph" hint="SK a 10 číslic">
          <InputText v-model="form.icdph" size="small" :invalid="!!errors.icdph" placeholder="SK…" />
        </FormField>
        <FormField label="Zdaňovacie obdobie" :error="errors.vatPeriod">
          <SelectButton
            v-model="form.vatPeriod"
            :options="[{ label: 'Mesačne', value: 'month' }, { label: 'Štvrťročne', value: 'quarter' }]"
            option-label="label"
            option-value="value"
            :allow-empty="false"
            size="small"
          />
        </FormField>
        <FormField label="Daňový úrad" :span="2" hint="Do hlavičky priznania k DPH">
          <InputText v-model="form.taxOffice" size="small" placeholder="Daňový úrad Banská Bystrica" />
        </FormField>
      </div>
    </section>

    <section v-else-if="step === 2" class="wz-body">
      <h2>Účtovníctvo</h2>
      <p class="hint">Podľa toho Fiškus ukáže v menu peňažný denník alebo účtovný denník s hlavnou knihou.</p>
      <div class="choices" role="radiogroup" aria-label="Druh účtovníctva">
        <label v-for="o in bookkeepingOptions" :key="o.value" class="choice" :class="{ on: form.bookkeeping === o.value }">
          <input v-model="form.bookkeeping" type="radio" name="bookkeeping" :value="o.value" />
          <strong>{{ o.label }}</strong>
          <span>{{ o.who }}</span>
          <small>{{ o.what }}</small>
        </label>
      </div>
      <small v-if="errors.bookkeeping && !form.bookkeeping" class="err">{{ errors.bookkeeping }}</small>
    </section>

    <section v-else class="wz-body">
      <h2>Začiatok</h2>
      <div class="choices" role="radiogroup" aria-label="Začiatok">
        <label class="choice" :class="{ on: start === 'new' }">
          <input v-model="start" type="radio" name="start" value="new" />
          <strong>Začínam vo Fiškuse</strong>
          <span>Nová firma alebo nový rok bez starých dokladov</span>
          <small>Pokračujete vystavením prvej faktúry.</small>
        </label>
        <label class="choice" :class="{ on: start === 'migrate' }">
          <input v-model="start" type="radio" name="start" value="migrate" />
          <strong>Prechádzam z iného programu</strong>
          <span>POHODA, Money S3, KROS, MRP, iDoklad, SuperFaktúra, Fakturoid, Excel</span>
          <small>Doklady z iného programu sa dajú preniesť iba raz — adresár a cenník kedykoľvek.</small>
        </label>
      </div>
    </section>

    <footer class="wz-foot">
      <Button v-if="step > 0" label="Späť" icon="pi pi-arrow-left" size="small" text @click="step--" />
      <button type="button" class="skip" :disabled="busy" @click="finish(true)">Preskočiť, nastavím neskôr</button>
      <div class="spacer" />
      <Button v-if="!last" label="Pokračovať" icon="pi pi-arrow-right" icon-pos="right" size="small" @click="next" />
      <Button v-else label="Dokončiť" icon="pi pi-check" size="small" :loading="busy" @click="finish()" />
    </footer>
  </div>
</template>

<style scoped>
.wizard {
  max-width: 760px;
  margin: 24px auto 40px;
  padding: 0 16px;
}
.wz-head h1 {
  font-size: var(--fs-xl);
  color: var(--blue-deep);
}
.wz-head p {
  margin-top: 4px;
  color: var(--ink-soft);
}
.steps {
  display: flex;
  gap: 4px;
  margin: 18px 0 0;
  padding: 0;
  list-style: none;
  border-bottom: 1px solid var(--rule);
}
.steps li {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px 8px;
  color: var(--ink-faint);
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
}
.steps li.current {
  color: var(--blue-deep);
  border-bottom-color: var(--blue);
  font-weight: 600;
}
.steps li.done {
  color: var(--ink-soft);
}
.num {
  display: inline-grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 1px solid currentColor;
  font-size: var(--fs-xs);
  font-variant-numeric: tabular-nums;
}
.current .num {
  background: var(--blue);
  border-color: var(--blue);
  color: #fff;
}
.wz-body {
  padding: 18px 0 8px;
}
.wz-body h2 {
  font-size: var(--fs-lg);
  margin-bottom: 4px;
}
.hint {
  margin-bottom: 12px;
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.switch {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  margin: 10px 0 14px;
  cursor: pointer;
}
.switch small {
  display: block;
  color: var(--ink-faint);
}
.choices {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 8px;
}
.choice {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 14px 14px 40px;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  cursor: pointer;
}
.choice input {
  position: absolute;
  left: 14px;
  top: 16px;
  accent-color: var(--blue);
}
.choice.on {
  border-color: var(--blue);
  box-shadow: inset 0 0 0 1px var(--blue);
  background: var(--blue-wash);
}
.choice strong {
  color: var(--blue-deep);
}
.choice span {
  font-size: var(--fs-sm);
}
.choice small {
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.err {
  display: block;
  margin-top: 8px;
  color: var(--tone-overdue);
}
.wz-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 14px;
  border-top: 1px solid var(--rule);
}
.skip {
  background: none;
  border: 0;
  color: var(--ink-faint);
  font: inherit;
  font-size: var(--fs-sm);
  text-decoration: underline;
  cursor: pointer;
}
.spacer {
  flex: 1;
}
@media (max-width: 640px) {
  .choices {
    grid-template-columns: 1fr;
  }
  .steps li {
    padding: 6px 4px 8px;
    font-size: var(--fs-sm);
  }
  .steps li:not(.current) {
    font-size: 0; /* na mobile iba čísla, názov aktuálneho kroku */
  }
}
</style>
