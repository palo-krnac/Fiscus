<script setup>
// Prepojenie s Google Kalendárom alebo Outlookom: Fiškus sám posiela
// termíny do kalendára „Fiškus – firma“ v účte (funguje aj v desktope).
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Checkbox from 'primevue/checkbox'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'

const toast = useToast()
const confirm = useConfirm()
const st = ref(null)
const busy = ref('')
const canManage = computed(() => can('settings.write'))
const showConfig = ref(false)
const cfg = ref({ googleClientId: '', googleClientSecret: '', microsoftClientId: '' })
const cfgErrors = ref({})
const removeCalendar = ref(true)

async function load() {
  try {
    st.value = await api.calendarSync(session.companyId)
    if (st.value.config) cfg.value = { googleClientId: st.value.config.googleClientId, googleClientSecret: '', microsoftClientId: st.value.config.microsoftClientId }
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Stav prepojenia sa nenačítal', detail: e.message, life: 6000 })
  }
}
onMounted(load)
watch(() => session.companyId, load)
defineExpose({ load })

async function connect(provider) {
  busy.value = provider
  try {
    const { url } = await api.connectCalendarSync(session.companyId, provider)
    window.location.href = url // po prihlásení sa vráti do kalendára
  } catch (e) {
    busy.value = ''
    toast.add({ severity: 'error', summary: 'Prepojenie sa nezačalo', detail: e.fields?.[0]?.message ?? e.message, life: 7000 })
  }
}

async function syncNow() {
  busy.value = 'sync'
  try {
    const r = await api.runCalendarSync(session.companyId)
    if (r.error) toast.add({ severity: 'warn', summary: 'Prenos s chybou', detail: r.error, life: 8000 })
    else toast.add({ severity: 'success', summary: 'Termíny prenesené', detail: `Nové ${r.created}, zmenené ${r.updated}, zmazané ${r.deleted}`, life: 4000 })
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Prenos sa nepodaril', detail: e.message, life: 7000 })
  } finally {
    busy.value = ''
  }
}

function disconnect() {
  confirm.require({
    header: 'Zrušiť prepojenie',
    message: `Fiškus prestane posielať termíny do ${st.value.label}.`,
    acceptLabel: 'Zrušiť prepojenie',
    rejectLabel: 'Späť',
    acceptProps: { severity: 'danger', size: 'small' },
    rejectProps: { severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.disconnectCalendarSync(session.companyId, removeCalendar.value)
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
      }
    },
  })
}

async function saveConfig() {
  busy.value = 'cfg'
  cfgErrors.value = {}
  try {
    await api.setCalendarSyncConfig(cfg.value)
    toast.add({ severity: 'success', summary: 'Uložené', life: 2500 })
    showConfig.value = false
    await load()
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) cfgErrors.value = e.fieldMap()
    else toast.add({ severity: 'error', summary: 'Neuložilo sa', detail: e.message, life: 6000 })
  } finally {
    busy.value = ''
  }
}

const fmt = (unix) => (unix ? new Date(unix * 1000).toLocaleString('sk-SK', { dateStyle: 'medium', timeStyle: 'short' }) : '')
const anyProvider = computed(() => st.value && (st.value.google || st.value.microsoft))
</script>

