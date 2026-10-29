<script setup>
// Pokladničný doklad na tlač (polovica A4, dva doklady sa zmestia na jeden hárok).
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatDate } from '../lib/format.js'
import { amountInWords } from '../lib/words.js'
import CompanyLogo from '../components/CompanyLogo.vue'

const props = defineProps({ id: String })
const print = () => window.print()
const router = useRouter()
const doc = ref(null)
const error = ref(null)
const company = computed(() => session.company ?? {})
const income = computed(() => doc.value?.kind === 'in')
const title = computed(() => (income.value ? 'Príjmový pokladničný doklad' : 'Výdavkový pokladničný doklad'))

onMounted(async () => {
  try {
    doc.value = await api.cash(session.companyId, props.id)
    document.title = `${income.value ? 'PPD' : 'VPD'} ${doc.value.number}`
  } catch (e) {
    error.value = e.message
  }
})
const addr = (c) => [c.street, [c.zip, c.city].filter(Boolean).join(' ')].filter(Boolean).join(', ')
</script>

<template>
  <div class="print-root">
    <nav class="toolbar">
      <Button icon="pi pi-arrow-left" label="Späť" text size="small" @click="router.push('/pokladna')" />
      <div class="spacer" />
      <span class="tip">V okne tlače zvoľte „Uložiť ako PDF“</span>
      <Button icon="pi pi-print" label="Tlačiť / PDF" size="small" :disabled="!doc" @click="print" />
    </nav>
    <p v-if="error" class="error">{{ error }}</p>

    <article v-else-if="doc" class="sheet">
      <div v-if="doc.cancelled" class="stamp">Stornovaný</div>
      <header class="head">
        <div>
          <CompanyLogo class="logo" height="12mm" />
          <p class="kind">{{ title }}</p>
          <h1>{{ doc.number }}</h1>
        </div>
        <div class="company">
          <strong>{{ company.name }}</strong>
          <span>{{ addr(company) }}</span>
          <span>
            <template v-if="company.ico">IČO {{ company.ico }}</template>
            <template v-if="company.dic"> · DIČ {{ company.dic }}</template>
            <template v-if="company.icdph"> · IČ DPH {{ company.icdph }}</template>
          </span>
        </div>
      </header>

      <dl class="rows">
        <div><dt>Dátum</dt><dd>{{ formatDate(doc.date) }}</dd></div>
        <div><dt>{{ income ? 'Prijaté od' : 'Vyplatené komu' }}</dt><dd>{{ doc.partner || '—' }}</dd></div>
        <div class="wide"><dt>Účel platby</dt><dd>{{ doc.purpose }}</dd></div>
        <div v-if="doc.linkNumber"><dt>Uhrádza doklad</dt><dd>{{ doc.linkNumber }}</dd></div>
        <div v-if="doc.note" class="wide"><dt>Poznámka</dt><dd>{{ doc.note }}</dd></div>
      </dl>

      <table v-if="doc.tax?.rows?.length" class="vat">
        <thead><tr><th>Sadzba DPH</th><th>Základ</th><th>DPH</th><th>Spolu</th></tr></thead>
        <tbody>
          <tr v-for="r in doc.tax.rows" :key="r.vatRate">
            <td>{{ r.vatRate }} %</td><td>{{ formatAmount(r.base) }}</td><td>{{ formatAmount(r.vat) }}</td><td>{{ formatAmount(r.gross) }}</td>
          </tr>
        </tbody>
        <tfoot v-if="doc.tax.supplierIcdph"><tr><td colspan="4">IČ DPH predajcu: {{ doc.tax.supplierIcdph }}</td></tr></tfoot>
      </table>

      <section class="sum">
        <div>
          <span>Suma</span>
          <strong class="grand">{{ formatAmount(doc.amount) }} €</strong>
        </div>
        <p class="words">Slovom: {{ amountInWords(doc.amount) }}</p>
      </section>

      <footer class="signs">
        <div><span>Vyhotovil</span></div>
        <div><span>Schválil</span></div>
        <div><span>{{ income ? 'Prijal (pokladník)' : 'Vyplatil (pokladník)' }}</span></div>
        <div><span>{{ income ? 'Zaplatil' : 'Prijal' }}</span></div>
      </footer>
    </article>
  </div>
