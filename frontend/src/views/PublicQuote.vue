<script setup>
// Cenová ponuka pre zákazníka cez tajný odkaz: zobrazenie, PDF, prijatie s objednávkou alebo odmietnutie.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import { api, ApiError } from '../lib/api.js'
import { formatAmount, formatDate, formatMoney, formatQuantity } from '../lib/format.js'
import { publicVerdict } from '../lib/quotes.js'

const props = defineProps({ token: { type: String, required: true } })
const q = ref(null)
const error = ref('')
const mode = ref('') // '' | 'accept' | 'reject'
const form = ref({ name: '', order: '', note: '' })
const errors = ref({})
const busy = ref(false)

onMounted(async () => {
  document.title = 'Cenová ponuka'
  try {
    q.value = await api.publicQuote(props.token)
    document.title = `Cenová ponuka ${q.value.number} – ${q.value.supplier.Name}`
  } catch (e) {
    error.value = e.status === 404 ? 'Odkaz na ponuku neplatí. Mohol byť zrušený — kontaktujte dodávateľa.' : e.message
  }
})

const cur = computed(() => q.value?.currency || 'EUR')
const verdict = computed(() => (q.value ? publicVerdict(q.value) : 'open'))
const vatPayer = computed(() => q.value?.supplier.VATPayer && !q.value?.nonVatPayer)
const addr = (s) => [s.Street, [s.Zip, s.City].filter(Boolean).join(' ')].filter(Boolean).join(', ')

