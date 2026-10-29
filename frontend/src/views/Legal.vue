<script setup>
// Legislatíva: zákonné parametre spoločné pre všetky firmy (sadzby DPH,
// sadzby ECB, úrok z omeškania). Nová inštalácia ich má predvyplnené.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import { useConfirm } from 'primevue/useconfirm'
import { useToast } from 'primevue/usetoast'
import { api, ApiError } from '../lib/api.js'
import { loadCodes } from '../lib/codes.js'
import DatePicker from 'primevue/datepicker'
import { formatDate, fromISODate, toISODate } from '../lib/format.js'
import { matches, searchTerms } from '../lib/search.js'
import { ecbColumns, vatPeriodColumns } from '../lib/columns.js'
import { ecbErrors, formatRates, parseRates } from '../lib/legal.js'
import ExportButton from '../components/ExportButton.vue'
import FormField from '../components/FormField.vue'

const toast = useToast()
const confirm = useConfirm()

const form = ref(null) // { vatPeriods: [{from, to, ratesText}], ecb: [{from, rate, _k}], businessMargin, consumerMargin, flatFee }
const errors = ref({})
const busy = ref(false)
const dirty = ref(false)
const search = ref('')
let key = 0

function toForm(p) {
  return {
    vatPeriods: p.vatPeriods.map((v) => ({ from: v.from, to: v.to ?? '', ratesText: formatRates(v.rates), _k: ++key })),
    // najnovšie hore — tie sa menia najčastejšie
    ecb: [...p.ecb].reverse().map((r) => ({ from: r.from, rate: Number(r.rate), _k: ++key })),
    businessMargin: Number(p.businessMargin),
    consumerMargin: Number(p.consumerMargin),
    flatFee: Number(p.flatFee),
  }
}
const toAPI = (f) => ({
  vatPeriods: f.vatPeriods.map((v) => ({ from: v.from, to: v.to || '', rates: parseRates(v.ratesText) })),
  ecb: f.ecb.map((r) => ({ from: r.from || null, rate: String(r.rate ?? '') })),
  businessMargin: String(f.businessMargin ?? 0),
  consumerMargin: String(f.consumerMargin ?? 0),
  flatFee: String(f.flatFee ?? 0),
})

async function load() {
  form.value = toForm(await api.legal())
  dirty.value = false
  errors.value = {}
}
onMounted(load)
const touch = () => (dirty.value = true)
function setDate(row, key, d) {
  row[key] = d ? toISODate(d) : ''
  touch()
}

const visibleECB = computed(() => {
  const terms = searchTerms(search.value)
  return (form.value?.ecb ?? []).map((r, i) => ({ ...r, _i: i })).filter((r) => matches(terms, [formatDate(r.from), r.from, String(r.rate)]))
})
const ecbErr = computed(() => ecbErrors(errors.value))
const ecbExport = () => visibleECB.value.map((r) => ({ from: r.from, rate: r.rate }))
const vatExport = () => form.value.vatPeriods.map((v) => ({ from: v.from, to: v.to, rates: parseRates(v.ratesText) }))
const latestECB = computed(() => [...(form.value?.ecb ?? [])].filter((r) => r.from).sort((a, b) => b.from.localeCompare(a.from))[0])

function addECB() {
  form.value.ecb.unshift({ from: new Date().toISOString().slice(0, 10), rate: latestECB.value?.rate ?? 0, _k: ++key })
  search.value = ''
  touch()
}
function removeECB(i) {
  form.value.ecb.splice(i, 1)
  touch()
}
function addPeriod() {
  const ps = form.value.vatPeriods
  const last = ps[ps.length - 1]
  ps.push({ from: '', to: '', ratesText: last?.ratesText ?? '', _k: ++key })
  touch()
}
function removePeriod(i) {
  form.value.vatPeriods.splice(i, 1)
  touch()
}

