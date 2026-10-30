<script setup>
// Kniha jázd na tlač (A4 na šírku): vozidlo, všetky jazdy roka, tankovanie,
// doklady nákladov s DPH a vyúčtovanie. Slúži aj ako podrobná evidencia
// vozidla podľa § 85n zákona o DPH (jazdy, stav tachometra, náklady).
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney } from '../lib/format.js'
import { fuelUnit, ownerships, tripTime } from '../lib/logbook.js'
import CompanyLogo from '../components/CompanyLogo.vue'

const props = defineProps({ id: String })
const route = useRoute()
const router = useRouter()
const year = Number(route.query.rok) || new Date().getFullYear()
const trips = ref([])
const fuel = ref([])
const report = ref(null)
const error = ref(null)
const company = computed(() => session.company ?? {})
const v = computed(() => report.value?.vehicle)
const unit = computed(() => fuelUnit(v.value?.fuel))
const costs = computed(() => new Map((report.value?.trips ?? []).map((c) => [c.tripId, c])))
const isPrivate = computed(() => v.value?.ownership === 'private')
const km = (n) => new Intl.NumberFormat('sk-SK').format(n ?? 0)

onMounted(async () => {
  try {
    const cid = session.companyId
    ;[trips.value, fuel.value, report.value] = await Promise.all([
      api.trips(cid, props.id, year), api.fuel(cid, props.id, year), api.vehicleReport(cid, props.id, year),
    ])
    document.title = `Kniha jázd ${report.value.vehicle.plate} ${year}`
  } catch (e) {
    error.value = e.message
  }
})
const print = () => window.print()
</script>

