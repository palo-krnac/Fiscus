<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import ToggleSwitch from 'primevue/toggleswitch'
import AutoComplete from 'primevue/autocomplete'
import SelectButton from 'primevue/selectbutton'
import { codes, previewNumber } from '../lib/codes.js'
import FormField from '../components/FormField.vue'
import IcoInput from '../components/IcoInput.vue'
import { fillFromRegistry } from '../lib/registry.js'
import { api, ApiError } from '../lib/api.js'
import { session, replaceCompany } from '../lib/session.js'
import CompanyLogo from '../components/CompanyLogo.vue'
import EInvoiceSettings from '../components/EInvoiceSettings.vue'
import SecuritySettings from '../components/SecuritySettings.vue'

const toast = useToast()
const clone = (c) => ({ ...c, units: [...(c.units ?? [])], numberFormats: { ...(c.numberFormats ?? {}) } })
const form = ref(clone(session.company))
const errors = ref({})
const busy = ref(false)
const dirty = computed(() => JSON.stringify(form.value) !== JSON.stringify(session.company))

const seriesFields = [
  { kind: 'invoice', label: 'Faktúry' },
  { kind: 'credit_note', label: 'Dobropisy' },
  { kind: 'proforma', label: 'Zálohové faktúry' },
  { kind: 'advance_receipt', label: 'Daňové doklady k prijatej platbe' },
  { kind: 'quote', label: 'Cenové ponuky' },
  { kind: 'reminder', label: 'Upomienky' },
  { kind: 'purchase', label: 'Prijaté faktúry (interné číslo)' },
  { kind: 'cash_in', label: 'Príjmové pokladničné doklady' },
  { kind: 'cash_out', label: 'Výdavkové pokladničné doklady' },
]
const unitSuggestions = ref([])
function suggestUnits(e) {
  const q = e.query.trim().toLowerCase()
  const known = codes.defaultUnits.filter((u) => u.toLowerCase().startsWith(q) && !form.value.units.includes(u))
  // napísaná jednotka je prvá, aby ju Enter hneď pridal
  const typed = e.query.trim()
  unitSuggestions.value = typed && !known.some((u) => u.toLowerCase() === q) ? [typed, ...known] : known
}
function resetUnits() {
  form.value.units = [...codes.defaultUnits]
}

const companyFields = [
  { key: 'name', label: 'Obchodné meno', span: 2, autocomplete: 'organization' },
  { key: 'street', label: 'Ulica a číslo', span: 2, autocomplete: 'street-address' },
  { key: 'zip', label: 'PSČ', autocomplete: 'postal-code' },
  { key: 'city', label: 'Obec', autocomplete: 'address-level2' },
  { key: 'ico', label: 'IČO', inputmode: 'numeric' },
  { key: 'dic', label: 'DIČ', inputmode: 'numeric' },
  { key: 'email', label: 'E-mail', span: 2, autocomplete: 'email' },
  { key: 'phone', label: 'Telefón', span: 2, autocomplete: 'tel' },
]

// ---- logo ----
const logoInput = ref(null)
const logoBusy = ref(false)
const hasLogo = ref(false)
async function checkLogo() {
  try {
    const res = await fetch(api.logoURL(session.companyId, session.logoVersion), { method: 'HEAD', credentials: 'same-origin' })
    hasLogo.value = res.ok
  } catch {
    hasLogo.value = false
  }
}
onMounted(checkLogo)
async function uploadLogo(e) {
  const file = e.target.files?.[0]
  e.target.value = ''
  if (!file) return
  if (file.size > 512 * 1024) {
    toast.add({ severity: 'error', summary: 'Logo je príliš veľké', detail: 'Najviac 512 kB — zmenšite obrázok.', life: 5000 })
    return
  }
  logoBusy.value = true
  try {
    await api.uploadLogo(session.companyId, file)
    session.logoVersion++
    hasLogo.value = true
    toast.add({ severity: 'success', summary: 'Logo nahrané', life: 2500 })
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Logo sa nenahralo', detail: err.message, life: 6000 })
  } finally {
    logoBusy.value = false
  }
}
async function removeLogo() {
  logoBusy.value = true
  try {
    await api.deleteLogo(session.companyId)
    session.logoVersion++
    hasLogo.value = false
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Logo sa neodstránilo', detail: err.message, life: 6000 })
  } finally {
    logoBusy.value = false
  }
}

async function save() {
  busy.value = true
  try {
    const saved = await api.updateCompany(form.value)
    replaceCompany(saved)
    form.value = clone(saved)
    errors.value = {}
    toast.add({ severity: 'success', summary: 'Nastavenia uložené', life: 2500 })
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) {
      errors.value = e.fieldMap()
      toast.add({ severity: 'error', summary: 'Skontrolujte vyznačené polia', life: 4000 })
    } else {
      toast.add({ severity: 'error', summary: 'Uloženie zlyhalo', detail: e.message, life: 6000 })
    }
  } finally {
    busy.value = false
  }
}

