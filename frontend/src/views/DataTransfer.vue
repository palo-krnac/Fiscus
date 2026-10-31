<script setup>
// Prenos dát z iného programu: výber zdroja, náhľad (nič sa nezapíše) a prenos.
import { computed, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Message from 'primevue/message'
import ExportButton from '../components/ExportButton.vue'
import { api } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { importWarningColumns } from '../lib/columns.js'
import { filterWarnings, maxImportBytes, newTotal, otherText, sources, summaryRows } from '../lib/migrate.js'

const toast = useToast()
const canWrite = computed(() => can('invoices.write') && can('purchases.write') && can('contacts.write'))

const selected = ref(sources[0])
const fileInput = ref(null)
const file = ref(null)
const report = ref(null)
const busy = ref(false)
const search = ref('')

const rows = computed(() => summaryRows(report.value))
const warnings = computed(() => filterWarnings(report.value?.warnings, search.value))
const other = computed(() => otherText(report.value?.other))
const nothingNew = computed(() => report.value && !report.value.done && newTotal(report.value) === 0)

function choose(s) {
  if (!s.ready) return
  selected.value = s
  reset()
}
function reset() {
  file.value = null
  report.value = null
  search.value = ''
}

async function pick(e) {
  const f = e.target.files?.[0]
  e.target.value = ''
  if (!f) return
  if (f.size > maxImportBytes) {
    toast.add({ severity: 'error', summary: 'Súbor je príliš veľký', detail: 'Najviac 50 MB. Exportujte agendy po častiach.', life: 6000 })
    return
  }
  busy.value = true
  try {
    report.value = await api.importPohoda(session.companyId, f)
    file.value = f
  } catch (err) {
    report.value = null
    toast.add({ severity: 'error', summary: 'Súbor sa nedá načítať', detail: err.message, life: 8000 })
  } finally {
    busy.value = false
  }
}

async function run() {
  busy.value = true
  try {
    report.value = await api.importPohoda(session.companyId, file.value, true)
    toast.add({ severity: 'success', summary: 'Dáta prenesené', detail: `${newTotal(report.value)} nových záznamov`, life: 5000 })
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Prenos zlyhal', detail: err.message, life: 8000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="transfer">
    <header class="head">
      <h1>Prenos dát</h1>
      <p class="lead">Prechádzate z iného programu? Načítajte jeho export – najprv uvidíte, čo pribudne, a až potom sa niečo zapíše.</p>
    </header>

    <div class="layout">
      <nav class="sources" aria-label="Zdroj dát">
        <button
          v-for="s in sources"
          :key="s.id"
          type="button"
          class="source"
          :class="{ active: selected.id === s.id, soon: !s.ready }"
          :disabled="!s.ready"
          :aria-pressed="selected.id === s.id"
          @click="choose(s)"
        >
          <span class="name">{{ s.name }}</span>
          <span v-if="!s.ready" class="soon-tag">pripravuje sa</span>
        </button>
      </nav>

      <section class="work">
        <h2>{{ selected.name }}</h2>
        <p class="hint">{{ selected.hint }}</p>
        <p class="hint">
          Prenesú sa kontakty, skladové karty (do cenníka), vydané faktúry s dobropismi a prijaté faktúry v eurách.
          Čísla dokladov ostanú, uhradené faktúry budú označené ako uhradené. Čo už vo Fiškuse je, sa znova nevytvorí.
        </p>

        <div v-if="!canWrite" class="note">Na prenos potrebujete právo zapisovať faktúry, prijaté faktúry aj adresár.</div>
        <template v-else>
          <input ref="fileInput" type="file" :accept="selected.accept" class="visually-hidden" @change="pick" />
          <div class="actions">
            <Button icon="pi pi-upload" :label="file ? 'Vybrať iný súbor' : 'Vybrať súbor XML'" :severity="file ? 'secondary' : undefined"
              :outlined="!!file" size="small" :loading="busy && !file" @click="fileInput.click()" />
            <span v-if="file" class="file">{{ file.name }}</span>
          </div>
        </template>

        <template v-if="report">
          <Message v-if="report.done" severity="success" :closable="false">Prenos je hotový. Doklady nájdete vo Vydaných a Prijatých faktúrach.</Message>
          <Message v-else-if="nothingNew" severity="info" :closable="false">V súbore nie je nič nové – všetko už vo Fiškuse je.</Message>

          <table class="summary">
            <thead>
              <tr>
                <th>Agenda</th>
                <th class="num">{{ report.done ? 'Prenesené' : 'Pribudne' }}</th>
                <th class="num">Už je</th>
                <th class="num">Preskočené</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in rows" :key="r.key">
                <td>{{ r.label }}</td>
                <td class="num strong">{{ r.new }}</td>
                <td class="num">{{ r.existing }}</td>
                <td class="num" :class="{ warn: r.skipped }">{{ r.skipped }}</td>
              </tr>
            </tbody>
          </table>
          <p v-if="other" class="hint">V súbore sú aj ďalšie agendy, ktoré sa neprenášajú: {{ other }}.</p>

          <div v-if="!report.done" class="actions">
            <Button icon="pi pi-download" label="Preniesť do Fiškusu" size="small" :disabled="nothingNew" :loading="busy" @click="run" />
            <Button label="Zrušiť" text size="small" @click="reset" />
          </div>

          <section v-if="report.warnings?.length" class="warnings">
            <div class="warn-head">
              <h3>Upozornenia <span class="count">{{ report.warnings.length }}</span></h3>
              <IconField class="search">
                <InputIcon class="pi pi-search" />
                <InputText v-model="search" placeholder="Hľadať v upozorneniach" size="small" aria-label="Hľadať v upozorneniach" />
              </IconField>
              <ExportButton name="Prenos dát – upozornenia" :columns="importWarningColumns" :rows="warnings" />
            </div>
            <ol>
              <li v-for="w in warnings" :key="w.n" :value="w.n">{{ w.text }}</li>
            </ol>
            <p v-if="!warnings.length" class="hint">Nič sa nenašlo.</p>
          </section>
        </template>
      </section>
    </div>
  </div>
</template>

<style scoped>
.transfer {
  padding: 16px 20px 32px;
  max-width: 1040px;
}
.head h1 {
  font-size: var(--fs-xl);
  font-weight: 600;
  color: var(--blue-deep);
  margin: 0;
}
.lead {
  margin: 4px 0 16px;
  color: var(--ink-soft);
  font-size: var(--fs-md);
  max-width: 70ch;
}
.layout {
  display: grid;
  grid-template-columns: 200px 1fr;
  gap: 20px;
  align-items: start;
}
.sources {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  background: var(--sheet);
}
.source {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
  padding: 8px 12px;
  border: 0;
  border-bottom: 1px solid var(--rule-soft);
  background: none;
  font: inherit;
  font-size: var(--fs-md);
  color: var(--ink);
  text-align: left;
  cursor: pointer;
}
.source:last-child {
  border-bottom: 0;
}
.source.active {
  background: var(--blue-wash);
  color: var(--blue-deep);
  font-weight: 600;
  box-shadow: inset 3px 0 0 var(--blue);
}
.source.soon {
  color: var(--ink-faint);
  cursor: default;
}
.source:focus-visible {
  outline: 2px solid var(--blue);
  outline-offset: -2px;
}
.soon-tag {
  font-size: var(--fs-xs);
  font-weight: 400;
}
.work {
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  padding: 14px 16px 18px;
  min-width: 0;
}
.work h2 {
  margin: 0 0 6px;
  font-size: var(--fs-lg);
  font-weight: 600;
  color: var(--blue-deep);
}
.hint {
  margin: 0 0 8px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  max-width: 72ch;
}
.note {
  font-size: var(--fs-sm);
  color: var(--tone-draft);
  background: var(--tone-draft-bg);
  padding: 8px 10px;
  border-radius: var(--radius);
}
.actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 12px 0;
  flex-wrap: wrap;
}
.file {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  overflow-wrap: anywhere;
}
.summary {
  width: 100%;
  max-width: 560px;
  border-collapse: collapse;
  font-size: var(--fs-md);
  font-variant-numeric: tabular-nums;
  margin: 12px 0 8px;
}
.summary th {
  text-align: left;
  font-weight: 600;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  border-bottom: 1px solid var(--rule);
  padding: 4px 8px;
}
.summary td {
  padding: 5px 8px;
  border-bottom: 1px solid var(--rule-soft);
}
.summary .num {
  text-align: right;
  width: 90px;
}
.summary .strong {
  font-weight: 600;
  color: var(--blue-deep);
}
.summary .warn {
  color: var(--tone-draft);
  font-weight: 600;
}
.warnings {
  margin-top: 16px;
  border-top: 1px solid var(--rule);
  padding-top: 12px;
}
.warn-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.warn-head h3 {
  margin: 0;
  font-size: var(--fs-md);
  font-weight: 600;
  margin-right: auto;
}
.count {
  color: var(--ink-faint);
  font-weight: 400;
}
.warnings ol {
  margin: 10px 0 0;
  padding-left: 2.2em;
  font-size: var(--fs-sm);
  max-height: 320px;
  overflow: auto;
}
.warnings li {
  padding: 2px 0;
}
.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
}
@media (max-width: 720px) {
  .transfer {
    padding: 12px 16px 24px;
  }
  .layout {
    grid-template-columns: 1fr;
    gap: 12px;
  }
  .sources {
    flex-direction: row;
    overflow-x: auto;
  }
  .source {
    flex: none;
    flex-direction: column;
    align-items: flex-start;
    gap: 0;
    border-bottom: 0;
    border-right: 1px solid var(--rule-soft);
  }
  .source.active {
    box-shadow: inset 0 -3px 0 var(--blue);
  }
  .summary .num {
    width: auto;
  }
  .warn-head .search {
    flex: 1 1 100%;
    order: 3;
  }
}
</style>
