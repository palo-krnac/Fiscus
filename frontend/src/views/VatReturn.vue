<script setup>
// Priznanie k DPH (DPHv25) a kontrolný výkaz (KVDPHv25). Všetky sumy počíta
// backend (/api/companies/{cid}/vat); rozhranie ich iba zobrazuje.
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { codes } from '../lib/codes.js'
import { formatAmount, formatDate, formatQuantity } from '../lib/format.js'
import { session } from '../lib/session.js'
import { defaultPeriod, dueDate, periodOptions } from '../lib/vat.js'
import { matches, searchTerms } from '../lib/search.js'
import { kvColumns, svColumns, vatRowColumns } from '../lib/columns.js'
import { saveBlob } from '../lib/export.js'
import ExportButton from '../components/ExportButton.vue'

const props = defineProps({ tab: { type: String, default: 'dph' } })
const router = useRouter()
const toast = useToast()

const KEY = `fiskus.vatPeriodKind.${session.companyId}`
function storedKind() {
  try {
    return localStorage.getItem(KEY)
  } catch {
    return null
  }
}
const kind = ref(storedKind() || session.company?.vatPeriod || 'month')
const kinds = [
  { label: 'Mesiac', value: 'month' },
  { label: 'Štvrťrok', value: 'quarter' },
]
const options = computed(() => periodOptions(kind.value))
const period = ref(defaultPeriod(kind.value))
const excess = ref('')
const data = ref(null)
const error = ref('')
const loading = ref(false)
const showAll = ref(false)
const search = ref('')
const busyXML = ref('')

watch(kind, (k) => {
  try {
    localStorage.setItem(KEY, k)
  } catch {
    /* ignorovať */
  }
  period.value = defaultPeriod(k)
})

async function load() {
  if (!period.value) return
  loading.value = true
  try {
    data.value = await api.vat(session.companyId, period.value, excess.value.trim().replace(',', '.'))
    error.value = ''
  } catch (e) {
    data.value = null
    error.value = e.message
  } finally {
    loading.value = false
  }
}
let timer
watch([period, excess], () => {
  clearTimeout(timer)
  timer = setTimeout(load, 200)
})
onMounted(load)

const rows = computed(() => data.value?.rows ?? {})
const val = (code) => rows.value[code] ?? '0'

// riadky tlačiva po sekciách; predvolene iba vyplnené a súčtové
const sections = computed(() => {
  const out = []
  for (const def of codes.vatRows ?? []) {
    const value = rows.value[def.code]
    if (!showAll.value && !value && !def.total) continue
    let s = out.find((x) => x.name === def.section)
    if (!s) out.push((s = { name: def.section, rows: [] }))
    s.rows.push({ ...def, value: value ?? '' })
  }
  return out
})
const flatRows = computed(() => sections.value.flatMap((s) => s.rows))

const result = computed(() => {
  if (rows.value.r33) return { label: 'Nadmerný odpočet', value: rows.value.r33, tone: 'paid' }
  return { label: 'Vlastná daňová povinnosť na úhradu', value: rows.value.r35 ?? rows.value.r32 ?? '0', tone: 'issued' }
})

// kontrolný výkaz
const kvSections = computed(() => {
  const kv = data.value?.kv
  if (!kv) return []
  const terms = searchTerms(search.value)
  const f = (list, section) =>
    list
      .map((x) => ({ ...x, section }))
      .filter((x) => matches(terms, [x.partner, x.name, x.number, x.original, x.base, x.vat, x.baseDiff, x.label, x.code, x.goods]))
  return [
    { code: 'A.1', title: 'Vydané faktúry platiteľom DPH', rows: f(kv.a1, 'A.1'), kind: 'invoice' },
    { code: 'A.2', title: 'Vydané faktúry – prenesenie daňovej povinnosti (§ 69 ods. 12)', rows: f(kv.a2 ?? [], 'A.2'), kind: 'invoice', a2: true },
    { code: 'B.1', title: 'Prijaté faktúry – prenesenie daňovej povinnosti', rows: f(kv.b1, 'B.1'), kind: 'purchase' },
    { code: 'B.2', title: 'Prijaté faktúry s odpočtom DPH', rows: f(kv.b2, 'B.2'), kind: 'purchase' },
    { code: 'C.1', title: 'Vydané opravné faktúry (dobropisy) platiteľom', rows: f(kv.c1, 'C.1'), kind: 'invoice', correction: true },
  ]
})
const kvAll = computed(() => kvSections.value.flatMap((s) => s.rows))