function onKey(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    if (!busy.value) save()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="settings">
    <header class="bar">
      <h1>Nastavenia firmy</h1>
      <span v-if="dirty" class="unsaved">neuložené zmeny</span>
      <div class="spacer" />
      <Button label="Uložiť" icon="pi pi-check" size="small" :loading="busy" :disabled="!dirty" @click="save" v-tooltip.bottom="'Ctrl+S'" />
    </header>

    <div class="body">
      <fieldset class="form-section">
        <legend>Údaje na faktúre</legend>
        <div class="form-grid">
          <FormField v-for="f in companyFields" :key="f.key" :label="f.label" :span="f.span" :error="errors[f.key]">
            <IcoInput v-if="f.key === 'ico'" v-model="form.ico" :invalid="!!errors.ico" @found="(c) => (form = fillFromRegistry(form, c))" />
            <InputText v-else v-model="form[f.key]" size="small" :invalid="!!errors[f.key]" :inputmode="f.inputmode" :autocomplete="f.autocomplete" />
          </FormField>
        </div>
      </fieldset>

      <fieldset class="form-section">
        <legend>Logo na dokladoch</legend>
        <div class="logo-row">
          <div class="logo-box">
            <CompanyLogo height="64px" />
            <span v-if="!hasLogo" class="logo-empty">bez loga</span>
          </div>
          <div class="logo-actions">
            <input ref="logoInput" type="file" accept="image/png,image/jpeg,image/webp" class="visually-hidden" @change="uploadLogo" />
            <Button :label="hasLogo ? 'Zmeniť logo' : 'Nahrať logo'" icon="pi pi-upload" severity="secondary" outlined size="small" :loading="logoBusy" @click="logoInput.click()" />
            <Button v-if="hasLogo" label="Odstrániť" severity="danger" text size="small" :disabled="logoBusy" @click="removeLogo" />
            <small class="help">PNG, JPEG alebo WebP do 512 kB. Zobrazí sa na faktúre, ponuke, upomienke a pokladničnom doklade.</small>
          </div>
        </div>
      </fieldset>

      <fieldset class="form-section">
        <legend>DPH</legend>
        <label class="switch">
          <ToggleSwitch v-model="form.vatPayer" input-id="vat-payer" />
          <span>
            <strong>Som platiteľ DPH</strong>
            <small>Ak nie ste platiteľ, faktúry sa vystavujú bez DPH a s poznámkou „Nie som platiteľ DPH“.</small>
          </span>
        </label>
        <div v-if="form.vatPayer" class="form-grid vat-grid">
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
          <FormField label="Daňový úrad" :error="errors.taxOffice" :span="2" hint="Do hlavičky priznania k DPH">
            <InputText v-model="form.taxOffice" size="small" placeholder="Daňový úrad Banská Bystrica" />
          </FormField>
        </div>
      </fieldset>

      <fieldset class="form-section">
        <legend>Číselné rady</legend>
        <p class="help">
          Zástupné znaky: <code>{RRRR}</code> rok, <code>{RR}</code> rok dvojciferne, <code>{MM}</code> mesiac,
          <code>{NNNN}</code> poradové číslo (počet N = počet číslic). Poradie sa každý rok čísluje od 1.
        </p>
        <div class="form-grid">
          <FormField
            v-for="f in seriesFields"
            :key="f.kind"
            :label="f.label"
            :error="errors[`numberFormats.${f.kind}`]"
            :hint="`Napr. ${previewNumber(form.numberFormats[f.kind] || codes.defaultFormats[f.kind])}`"
          >
            <InputText
              v-model="form.numberFormats[f.kind]"
              size="small"
              :placeholder="codes.defaultFormats[f.kind]"
              :invalid="!!errors[`numberFormats.${f.kind}`]"
              spellcheck="false"
            />
          </FormField>
        </div>
      </fieldset>

      <fieldset class="form-section">
        <legend>Merné jednotky</legend>
        <FormField label="Jednotky ponúkané pri položkách faktúry" :error="errors.units" hint="Novú jednotku napíšte a potvrďte Enterom">
          <AutoComplete
            v-model="form.units"
            multiple
            :typeahead="true"
            :auto-option-focus="true"
            :suggestions="unitSuggestions"
            :show-empty-message="false"
            fluid
            size="small"
            @complete="suggestUnits"
          />
        </FormField>
        <button type="button" class="link" @click="resetUnits">Obnoviť predvolené jednotky</button>
      </fieldset>

      <fieldset class="form-section">
        <legend>Platby</legend>
        <div class="form-grid">
          <FormField label="IBAN" :span="2" :error="errors.iban" hint="Zobrazí sa na faktúre a v QR kóde">
            <InputText v-model="form.iban" size="small" :invalid="!!errors.iban" autocomplete="off" />
          </FormField>
          <FormField label="Predvolená splatnosť" :error="errors.defaultDueDays" hint="Dá sa zmeniť pri každom odberateľovi">
            <InputNumber v-model="form.defaultDueDays" :min="0" :max="365" suffix=" dní" size="small" :invalid="!!errors.defaultDueDays" />
          </FormField>
        </div>
      </fieldset>

      <EInvoiceSettings />
      <SecuritySettings />
    </div>
  </div>
</template>

<style scoped>
.logo-row {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}
.logo-box {
  display: grid;
  place-items: center;
  min-width: 160px;
  height: 80px;
  padding: 8px;
  background: #fff;
  border: 1px dashed var(--rule);
}
.logo-empty {
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.logo-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  max-width: 460px;
}
.logo-actions .help {
  flex-basis: 100%;
}
.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
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
}
.unsaved {
  font-size: var(--fs-sm);
  color: var(--tone-draft);
}
.spacer {
  flex: 1;
}
.body {
  max-width: 880px;
  padding: 14px 16px 32px;
}
.switch {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  cursor: pointer;
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
.help {
  margin: 0 0 10px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  max-width: 72ch;
}
.help code {
  font-size: var(--fs-xs);
  background: var(--rule-soft);
  padding: 1px 4px;
  border-radius: 3px;
}
.link {
  margin-top: 6px;
  font: inherit;
  font-size: var(--fs-sm);
  border: 0;
  background: none;
  padding: 0;
  color: var(--blue);
  cursor: pointer;
}
.vat-grid {
  margin-top: 12px;
}
@media (max-width: 760px) {
  .bar {
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