</template>

<style scoped>
.vat {
  width: 100%;
  border-collapse: collapse;
  margin: 10px 0 0;
  font-size: 0.85rem;
  font-variant-numeric: tabular-nums;
}
.vat th,
.vat td {
  border-bottom: 1px solid #d5dbe2;
  padding: 3px 6px;
  text-align: right;
}
.vat th:first-child,
.vat td:first-child {
  text-align: left;
}
.vat th {
  font-weight: 500;
  color: #4a5a6c;
}
.vat tfoot td {
  text-align: left;
  border: 0;
  color: #4a5a6c;
}
.print-root {
  min-height: 100%;
  background: #e9ecf0;
  padding: 0 0 40px;
}
.toolbar {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  background: var(--sheet);
  border-bottom: 1px solid var(--rule);
}
.spacer {
  flex: 1;
}
.tip {
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.error {
  text-align: center;
  color: var(--tone-overdue);
  padding: 40px;
}
/* polovica A4 na šírku hárku: 210 × 148 mm */
.sheet {
  position: relative;
  width: 210mm;
  min-height: 140mm;
  margin: 24px auto 0;
  padding: 12mm 14mm;
  background: #fff;
  color: #1d2633;
  font-size: 10pt;
  line-height: 1.4;
  box-shadow: 0 1px 3px rgba(18, 24, 32, 0.12), 0 8px 28px rgba(18, 24, 32, 0.08);
  overflow: hidden;
}
.stamp {
  position: absolute;
  top: 50mm;
  left: 50%;
  transform: translateX(-50%) rotate(-14deg);
  font-size: 40pt;
  font-weight: 700;
  color: rgba(176, 58, 38, 0.15);
  border: 4px solid rgba(176, 58, 38, 0.15);
  padding: 1mm 6mm;
}
.head {
  display: flex;
  justify-content: space-between;
  gap: 10mm;
  border-bottom: 2px solid #1f3a5c;
  padding-bottom: 4mm;
}
.logo {
  margin-bottom: 3mm;
}
.kind {
  margin: 0;
  font-size: 9pt;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #4e5866;
}
h1 {
  margin: 0;
  font-size: 20pt;
  color: #1f3a5c;
}
.company {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  text-align: right;
  font-size: 9pt;
}
.company strong {
  font-size: 10.5pt;
}
.rows {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 3mm 8mm;
  margin: 6mm 0;
}
.rows div {
  display: flex;
  flex-direction: column;
  border-bottom: 1px solid #dce1e8;
  padding-bottom: 1.5mm;
}
.rows .wide {
  grid-column: 1 / -1;
}
.rows dt {
  font-size: 8pt;
  color: #6b7685;
}
.rows dd {
  margin: 0;
  font-weight: 600;
}
.sum {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2mm;
}
.sum div {
  display: flex;
  align-items: baseline;
  gap: 6mm;
}
.sum span {
  color: #4e5866;
}
.grand {
  font-size: 18pt;
  font-variant-numeric: tabular-nums;
  border-bottom: 3pt double #1d2633;
  line-height: 1.15;
}
.words {
  margin: 0;
  font-style: italic;
  color: #4e5866;
}
.signs {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 6mm;
  margin-top: 16mm;
}
.signs div {
  border-top: 1px solid #1d2633;
  padding-top: 1mm;
  font-size: 8pt;
  color: #4e5866;
  text-align: center;
}
@media (max-width: 860px) {
  .print-root {
    overflow-x: auto;
  }
  .tip {
    display: none;
  }
  .sheet {
    margin: 12px;
  }
}
@media print {
  .print-root {
    background: none;
    padding: 0;
  }
  .toolbar {
    display: none;
  }
  .sheet {
    width: auto;
    margin: 0;
    padding: 0;
    box-shadow: none;
  }
}
</style>