async function send() {
  busy.value = true
  errors.value = {}
  try {
    q.value = await api.respondQuote(props.token, { accepted: mode.value === 'accept', ...form.value })
    mode.value = ''
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) errors.value = e.fieldMap()
    else error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="pq">
    <p v-if="error" class="pq-error" role="alert">{{ error }}</p>
    <article v-if="q" class="sheet">
      <header class="top">
        <div class="supplier">
          <strong>{{ q.supplier.Name }}</strong>
          <span>{{ addr(q.supplier) }}</span>
          <span>
            <template v-if="q.supplier.ICO">IČO {{ q.supplier.ICO }}</template>
            <template v-if="q.supplier.ICDPH"> · IČ DPH {{ q.supplier.ICDPH }}</template>
          </span>
          <span v-if="q.supplier.Email || q.supplier.Phone">{{ [q.supplier.Email, q.supplier.Phone].filter(Boolean).join(' · ') }}</span>
        </div>
        <a class="pdf" :href="api.publicQuotePDF(token)" download><i class="pi pi-file-pdf" /> PDF</a>
      </header>

      <h1>Cenová ponuka <span class="num">{{ q.number }}</span></h1>
      <p v-if="q.subject" class="subject">{{ q.subject }}</p>

      <dl class="facts">
        <div><dt>Pre</dt><dd>{{ q.customer.name }}</dd></div>
        <div><dt>Dátum</dt><dd>{{ formatDate(q.issueDate) }}</dd></div>
        <div><dt>Platí do</dt><dd :class="{ late: q.expired }">{{ formatDate(q.validUntil) }}</dd></div>
      </dl>

      <p v-if="q.intro" class="text">{{ q.intro }}</p>

      <div class="lines-wrap">
        <table class="lines">
          <thead>
            <tr>
              <th>Položka</th>
              <th class="r">Množstvo</th>
              <th class="r">Cena za MJ</th>
              <th v-if="vatPayer" class="r hide-s">DPH</th>
              <th class="r">Suma</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(l, i) in q.lines" :key="i">
              <td>{{ l.description }}</td>
              <td class="r nowrap">{{ formatQuantity(l.quantity) }} {{ l.unit }}</td>
              <td class="r nowrap">{{ formatAmount(l.unitPrice) }}</td>
              <td v-if="vatPayer" class="r hide-s">{{ l.vatRate }} %</td>
              <td class="r nowrap">{{ formatAmount(l.amount) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="mode">{{ vatPayer ? (q.priceMode === 'gross' ? 'Ceny sú s DPH.' : 'Ceny sú bez DPH.') : 'Dodávateľ nie je platiteľ DPH.' }}</p>

      <section class="totals">
        <dl v-if="vatPayer">
          <template v-for="r in q.summary.rates" :key="r.rate">
            <dt>Základ {{ r.rate }} %</dt><dd>{{ formatMoney(r.base, cur) }}</dd>
            <dt>DPH {{ r.rate }} %</dt><dd>{{ formatMoney(r.vat, cur) }}</dd>
          </template>
        </dl>
        <div class="grand-row">
          <span>Spolu</span>
          <strong class="grand">{{ formatMoney(q.summary.total, cur) }}</strong>
        </div>
      </section>

      <p v-if="q.note" class="text">{{ q.note }}</p>

      <!-- odpoveď -->
      <section class="answer" :data-verdict="verdict" aria-live="polite">
        <template v-if="verdict === 'accepted'">
          <h2><i class="pi pi-check-circle" /> Ponuka je prijatá</h2>
          <p v-if="q.response">
            Prijal {{ q.response.name }}<template v-if="q.response.order">, objednávka {{ q.response.order }}</template>. Ďakujeme — dodávateľ sa vám ozve.
          </p>
        </template>
        <template v-else-if="verdict === 'rejected'">
          <h2><i class="pi pi-times-circle" /> Ponuka je odmietnutá</h2>
          <p v-if="q.response">Odpovedal {{ q.response.name }}. Ďakujeme za odpoveď.</p>
        </template>
        <template v-else-if="q.canRespond && !mode">
          <h2>Vyhovuje vám ponuka?</h2>
          <p v-if="q.expired">Platnosť ponuky uplynula. Prijať ju už nemožno — dodávateľ vám pripraví novú.</p>
          <div class="buttons">
            <Button v-if="!q.expired" label="Prijať ponuku" icon="pi pi-check" @click="mode = 'accept'" />
            <Button label="Odmietnuť" severity="secondary" outlined @click="mode = 'reject'" />
          </div>
        </template>
        <form v-else-if="q.canRespond" class="respond" @submit.prevent="send">
          <h2>{{ mode === 'accept' ? 'Prijatie ponuky' : 'Odmietnutie ponuky' }}</h2>
          <label>
            <span>Vaše meno</span>
            <InputText v-model="form.name" autocomplete="name" :invalid="!!errors.name" />
            <small v-if="errors.name" class="err">{{ errors.name }}</small>
          </label>
          <label v-if="mode === 'accept'">
            <span>Číslo vašej objednávky <em>(nepovinné)</em></span>
            <InputText v-model="form.order" :invalid="!!errors.order" />
            <small v-if="errors.order" class="err">{{ errors.order }}</small>
          </label>
          <label>
            <span>{{ mode === 'accept' ? 'Poznámka pre dodávateľa' : 'Dôvod (nepovinné)' }}</span>
            <Textarea v-model="form.note" rows="3" auto-resize :invalid="!!errors.note" />
            <small v-if="errors.note" class="err">{{ errors.note }}</small>
          </label>
          <div class="buttons">
            <Button type="submit" :label="mode === 'accept' ? 'Potvrdiť prijatie' : 'Odoslať odmietnutie'" :icon="mode === 'accept' ? 'pi pi-check' : undefined" :loading="busy" />
            <Button label="Späť" severity="secondary" text @click="mode = ''" />
          </div>
          <p v-if="mode === 'accept'" class="small">Prijatím súhlasíte s dodávkou za uvedené ceny. Dodávateľ dostane správu.</p>
        </form>
        <p v-else-if="verdict === 'expired'">Platnosť ponuky uplynula.</p>
      </section>
    </article>
    <p v-else-if="!error" class="loading">Načítavam ponuku…</p>
    <footer class="made">Ponuka vystavená v programe Fiškus</footer>
  </div>
</template>

<style scoped>
.pq {
  min-height: 100vh;
  background: var(--paper, #eef1f4);
  padding: 32px 16px 24px;
  font-variant-numeric: tabular-nums;
}
.sheet {
  max-width: 760px;
  margin: 0 auto;
  background: var(--sheet);
  border-top: 3px solid var(--ink);
  box-shadow: 0 1px 3px rgba(31, 58, 92, 0.12);
  padding: 28px 32px 24px;
}
.top {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
}
.supplier {
  display: flex;
  flex-direction: column;
  gap: 1px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.supplier strong {
  font-size: var(--fs-lg);
  color: var(--ink);
}
.pdf {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--blue-deep);
  font-weight: 600;
  text-decoration: none;
  white-space: nowrap;
}
h1 {
  margin: 24px 0 2px;
  font-size: 1.6rem;
  color: var(--blue-deep);
}
h1 .num {
  color: var(--ink);
}
.subject {
  margin: 0;
  font-size: var(--fs-lg);
}
.facts {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 28px;
  margin: 14px 0;
  padding: 10px 0;
  border-top: 1px solid var(--rule);
  border-bottom: 1px solid var(--rule);
}
.facts dt {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.facts dd {
  margin: 0;
  font-weight: 600;
}
.facts .late {
  color: var(--tone-overdue);
}
.text {
  white-space: pre-line;
}
.lines-wrap {
  overflow-x: auto;
}
.lines {
  width: 100%;
  border-collapse: collapse;
  margin-top: 8px;
}
.lines th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 500;
  color: var(--ink-soft);
  border-bottom: 2px solid var(--ink);
  padding: 4px 6px;
}
.lines td {
  padding: 7px 6px;
  border-bottom: 1px solid var(--rule);
  vertical-align: top;
}
.r {
  text-align: right !important;
}
.nowrap {
  white-space: nowrap;
}
.mode {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  margin: 6px 0 0;
}
.totals {
  margin: 14px 0 0 auto;
  max-width: 320px;
}
.totals dl {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 2px 16px;
  margin: 0 0 6px;
  font-size: var(--fs-sm);
}
.totals dd {
  margin: 0;
  text-align: right;
}
.grand-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  border-top: 1px solid var(--rule);
  padding-top: 8px;
}
.grand {
  font-size: 1.5rem;
  border-bottom: 3px double currentColor;
  line-height: 1.15;
}
.answer {
  margin-top: 28px;
  padding: 16px 18px;
  background: var(--blue-wash);
  border-left: 3px solid var(--blue);
}
.answer[data-verdict='accepted'] {
  border-left-color: var(--tone-paid);
}
.answer[data-verdict='rejected'],
.answer[data-verdict='expired'] {
  border-left-color: var(--ink-faint);
}
.answer h2 {
  font-size: var(--fs-lg);
  margin: 0 0 6px;
}
.answer[data-verdict='accepted'] h2 {
  color: var(--tone-paid);
}
.answer p {
  margin: 0 0 6px;
}
.buttons {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 8px;
}
.respond {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.respond label {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.respond label span {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.respond em {
  font-style: normal;
  color: var(--ink-faint);
}
.err {
  color: var(--tone-overdue);
}
.small {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.pq-error {
  max-width: 760px;
  margin: 0 auto 12px;
  color: var(--tone-overdue);
  font-weight: 600;
}
.loading,
.made {
  text-align: center;
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.made {
  margin-top: 16px;
}
@media (max-width: 760px) {
  .pq {
    padding: 0 0 16px;
  }
  .sheet {
    padding: 18px 16px;
    box-shadow: none;
  }
  .hide-s {
    display: none;
  }
  h1 {
    font-size: 1.3rem;
  }
  .totals {
    max-width: none;
  }
}
</style>
