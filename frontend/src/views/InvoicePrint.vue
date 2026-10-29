<script setup>
// Faktúra alebo cenová ponuka (quote) na tlač / uloženie do PDF (A4).
// PDF vytvorí prehliadač cez „Tlačiť → Uložiť ako PDF“.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, formatQuantity } from '../lib/format.js'
import { payBySquare, qrSvg } from '../lib/payqr.js'
import { countryName, domesticReverseNote, paymentLabel, vatExemptionNote } from '../lib/codes.js'
import CompanyLogo from '../components/CompanyLogo.vue'
import SendMailDialog from '../components/SendMailDialog.vue'
import { rateText } from '../lib/fx.js'

const props = defineProps({ id: String, quote: Boolean })
const back = computed(() => `${props.quote ? '/ponuky' : '/faktury'}/${props.id}`)
const print = () => window.print()
const mailOpen = ref(false)
// PDF vytvorí server (rovnaký súbor ide aj do prílohy e-mailu)
const download = () => {
  window.location.href = api.pdfURL(session.companyId, props.quote ? 'quotes' : 'invoices', props.id)
}
const router = useRouter()

const inv = ref(null)
const error = ref(null)
const qr = ref('')

const company = computed(() => session.company ?? {})
const nonVat = computed(() => !!inv.value?.nonVatPayer)
const receipt = computed(() => inv.value?.kind === 'advance_receipt')
const deductions = computed(() => inv.value?.advance?.deductions ?? [])
const docTitle = computed(() => {
  if (props.quote) return 'Cenová ponuka'
  switch (inv.value?.kind) {
    case 'credit_note': return nonVat.value ? 'Dobropis – opravná faktúra' : 'Dobropis – opravný daňový doklad'
    case 'proforma': return 'Zálohová faktúra – nie je daňový doklad'
    case 'advance_receipt': return 'Daňový doklad k prijatej platbe'
  }
  return nonVat.value ? 'Faktúra' : 'Faktúra – daňový doklad'
})

onMounted(async () => {
  try {
    if (props.quote) {
      inv.value = await api.quote(session.companyId, props.id)
      document.title = inv.value.number ? `Cenová ponuka ${inv.value.number}` : 'Koncept ponuky'
      return // ponuka sa neplatí — bez QR kódu
    }
    inv.value = await api.invoice(session.companyId, props.id)
    document.title = inv.value.number ? `Faktúra ${inv.value.number}` : 'Koncept faktúry'
    const code = payBySquare(inv.value, company.value)
    if (code) qr.value = await qrSvg(code)
  } catch (e) {
    error.value = e.message
  }
})

const addressLines = (p) => [p.street, [p.zip, p.city].filter(Boolean).join(' ')].filter(Boolean)
const ids = (p) =>
  [
    p.ico && ['IČO', p.ico],
    p.dic && ['DIČ', p.dic],
    p.icdph && ['IČ DPH', p.icdph],
  ].filter(Boolean)
</script>

