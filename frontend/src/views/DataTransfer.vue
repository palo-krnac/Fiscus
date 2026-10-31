<script setup>
// Prenos dát z iného programu: výber zdroja, náhľad (nič sa nezapíše) a prenos.
import { computed, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import Select from 'primevue/select'
import ImportWarnings from '../components/ImportWarnings.vue'
import { api, ApiError } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { toISODate } from '../lib/format.js'
import { downloadCSV } from '../lib/export.js'
import { kindLabel, maxImportBytes, newTotal, otherText, sources, summaryRows, tableKinds, templateExport } from '../lib/migrate.js'

const toast = useToast()
const canWrite = computed(() => can('invoices.write') && can('purchases.write') && can('contacts.write'))

const selected = ref(sources[0])
const fileInput = ref(null)
const file = ref(null)
const report = ref(null)
const busy = ref(false)
// faktúry so splatnosťou do dátumu budú uhradené (staré roky sú vyrovnané)
const paidOn = ref(false)
const paidUntil = ref(null)
// obsah tabuľky Excel/CSV ('auto' = rozpoznať z hlavičky)
const kind = ref('auto')
const needKind = ref(false)
const kindParam = computed(() => (selected.value.kinds && kind.value !== 'auto' ? kind.value : ''))
const templateKind = ref('invoices')
const templateKinds = tableKinds.filter((k) => k.value !== 'auto')
function downloadTemplate() {
  const { columns, rows: sample } = templateExport(templateKind.value)
  downloadCSV(`Vzor – ${kindLabel(templateKind.value)}`, columns, sample)
}
const paidISO = computed(() => (paidOn.value && paidUntil.value ? toISODate(paidUntil.value) : ''))

const rows = computed(() => summaryRows(report.value))
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
  paidOn.value = false
  paidUntil.value = null
  kind.value = 'auto'
  needKind.value = false
}

async function pick(e) {
  const f = e.target.files?.[0]
  e.target.value = ''
  if (!f) return
  if (f.size > maxImportBytes) {
    toast.add({ severity: 'error', summary: 'Súbor je príliš veľký', detail: 'Najviac 50 MB. Exportujte agendy po častiach.', life: 6000 })
    return
  }
  file.value = f
  await preview()
}

// náhľad prenosu; pri tabuľke s faktúrami treba zvoliť vydané alebo prijaté
async function preview() {
  if (!file.value) return
  busy.value = true
  needKind.value = false
  try {
    report.value = await api.importMigrate(session.companyId, selected.value.id, file.value, { kind: kindParam.value })
    if (selected.value.kinds && report.value.kind && kind.value === 'auto') kind.value = report.value.kind
  } catch (err) {
    report.value = null
    if (err instanceof ApiError && err.fieldMap().kind) {
      needKind.value = true
    } else {
      file.value = null
      toast.add({ severity: 'error', summary: 'Súbor sa nedá načítať', detail: err.message, life: 8000 })
    }
  } finally {
    busy.value = false
  }
}

async function run() {
  busy.value = true
  try {
    report.value = await api.importMigrate(session.companyId, selected.value.id, file.value, { apply: true, paidUntil: paidISO.value, kind: kindParam.value })
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
          Čísla dokladov ostanú. Čo už vo Fiškuse je, sa znova nevytvorí.
        </p>

        <div v-if="!canWrite" class="note">Na prenos potrebujete právo zapisovať faktúry, prijaté faktúry aj adresár.</div>
        <template v-else>
          <input ref="fileInput" type="file" :accept="selected.accept" class="visually-hidden" @change="pick" />
          <div class="actions">
            <Button icon="pi pi-upload" :label="file ? 'Vybrať iný súbor' : selected.pick ?? 'Vybrať súbor XML'" :severity="file ? 'secondary' : undefined"
              :outlined="!!file" size="small" :loading="busy && !file" @click="fileInput.click()" />
            <span v-if="file" class="file">{{ file.name }}</span>
          </div>
          <div v-if="selected.kinds" class="kind-row">
            <label for="tab-kind">V tabuľke sú</label>
            <Select v-model="kind" input-id="tab-kind" :options="tableKinds" option-label="label" option-value="value" size="small"
              class="kind-select" :invalid="needKind" @change="preview" />
            <span class="spacer" />
            <span class="tpl">
              <label for="tpl-kind" class="tpl-label">Vzor</label>
              <Select v-model="templateKind" input-id="tpl-kind" :options="templateKinds" option-label="label" option-value="value" size="small" class="kind-select" />
              <Button icon="pi pi-download" label="Stiahnuť" text size="small" v-tooltip.bottom="'Vzor tabuľky (CSV)'" @click="downloadTemplate" />
            </span>
          </div>
          <Message v-if="needKind" severity="warn" :closable="false">V tabuľke sú faktúry — vyberte, či sú vydané alebo prijaté.</Message>
          <p v-if="report?.columns?.length" class="hint">Rozpoznané stĺpce: {{ report.columns.join(', ') }}.</p>
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

          <div v-if="!report.done" class="paid">
            <label class="paid-check">
              <Checkbox v-model="paidOn" binary input-id="paid-on" />
              <span>Faktúry so splatnosťou do</span>
            </label>
            <DatePicker v-model="paidUntil" :disabled="!paidOn" date-format="d. m. yy" size="small" show-icon icon-display="input"
              placeholder="dátum" aria-label="Uhradené so splatnosťou do" class="paid-date" />
            <span class="paid-text">sú uhradené (vydané aj prijaté)</span>
          </div>
          <div v-if="!report.done" class="actions">
            <Button icon="pi pi-download" label="Preniesť do Fiškusu" size="small" :disabled="nothingNew || (paidOn && !paidUntil)" :loading="busy" @click="run" />
            <Button label="Zrušiť" text size="small" @click="reset" />
          </div>

          <ImportWarnings :warnings="report.warnings ?? []" />
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
.kind-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin: 0 0 10px;
  font-size: var(--fs-sm);
}
.kind-row .spacer {
  flex: 1;
}
.kind-select {
  min-width: 170px;
}
.tpl {
  display: flex;
  align-items: center;
  gap: 8px;
}
.tpl-label {
  color: var(--ink-soft);
}
.paid {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin: 12px 0 0;
  font-size: var(--fs-sm);
}
.paid-check {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
}
.paid-date {
  width: 150px;
}
.paid-text {
  color: var(--ink-soft);
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
}
</style>
