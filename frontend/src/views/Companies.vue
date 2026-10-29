<script setup>
// Firmy a licencia: založenie firmy (aj prvé spustenie) a licencia pre viac firiem.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Dialog from 'primevue/dialog'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { formatDate } from '../lib/format.js'
import { fillFromRegistry } from '../lib/registry.js'
import { session, selectCompany } from '../lib/session.js'
import FormField from '../components/FormField.vue'
import IcoInput from '../components/IcoInput.vue'
import ExportButton from '../components/ExportButton.vue'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { matches, searchTerms } from '../lib/search.js'
import { companyColumns } from '../lib/columns.js'

const router = useRouter()
const toast = useToast()
const server = computed(() => session.caps.mode === 'server')
const info = ref(null) // capabilities s licenciou (iba správca inštalácie)
const key = ref('')
const keyError = ref('')
const busy = ref(false)
const creating = ref(false)
const form = ref(null)
const errors = ref({})

const caps = computed(() => info.value ?? session.caps)
const lic = computed(() => info.value?.license ?? null)
const count = computed(() => session.companies.length)
const search = ref('')
const visible = computed(() => {
  const terms = searchTerms(search.value)
  return session.companies.filter((c) => matches(terms, [c.name, c.ico, c.dic, c.icdph, c.city]))
})
const canAdd = computed(() => !caps.value.maxCompanies || count.value < caps.value.maxCompanies)
const limitText = computed(() => {
  if (canAdd.value) return ''
  if (!server.value) return 'Desktopová verzia je pre jednu firmu. Viac firiem zvládne serverová verzia.'
  if (lic.value && !lic.value.valid) return 'Licencia vypršala — nové firmy založíte po jej obnovení.'
  if (caps.value.plan === 'multi') return `Licencia je na ${caps.value.maxCompanies} firiem a všetky sú využité.`
  return `Bezplatná serverová verzia je pre jednu firmu. Pre viac firiem nahrajte licenciu (${caps.value.price}).`
})

async function loadLicense() {
  if (!server.value) return
  try {
    info.value = await api.license()
  } catch {
    info.value = null
  }
}
onMounted(loadLicense)

async function upload() {
  keyError.value = ''
  if (!key.value.trim()) {
    keyError.value = 'Vložte licenčný kľúč'
    return
  }
  busy.value = true
  try {
    info.value = await api.setLicense(key.value.trim())
    session.caps = { ...session.caps, ...info.value, license: undefined }
    key.value = ''
    toast.add({ severity: 'success', summary: 'Licencia nahraná', detail: `Platí do ${formatDate(info.value.license.expires.slice(0, 10))}`, life: 4000 })
  } catch (e) {
    keyError.value = e.message
  } finally {
    busy.value = false
  }
}

function openCreate() {
  form.value = { name: '', ico: '', dic: '', icdph: '', street: '', city: '', zip: '', vatPayer: false }
  errors.value = {}
  creating.value = true
}

