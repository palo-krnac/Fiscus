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
        <pre>FISKUS_BACKUP_PASSWORD=… fiskus -db fiskus.db -backup-dir zalohy</pre>
        <p class="muted">Heslo si dobre uschovajte — bez neho sa zo zálohy nedá obnoviť.</p>
      </section>

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
        </p>

        <div class="toolbar">
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
@media (max-width: 640px) {
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