<template>
  <div class="print-root">
    <nav class="toolbar">
      <Button icon="pi pi-arrow-left" label="Späť" text size="small" @click="router.push({ path: '/kniha-jazd', query: { vozidlo: id } })" />
      <div class="spacer" />
      <span class="tip">V okne tlače zvoľte „Uložiť ako PDF“ a orientáciu na šírku</span>
      <Button icon="pi pi-print" label="Tlačiť / PDF" size="small" :disabled="!report" @click="print" />
    </nav>
    <p v-if="error" class="error">{{ error }}</p>

    <article v-else-if="report" class="sheet">
      <header class="head">
        <div>
          <CompanyLogo class="logo" height="10mm" />
          <p class="kind">Kniha jázd {{ year }}</p>
          <h1>{{ v.plate }} {{ v.name }}</h1>
        </div>
        <div class="company">
          <strong>{{ company.name }}</strong>
          <span v-if="company.ico">IČO {{ company.ico }}<template v-if="company.icdph">, IČ DPH {{ company.icdph }}</template></span>
        </div>
      </header>

      <dl class="vehicle">
        <div><dt>Kategória</dt><dd>{{ v.category }}</dd></div>
        <div v-if="v.vin"><dt>VIN</dt><dd>{{ v.vin }}</dd></div>
        <div><dt>Vzťah k podnikaniu</dt><dd>{{ ownerships.find((o) => o.value === v.ownership)?.label }}</dd></div>
        <div><dt>Spotreba podľa TP</dt><dd>{{ formatAmount(v.consumption) }} {{ unit }}/100 km</dd></div>
        <div><dt>Tachometer</dt><dd>{{ km(report.odoStart) }} → {{ km(report.odoEnd) }} km</dd></div>
        <div><dt>Odpočet DPH</dt><dd>{{ report.vat.percent }} %<template v-if="v.fullVat && v.notified">, oznámené {{ formatDate(v.notified) }}</template></dd></div>
      </dl>

      <h2>Jazdy</h2>
      <table class="t">
        <thead>
          <tr>
            <th>Dátum</th><th>Čas</th><th>Odkiaľ</th><th>Kam</th><th>Účel</th><th>Druh</th><th>Vodič</th>
            <th class="n">Tach. začiatok</th><th class="n">Tach. koniec</th><th class="n">Km</th><th class="n">Náklady €</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in trips" :key="t.id" :class="{ private: t.kind === 'private' }">
            <td>{{ formatDate(t.date) }}</td><td>{{ tripTime(t) }}</td><td>{{ t.from }}</td><td>{{ t.to }}</td><td>{{ t.purpose }}</td>
            <td>{{ t.kind === 'private' ? 'súkromná' : 'služobná' }}</td><td>{{ t.driver }}</td>
            <td class="n">{{ t.odoStart || '' }}</td><td class="n">{{ t.odoEnd || '' }}</td><td class="n">{{ t.km }}</td>
            <td class="n">{{ costs.get(t.id) ? formatAmount(costs.get(t.id).total) : '' }}</td>
          </tr>
        </tbody>
        <tfoot>
          <tr>
            <td colspan="9">Služobne {{ km(report.kmBusiness) }} km, súkromne {{ km(report.kmPrivate) }} km (služobné {{ formatAmount(report.share) }} %)</td>
            <td class="n">{{ km(report.kmTotal) }}</td><td />
          </tr>
        </tfoot>
      </table>

      <h2>Tankovanie</h2>
      <table class="t">
        <thead><tr><th>Dátum</th><th>Čerpacia stanica</th><th>Doklad</th><th class="n">Množstvo ({{ unit }})</th><th class="n">Cena s DPH €</th><th class="n">Tachometer</th></tr></thead>
        <tbody>
          <tr v-if="!fuel.length"><td colspan="6">Bez tankovania.</td></tr>
          <tr v-for="f in fuel" :key="f.id">
            <td>{{ formatDate(f.date) }}</td><td>{{ f.station }}</td><td>{{ f.docNumber }}</td>
            <td class="n">{{ formatAmount(f.quantity) }}</td><td class="n">{{ formatAmount(f.amount) }}</td><td class="n">{{ f.odometer || '' }}</td>
          </tr>
        </tbody>
        <tfoot>
          <tr><td colspan="3">Spolu, priemerná spotreba {{ formatAmount(report.actual) }} {{ unit }}/100 km</td><td class="n">{{ formatAmount(report.quantity) }}</td><td class="n">{{ formatAmount(report.fuelCost) }}</td><td /></tr>
        </tfoot>
      </table>

      <template v-if="report.vat.docs.length">
        <h2>Náklady s DPH</h2>
        <table class="t">
          <thead><tr><th>Dátum</th><th>Doklad</th><th>Predmet</th><th class="n">DPH €</th><th class="n">Odpočet</th><th class="n">Odpočítaná DPH €</th></tr></thead>
          <tbody>
            <tr v-for="d in report.vat.docs" :key="d.kind + d.id">
              <td>{{ formatDate(d.date) }}</td><td>{{ d.number }}</td><td>{{ d.text }}</td>
              <td class="n">{{ formatAmount(d.vat) }}</td><td class="n">{{ d.percent }} %</td><td class="n">{{ formatAmount(d.deducted) }}</td>
            </tr>
          </tbody>
          <tfoot>
            <tr><td colspan="3">Spolu</td><td class="n">{{ formatAmount(report.vat.vat) }}</td><td /><td class="n">{{ formatAmount(report.vat.deducted) }}</td></tr>
          </tfoot>
        </table>
      </template>

      <h2>Vyúčtovanie {{ year }}</h2>
      <table class="t sum">
        <tbody>
          <template v-if="isPrivate">
            <tr><td>Náhrada za PHM</td><td class="n">{{ formatMoney(report.fuelExpense) }}</td></tr>
            <tr><td>Základná náhrada za {{ km(report.kmBusiness) }} služobných km</td><td class="n">{{ formatMoney(report.baseAllowance) }}</td></tr>
            <tr class="grand"><td>Cestovné náhrady spolu</td><td class="n"><span>{{ formatMoney(report.allowance) }}</span></td></tr>
          </template>
          <template v-else>
            <tr><td>PHM podľa dokladov</td><td class="n">{{ formatMoney(report.fuelCost) }}</td></tr>
            <tr v-if="Number(report.fuelPrivate)"><td>PHM na súkromné jazdy (nie je výdavok)</td><td class="n">−{{ formatMoney(report.fuelPrivate) }}</td></tr>
            <tr v-if="Number(report.fuelOveruse)"><td>Nadspotreba nad technický preukaz</td><td class="n">−{{ formatMoney(report.fuelOveruse) }}</td></tr>
            <tr><td>PHM – daňový výdavok</td><td class="n">{{ formatMoney(report.fuelExpense) }}</td></tr>
            <tr><td>Odpis vozidla</td><td class="n">{{ formatMoney(report.depreciation) }}</td></tr>
            <tr class="grand"><td>Výdavky na vozidlo spolu</td><td class="n"><span>{{ formatMoney(report.total) }}</span></td></tr>
          </template>
          <tr><td>Náklad na služobný km</td><td class="n">{{ formatMoney(report.perKm) }}</td></tr>
        </tbody>
      </table>

      <footer class="signs">
        <div><span>Vyhotovil</span></div>
        <div><span>Schválil</span></div>
      </footer>
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
/* A4 na šírku */
.sheet {
  width: 297mm;
  min-height: 210mm;
  margin: 24px auto 0;
  padding: 12mm 12mm;
  background: #fff;
  color: #1d2633;
  font-size: 8.5pt;
  line-height: 1.35;
  box-shadow: 0 1px 3px rgba(18, 24, 32, 0.12), 0 8px 28px rgba(18, 24, 32, 0.08);
}
.head {
  display: flex;
  justify-content: space-between;
  gap: 10mm;
  border-bottom: 2px solid #1f3a5c;
  padding-bottom: 3mm;
}
.kind {
  margin: 2mm 0 0;
  color: #4a5a6c;
  font-size: 9pt;
}
h1 {
  margin: 0;
  font-size: 15pt;
}
h2 {
  font-size: 10pt;
  margin: 6mm 0 2mm;
}
.company {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  text-align: right;
}
.vehicle {
  display: flex;
  flex-wrap: wrap;
  gap: 2mm 8mm;
  margin: 4mm 0 0;
}
.vehicle dt {
  color: #4a5a6c;
  font-size: 7.5pt;
}
.vehicle dd {
  margin: 0;
  font-weight: 600;
}
.t {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}
.t th {
  text-align: left;
  font-weight: 600;
  color: #4a5a6c;
  border-bottom: 1px solid #1d2633;
  padding: 1.2mm 1.5mm;
}
.t td {
  border-bottom: 1px solid #e1e6ec;
  padding: 1mm 1.5mm;
  vertical-align: top;
}
.t .n {
  text-align: right;
  white-space: nowrap;
}
.t tr.private td {
  color: #6b7685;
}
.t tfoot td {
  font-weight: 600;
  border-top: 1px solid #1d2633;
  border-bottom: 0;
}
.t thead {
  display: table-header-group;
}
.t tr {
  break-inside: avoid;
}
.sum {
  width: 110mm;
}
.sum .grand td {
  font-weight: 700;
  font-size: 10pt;
}
.sum .grand span {
  border-bottom: 3px double currentColor;
}
.signs {
  display: flex;
  gap: 20mm;
  margin-top: 14mm;
}
.signs div {
  width: 50mm;
  border-top: 1px solid #1d2633;
  padding-top: 1mm;
  color: #4a5a6c;
}
@media print {
  @page {
    size: A4 landscape;
    margin: 10mm;
  }
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