// súhrnný výkaz
const SV_CODES = { 0: 'tovar', 1: 'trojstranný obchod', 2: 'služby' }
const svRows = computed(() => {
  const terms = searchTerms(search.value)
  return (data.value?.sv ?? []).filter((l) => matches(terms, [l.country, l.vatId, l.name, l.value, SV_CODES[l.code]]))
})
const d2 = computed(() => data.value?.kv?.d2)
const d1 = computed(() => data.value?.kv?.d1)
// B.3: súhrn (B.3.1) alebo po dodávateľoch (B.3.2) — rozhodol server podľa limitu
const b3 = computed(() => {
  const kv = data.value?.kv
  if (kv?.b31) return { part: 'B.3.1', rows: [kv.b31] }
  const rows = kv?.b32 ?? []
  return { part: rows.length ? 'B.3.2' : 'B.3', rows }
})

function openDoc(kind, id) {
  if (!id) return
  if (kind === 'cash') router.push(`/pokladna/${id}/tlac`)
  else router.push(kind === 'purchase' ? `/prijate/${id}` : `/faktury/${id}`)
}

async function downloadXML(which) {
  busyXML.value = which
  try {
    const { blob, name } = await api.vatXML(session.companyId, which, period.value, excess.value.trim().replace(',', '.'))
    saveBlob(blob, name)
    toast.add({ severity: 'success', summary: `Uložené ${name}`, detail: 'Súbor načítajte v eDane alebo na portáli finančnej správy.', life: 5000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'XML nemožno vytvoriť', detail: e.message, life: 6000 })
  } finally {
    busyXML.value = ''
  }
}
</script>

