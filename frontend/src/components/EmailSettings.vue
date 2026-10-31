<script setup>
// Nastavenie e-mailu firmy: odosielateľ, server SMTP (heslo sa iba zadáva)
// a texty správ. V Nastaveniach → E-mail; zoznam správ je v module E-maily.
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Password from 'primevue/password'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { guessProvider, placeholders, portFor, securityOptions, templateKinds } from '../lib/emails.js'
import FormField from './FormField.vue'

const toast = useToast()
const canSettings = ref(can('settings.write'))
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
  }
}


function onKey(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    if (!saving.value && form.value) saveSettings()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="save-row">
    <Button label="Uložiť nastavenie e-mailu" icon="pi pi-check" size="small" :loading="saving" :disabled="!form" @click="saveSettings" v-tooltip.bottom="'Ctrl+S'" />
  </div>
    <div v-if="form" class="body">
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
          <FormField label="Upozornenia na koncepty na" :span="2" :error="errors.notifyDrafts"
            hint="Keď pravidelná fakturácia pripraví koncept na kontrolu, príde sem e-mail. Prázdne = neposielať.">
            <InputText v-model="form.notifyDrafts" size="small" inputmode="email" placeholder="napr. uctovnictvo@firma.sk" :invalid="!!errors.notifyDrafts" />
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
</template>

<style scoped>
.save-row {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 8px;
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
  .span2 {
    grid-column: span 1;
  }
}
</style>
