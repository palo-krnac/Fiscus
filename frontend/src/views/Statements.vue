<script setup>
// Účtovná závierka v jednoduchom účtovníctve: Výkaz o príjmoch a výdavkoch
// (Úč FO 1-01) a Výkaz o majetku a záväzkoch (Úč FO 2-01), v celých eurách.
// Sumy počíta server; riadky bez podkladov v dokladoch sa zadávajú ručne.
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import Button from 'primevue/button'
import Select from 'primevue/select'
import InputText from 'primevue/inputtext'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { formatAmount } from '../lib/format.js'
import { session, can } from '../lib/session.js'
import { statementColumns } from '../lib/columns.js'
import ExportButton from '../components/ExportButton.vue'

const toast = useToast()
const canWrite = computed(() => can('ledger.write'))
const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
// v prvom polroku sa zostavuje závierka za minulý rok
const year = ref(new Date().getMonth() < 6 ? thisYear - 1 : thisYear)
const st = ref(null)
const loadError = ref('')
const inputs = ref({})
const busy = ref(false)

async function load() {
  try {
    st.value = await api.statements(session.companyId, year.value)
    inputs.value = Object.fromEntries(Object.entries(st.value.inputs).map(([k, v]) => [k, Number(v) ? String(v) : '']))
    loadError.value = ''
  } catch (e) {
    loadError.value = e.message
  }
}
watch(year, load)
onMounted(load)

const euro = (v) => formatAmount(v).replace(/,00$/, '')
const prevText = (r) => (st.value?.previousEmpty ? '' : euro(r.previous))

// riadky 08 a 17: z dokladov + doplnené ručne
const fromDocs = (r) => (r.input === 'r08x' ? st.value.facts.receivables : r.input === 'r17x' ? st.value.facts.payables : null)
const editable = (r) => canWrite.value && !!r.input

async function save() {
  busy.value = true
  try {
    const body = Object.fromEntries(Object.entries(inputs.value).map(([k, v]) => [k, String(v || '0').replace(/\s/g, '').replace(',', '.')]))
    await api.setStatementInputs(session.companyId, year.value, body)
    toast.add({ severity: 'success', summary: 'Zadané riadky uložené', life: 2000 })
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Neuložené', detail: e.fields?.[0]?.message ?? e.message, life: 5000 })
  } finally {
    busy.value = false
  }
}

