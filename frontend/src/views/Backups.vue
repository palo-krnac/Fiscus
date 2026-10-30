<script setup>
// Zálohy databázy: stav, ručná záloha, stiahnutie, naplánovaná obnova.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { api } from '../lib/api.js'
import { matches, searchTerms } from '../lib/search.js'
import { backupColumns } from '../lib/columns.js'
import ExportButton from '../components/ExportButton.vue'
import BackupSettingsForm from '../components/BackupSettingsForm.vue'
import BackupRemoteForm from '../components/BackupRemoteForm.vue'
import { remoteStateText, remoteTarget } from '../lib/backups.js'

const toast = useToast()
const confirm = useConfirm()
const st = ref(null)
const busy = ref(false)
const search = ref('')
const error = ref('')

async function load() {
  try {
    st.value = await api.backups()
    error.value = ''
  } catch (e) {
    error.value = e.message
  }
}
onMounted(load)

const fmtTime = new Intl.DateTimeFormat('sk-SK', { dateStyle: 'medium', timeStyle: 'short' })
const kb = (n) => (n >= 1048576 ? `${(n / 1048576).toFixed(1).replace('.', ',')} MB` : `${Math.max(1, Math.round(n / 1024))} kB`)
const visible = computed(() => {
  const terms = searchTerms(search.value)
  return (st.value?.backups ?? []).filter((b) => matches(terms, [b.name, fmtTime.format(new Date(b.time))]))
})
const last = computed(() => st.value?.backups?.[0])
const showSettings = ref(false)
function onSaved(s) {
  st.value = s
  showSettings.value = false
  showRemote.value = false
  cloud.value = null
}

// kópia v cloude
const showRemote = ref(false)
const remoteState = computed(() => remoteStateText(st.value?.remote?.state))
const cloud = ref(null) // zálohy v cloude (načítané na požiadanie)
const cloudBusy = ref('')
const cloudSearch = ref('')
const cloudVisible = computed(() => {
  const terms = searchTerms(cloudSearch.value)
  return (cloud.value ?? []).filter((b) => matches(terms, [b.name, fmtTime.format(new Date(b.time))]))
})
const localNames = computed(() => new Set((st.value?.backups ?? []).map((b) => b.name)))

async function loadCloud() {
  cloudBusy.value = 'list'
  try {
    cloud.value = await api.remoteBackups()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Zoznam z cloudu sa nenačítal', detail: e.message, life: 8000 })
  } finally {
    cloudBusy.value = ''
  }
}

async function fetchCloud(b) {
  cloudBusy.value = b.name
  try {
    await api.fetchRemoteBackup(b.name)
    toast.add({ severity: 'success', summary: 'Záloha stiahnutá z cloudu', detail: 'Nájdete ju v zozname nižšie, odkiaľ ju môžete obnoviť.', life: 6000 })
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Stiahnutie zlyhalo', detail: e.message, life: 8000 })
  } finally {
    cloudBusy.value = ''
  }
}

async function backupNow() {
  busy.value = true
  try {
    const it = await api.createBackup()
    toast.add({ severity: 'success', summary: 'Záloha vytvorená a overená', detail: it.name, life: 4000 })
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Záloha zlyhala', detail: e.message, life: 8000 })
  } finally {
    busy.value = false
  }
}

function restore(b) {
  confirm.require({
    header: 'Obnoviť zo zálohy',
    message: `Pri najbližšom spustení Fiškusu sa databáza nahradí zálohou z ${fmtTime.format(new Date(b.time))}. Zmeny urobené po tomto čase sa stratia; súčasná databáza sa uloží vedľa ako „.pred-obnovou“. Pokračovať?`,
    acceptLabel: 'Naplánovať obnovu',
    rejectLabel: 'Späť',
    acceptProps: { severity: 'danger', size: 'small' },
    rejectProps: { severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.restoreBackup(b.name)
        toast.add({ severity: 'warn', summary: 'Obnova naplánovaná', detail: 'Reštartujte Fiškus — obnova prebehne pri spustení.', life: 8000 })
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Obnovu nemožno naplánovať', detail: e.message, life: 6000 })
      }
    },
  })
}