async function create() {
  busy.value = true
  errors.value = {}
  try {
    const c = await api.createCompany(form.value)
    session.companies.push(c)
    selectCompany(c.id)
    // nová firma: správca je jej členom → obnoviť práva
    session.me = await api.me()
    creating.value = false
    toast.add({ severity: 'success', summary: 'Firma založená', detail: c.name, life: 3000 })
    router.push('/nastavenia')
  } catch (e) {
    errors.value = e.fieldMap?.() ?? {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Firmu nemožno založiť', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>{{ server ? 'Firmy a licencia' : 'Firma' }}</h1>
      <span class="count">{{ count }}{{ caps.maxCompanies ? ` / ${caps.maxCompanies}` : '' }}</span>
      <div class="spacer" />
      <ExportButton v-if="count" name="Firmy" :columns="companyColumns" :rows="visible" />
      <Button size="small" icon="pi pi-plus" label="Založiť firmu" :disabled="!canAdd" @click="openCreate" />
    </header>

    <p v-if="!count" class="welcome">
      Vitajte vo Fiškuse. Začnite založením firmy — IČO stačí zadať a údaje doplníme z registra.
    </p>
    <p v-else-if="limitText" class="note">{{ limitText }}</p>

    <IconField v-if="count" class="search">
      <InputIcon class="pi pi-search" />
      <InputText v-model="search" placeholder="Hľadať firmu, IČO, obec" size="small" aria-label="Hľadať firmu" />
    </IconField>

    <table v-if="count" class="companies">
      <thead>
        <tr><th>Firma</th><th>IČO</th><th>IČ DPH</th><th aria-label="Akcie" /></tr>
      </thead>
      <tbody>
        <tr v-if="!visible.length"><td colspan="4" class="empty">Žiadna firma nevyhovuje hľadaniu.</td></tr>
        <tr v-for="c in visible" :key="c.id" :class="{ current: c.id === session.companyId }">
          <td class="name" data-label="Firma">{{ c.name }}</td>
          <td data-label="IČO">{{ c.ico || '—' }}</td>
          <td data-label="IČ DPH">{{ c.icdph || '—' }}</td>
          <td class="act">
            <span v-if="c.id === session.companyId" class="badge">aktuálna</span>
            <Button v-else label="Prepnúť" text size="small" @click="selectCompany(c.id)" />
          </td>
        </tr>
      </tbody>
    </table>

    <section class="plans" aria-label="Edícia">
      <h2>Edícia</h2>
      <div class="plan-grid">
        <div class="plan" :class="{ on: !server }">
          <strong>Desktop</strong>
          <span class="price">zadarmo</span>
          <small>jedna firma, offline, na vašom počítači</small>
        </div>
        <div class="plan" :class="{ on: server && caps.plan === 'free' }">
          <strong>Server – jedna firma</strong>
          <span class="price">zadarmo</span>
          <small>prístup z prehliadača, používatelia a roly</small>
        </div>
        <div class="plan" :class="{ on: server && caps.plan === 'multi' }">
          <strong>Server – viac firiem</strong>
          <span class="price">99 € <small>ročne</small></span>
          <small>pre účtovné kancelárie a poskytovateľov služieb</small>
          <a class="buy" href="https://fiskus.sk" target="_blank" rel="noopener">Kúpiť na fiskus.sk</a>
        </div>
      </div>
    </section>

    <section v-if="server && info" class="license" aria-label="Licencia">
      <h2>Licencia</h2>
      <dl v-if="lic" class="lic" :class="{ bad: !lic.valid }">
        <dt>Držiteľ</dt><dd>{{ lic.customer }}<span v-if="lic.email" class="muted"> · {{ lic.email }}</span></dd>
        <dt>Platí do</dt><dd>{{ formatDate(lic.expires.slice(0, 10)) }}</dd>
        <dt>Počet firiem</dt><dd>{{ lic.companies || 'bez obmedzenia' }}</dd>
        <dt>Stav</dt>
        <dd>
          <span :class="lic.valid ? 'ok' : 'err'">{{ lic.valid ? 'platná' : 'neplatná' }}</span>
          <span v-if="lic.warning" class="warn"> — {{ lic.warning }}</span>
        </dd>
      </dl>
      <p v-else class="muted">Bez licencie beží bezplatná verzia pre jednu firmu.</p>

      <form class="key" @submit.prevent="upload">
        <FormField :label="lic ? 'Nový licenčný kľúč (predĺženie)' : 'Licenčný kľúč'" :error="keyError" hint="Kľúč FISKUS1… dostanete e-mailom po zaplatení">
          <Textarea v-model="key" rows="3" auto-resize class="mono" spellcheck="false" placeholder="FISKUS1.…" />
        </FormField>
        <Button type="submit" label="Nahrať licenciu" icon="pi pi-key" size="small" :loading="busy" />
      </form>
    </section>

    <Dialog v-model:visible="creating" header="Založiť firmu" modal :style="{ width: '34rem' }" :breakpoints="{ '560px': '94vw' }">
      <form v-if="form" class="form-grid create" @submit.prevent="create">
        <FormField label="IČO" :error="errors.ico">
          <IcoInput v-model="form.ico" :invalid="!!errors.ico" auto @found="(c) => (form = fillFromRegistry(form, c))" />
        </FormField>
        <FormField label="DIČ" :error="errors.dic"><InputText v-model="form.dic" fluid /></FormField>
        <FormField label="Obchodné meno" :error="errors.name" :span="2"><InputText v-model="form.name" fluid /></FormField>
        <FormField label="Ulica" :error="errors.street" :span="2"><InputText v-model="form.street" fluid /></FormField>
        <FormField label="PSČ" :error="errors.zip"><InputText v-model="form.zip" fluid /></FormField>
        <FormField label="Obec" :error="errors.city"><InputText v-model="form.city" fluid /></FormField>
        <FormField label="IČ DPH" :error="errors.icdph" :span="2" hint="Vyplňte, ak je firma platiteľom DPH">
          <InputText v-model="form.icdph" fluid @update:model-value="(v) => (form.vatPayer = !!v)" />
        </FormField>
        <p class="hint">Ostatné údaje (IBAN, číslovanie, jednotky) doplníte v Nastaveniach.</p>
        <div class="actions">
          <Button label="Zrušiť" severity="secondary" text size="small" @click="creating = false" />
          <Button type="submit" label="Založiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  max-width: 980px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.head h1 {
  font-size: var(--fs-xl);
  margin: 0;
}
.count {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.spacer {
  flex: 1;
}
.welcome,
.note {
  margin: 0;
  padding: 8px 12px;
  background: var(--blue-wash);
  border-left: 3px solid var(--blue);
  font-size: var(--fs-md);
}
.note {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
h2 {
  font-size: var(--fs-md);
  margin: 6px 0 6px;
  color: var(--ink-soft);
}
.search {
  max-width: 320px;
}
.search :deep(input) {
  width: 100%;
}
.empty {
  color: var(--ink-faint);
}
.companies {
  width: 100%;
  border-collapse: collapse;
  background: var(--sheet);
  border: 1px solid var(--rule);
  font-size: var(--fs-md);
}
.companies th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
}
.companies td {
  padding: 4px 10px;
  border-bottom: 1px solid var(--rule-soft);
  font-variant-numeric: tabular-nums;
}
.companies tr.current .name {
  font-weight: 600;
}
.act {
  text-align: right;
  width: 110px;
}
.badge {
  font-size: var(--fs-xs);
  padding: 0 6px;
  border-radius: 8px;
  background: var(--blue-wash);
  color: var(--blue-deep);
}
.plan-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}
.plan {
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.plan.on {
  border-color: var(--blue);
  box-shadow: inset 0 3px 0 var(--blue-deep);
}
.plan .price {
  font-size: var(--fs-lg);
  font-weight: 700;
  color: var(--blue-deep);
  font-variant-numeric: tabular-nums;
}
.plan .price small {
  font-size: var(--fs-sm);
  font-weight: 400;
  color: var(--ink-soft);
}
.plan > small {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
}
.buy {
  margin-top: 4px;
  font-size: var(--fs-sm);
  color: var(--blue);
}
.lic {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 3px 14px;
  margin: 0 0 12px;
  font-size: var(--fs-md);
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 10px 12px;
}
.lic dt {
  color: var(--ink-faint);
}
.lic dd {
  margin: 0;
}
.ok {
  color: var(--tone-paid);
  font-weight: 600;
}
.err,
.warn {
  color: var(--tone-overdue);
}
.err {
  font-weight: 600;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.key {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  max-width: 640px;
}
.key :deep(.ff) {
  width: 100%;
}
.mono {
  font-family: ui-monospace, monospace;
  font-size: var(--fs-xs);
  width: 100%;
  word-break: break-all;
}
.create {
  grid-template-columns: 1fr 1fr;
}
.hint {
  grid-column: 1 / -1;
  margin: 0;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@media (max-width: 640px) {
  .plan-grid {
    grid-template-columns: 1fr;
  }
  .companies thead {
    display: none;
  }
  .companies,
  .companies tbody,
  .companies tr,
  .companies td {
    display: block;
    width: auto;
  }
  .companies tr {
    position: relative;
    padding: 6px 10px;
    border-bottom: 1px solid var(--rule);
  }
  .companies td {
    border: 0;
    padding: 0;
    font-size: var(--fs-sm);
  }
  .companies td.name {
    font-size: var(--fs-md);
  }
  .companies td[data-label='IČO']::before {
    content: 'IČO ';
    color: var(--ink-faint);
  }
  .companies td[data-label='IČ DPH']::before {
    content: 'IČ DPH ';
    color: var(--ink-faint);
  }
  .companies td.act {
    position: absolute;
    top: 6px;
    right: 6px;
    width: auto;
  }
}
</style>