<template>
  <div class="print-root">
    <nav class="toolbar">
      <Button icon="pi pi-arrow-left" label="Späť" text size="small" @click="router.push(back)" />
      <div class="spacer" />
      <Button v-if="can('invoices.write')" icon="pi pi-envelope" label="E-mailom" aria-label="Poslať e-mailom" severity="secondary" text size="small" :disabled="!inv" @click="mailOpen = true" />
      <Button icon="pi pi-download" label="Stiahnuť PDF" aria-label="Stiahnuť PDF" severity="secondary" outlined size="small" :disabled="!inv" @click="download" />
      <Button icon="pi pi-print" label="Tlačiť" aria-label="Tlačiť" size="small" :disabled="!inv" @click="print" />
    </nav>

    <p v-if="error" class="error">{{ error }}</p>

    <article v-else-if="inv" class="sheet" :class="{ draft: inv.status === 'draft', cancelled: inv.status === 'cancelled' }">
      <div v-if="inv.status === 'draft'" class="stamp">Koncept</div>
      <div v-else-if="inv.status === 'cancelled'" class="stamp">Stornovaná</div>

      <header class="head">
        <div>
          <CompanyLogo class="logo" />
          <p class="kind">{{ docTitle }}</p>
          <h1 class="number">{{ inv.number || 'Koncept' }}</h1>
        </div>
        <dl v-if="quote" class="dates">
          <div><dt>Dátum ponuky</dt><dd>{{ formatDate(inv.issueDate) }}</dd></div>
          <div class="due"><dt>Platná do</dt><dd>{{ formatDate(inv.validUntil) }}</dd></div>
        </dl>
        <dl v-else class="dates">
          <div><dt>Dátum vystavenia</dt><dd>{{ formatDate(inv.issueDate) }}</dd></div>
          <div><dt>{{ receipt ? 'Dátum prijatia platby' : 'Dátum dodania' }}</dt><dd>{{ formatDate(inv.deliveryDate) }}</dd></div>
          <div v-if="!receipt" class="due"><dt>Dátum splatnosti</dt><dd>{{ formatDate(inv.dueDate) }}</dd></div>
        </dl>
      </header>

      <section v-if="inv.correction" class="correction">
        <div><span>Opravuje faktúru</span><strong>{{ inv.correction.originalNumber }}</strong></div>
        <div><span>Zo dňa</span><strong>{{ formatDate(inv.correction.originalIssueDate) }}</strong></div>
        <div><span>Dátum dodania pôvodného plnenia</span><strong>{{ formatDate(inv.correction.originalDeliveryDate) }}</strong></div>
        <div class="why"><span>Dôvod opravy</span><strong>{{ inv.correction.reason }}</strong></div>
      </section>

      <section class="parties">
        <div class="party">
          <h2>Dodávateľ</h2>
          <p class="name">{{ company.name }}</p>
          <p v-for="l in addressLines(company)" :key="l">{{ l }}</p>
          <dl class="ids">
            <template v-for="[k, v] in ids(company)" :key="k"><dt>{{ k }}</dt><dd>{{ v }}</dd></template>
          </dl>
          <p v-if="company.email || company.phone" class="contact">{{ [company.email, company.phone].filter(Boolean).join(', ') }}</p>
        </div>
        <div class="party">
          <h2>{{ quote ? 'Pre' : 'Odberateľ' }}</h2>
          <p class="name">{{ inv.customer.name }}</p>
          <p v-for="l in addressLines(inv.customer)" :key="l">{{ l }}</p>
          <p v-if="inv.customer.country && inv.customer.country !== 'SK'">{{ countryName(inv.customer.country) }}</p>
          <dl class="ids">
            <template v-for="[k, v] in ids(inv.customer)" :key="k"><dt>{{ k }}</dt><dd>{{ v }}</dd></template>
          </dl>
        </div>
      </section>

      <section v-if="inv.advance && (receipt || deductions.length)" class="correction">
        <div><span>{{ receipt ? 'K zálohovej faktúre' : 'Vyúčtovanie zálohovej faktúry' }}</span><strong>{{ inv.advance.proformaNumber }}</strong></div>
        <div v-if="receipt"><span>Daňová povinnosť vznikla dňom</span><strong>{{ formatDate(inv.deliveryDate) }}</strong></div>
      </section>

      <section v-if="!quote && !receipt" class="payment">
        <div><span>Spôsob úhrady</span><strong>{{ paymentLabel(inv.paymentMethod) }}</strong></div>
        <div v-if="(inv.paymentMethod || 'transfer') === 'transfer'"><span>IBAN</span><strong>{{ company.iban || '—' }}</strong></div>
        <div v-else><span>&nbsp;</span><strong>&nbsp;</strong></div>
        <div><span>Variabilný symbol</span><strong>{{ inv.variableSymbol || '—' }}</strong></div>
      </section>

      <table class="lines">
        <thead>
          <tr>
            <th class="desc">Popis</th>
            <th class="num">Množstvo</th>
            <th>MJ</th>
            <th class="num">Cena za MJ{{ nonVat ? '' : inv.priceMode === 'gross' ? ' s DPH' : ' bez DPH' }}</th>
            <th v-if="!nonVat" class="num">DPH</th>
            <th class="num">{{ nonVat ? 'Spolu' : inv.priceMode === 'gross' ? 'Spolu s DPH' : 'Základ' }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(l, i) in inv.lines" :key="i">
            <td class="desc">{{ l.description }}</td>
            <td class="num">{{ formatQuantity(l.quantity) }}</td>
            <td>{{ l.unit }}</td>
            <td class="num">{{ formatAmount(l.unitPrice) }}</td>
            <td v-if="!nonVat" class="num">{{ inv.reverseCharge ? '—' : l.reverse ? 'PDP' : `${l.vatRate} %` }}</td>
            <td class="num">{{ formatAmount(l.amount) }}</td>
          </tr>
        </tbody>
      </table>

      <table v-if="deductions.length" class="lines deduct">
        <thead>
          <tr>
            <th class="desc">Odpočet zálohy</th>
            <th>Prijatá</th>
            <th v-if="!nonVat" class="num">Sadzba</th>
            <th v-if="!nonVat" class="num">Základ</th>
            <th v-if="!nonVat" class="num">DPH</th>
            <th class="num">Spolu</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="d in deductions" :key="d.number">
            <tr v-for="(rs, i) in d.rates" :key="d.number + rs.rate">
              <td class="desc">{{ i === 0 ? (d.receiptId ? `Daňový doklad k prijatej platbe ${d.number}` : `Uhradená záloha ${d.number}`) : '' }}</td>
              <td>{{ i === 0 ? formatDate(d.date) : '' }}</td>
              <td v-if="!nonVat" class="num">{{ rs.rate }} %</td>
              <td v-if="!nonVat" class="num">−{{ formatAmount(rs.base) }}</td>
              <td v-if="!nonVat" class="num">−{{ formatAmount(rs.vat) }}</td>
              <td class="num">−{{ formatAmount(rs.total) }}</td>
            </tr>
          </template>
        </tbody>
      </table>

      <section class="bottom">
        <div class="qr-block">
          <template v-if="qr">
            <div class="qr" v-html="qr" />
            <p>Zaplaťte mobilom<br /><strong>PAY by square</strong></p>
          </template>
        </div>

        <div class="totals">
          <table v-if="!nonVat" class="recap">
            <thead>
              <tr><th>{{ deductions.length ? 'Po odpočte zálohy' : 'Sadzba DPH' }}</th><th class="num">Základ</th><th class="num">DPH</th><th class="num">Spolu</th></tr>
            </thead>
            <tbody>
              <tr v-for="r in inv.summary.rates" :key="`${r.rate}${r.reverse ? 'p' : ''}`">
                <td>{{ r.reverse ? 'PDP' : `${r.rate} %` }}</td>
                <td class="num">{{ formatAmount(r.base) }}</td>
                <td class="num">{{ formatAmount(r.vat) }}</td>
                <td class="num">{{ formatAmount(r.total) }}</td>
              </tr>
            </tbody>
            <tfoot>
              <tr>
                <td>Spolu</td>
                <td class="num">{{ formatAmount(inv.summary.base) }}</td>
                <td class="num">{{ formatAmount(inv.summary.vat) }}</td>
                <td class="num">{{ formatAmount(inv.summary.total) }}</td>
              </tr>
            </tfoot>
          </table>
          <div v-if="deductions.length && inv.advance.supply" class="supply-line">
            <span>Dodávka spolu</span>
            <span>{{ formatMoney(inv.advance.supply.total, inv.currency) }}</span>
          </div>
          <div class="payable">
            <span>{{ quote ? 'Cena spolu' : inv.kind === 'credit_note' ? 'Suma dobropisu' : receipt ? 'Prijatá platba' : 'K úhrade' }}</span>
            <strong>{{ formatMoney(inv.summary.total, inv.currency) }}</strong>
          </div>
          <div v-if="inv.local && !quote" class="local">
            <p class="rate">Kurz: {{ rateText(inv) }}</p>
            <table v-if="!nonVat" class="recap">
              <thead>
                <tr><th>V eurách</th><th class="num">Základ</th><th class="num">DPH</th><th class="num">Spolu</th></tr>
              </thead>
              <tbody>
                <tr v-for="r in inv.local.rates" :key="`${r.rate}${r.reverse ? 'p' : ''}`">
                  <td>{{ r.reverse ? 'PDP' : `${r.rate} %` }}</td>
                  <td class="num">{{ formatAmount(r.base) }}</td>
                  <td class="num">{{ formatAmount(r.vat) }}</td>
                  <td class="num">{{ formatAmount(r.total) }}</td>
                </tr>
              </tbody>
              <tfoot>
                <tr>
                  <td>Spolu €</td>
                  <td class="num">{{ formatAmount(inv.local.base) }}</td>
                  <td class="num">{{ formatAmount(inv.local.vat) }}</td>
                  <td class="num">{{ formatAmount(inv.local.total) }}</td>
                </tr>
              </tfoot>
            </table>
            <p v-else class="rate">Spolu v eurách: {{ formatMoney(inv.local.total) }}</p>
          </div>
        </div>
      </section>

      <footer class="notes">
        <p v-if="nonVat">Dodávateľ nie je platiteľ DPH.</p>
        <p v-if="inv.reverseCharge && !nonVat">{{ vatExemptionNote(inv) }}</p>
        <p v-if="!nonVat && domesticReverseNote(inv.lines)">{{ domesticReverseNote(inv.lines) }}</p>
        <p v-if="inv.note" class="user-note">{{ inv.note }}</p>
        <p v-if="quote">Cenová ponuka nie je daňový doklad.</p>
      </footer>
    </article>
    <SendMailDialog v-if="inv" v-model:visible="mailOpen" :kind="quote ? 'quotes' : 'invoices'" :id="id" />
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
.error {
  text-align: center;
  color: var(--tone-overdue);
  padding: 40px;
}

/* A4 hárok: 210 × 297 mm */
.sheet {
  position: relative;
  width: 210mm;
  min-height: 297mm;
  margin: 24px auto 0;
  padding: 16mm 16mm 14mm;
  background: #fff;
  color: #1d2633;
  font-size: 9.5pt;
  line-height: 1.38;
  box-shadow: 0 1px 3px rgba(18, 24, 32, 0.12), 0 8px 28px rgba(18, 24, 32, 0.08);
  overflow: hidden;
}

.stamp {
  position: absolute;
  top: 92mm;
  left: 50%;
  transform: translateX(-50%) rotate(-18deg);
  font-size: 54pt;
  font-weight: 700;
  letter-spacing: 0.06em;
  color: rgba(176, 58, 38, 0.13);
  border: 5px solid rgba(176, 58, 38, 0.13);
  padding: 2mm 8mm;
  pointer-events: none;
}

.head {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 10mm;
  padding-bottom: 5mm;
  border-bottom: 2.5pt solid #1f3a5c;
}
.logo {
  margin-bottom: 4mm;
}
.kind {
  margin: 0;
  font-size: 10pt;
  color: #4e5866;
}
.number {
  margin: 1mm 0 0;
  font-size: 24pt;
  font-weight: 700;
  letter-spacing: -0.01em;
  font-variant-numeric: tabular-nums;
}
.dates {
  display: flex;
  gap: 7mm;
  margin: 0;
}
.dates div {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
}
.dates dt {
  font-size: 8pt;
  color: #4e5866;
}
.dates dd {
  margin: 0;
  font-size: 11pt;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.dates .due dd {
  color: #1f3a5c;
}

.correction {
  display: grid;
  grid-template-columns: auto auto auto 1fr;
  gap: 0 8mm;
  margin-top: 5mm;
  padding: 2.5mm 3mm;
  background: #f6f7f9;
  border-left: 2pt solid #1f3a5c;
}
.correction div {
  display: flex;
  flex-direction: column;
}
.correction span {
  font-size: 8pt;
  color: #4e5866;
}
.parties {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10mm;
  margin: 7mm 0 6mm;
}
.party h2 {
  margin: 0 0 1.5mm;
  font-size: 8pt;
  font-weight: 600;
  color: #4e5866;
}
.party p {
  margin: 0;
}
.party .name {
  font-size: 11.5pt;
  font-weight: 700;
  margin-bottom: 0.5mm;
}
.ids {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 0 4mm;
  margin: 2mm 0 0;
}
.ids dt {
  color: #4e5866;
}
.ids dd {
  margin: 0;
  font-variant-numeric: tabular-nums;
}
.contact {
  margin-top: 2mm !important;
  color: #4e5866;
}

.payment {
  display: grid;
  grid-template-columns: auto 1.6fr 1fr;
  border-top: 0.75pt solid #c5ccd6;
  border-bottom: 0.75pt solid #c5ccd6;
  margin-bottom: 6mm;
}
.payment div {
  display: flex;
  flex-direction: column;
  padding: 2mm 6mm 2mm 0;
}
.payment div + div {
  padding-left: 6mm;
  border-left: 0.75pt solid #dce1e8;
}
.payment span {
  font-size: 8pt;
  color: #4e5866;
}
.payment strong {
  font-variant-numeric: tabular-nums;
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
  padding: 1.8mm 2mm;
  border-bottom: 0.5pt solid #dce1e8;
  vertical-align: top;
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
  font-variant-numeric: tabular-nums;
}
.lines .desc {
  width: 46%;
}
.lines td:last-child {
  font-weight: 600;
}

.bottom {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 10mm;
  align-items: start;
  margin-top: 7mm;
  break-inside: avoid;
}
.qr-block {
  display: flex;
  align-items: center;
  gap: 4mm;
}
.qr {
  width: 30mm;
  height: 30mm;
}
.qr :deep(svg) {
  width: 100%;
  height: 100%;
  display: block;
}
.qr-block p {
  margin: 0;
  font-size: 8.5pt;
  color: #4e5866;
}
.qr-block strong {
  color: #1d2633;
}
.totals {
  min-width: 92mm;
}
.recap td,
.recap th {
  padding-top: 1.2mm;
  padding-bottom: 1.2mm;
}
.recap tfoot td {
  font-weight: 600;
  border-bottom: 0;
  border-top: 0.75pt solid #1d2633;
}
.deduct {
  margin-top: 4mm;
}
.deduct td:last-child {
  font-weight: 400;
}
.supply-line {
  display: flex;
  justify-content: space-between;
  margin-top: 3mm;
  color: #4e5866;
}
.payable {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-top: 4mm;
}
.payable span {
  font-size: 10pt;
  color: #4e5866;
}
.payable strong {
  font-size: 18pt;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  border-bottom: 3pt double #1d2633;
  line-height: 1.15;
}

.local {
  margin-top: 5mm;
}
.local .rate {
  margin: 0 0 1.5mm;
  font-size: 8.5pt;
  color: #4e5866;
}
.notes {
  margin-top: 8mm;
  color: #4e5866;
}
.notes p {
  margin: 0 0 1.5mm;
}
.user-note {
  color: #1d2633;
  white-space: pre-line;
}

@media (max-width: 860px) {
  .print-root {
    overflow-x: auto;
  }
  /* na mobile iba ikony, nech sa tlačidlá zmestia */
  .toolbar :deep(.p-button-label) {
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

<style>
@page {
  size: A4;
  margin: 14mm 16mm;
}
@media print {
  html,
  body,
  #app {
    height: auto !important;
    background: #fff !important;
  }
  .p-toast {
    display: none !important;
  }
}
</style>