<template>
  <section v-if="st" class="sync">
    <h3>Google Kalendár alebo Outlook</h3>
    <template v-if="st.connected">
      <p>
        <i class="pi pi-check-circle ok" /> Termíny sa posielajú do <b>{{ st.label }}</b><template v-if="st.account"> ({{ st.account }})</template>,
        kalendár „Fiškus“. Prenášajú sa každých 30 minút, kým Fiškus beží.
      </p>
      <p class="muted">
        <template v-if="st.lastSync">Naposledy {{ fmt(st.lastSync) }}, {{ st.events }} termínov.</template>
        <template v-else>Zatiaľ neprenesené.</template>
      </p>
      <p v-if="st.lastError" class="err"><i class="pi pi-exclamation-triangle" /> {{ st.lastError }}</p>
      <div class="actions">
        <Button label="Preniesť teraz" icon="pi pi-sync" size="small" :loading="busy === 'sync'" @click="syncNow" />
        <span class="spacer" />
        <label v-if="canManage" class="check"><Checkbox v-model="removeCalendar" binary /> <span>zmazať aj kalendár v účte</span></label>
        <Button v-if="canManage" label="Zrušiť prepojenie" severity="danger" text size="small" @click="disconnect" />
      </div>
    </template>
    <template v-else>
      <p>
        Fiškus bude sám posielať daňové lehoty, splatnosti faktúr a vlastné termíny do samostatného kalendára „Fiškus“
        vo vašom účte — aj keď beží iba na tomto počítači. Pri lehotách nastaví pripomienky.
      </p>
      <div v-if="anyProvider && canManage" class="actions">
        <Button v-if="st.google" label="Prepojiť Google Kalendár" icon="pi pi-google" size="small" severity="secondary" outlined
          :loading="busy === 'google'" @click="connect('google')" />
        <Button v-if="st.microsoft" label="Prepojiť Outlook" icon="pi pi-microsoft" size="small" severity="secondary" outlined
          :loading="busy === 'microsoft'" @click="connect('microsoft')" />
      </div>
      <p v-else-if="anyProvider" class="muted">Kalendár môže prepojiť správca firmy.</p>
      <p v-else class="muted">
        Prepojenie ešte nie je v programe nastavené: chýbajú prihlasovacie údaje aplikácie Fiškus pre Google a Microsoft.
        <template v-if="st.config">Zadajte ich nižšie.</template><template v-else>Zadá ich hlavný správca.</template>
      </p>
    </template>

    <template v-if="st.config && !st.config.builtin">
      <Button :label="showConfig ? 'Skryť prihlasovacie údaje aplikácie' : 'Prihlasovacie údaje aplikácie (správca)'" text size="small"
        :icon="showConfig ? 'pi pi-chevron-up' : 'pi pi-cog'" class="cfg-toggle" @click="showConfig = !showConfig" />
      <form v-if="showConfig" class="cfg" @submit.prevent="saveConfig">
        <p class="muted">
          Google Cloud Console → Prihlasovacie údaje → OAuth klient typu <b>Počítačová aplikácia</b> (Desktop app), povoľte Google Calendar API.
          Microsoft Entra → Registrácia aplikácie → platforma <b>Mobilné a počítačové aplikácie</b> s adresou návratu
          <code>http://localhost/api/calendar-sync/callback</code>, povolenie Calendars.ReadWrite.
        </p>
        <FormField label="Google – ID klienta" :error="cfgErrors.googleClientId">
          <InputText v-model="cfg.googleClientId" size="small" fluid placeholder="…apps.googleusercontent.com" />
        </FormField>
        <FormField label="Google – tajomstvo klienta" :hint="st.config.googleSecretSet ? 'Uložené — vyplňte iba pri zmene' : ''">
          <InputText v-model="cfg.googleClientSecret" type="password" size="small" fluid autocomplete="off" />
        </FormField>
        <FormField label="Microsoft – ID aplikácie (klienta)">
          <InputText v-model="cfg.microsoftClientId" size="small" fluid placeholder="00000000-0000-0000-0000-000000000000" />
        </FormField>
        <div class="actions">
          <span class="spacer" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy === 'cfg'" />
        </div>
      </form>
    </template>
  </section>
</template>

<style scoped>
.sync {
  border: 1px solid var(--rule);
  background: var(--paper);
  padding: 10px 12px;
  margin: 0 0 12px;
}
.sync h3 {
  margin: 0 0 6px;
  font-size: var(--fs-md);
}
.sync p {
  margin: 0 0 8px;
  font-size: var(--fs-sm);
}
.actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.spacer {
  flex: 1;
}
.check {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.ok {
  color: var(--tone-paid);
}
.err {
  color: var(--tone-overdue);
}
.muted {
  color: var(--ink-faint);
}
.cfg-toggle {
  margin-top: 4px;
  padding-left: 0;
}
.cfg {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 6px;
}
.cfg code {
  font-size: var(--fs-xs);
  word-break: break-all;
}
</style>
