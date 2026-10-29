<script setup>
// Upomienka na tlač / do PDF (A4 list): odberateľ, zoznam faktúr, úrok, náhrada.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { countryName } from '../lib/codes.js'
import { introText, legalNotes, levelTitle } from '../lib/reminders.js'

const props = defineProps({ id: String })
const router = useRouter()
const r = ref(null)
const error = ref(null)
const company = computed(() => session.company ?? {})
const hasFees = computed(() => Number(r.value?.fees) > 0)
const hasInterest = computed(() => Number(r.value?.interestTotal) > 0)

onMounted(async () => {
  try {
    r.value = await api.reminder(session.companyId, props.id)
    document.title = `${levelTitle(r.value.level)} ${r.value.number}`
  } catch (e) {
    error.value = e.message
  }
})
const addressLines = (p) => [p.street, [p.zip, p.city].filter(Boolean).join(' ')].filter(Boolean)
const print = () => window.print()
</script>

<template>
  <div class="print-root">
    <nav class="toolbar">
      <Button icon="pi pi-arrow-left" label="Späť" text size="small" @click="router.push('/upomienky')" />
      <div class="spacer" />
      <span class="tip">V okne tlače zvoľte „Uložiť ako PDF“</span>
      <Button icon="pi pi-print" label="Tlačiť / PDF" size="small" :disabled="!r" @click="print" />
    </nav>
    <p v-if="error" class="error">{{ error }}</p>

    <article v-else-if="r" class="sheet">
      <header class="head">
        <div class="from">
          <strong>{{ company.name }}</strong>
          <span>{{ addressLines(company).join(', ') }}</span>
          <span>
            <template v-if="company.ico">IČO {{ company.ico }}</template>
            <template v-if="company.dic"> · DIČ {{ company.dic }}</template>
            <template v-if="company.icdph"> · IČ DPH {{ company.icdph }}</template>
          </span>
          <span v-if="company.email || company.phone">{{ [company.email, company.phone].filter(Boolean).join(', ') }}</span>
        </div>
        <div class="to">
          <p class="name">{{ r.customer.name }}</p>
          <p v-for="l in addressLines(r.customer)" :key="l">{{ l }}</p>
          <p v-if="r.customer.country && r.customer.country !== 'SK'">{{ countryName(r.customer.country) }}</p>
          <p v-if="r.customer.ico" class="ico">IČO {{ r.customer.ico }}</p>
        </div>
      </header>

      <div class="meta">
        <div><span>Číslo</span><strong>{{ r.number }}</strong></div>
        <div><span>Dátum</span><strong>{{ formatDate(r.date) }}</strong></div>
        <div v-if="company.city"><span>Miesto</span><strong>{{ company.city }}</strong></div>
      </div>

      <h1 :class="{ last: r.level === 3 }">{{ levelTitle(r.level) }}</h1>
      <p class="intro">{{ introText(r) }}</p>

      <table class="items">
        <thead>
          <tr>
            <th>Faktúra</th>
            <th>Variabilný symbol</th>
            <th>Vystavená</th>
            <th>Splatná</th>
            <th class="num">Dní</th>
            <th class="num">Suma</th>
            <th v-if="hasInterest" class="num">Úrok</th>
            <th v-if="hasFees" class="num">Náhrada</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="it in r.items" :key="it.invoiceId">
            <td>{{ it.number }}</td>
            <td>{{ it.variableSymbol }}</td>
            <td>{{ formatDate(it.issueDate) }}</td>
            <td>{{ formatDate(it.dueDate) }}</td>
            <td class="num">{{ it.days }}</td>
            <td class="num">{{ formatAmount(it.amount) }}</td>
            <td v-if="hasInterest" class="num">{{ formatAmount(it.interest) }}<small v-if="Number(it.rate)"> ({{ formatAmount(it.rate) }} %)</small></td>
            <td v-if="hasFees" class="num">{{ formatAmount(it.fee) }}</td>
          </tr>
        </tbody>
      </table>

      <section class="totals">
        <dl>
          <template v-if="hasInterest || hasFees">
            <dt>Dlžná suma</dt><dd>{{ formatMoney(r.principal) }}</dd>
            <template v-if="hasInterest"><dt>Úrok z omeškania</dt><dd>{{ formatMoney(r.interestTotal) }}</dd></template>
            <template v-if="hasFees"><dt>Paušálna náhrada nákladov</dt><dd>{{ formatMoney(r.fees) }}</dd></template>
          </template>
        </dl>
        <div class="payable">
          <span>Spolu na úhradu do {{ formatDate(r.payBy) }}</span>
          <strong>{{ formatMoney(r.total) }}</strong>
        </div>
      </section>

      <section class="pay">
        <div><span>IBAN</span><strong>{{ company.iban || '—' }}</strong></div>
        <div><span>Variabilný symbol</span><strong>číslo príslušnej faktúry</strong></div>
      </section>

      <p v-if="r.note" class="note">{{ r.note }}</p>
      <footer class="legal">
        <p v-for="n in legalNotes(r)" :key="n">{{ n }}</p>
      </footer>

      <div class="sign">
        <span>S pozdravom</span>
        <strong>{{ company.name }}</strong>
      </div>
    </article>
  </div>