<template>
  <div class="page">
    <header class="head">
      <nav class="tabs" aria-label="Výkazy DPH">
        <RouterLink to="/dph" :class="{ on: tab === 'dph' }">Priznanie k DPH</RouterLink>
        <RouterLink to="/kontrolny-vykaz" :class="{ on: tab === 'kv' }">Kontrolný výkaz</RouterLink>
        <RouterLink to="/suhrnny-vykaz" :class="{ on: tab === 'sv' }">Súhrnný výkaz</RouterLink>
      </nav>
      <div class="spacer" />
      <SelectButton v-model="kind" :options="kinds" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Druh obdobia" />
      <Select v-model="period" :options="options" option-label="label" option-value="value" size="small" class="period" aria-label="Obdobie" />
    </header>

    <p v-if="error" class="error"><i class="pi pi-exclamation-circle" /> {{ error }}</p>

    <template v-if="data">
      <section class="ledger" aria-label="Výsledok">
        <div class="figure">
          <span class="label">Daň na výstupe (r. 17)</span>
          <span class="value">{{ formatAmount(val('r17')) }}</span>
        </div>
        <div class="figure">
          <span class="label">Odpočet dane (r. 18–19)</span>
          <span class="value">{{ formatAmount(data.deduction) }}</span>
        </div>
        <div class="figure" v-if="rows.r25">
          <span class="label">Dobropisy (r. 25)</span>
          <span class="value">{{ formatAmount(val('r25')) }}</span>
        </div>
        <div class="figure result" :data-tone="result.tone">
          <span class="label">{{ result.label }}</span>
          <span class="value"><span class="grand">{{ formatAmount(result.value) }} €</span></span>
        </div>
        <div class="figure due">
          <span class="label">Podať a zaplatiť do</span>
          <span class="value small">{{ formatDate(dueDate(period)) }}</span>
        </div>
      </section>

      <section v-if="data.warnings.length" class="warnings" aria-label="Na kontrolu">
        <h2><i class="pi pi-exclamation-triangle" /> Na kontrolu ({{ data.warnings.length }})</h2>
        <ul>
          <li v-for="(w, i) in data.warnings" :key="i">
            <a v-if="w.id" href="#" @click.prevent="openDoc(w.kind, w.id)">{{ w.doc || 'koncept' }}</a>
            <span v-else>{{ w.doc }}</span>
            — {{ w.message }}
          </li>
        </ul>
      </section>

      <!-- PRIZNANIE -->
      <template v-if="tab === 'dph'">
        <div class="toolbar">
          <span class="muted">{{ data.invoices }} vydaných a {{ data.purchases }} prijatých dokladov<template v-if="data.cash">, {{ data.cash }} pokladničných</template> · {{ data.label }}</span>
          <div class="spacer" />
          <label class="excess">
            <span>Nadmerný odpočet z minulého obdobia (r. 34)</span>
            <InputText v-model="excess" size="small" inputmode="decimal" placeholder="0,00" aria-label="Nadmerný odpočet z minulého obdobia" />
          </label>
          <label class="all"><input v-model="showAll" type="checkbox" /> všetky riadky</label>
          <ExportButton :name="`Priznanie DPH ${period}`" :columns="vatRowColumns" :rows="flatRows" />
          <Button label="XML pre eDane" icon="pi pi-file-export" size="small" :loading="busyXML === 'dph'" @click="downloadXML('dph')" />
        </div>

        <table class="form">
          <tbody v-for="s in sections" :key="s.name">
            <tr class="sec"><th colspan="3">{{ s.name }}</th></tr>
            <tr v-for="r in s.rows" :key="r.code" :class="{ total: r.total, zero: !r.value }">
              <td class="code">{{ r.code.slice(1) }}</td>
              <td class="label">{{ r.label }}</td>
              <td class="amount">
                <span :class="{ grand: r.total && r.value && ['r32', 'r33', 'r35'].includes(r.code) }">{{ r.value ? formatAmount(r.value) : '' }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </template>

      <!-- SÚHRNNÝ VÝKAZ -->
      <template v-else-if="tab === 'sv'">
        <div class="toolbar">
          <IconField class="search">
            <InputIcon class="pi pi-search" />
            <InputText v-model="search" placeholder="Hľadať štát, IČ DPH, odberateľa" size="small" aria-label="Hľadať v súhrnnom výkaze" />
          </IconField>
          <div class="spacer" />
          <ExportButton :name="`Súhrnný výkaz ${period}`" :columns="svColumns" :rows="svRows" />
          <Button label="XML pre eDane" icon="pi pi-file-export" size="small" :loading="busyXML === 'sv'" :disabled="!data.sv.length" @click="downloadXML('sv')" />
        </div>
        <section class="kv">
          <h2>Dodania tovarov a služieb do iných štátov EÚ <span class="muted">{{ svRows.length }}</span></h2>
          <div v-if="svRows.length" class="table-wrap">
            <table class="lines">
              <thead>
                <tr><th>Štát</th><th>IČ DPH</th><th>Odberateľ</th><th class="num">Hodnota</th><th>Kód</th><th class="num">Dokladov</th></tr>
              </thead>
              <tbody>
                <tr v-for="(l, i) in svRows" :key="i">
                  <td>{{ l.country }}</td>
                  <td class="mono">{{ l.vatId }}</td>
                  <td class="partner">{{ l.name }}</td>
                  <td class="num">{{ formatAmount(l.value) }}</td>
                  <td>{{ l.code }} – {{ SV_CODES[l.code] }}</td>
                  <td class="num">{{ l.count }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="empty">V tomto období nie sú žiadne dodania do iných štátov EÚ — súhrnný výkaz sa nepodáva.</p>
        </section>
      </template>

      <!-- KONTROLNÝ VÝKAZ -->
      <template v-else>
        <div class="toolbar">
          <IconField class="search">
            <InputIcon class="pi pi-search" />
            <InputText v-model="search" placeholder="Hľadať IČ DPH, partnera, číslo faktúry" size="small" aria-label="Hľadať v kontrolnom výkaze" />
          </IconField>
          <div class="spacer" />
          <ExportButton :name="`Kontrolný výkaz ${period}`" :columns="kvColumns" :rows="kvAll" />
          <Button label="XML pre eDane" icon="pi pi-file-export" size="small" :loading="busyXML === 'kv'" @click="downloadXML('kv')" />
        </div>

        <section v-for="s in kvSections" :key="s.code" class="kv">
          <h2><span class="part">{{ s.code }}</span> {{ s.title }} <span class="muted">{{ s.rows.length }}</span></h2>
          <div v-if="s.rows.length" class="table-wrap">
            <table class="lines">
              <thead>
                <tr>
                  <th>IČ DPH</th>
                  <th>Partner</th>
                  <th>{{ s.correction ? 'Opravná faktúra' : 'Faktúra' }}</th>
                  <th v-if="s.correction">Pôvodná</th>
                  <th v-else>Dodanie</th>
                  <th class="num">Základ</th>
                  <template v-if="s.a2">
                    <th>Plnenie</th>
                    <th>Kód / druh</th>
                    <th class="num">Množstvo</th>
                  </template>
                  <template v-else>
                    <th class="num">Daň</th>
                    <th class="num">%</th>
                  </template>
                  <th v-if="s.kind === 'purchase'" class="num">Odpočet</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(l, i) in s.rows" :key="i" tabindex="0" @click="openDoc(s.kind, l.id)" @keydown.enter="openDoc(s.kind, l.id)">
                  <td class="mono">{{ l.partner || '—' }}</td>
                  <td class="partner">{{ l.name }}</td>
                  <td class="mono">{{ l.number }}</td>
                  <td v-if="s.correction" class="mono">{{ l.original || '—' }}</td>
                  <td v-else>{{ formatDate(l.date) }}</td>
                  <td class="num">{{ formatAmount(l.base ?? l.baseDiff) }}</td>
                  <template v-if="s.a2">
                    <td>{{ l.label }}</td>
                    <td class="mono">{{ l.code || l.goods || '—' }}</td>
                    <td class="num">{{ l.quantity ? `${formatQuantity(l.quantity)} ${l.unit}` : '—' }}</td>
                  </template>
                  <template v-else>
                    <td class="num">{{ formatAmount(l.vat ?? l.vatDiff) }}</td>
                    <td class="num">{{ l.rate }}</td>
                  </template>
                  <td v-if="s.kind === 'purchase'" class="num">{{ formatAmount(l.deducted) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="empty">Žiadne záznamy.</p>
        </section>

        <section class="kv">
          <h2><span class="part">{{ b3.part }}</span> Pokladničné bloky (zjednodušené faktúry) s odpočtom <span class="muted">{{ b3.rows.reduce((n, r) => n + r.count, 0) }}</span></h2>
          <div v-if="b3.rows.length" class="table-wrap">
            <table class="lines">
              <thead>
                <tr><th v-if="b3.part === 'B.3.2'">IČ DPH dodávateľa</th><th v-if="b3.part === 'B.3.2'">Dodávateľ</th><th class="num">Základ</th><th class="num">Daň</th><th class="num">Odpočet</th></tr>
              </thead>
              <tbody>
                <tr v-for="(r, i) in b3.rows" :key="i">
                  <td v-if="b3.part === 'B.3.2'" class="mono">{{ r.partner }}</td>
                  <td v-if="b3.part === 'B.3.2'">{{ r.name }}</td>
                  <td class="num">{{ formatAmount(r.base) }}</td>
                  <td class="num">{{ formatAmount(r.vat) }}</td>
                  <td class="num">{{ formatAmount(r.deducted) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="empty">Žiadne záznamy.</p>
        </section>

        <section class="kv">
          <h2><span class="part">D.1</span> Tržby z e-kasy súhrnne <span class="muted">{{ d1?.count ?? 0 }}</span></h2>
          <table v-if="d1?.count" class="lines d2">
            <thead>
              <tr><th class="num">Základ 23 %</th><th class="num">Daň 23 %</th><th class="num">Základ znížené</th><th class="num">Daň znížené</th></tr>
            </thead>
            <tbody>
              <tr>
                <td class="num">{{ formatAmount(d1.base) }}</td>
                <td class="num">{{ formatAmount(d1.vat) }}</td>
                <td class="num">{{ formatAmount(d1.baseReduced) }}</td>
                <td class="num">{{ formatAmount(d1.vatReduced) }}</td>
              </tr>
            </tbody>
          </table>
          <p v-else class="empty">Žiadne záznamy.</p>
        </section>

        <section class="kv">
          <h2><span class="part">D.2</span> Ostatné vydané faktúry súhrnne (neplatitelia DPH) <span class="muted">{{ d2?.count ?? 0 }}</span></h2>
          <table v-if="d2?.count" class="lines d2">
            <thead>
              <tr><th class="num">Základ 23 %</th><th class="num">Daň 23 %</th><th class="num">Základ znížené</th><th class="num">Daň znížené</th></tr>
            </thead>
            <tbody>
              <tr>
                <td class="num">{{ formatAmount(d2.base) }}</td>
                <td class="num">{{ formatAmount(d2.vat) }}</td>
                <td class="num">{{ formatAmount(d2.baseReduced) }}</td>
                <td class="num">{{ formatAmount(d2.vatReduced) }}</td>
              </tr>
            </tbody>
          </table>
          <p v-else class="empty">Žiadne záznamy.</p>
        </section>
      </template>

      <p class="note">
        Výpočet vychádza z vystavených faktúr (podľa dátumu dodania), dobropisov (podľa dátumu vyhotovenia) a prijatých faktúr
        (podľa neskoršieho z dátumov dodania a doručenia). XML pred podaním skontrolujte v eDane.
      </p>
    </template>
    <p v-else-if="loading" class="muted">Počítam…</p>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 1100px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.tabs {
  display: flex;
  gap: 2px;
}
.tabs a {
  padding: 4px 12px 6px;
  font-size: var(--fs-xl);
  font-weight: 600;
  color: var(--ink-faint);
  text-decoration: none;
  border-bottom: 2px solid transparent;
}
.tabs a.on {
  color: var(--ink);
  border-bottom-color: var(--blue-deep);
}
.spacer {
  flex: 1;
}
.period {
  min-width: 170px;
}
.error {
  color: var(--tone-overdue);
  margin: 0;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
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
  font-size: var(--fs-xl);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.figure .value.small {
  font-size: var(--fs-lg);
}
.figure.result[data-tone='paid'] .value {
  color: var(--tone-paid);
}
.figure.due {
  margin-left: auto;
  border-right: 0;
}
.grand {
  border-bottom: 3px double currentColor;
  line-height: 1.15;
}
.warnings {
  border-left: 3px solid var(--tone-overdue);
  background: var(--tone-overdue-bg);
  padding: 8px 12px;
}
.warnings h2 {
  margin: 0 0 4px;
  font-size: var(--fs-md);
  color: var(--tone-overdue);
}
.warnings ul {
  margin: 0;
  padding-left: 18px;
  font-size: var(--fs-sm);
}
.warnings a {
  color: var(--blue-deep);
  font-weight: 600;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.excess {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.excess input {
  width: 110px;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.all {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  display: flex;
  align-items: center;
  gap: 4px;
}
.search {
  width: 320px;
}
.search :deep(input) {
  width: 100%;
}
table {
  border-collapse: collapse;
  width: 100%;
  background: var(--sheet);
  font-size: var(--fs-md);
}
.form {
  border: 1px solid var(--rule);
}
.form .sec th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 6px 10px 4px;
  border-top: 1px solid var(--rule);
}
.form td {
  padding: 3px 10px;
  border-bottom: 1px solid var(--rule-soft);
}
.form .code {
  width: 48px;
  color: var(--ink-faint);
  font-variant-numeric: tabular-nums;
}
.form .amount {
  width: 160px;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.form .zero .label {
  color: var(--ink-faint);
}
.form .total td {
  font-weight: 700;
}
.kv h2 {
  font-size: var(--fs-md);
  margin: 4px 0 6px;
  display: flex;
  gap: 8px;
  align-items: baseline;
}
.part {
  background: var(--blue-deep);
  color: #fff;
  font-size: var(--fs-xs);
  padding: 1px 6px;
  border-radius: 3px;
}
.table-wrap {
  overflow-x: auto;
  border: 1px solid var(--rule);
}
.lines th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 5px 10px;
  border-bottom: 1px solid var(--rule);
  white-space: nowrap;
}
.lines td {
  padding: 3px 10px;
  border-bottom: 1px solid var(--rule-soft);
  white-space: nowrap;
}
.lines tbody tr {
  cursor: pointer;
}
.lines tbody tr:hover {
  background: var(--blue-wash);
}
.lines .num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.lines .partner {
  white-space: normal;
  min-width: 160px;
}
.mono {
  font-variant-numeric: tabular-nums;
}
.d2 {
  border: 1px solid var(--rule);
  max-width: 560px;
}
.empty {
  margin: 0;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.note {
  margin: 4px 0 0;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}

@media (max-width: 640px) {
  .tabs {
    max-width: 100%;
    overflow-x: auto;
  }
  .tabs a {
    font-size: var(--fs-md);
    padding: 4px 8px 6px;
    white-space: nowrap;
  }
  .period {
    flex: 1;
  }
  .figure {
    flex: 1 1 45%;
    padding: 6px 10px;
  }
  .figure .value {
    font-size: var(--fs-lg);
  }
  .figure.due {
    margin-left: 0;
  }
  .excess {
    flex-wrap: wrap;
  }
  .search {
    width: 100%;
  }
  .form .amount {
    width: 110px;
  }
  .form td {
    padding: 3px 6px;
  }
}
</style>