async function cancelRestore() {
  try {
    await api.cancelRestore()
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 5000 })
  }
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Zálohy</h1>
      <div class="spacer" />
      <Button v-if="st?.enabled" label="Zálohovať teraz" icon="pi pi-save" size="small" :loading="busy" @click="backupNow" />
    </header>

    <p v-if="error" class="error">{{ error }}</p>

    <template v-if="st">
      <section v-if="!st.enabled" class="card off">
        <h2><i class="pi pi-exclamation-triangle" /> Zálohovanie nie je zapnuté</h2>
        <p>{{ st.reason }}</p>
        <template v-if="!st.settings">
          <pre>FISKUS_BACKUP_PASSWORD=… fiskus -db fiskus.db -backup-dir zalohy</pre>
          <p class="muted">Heslo si dobre uschovajte — bez neho sa zo zálohy nedá obnoviť.</p>
        </template>
      </section>
      <BackupSettingsForm v-if="!st.enabled && st.settings" :status="st" @saved="onSaved" />

      <template v-else>
        <section v-if="st.pending" class="card banner" role="status">
          <p>
            <i class="pi pi-history" /> Pri ďalšom spustení sa databáza obnoví zo zálohy <strong>{{ st.pending }}</strong>.
            Reštartujte Fiškus.
          </p>
          <Button label="Zrušiť obnovu" severity="secondary" outlined size="small" @click="cancelRestore" />
        </section>

        <section class="ledger">
          <div class="figure">
            <span class="label">Posledná záloha</span>
            <span class="value">{{ last ? fmtTime.format(new Date(last.time)) : '—' }}</span>
          </div>
          <div class="figure">
            <span class="label">Počet záloh</span>
            <span class="value">{{ st.backups.length }}</span>
          </div>
          <div class="figure">
            <span class="label">Uchováva sa</span>
            <span class="value small">{{ st.policy.Daily }} denných · {{ st.policy.Weekly }} týždenných · {{ st.policy.Monthly }} mesačných</span>
          </div>
        </section>
        <p class="muted">
          Zálohy sa robia automaticky pri spustení, raz denne a pri ukončení programu. Sú šifrované (AES-256) a pred uložením
          sa overí, že sa dajú obnoviť. Priečinok: <code>{{ st.dir }}</code>. Pre istotu si zálohu občas stiahnite aj mimo počítača.
          <Button v-if="st.settings && !showSettings" label="Zmeniť priečinok alebo heslo" text size="small" class="inline" @click="showSettings = true" />
        </p>
        <BackupSettingsForm v-if="st.settings && showSettings" :status="st" @saved="onSaved" />

        <section v-if="st.settings && !showRemote" class="card cloud" :class="'tone-' + (st.remote.kind ? remoteState.tone : 'none')">
          <div class="cloud-text">
            <h2><i class="pi pi-cloud" /> Kópia v cloude</h2>
            <template v-if="st.remote.kind">
              <p><code>{{ remoteTarget(st.remote) }}</code></p>
              <p class="state">{{ remoteState.text }}</p>
            </template>
            <p v-else>Vypnutá. Druhá kópia mimo počítača (Nextcloud, Synology, S3…) chráni aj pri krádeži alebo požiari.</p>
          </div>
          <div class="cloud-act">
            <Button v-if="st.remote.kind" label="Zálohy v cloude" severity="secondary" outlined size="small" :loading="cloudBusy === 'list'" @click="loadCloud" />
            <Button :label="st.remote.kind ? 'Zmeniť' : 'Nastaviť cloud'" :icon="st.remote.kind ? '' : 'pi pi-cloud-upload'" size="small"
              :severity="st.remote.kind ? 'secondary' : undefined" :text="!!st.remote.kind" @click="showRemote = true" />
          </div>
        </section>
        <BackupRemoteForm v-if="st.settings && showRemote" :remote="st.remote" @saved="onSaved" @cancel="showRemote = false" />

        <section v-if="cloud" class="cloud-list" aria-labelledby="cl-title">
          <div class="toolbar">
            <h2 id="cl-title">Zálohy v cloude</h2>
            <IconField class="search">
              <InputIcon class="pi pi-search" />
              <InputText v-model="cloudSearch" placeholder="Hľadať v cloude" size="small" aria-label="Hľadať v cloude" />
            </IconField>
            <div class="spacer" />
            <ExportButton name="Zálohy v cloude" :columns="backupColumns" :rows="cloudVisible" />
            <Button icon="pi pi-times" severity="secondary" text size="small" aria-label="Zavrieť zoznam z cloudu" @click="cloud = null" />
          </div>
          <table class="list">
            <thead>
              <tr><th>Čas</th><th>Súbor</th><th class="num">Veľkosť</th><th aria-label="Akcie" /></tr>
            </thead>
            <tbody>
              <tr v-if="!cloudVisible.length"><td colspan="4" class="empty">V cloude zatiaľ nie je žiadna záloha.</td></tr>
              <tr v-for="b in cloudVisible" :key="b.name">
                <td>{{ fmtTime.format(new Date(b.time)) }}</td>
                <td><code>{{ b.name }}</code></td>
                <td class="num">{{ kb(b.size) }}</td>
                <td class="act">
                  <span v-if="localNames.has(b.name)" class="have">aj v počítači</span>
                  <Button v-else label="Stiahnuť do počítača" text size="small" :loading="cloudBusy === b.name" :disabled="!!cloudBusy" @click="fetchCloud(b)" />
                </td>
              </tr>
            </tbody>
          </table>
        </section>

        <div class="toolbar">
          <h2 v-if="cloud" class="local-title">Zálohy v počítači</h2>
          <IconField class="search">
            <InputIcon class="pi pi-search" />
            <InputText v-model="search" placeholder="Hľadať zálohu" size="small" aria-label="Hľadať zálohu" />
          </IconField>
          <div class="spacer" />
          <ExportButton name="Zálohy" :columns="backupColumns" :rows="visible" />
        </div>

        <table class="list">
          <thead>
            <tr><th>Čas</th><th>Súbor</th><th class="num">Veľkosť</th><th aria-label="Akcie" /></tr>
          </thead>
          <tbody>
            <tr v-if="!visible.length"><td colspan="4" class="empty">Zatiaľ žiadna záloha.</td></tr>
            <tr v-for="b in visible" :key="b.name" :class="{ queued: st.pending === b.name }">
              <td>{{ fmtTime.format(new Date(b.time)) }}</td>
              <td><code>{{ b.name }}</code></td>
              <td class="num">{{ kb(b.size) }}</td>
              <td class="act">
                <a :href="api.backupURL(b.name)" class="p-button p-button-text p-button-sm" download>Stiahnuť</a>
                <Button label="Obnoviť" severity="danger" text size="small" @click="restore(b)" />
              </td>
            </tr>
          </tbody>
        </table>
      </template>
    </template>
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
.spacer {
  flex: 1;
}
.error {
  color: var(--tone-overdue);
  margin: 0;
}
.inline {
  padding: 0 4px;
  vertical-align: baseline;
}
.card {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 12px 14px;
}
.card h2 {
  font-size: var(--fs-md);
  margin: 0 0 6px;
}
.card p {
  margin: 0 0 6px;
}
.off {
  border-left: 3px solid var(--tone-draft);
}
.off h2 {
  color: var(--tone-draft);
}
.off pre {
  background: var(--paper);
  padding: 6px 8px;
  font-size: var(--fs-sm);
  white-space: pre-wrap;
  word-break: break-all;
}
.banner {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  border-left: 3px solid var(--tone-overdue);
  background: var(--tone-overdue-bg);
}
.banner p {
  margin: 0;
  flex: 1;
}
.ledger {
  display: flex;
  flex-wrap: wrap;
  border-top: 2px solid var(--ink);
  border-bottom: 1px solid var(--rule);
  background: var(--sheet);
}
.figure {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 8px 20px 9px 14px;
  border-right: 1px solid var(--rule);
}
.figure .label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.figure .value {
  font-size: var(--fs-lg);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.figure .value.small {
  font-size: var(--fs-md);
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  margin: 0;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
}
.search {
  width: 260px;
}
.search :deep(input) {
  width: 100%;
}
.list {
  width: 100%;
  border-collapse: collapse;
  background: var(--sheet);
  border: 1px solid var(--rule);
  font-size: var(--fs-md);
}
.list th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 5px 10px;
  border-bottom: 1px solid var(--rule);
}
.list td {
  padding: 3px 10px;
  border-bottom: 1px solid var(--rule-soft);
  font-variant-numeric: tabular-nums;
}
.list tr.queued td {
  background: var(--tone-overdue-bg);
}
.num {
  text-align: right;
}
.act {
  text-align: right;
  white-space: nowrap;
}
.act a {
  text-decoration: none;
}
.empty {
  color: var(--ink-faint);
}
.cloud {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  border-left: 3px solid var(--rule);
}
.cloud.tone-ok {
  border-left-color: var(--blue);
}
.cloud.tone-error {
  border-left-color: var(--tone-overdue);
}
.cloud.tone-error .state {
  color: var(--tone-overdue);
}
.cloud.tone-wait .state {
  color: var(--tone-draft);
}
.cloud-text {
  flex: 1 1 280px;
  min-width: 0;
}
.cloud-text code {
  word-break: break-all;
}
.cloud-text p:last-child {
  margin-bottom: 0;
}
.state {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.cloud-act {
  display: flex;
  gap: 6px;
}
.cloud-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.cloud-list h2,
.local-title {
  font-size: var(--fs-md);
  margin: 0;
}
.have {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  padding: 0 10px;
}
@media (max-width: 640px) {
  .cloud-list .toolbar {
    flex-wrap: wrap;
  }
  .cloud-list h2,
  .local-title {
    flex: 1 1 100%;
  }
  .toolbar {
    flex-wrap: wrap;
  }
  .search {
    flex: 1;
    width: auto;
  }
  .list th:nth-child(2),
  .list td:nth-child(2) {
    display: none;
  }
  .figure {
    flex: 1 1 45%;
  }
}
</style>