</template>

<style scoped>
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
.sheet {
  width: 210mm;
  min-height: 297mm;
  margin: 24px auto 0;
  padding: 16mm 18mm 14mm;
  background: #fff;
  color: #1d2633;
  font-size: 10pt;
  line-height: 1.45;
  box-shadow: 0 1px 3px rgba(18, 24, 32, 0.12), 0 8px 28px rgba(18, 24, 32, 0.08);
}
.head {
  display: grid;
  grid-template-columns: 1fr 78mm;
  gap: 10mm;
  align-items: start;
}
.from {
  display: flex;
  flex-direction: column;
  font-size: 8.5pt;
  color: #4e5866;
  border-top: 2.5pt solid #1f3a5c;
  padding-top: 2mm;
}
.from strong {
  font-size: 11pt;
  color: #1d2633;
}
/* adresné okienko obálky */
.to {
  margin-top: 14mm;
  min-height: 30mm;
}
.to p {
  margin: 0;
}
.to .name {
  font-weight: 700;
  font-size: 11pt;
}
.to .ico {
  margin-top: 1.5mm;
  color: #4e5866;
  font-size: 9pt;
}
.meta {
  display: flex;
  gap: 10mm;
  margin: 8mm 0 4mm;
  font-size: 9pt;
}
.meta div {
  display: flex;
  flex-direction: column;
}
.meta span {
  font-size: 8pt;
  color: #4e5866;
}
h1 {
  margin: 0 0 3mm;
  font-size: 18pt;
  color: #1f3a5c;
}
h1.last {
  color: #b03a26;
}
.intro {
  margin: 0 0 6mm;
  max-width: 160mm;
}
table {
  width: 100%;
  border-collapse: collapse;
}
th {
  font-size: 8pt;
  font-weight: 600;
  color: #4e5866;
  text-align: left;
  padding: 1.5mm 2mm;
  border-bottom: 0.75pt solid #1d2633;
}
td {
  padding: 1.6mm 2mm;
  border-bottom: 0.5pt solid #dce1e8;
  font-variant-numeric: tabular-nums;
}
th:first-child,
td:first-child {
  padding-left: 0;
}
th:last-child,
td:last-child {
  padding-right: 0;
}
.num {
  text-align: right;
  white-space: nowrap;
}
td small {
  color: #6b7685;
  font-size: 7.5pt;
}
.totals {
  margin: 5mm 0 0 auto;
  width: 92mm;
}
.totals dl {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 1mm 6mm;
  margin: 0;
}
.totals dt {
  color: #4e5866;
}
.totals dd {
  margin: 0;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.payable {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 6mm;
  margin-top: 3mm;
  padding-top: 2mm;
  border-top: 0.75pt solid #1d2633;
}
.payable span {
  color: #4e5866;
}
.payable strong {
  font-size: 16pt;
  font-variant-numeric: tabular-nums;
  border-bottom: 3pt double #1d2633;
  line-height: 1.15;
  white-space: nowrap;
}
.pay {
  display: flex;
  gap: 10mm;
  margin-top: 7mm;
  padding: 2mm 0;
  border-top: 0.75pt solid #c5ccd6;
  border-bottom: 0.75pt solid #c5ccd6;
}
.pay div {
  display: flex;
  flex-direction: column;
}
.pay span {
  font-size: 8pt;
  color: #4e5866;
}
.note {
  margin: 5mm 0 0;
  white-space: pre-line;
}
.legal {
  margin-top: 5mm;
  font-size: 8pt;
  color: #4e5866;
}
.legal p {
  margin: 0 0 1mm;
}
.sign {
  display: flex;
  flex-direction: column;
  margin-top: 12mm;
}
.sign span {
  color: #4e5866;
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
    min-height: 0;
    margin: 0;
    padding: 0;
    box-shadow: none;
  }
}
</style>