async function save() {
  busy.value = true
  try {
    form.value = toForm(await api.saveLegal(toAPI(form.value)))
    errors.value = {}
    dirty.value = false
    await loadCodes() // nové sadzby DPH hneď v editore faktúr
    toast.add({ severity: 'success', summary: 'Legislatíva uložená', detail: 'Platí pre všetky firmy.', life: 3000 })
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    toast.add({ severity: 'error', summary: 'Neuložené', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

function reset() {
  confirm.require({
    header: 'Obnoviť hodnoty programu',
    message: 'Všetky sadzby DPH, sadzby ECB a parametre úroku z omeškania sa nahradia hodnotami dodanými s programom. Ručné úpravy sa stratia (zostanú v audite).',
    acceptProps: { label: 'Obnoviť', severity: 'danger', size: 'small' },
    rejectProps: { label: 'Späť', severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      form.value = toForm(await api.resetLegal())
      dirty.value = false
      errors.value = {}
      await loadCodes()
      toast.add({ severity: 'info', summary: 'Obnovené hodnoty programu', life: 3000 })
    },
  })
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Legislatíva</h1>
      <span v-if="dirty" class="unsaved">neuložené zmeny</span>
      <div class="spacer" />
      <Button label="Obnoviť hodnoty programu" severity="secondary" text size="small" :disabled="busy" @click="reset" />
      <Button label="Uložiť" icon="pi pi-check" size="small" :loading="busy" :disabled="!form" @click="save" />
    </header>
    <p class="intro">
      Zákonné hodnoty platné pre všetky firmy. Nová inštalácia ich má predvyplnené. Keď sa zmení zákon alebo sadzba ECB, upravíte
      ich tu. Nová verzia programu doplní nové hodnoty sama a vaše úpravy nechá tak. Každá zmena sa zapíše do auditu.
    </p>

    <template v-if="form">
      <section class="card">
        <div class="card-head">
          <h2>Sadzby DPH</h2>
          <div class="spacer" />
          <ExportButton name="Sadzby DPH" :columns="vatPeriodColumns" :rows="vatExport" />
        </div>
        <p class="hint">Sadzby pre faktúry s dátumom dodania v danom období. Bez konca môže byť iba posledné obdobie. 0 % = oslobodené plnenie a prenesenie daňovej povinnosti.</p>
        <table class="grid vat">
          <thead><tr><th>Platí od</th><th>Platí do</th><th>Sadzby (%)</th><th aria-label="Akcie" /></tr></thead>
          <tbody>
            <tr v-for="(v, i) in form.vatPeriods" :key="v._k">
              <td>
                <DatePicker :model-value="fromISODate(v.from)" date-format="d. m. yy" size="small" fluid aria-label="Platí od"
                  @update:model-value="(d) => setDate(v, 'from', d)" />
              </td>
              <td>
                <DatePicker :model-value="fromISODate(v.to)" date-format="d. m. yy" show-clear size="small" fluid placeholder="doteraz" aria-label="Platí do"
                  @update:model-value="(d) => setDate(v, 'to', d)" />
              </td>
              <td><InputText v-model="v.ratesText" size="small" placeholder="23, 19, 5, 0" aria-label="Sadzby" @input="touch" /></td>
              <td class="act"><Button icon="pi pi-times" text severity="secondary" size="small" aria-label="Odstrániť obdobie" @click="removePeriod(i)" /></td>
            </tr>
          </tbody>
        </table>
        <small v-if="errors.vatPeriods" class="err">{{ errors.vatPeriods }}</small>
        <Button label="Pridať obdobie" icon="pi pi-plus" text size="small" @click="addPeriod" />
      </section>

      <section class="card">
        <h2>Úrok z omeškania a upomienky</h2>
        <div class="form-grid">
          <FormField label="Prirážka k ECB – podnikateľ (p. b.)" :error="errors.businessMargin" hint="§ 369 ObZ, NV č. 21/2013 Z. z.">
            <InputNumber v-model="form.businessMargin" :min-fraction-digits="0" :max-fraction-digits="2" locale="sk-SK" size="small" fluid @input="touch" />
          </FormField>
          <FormField label="Prirážka k ECB – spotrebiteľ (p. b.)" :error="errors.consumerMargin" hint="§ 517 OZ, NV č. 87/1995 Z. z.">
            <InputNumber v-model="form.consumerMargin" :min-fraction-digits="0" :max-fraction-digits="2" locale="sk-SK" size="small" fluid @input="touch" />
          </FormField>
          <FormField label="Paušálna náhrada za faktúru (€)" :error="errors.flatFee" hint="§ 369c ObZ, iba voči podnikateľovi">
            <InputNumber v-model="form.flatFee" :min-fraction-digits="2" :max-fraction-digits="2" locale="sk-SK" size="small" fluid @input="touch" />
          </FormField>
        </div>
      </section>

      <section class="card">
        <div class="card-head">
          <h2>Základná úroková sadzba ECB <span class="muted">{{ form.ecb.length }}</span></h2>
          <div class="spacer" />
          <IconField class="search">
            <InputIcon class="pi pi-search" />
            <InputText v-model="search" placeholder="Hľadať dátum alebo sadzbu" size="small" aria-label="Hľadať sadzbu ECB" />
          </IconField>
          <ExportButton name="Sadzby ECB" :columns="ecbColumns" :rows="ecbExport" />
        </div>
        <p class="hint">
          Sadzba hlavných refinančných operácií ECB od daného dňa. Pre úrok z omeškania sa použije sadzba platná k prvému dňu omeškania.
          <template v-if="latestECB">Posledná zmena: {{ formatDate(latestECB.from) }} — {{ String(latestECB.rate).replace('.', ',') }} %.</template>
        </p>
        <small v-if="errors.ecb" class="err">{{ errors.ecb }}</small>
        <Button label="Pridať sadzbu" icon="pi pi-plus" text size="small" @click="addECB" />
        <table class="grid ecb">
          <thead><tr><th>Platí od</th><th>Sadzba (%)</th><th aria-label="Akcie" /></tr></thead>
          <tbody>
            <tr v-if="!visibleECB.length"><td colspan="3" class="muted">Nič sa nenašlo.</td></tr>
            <tr v-for="r in visibleECB" :key="r._k">
              <td>
                <DatePicker :model-value="fromISODate(form.ecb[r._i].from)" date-format="d. m. yy" size="small" fluid
                  :invalid="!!ecbErr[r._i]?.from" aria-label="Platí od" @update:model-value="(d) => setDate(form.ecb[r._i], 'from', d)" />
                <small v-if="ecbErr[r._i]?.from" class="err">{{ ecbErr[r._i].from }}</small>
              </td>
              <td>
                <InputNumber v-model="form.ecb[r._i].rate" :min-fraction-digits="2" :max-fraction-digits="3" locale="sk-SK" size="small"
                  :invalid="!!ecbErr[r._i]?.rate" aria-label="Sadzba" @input="touch" />
                <small v-if="ecbErr[r._i]?.rate" class="err">{{ ecbErr[r._i].rate }}</small>
              </td>
              <td class="act"><Button icon="pi pi-times" text severity="secondary" size="small" aria-label="Odstrániť sadzbu" @click="removeECB(r._i)" /></td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  max-width: 900px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.head h1 {
  font-size: var(--fs-xl);
  margin: 0;
}
.unsaved {
  font-size: var(--fs-sm);
  color: var(--tone-draft);
}
.spacer {
  flex: 1;
}
.intro,
.hint {
  margin: 0;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.hint {
  color: var(--ink-faint);
  margin-bottom: 6px;
}
.card {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 12px 14px;
}
.card h2 {
  font-size: var(--fs-md);
  margin: 0 0 6px;
}
.card-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 4px;
}
.card-head h2 {
  margin: 0;
}
.muted {
  color: var(--ink-faint);
  font-weight: 400;
  font-size: var(--fs-sm);
}
.search {
  width: 220px;
}
.search :deep(input) {
  width: 100%;
}
.grid {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.grid th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 4px 8px;
  border-bottom: 1px solid var(--rule);
}
.grid td {
  padding: 3px 8px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: top;
}
.grid td :deep(input) {
  width: 100%;
  font-variant-numeric: tabular-nums;
}
.grid.ecb {
  max-width: 460px;
}
.act {
  width: 40px;
  text-align: right;
}
.err {
  display: block;
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.form-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}
@media (max-width: 640px) {
  .page {
    padding: 10px 10px 20px;
  }
  .form-grid {
    grid-template-columns: 1fr;
  }
  .search {
    flex: 1;
    width: auto;
  }
  .grid td {
    padding: 3px 4px;
  }
  /* obdobie DPH na mobile: od – do v riadku, sadzby pod nimi */
  .grid.vat thead {
    display: none;
  }
  .grid.vat tr {
    display: grid;
    grid-template-columns: 1fr 1fr 36px;
    border-bottom: 1px solid var(--rule);
    padding: 4px 0;
  }
  .grid.vat td {
    border: 0;
  }
  .grid.vat td:nth-child(3) {
    grid-column: 1 / 3;
  }
  .grid.vat td:nth-child(4) {
    grid-column: 3;
    grid-row: 1;
  }
}
</style>
