<script setup>
// Prístup pre AI asistentov (MCP): API kľúče používateľa a návod na pripojenie.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import ToggleSwitch from 'primevue/toggleswitch'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { api } from '../lib/api.js'
import { formatDate } from '../lib/format.js'
import { can } from '../lib/session.js'
import { matches, searchTerms } from '../lib/search.js'
import { apiKeyColumns } from '../lib/columns.js'
import { claudeCodeCommand, mcpURL, stdioConfig } from '../lib/mcp.js'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'

const toast = useToast()
const confirm = useConfirm()
const keys = ref([])
const loading = ref(true)
const name = ref('Claude')
const readOnly = ref(true)
const nameError = ref('')
const busy = ref(false)
const created = ref(null) // { token, key } — zobrazí sa iba raz
const search = ref('')
const url = mcpURL()

async function load() {
  loading.value = true
  try {
    keys.value = await api.keys()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Kľúče', detail: e.message, life: 5000 })
  } finally {
    loading.value = false
  }
}
onMounted(load)

const visible = computed(() => {
  const terms = searchTerms(search.value)
  return keys.value.filter((k) => matches(terms, [k.name, k.prefix, k.readOnly ? 'len čítanie' : 'plné', k.revoked ? 'zrušený' : 'aktívny']))
})
const unixDate = (sec) => (sec ? formatDate(new Date(sec * 1000).toISOString().slice(0, 10)) : '—')

async function create() {
  nameError.value = ''
  busy.value = true
  try {
    created.value = await api.createKey(name.value, readOnly.value)
    await load()
  } catch (e) {
    nameError.value = e.fields?.[0]?.message ?? e.message
  } finally {
    busy.value = false
  }
}

async function copy(text, what) {
  try {
    await navigator.clipboard.writeText(text)
    toast.add({ severity: 'success', summary: `${what} skopírovaný`, life: 2000 })
  } catch {
    toast.add({ severity: 'warn', summary: 'Kopírovanie nie je dostupné — označte text a skopírujte ho ručne', life: 4000 })
  }
}