const exportRows = computed(() =>
  st.value
    ? [
        ...st.value.income.map((r) => ({ ...r, statement: 'Výkaz o príjmoch a výdavkoch' })),
        ...st.value.assets.map((r) => ({ ...r, statement: 'Výkaz o majetku a záväzkoch' })),
        ...[
          { code: '39', label: 'Príjmy', current: st.value.taxB.income },
          { code: '40', label: 'Výdavky', current: st.value.taxB.expenses },
          { code: '41', label: 'Základ dane', current: st.value.taxB.profit },
          { code: '42', label: 'Strata', current: st.value.taxB.loss },
        ].map((r) => ({ ...r, previous: '', statement: 'DP typ B, VI. oddiel' })),
        ...st.value.taxB.table1a.map((r) => ({ ...r, code: `1a/${r.code}`, statement: 'DP typ B, tabuľka 1a (začiatok / koniec)' })),
      ]
    : [],
)
const exportCols = computed(() => statementColumns(st.value?.previousEmpty))
const printIt = () => window.print()
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Výkazy jednoduchého účtovníctva</h1>
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <ExportButton :name="`Výkazy JÚ ${year}`" :columns="exportCols" :rows="exportRows" />
      <Button label="Tlačiť" icon="pi pi-print" size="small" severity="secondary" outlined @click="printIt" />
    </header>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <template v-if="st">
      <p v-if="st.unclassified" class="warn">
        <i class="pi pi-exclamation-triangle" /> V peňažnom denníku za {{ year }} je {{ st.unclassified }} nerozpísaných pohybov — výkaz o príjmoch
        a výdavkoch ich nezahŕňa. <RouterLink to="/dennik">Rozpísať v denníku</RouterLink>
      </p>
      <p v-if="year === thisYear" class="note">Rok ešte neskončil — výkazy sú priebežné, stav k 31. 12. {{ year }} podľa doteraz zaúčtovaných dokladov.</p>

      <section class="sheet" aria-labelledby="inc-title">
        <header class="sheet-head">
          <h2 id="inc-title">Výkaz o príjmoch a výdavkoch</h2>
          <span>Úč FO 1-01 · za rok {{ year }} · v celých eurách</span>
        </header>
        <p class="who">{{ session.company?.name }}<template v-if="session.company?.ico"> · IČO {{ session.company.ico }}</template><template v-if="session.company?.dic"> · DIČ {{ session.company.dic }}</template></p>
        <table class="form">
          <thead>
            <tr><th class="code">r.</th><th>Názov</th><th class="num">Bežné obdobie</th><th class="num">Predchádzajúce obdobie</th></tr>
          </thead>
          <tbody>
            <tr v-for="r in st.income" :key="r.code" :class="{ sum: r.sum, result: r.code === '12' }">
              <td class="code">{{ r.code }}</td>
              <td>{{ r.label }}</td>
              <td class="num"><span :class="{ grand: r.code === '12' }">{{ euro(r.current) }}</span></td>
              <td class="num prev">{{ prevText(r) }}</td>
            </tr>
          </tbody>
        </table>
      </section>

      <section class="sheet" aria-labelledby="as-title">
        <header class="sheet-head">
          <h2 id="as-title">Výkaz o majetku a záväzkoch</h2>
          <span>Úč FO 2-01 · k 31. 12. {{ year }} · v celých eurách</span>
        </header>
        <table class="form">
          <thead>
            <tr><th class="code">r.</th><th>Názov</th><th class="num">Bežné obdobie</th><th class="num">Predchádzajúce obdobie</th></tr>
          </thead>
          <tbody>
            <template v-for="r in st.assets" :key="r.code">
              <tr v-if="r.code === '16'" class="group"><td /><td colspan="3">Záväzky</td></tr>
              <tr v-if="r.code === '01'" class="group"><td /><td colspan="3">Majetok</td></tr>
              <tr :class="{ sum: r.sum, result: r.code === '21' }">
                <td class="code">{{ r.code }}</td>
                <td>
                  {{ r.label }}
                  <small v-if="fromDocs(r) !== null" class="muted">z dokladov {{ formatAmount(fromDocs(r)) }} €<template v-if="editable(r)"> + doplnené</template></small>
                  <small v-else-if="['10', '11', '13'].includes(r.code)" class="muted">z peňažného denníka</small>
                </td>
                <td class="num">
                  <InputText v-if="editable(r)" v-model="inputs[r.input]" size="small" class="num cell-input" inputmode="decimal" placeholder="0"
                    :aria-label="`r. ${r.code} ${r.label}`" />
                  <span v-if="!editable(r) || fromDocs(r) !== null" :class="{ grand: r.code === '21', combined: editable(r) }">{{ euro(r.current) }}</span>
                </td>
                <td class="num prev">{{ prevText(r) }}</td>
              </tr>
            </template>
          </tbody>
        </table>
        <div v-if="canWrite" class="sheet-foot">
          <span class="muted">Dlhodobý majetok, zásoby, úvery a rezervy zadajte podľa inventúry k 31. 12. Pri r. 08 a 17 zadajte iba to, čo nie je vo faktúrach (napr. DPH, odvody, mzdy).</span>
          <Button label="Uložiť zadané riadky" icon="pi pi-check" size="small" :loading="busy" @click="save" />
        </div>
      </section>
      <section class="sheet" aria-labelledby="dp-title">
        <header class="sheet-head">
          <h2 id="dp-title">Podklady pre daňové priznanie typu B</h2>
          <span>VI. oddiel · zdaňovacie obdobie {{ year }} · v eurách s centami</span>
        </header>
        <table class="form">
          <thead><tr><th class="code">r.</th><th>Údaj</th><th class="num">Suma</th><th /></tr></thead>
          <tbody>
            <tr><td class="code">T1</td><td>Tabuľka č. 1, r. 2 (živnosť) – stĺpec 1 Príjmy</td><td class="num">{{ formatAmount(st.taxB.income) }}</td><td /></tr>
            <tr><td class="code">T1</td><td>Tabuľka č. 1, r. 2 (živnosť) – stĺpec 2 Výdavky</td><td class="num">{{ formatAmount(st.taxB.expenses) }}</td><td /></tr>
            <tr><td class="code">39</td><td>Príjmy (tab. 1, stĺ. 1, r. 10)</td><td class="num">{{ formatAmount(st.taxB.income) }}</td><td /></tr>
            <tr><td class="code">40</td><td>Výdavky (tab. 1, stĺ. 2, r. 10)</td><td class="num">{{ formatAmount(st.taxB.expenses) }}</td><td /></tr>
            <tr class="sum"><td class="code">41</td><td>Základ dane (kladný rozdiel r. 39 a 40)</td><td class="num">{{ formatAmount(st.taxB.profit) }}</td><td /></tr>
            <tr class="sum"><td class="code">42</td><td>Strata (záporný rozdiel r. 39 a 40)</td><td class="num">{{ formatAmount(st.taxB.loss) }}</td><td /></tr>
          </tbody>
        </table>
        <h3>Tabuľka č. 1a – údaje z účtovníctva</h3>
        <table class="form">
          <thead><tr><th class="code">r.</th><th>Údaj</th><th class="num">Na začiatku</th><th class="num">Na konci</th></tr></thead>
          <tbody>
            <tr v-for="r in st.taxB.table1a" :key="r.code">
              <td class="code">{{ r.code }}</td><td>{{ r.label }}</td>
              <td class="num prev">{{ formatAmount(r.previous) }}</td><td class="num">{{ formatAmount(r.current) }}</td>
            </tr>
          </tbody>
        </table>
        <p class="muted">
          Zaplatené poistné a príspevky v roku {{ year }} (stĺpec e4 denníka): {{ formatAmount(st.taxB.insurance) }} € — skontrolujte s potvrdeniami
          poisťovní. Ak príjmy nepatria pod živnosť, presuňte ich v priznaní do správneho riadku tabuľky č. 1. Úpravy základu dane
          (r. 43, 44) a odpočet straty sú na vás alebo na daňovom poradcovi. V r. 5 tabuľky 1a sú záväzky aj s úvermi.
        </p>
      </section>

      <p class="muted legal">Podľa opatrenia MF SR č. MF/27076/2007-74 v znení č. MF/15523/2014-74. Závierku podnikateľ – fyzická osoba ukladá do registra účtovných závierok v lehote na podanie daňového priznania.</p>
    </template>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 980px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
}
.spacer {
  flex: 1;
}
.warn,
.note {
  margin: 0;
  padding: 8px 10px;
  border-radius: var(--radius);
  font-size: var(--fs-sm);
}
.warn {
  background: var(--tone-overdue-bg);
  color: var(--tone-overdue);
}
.warn a {
  color: inherit;
  font-weight: 600;
}
.note {
  background: var(--blue-wash);
  color: var(--blue-deep);
}
.sheet {
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 2px solid var(--ink);
  padding: 10px 14px 12px;
}
.sheet-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
}
.sheet h3 {
  font-size: var(--fs-md);
  margin: 14px 0 2px;
}
.sheet-head h2 {
  font-size: var(--fs-lg);
  margin: 0;
}
.sheet-head span,
.who {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.who {
  margin: 2px 0 6px;
}
.form {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.form th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  padding: 6px 8px;
  border-bottom: 1px solid var(--ink);
}
.form td {
  padding: 5px 8px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: middle;
}
.form td small {
  display: block;
  font-size: var(--fs-xs);
}
.form .code {
  width: 2.5em;
  color: var(--blue-deep);
  font-weight: 600;
}
.form .num {
  text-align: right;
  white-space: nowrap;
  width: 11rem;
}
.form .prev {
  color: var(--ink-soft);
}
.form tr.sum td {
  font-weight: 600;
  background: var(--paper);
}
.form tr.group td {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  font-weight: 600;
  padding-top: 10px;
  border-bottom: 0;
}
.form tr.result td {
  border-top: 1px solid var(--ink);
  font-weight: 700;
}
.grand {
  border-bottom: 3px double currentColor;
  line-height: 1.15;
}
.cell-input {
  width: 8rem;
}
.cell-input :deep(input),
input.cell-input {
  text-align: right;
}
.combined {
  display: block;
  font-size: var(--fs-xs);
  color: var(--ink-soft);
}
.sheet-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.legal {
  margin: 0;
}
.error {
  color: var(--tone-overdue);
  margin: 0;
}
@media (max-width: 760px) {
  .page {
    padding: 10px 10px 16px;
  }
  .sheet {
    padding: 8px 8px 10px;
    overflow-x: auto;
  }
  .form .num {
    width: auto;
  }
  .form th:nth-child(4),
  .form td:nth-child(4) {
    display: none;
  }
  .cell-input {
    width: 6.5rem;
  }
}
@media print {
  :global(.sidebar),
  :global(.topbar),
  .head :deep(.p-button),
  .head :deep(.p-select),
  .sheet-foot,
  .warn,
  .note {
    display: none !important;
  }
  .page {
    max-width: none;
    padding: 0;
  }
  .sheet {
    border-color: #000;
    break-inside: avoid;
  }
}
</style>
