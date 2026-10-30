<script setup>
// Odber kalendára termínov v Google Kalendári, Outlooku a v telefóne.
// Tajný odkaz sa ukáže iba raz (server drží len odtlačok); nový odkaz zruší starý.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { feedURLs, localOnly } from '../lib/calendar.js'

const visible = defineModel('visible', { type: Boolean, default: false })
const toast = useToast()
const status = ref(null)
const token = ref('')
const busy = ref(false)

const origin = window.location.origin
const local = localOnly(origin)
const urls = computed(() => (token.value ? feedURLs(origin, session.companyId, token.value) : null))

watch(visible, async (v) => {
  if (!v) {
    token.value = ''
    return
  }
  try {
    status.value = await api.calendarFeed()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Stav odberu sa nenačítal', detail: e.message, life: 6000 })
  }
})

async function create() {
  busy.value = true
  try {
    token.value = (await api.createCalendarFeed()).token
    status.value = await api.calendarFeed()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Odkaz sa nevytvoril', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

async function revoke() {
  busy.value = true
  try {
    await api.revokeCalendarFeed()
    token.value = ''
    status.value = await api.calendarFeed()
    toast.add({ severity: 'info', summary: 'Odkaz zrušený', detail: 'Kalendáre, ktoré ho odoberali, sa už neobnovia.', life: 5000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

async function copy(text) {
  try {
    await navigator.clipboard.writeText(text)
    toast.add({ severity: 'success', summary: 'Adresa skopírovaná', life: 2500 })
  } catch {
    toast.add({ severity: 'warn', summary: 'Kopírovanie nefunguje', detail: 'Označte adresu a skopírujte ju ručne.', life: 5000 })
  }
}

const fmt = (unix) => (unix ? new Date(unix * 1000).toLocaleString('sk-SK', { dateStyle: 'medium', timeStyle: 'short' }) : '')
</script>

<template>
  <Dialog v-model:visible="visible" modal header="Termíny v mobile a v kalendári" class="feed-dialog" :style="{ width: 'min(560px, 96vw)' }">
    <p class="lead">
      Daňové lehoty, splatnosti faktúr a vlastné termíny sa objavia v kalendári v telefóne, v Google Kalendári alebo
      v Outlooku a budú sa samy obnovovať. Pri lehotách prídu pripomienky 3 dni a 1 deň vopred.
    </p>

    <p v-if="local" class="warn">
      <i class="pi pi-info-circle" /> Fiškus beží iba na tomto počítači, Google ani telefón sa k nemu cez internet nedostanú.
      Odber funguje v kalendári na tomto počítači (Apple Kalendár, Outlook). Do telefónu termíny preneste
      <a :href="api.calendarICSURL(session.companyId)" download>súborom .ics</a> — ten sa však sám neobnovuje.
    </p>

    <template v-if="urls">
      <p class="once"><i class="pi pi-key" /> Adresu si skopírujte teraz — z bezpečnostných dôvodov sa už znova neukáže.</p>
      <label class="url">
        <span>Google Kalendár, Outlook (Pridať kalendár → Z adresy URL)</span>
        <span class="row">
          <InputText :model-value="urls.https" readonly size="small" fluid @focus="$event.target.select()" />
          <Button icon="pi pi-copy" severity="secondary" outlined size="small" aria-label="Kopírovať adresu" @click="copy(urls.https)" />
        </span>
      </label>
      <div class="apple">
        <a :href="urls.webcal" class="p-button p-button-sm p-button-secondary p-button-outlined"><i class="pi pi-apple" /> Pridať do Apple Kalendára (iPhone, Mac)</a>
      </div>
      <p class="muted">
        Kto má adresu, vidí vaše termíny vrátane mien zákazníkov a súm — neposielajte ju ďalej. Odkaz má iba vaše práva
        na čítanie a dá sa kedykoľvek zrušiť.
      </p>
    </template>

    <template v-else-if="status?.active">
      <p>
        <i class="pi pi-check-circle ok" /> Odber je zapnutý od {{ fmt(status.createdAt) }}.
        <template v-if="status.lastUsedAt">Naposledy si ho kalendár stiahol {{ fmt(status.lastUsedAt) }}.</template>
        <template v-else>Zatiaľ ho žiadny kalendár nestiahol.</template>
      </p>
      <p class="muted">Ak ste adresu stratili, vytvorte novú — stará prestane fungovať.</p>
    </template>

    <template #footer>
      <Button v-if="status?.active" label="Zrušiť odkaz" severity="danger" text size="small" :disabled="busy" @click="revoke" />
      <span class="spacer" />
      <Button v-if="!urls" :label="status?.active ? 'Vytvoriť novú adresu' : 'Vytvoriť adresu na odber'" icon="pi pi-link" size="small" :loading="busy" @click="create" />
      <Button v-else label="Hotovo" size="small" @click="visible = false" />
    </template>
  </Dialog>
</template>

<style scoped>
.lead {
  margin: 0 0 10px;
}
.warn {
  border-left: 3px solid var(--tone-draft);
  background: var(--tone-draft-bg);
  padding: 8px 10px;
  font-size: var(--fs-sm);
  margin: 0 0 10px;
}
.once {
  font-weight: 600;
  margin: 0 0 8px;
}
.url {
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.row {
  display: flex;
  gap: 6px;
}
.row :deep(input) {
  font-family: ui-monospace, monospace;
  font-size: var(--fs-xs);
}
.apple {
  margin: 10px 0 0;
}
.apple a {
  text-decoration: none;
  gap: 6px;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  margin: 10px 0 0;
}
.ok {
  color: var(--tone-paid);
}
.spacer {
  flex: 1;
}
</style>