function revoke(k) {
  confirm.require({
    header: 'Zrušiť kľúč',
    message: `Zrušiť kľúč „${k.name}“ (${k.prefix}…)? Asistent, ktorý ho používa, stratí prístup okamžite.`,
    acceptLabel: 'Zrušiť kľúč',
    rejectLabel: 'Späť',
    acceptProps: { severity: 'danger', size: 'small' },
    rejectProps: { severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.revokeKey(k.id)
        if (created.value?.key.id === k.id) created.value = null
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Kľúč nemožno zrušiť', detail: e.message, life: 5000 })
      }
    },
  })
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Prístup pre AI</h1>
    </header>

    <p class="intro">
      AI asistent (napr. Claude) sa k Fiškusu pripojí cez <strong>MCP</strong> s vaším kľúčom. Vidí a smie presne to, čo vy
      vo svojich firmách. Kľúč <strong>len na čítanie</strong> mu nedovolí nič meniť. Každé volanie asistenta sa zapíše do
      <RouterLink v-if="can('audit.read')" to="/audit">auditu</RouterLink><template v-else>auditu</template>.
      Faktúry asistent vytvára ako koncepty a vystaví ich až na váš výslovný pokyn.
    </p>

    <section class="card">
      <h2>Nový kľúč</h2>
      <form class="new" @submit.prevent="create">
        <FormField label="Názov (kto ho bude používať)" :error="nameError">
          <InputText v-model="name" size="small" maxlength="60" />
        </FormField>
        <label class="switch">
          <ToggleSwitch v-model="readOnly" input-id="ro" />
          <span>
            <strong>Len na čítanie</strong>
            <small>{{ readOnly ? 'Asistent môže iba čítať doklady a výkazy.' : 'Asistent môže vytvárať koncepty, vystavovať faktúry a označovať úhrady.' }}</small>
          </span>
        </label>
        <Button type="submit" label="Vytvoriť kľúč" icon="pi pi-key" size="small" :loading="busy" />
      </form>

      <div v-if="created" class="created" role="status">
        <p class="once"><i class="pi pi-exclamation-triangle" /> Kľúč sa zobrazí <strong>iba teraz</strong>. Skopírujte si ho; neskôr ho už neuvidíte.</p>
        <div class="token">
          <code>{{ created.token }}</code>
          <Button icon="pi pi-copy" text size="small" aria-label="Kopírovať kľúč" @click="copy(created.token, 'Kľúč')" />
        </div>

        <h3>Claude Code</h3>
        <div class="snippet">
          <pre>{{ claudeCodeCommand(url, created.token) }}</pre>
          <Button icon="pi pi-copy" text size="small" aria-label="Kopírovať príkaz" @click="copy(claudeCodeCommand(url, created.token), 'Príkaz')" />
        </div>

        <h3>Claude Desktop a iné aplikácie (stdio)</h3>
        <p class="muted">Do konfigurácie MCP serverov pridajte (program <code>fiskus-mcp</code> je súčasťou inštalácie):</p>
        <div class="snippet">
          <pre>{{ stdioConfig(url, created.token) }}</pre>
          <Button icon="pi pi-copy" text size="small" aria-label="Kopírovať konfiguráciu" @click="copy(stdioConfig(url, created.token), 'Konfigurácia')" />
        </div>

        <h3>Iný klient</h3>
        <dl class="facts">
          <dt>Adresa</dt><dd><code>{{ url }}</code> (Streamable HTTP, POST)</dd>
          <dt>Hlavička</dt><dd><code>Authorization: Bearer {{ created.key.prefix }}…</code></dd>
        </dl>
      </div>
    </section>

    <section class="card">
      <div class="list-head">
        <h2>Moje kľúče <span class="muted">{{ keys.length }}</span></h2>
        <div class="spacer" />
        <IconField class="search">
          <InputIcon class="pi pi-search" />
          <InputText v-model="search" placeholder="Hľadať kľúč" size="small" aria-label="Hľadať kľúč" />
        </IconField>
        <ExportButton name="API kľúče" :columns="apiKeyColumns" :rows="visible" />
      </div>
      <table class="keys">
        <thead>
          <tr><th>Názov</th><th>Kľúč</th><th>Práva</th><th>Vytvorený</th><th>Naposledy použitý</th><th aria-label="Akcie" /></tr>
        </thead>
        <tbody>
          <tr v-if="!loading && !visible.length"><td colspan="6" class="empty">Zatiaľ žiadny kľúč.</td></tr>
          <tr v-for="k in visible" :key="k.id" :class="{ revoked: k.revoked }">
            <td class="name" data-label="Názov">{{ k.name }}</td>
            <td data-label="Kľúč"><code>{{ k.prefix }}…</code></td>
            <td data-label="Práva">{{ k.readOnly ? 'len čítanie' : 'ako používateľ' }}</td>
            <td data-label="Vytvorený">{{ unixDate(k.createdAt) }}</td>
            <td data-label="Použitý">{{ unixDate(k.lastUsedAt) }}</td>
            <td class="act">
              <span v-if="k.revoked" class="badge">zrušený</span>
              <Button v-else label="Zrušiť" severity="danger" text size="small" @click="revoke(k)" />
            </td>
          </tr>
        </tbody>
      </table>
    </section>
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
.head h1 {
  font-size: var(--fs-xl);
  margin: 0;
}
.intro {
  margin: 0;
  font-size: var(--fs-md);
  color: var(--ink-soft);
  max-width: 760px;
}
.intro a {
  color: var(--blue);
}
.card {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 12px 14px;
}
h2 {
  font-size: var(--fs-md);
  margin: 0 0 8px;
}
h3 {
  font-size: var(--fs-sm);
  margin: 12px 0 4px;
  color: var(--ink-soft);
}
.new {
  display: flex;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 12px 18px;
}
.new :deep(.ff) {
  width: 240px;
}
.switch {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  max-width: 360px;
}
.switch span {
  display: flex;
  flex-direction: column;
}
.switch small {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.created {
  margin-top: 14px;
  border-top: 1px solid var(--rule);
  padding-top: 10px;
}
.once {
  margin: 0 0 6px;
  color: var(--tone-draft);
  font-size: var(--fs-sm);
}
.token,
.snippet {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  background: var(--paper);
  border: 1px solid var(--rule);
  padding: 6px 8px;
}
.token code {
  font-size: var(--fs-md);
  word-break: break-all;
  flex: 1;
}
.snippet pre {
  margin: 0;
  flex: 1;
  font-size: var(--fs-xs);
  white-space: pre-wrap;
  word-break: break-all;
}
.facts {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 2px 12px;
  margin: 0;
  font-size: var(--fs-sm);
}
.facts dt {
  color: var(--ink-faint);
}
.facts dd {
  margin: 0;
  word-break: break-all;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.list-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.list-head h2 {
  margin: 0;
}
.spacer {
  flex: 1;
}
.search {
  width: 220px;
}
.search :deep(input) {
  width: 100%;
}
.keys {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.keys th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 5px 8px;
  border-bottom: 1px solid var(--rule);
}
.keys td {
  padding: 4px 8px;
  border-bottom: 1px solid var(--rule-soft);
}
.keys .revoked td {
  color: var(--ink-faint);
}
.keys .act {
  text-align: right;
}
.empty {
  color: var(--ink-faint);
}
.badge {
  font-size: var(--fs-xs);
  padding: 0 6px;
  border-radius: 8px;
  background: var(--tone-cancelled-bg);
  color: var(--tone-cancelled);
}
@media (max-width: 640px) {
  .new :deep(.ff) {
    width: 100%;
  }
  .search {
    width: auto;
    flex: 1;
  }
  .keys thead {
    display: none;
  }
  .keys,
  .keys tbody,
  .keys tr,
  .keys td {
    display: block;
  }
  .keys tr {
    position: relative;
    padding: 6px 0;
    border-bottom: 1px solid var(--rule);
  }
  .keys td {
    border: 0;
    padding: 1px 0;
    font-size: var(--fs-sm);
  }
  .keys td[data-label='Vytvorený']::before {
    content: 'vytvorený ';
    color: var(--ink-faint);
  }
  .keys td[data-label='Použitý']::before {
    content: 'naposledy použitý ';
    color: var(--ink-faint);
  }
  .keys td.name {
    font-weight: 600;
    font-size: var(--fs-md);
  }
  .keys td.act {
    position: absolute;
    top: 4px;
    right: 0;
  }
}
</style>
