<script setup>
// Kniha jázd: jazdy, tankovanie a ročné vyúčtovanie vozidla (PHM, cestovné
// náhrady, odpis, náklady na km). Všetky sumy počíta server.
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { AgGridVue } from 'ag-grid-vue3'
import Button from 'primevue/button'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import AutoComplete from 'primevue/autocomplete'
import DatePicker from 'primevue/datepicker'
import Checkbox from 'primevue/checkbox'
import Drawer from 'primevue/drawer'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { fiskusGridTheme, skGridLocale, gridRows } from '../lib/grid.js'
import { fuelColumns, tripColumns } from '../lib/columns.js'
import {
  blankTrip, filterFuel, filterTrips, fuelKinds, fuelUnit, logbookPosting, methodHint, methodOptions, ownerships,
  purposeSuggestions, returnTrip, tripKinds, tripRoute, tripTime, vehicleLabel,
} from '../lib/logbook.js'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('ledger.write'))
const todayISO = toISODate(new Date())

const thisYear = new Date().getFullYear()
const years = [thisYear, thisYear - 1, thisYear - 2].map((y) => ({ label: String(y), value: y }))
const year = ref(thisYear)
const tab = ref('trips')
const tabs = [
  { value: 'trips', label: 'Jazdy' },
  { value: 'fuel', label: 'Tankovanie' },
  { value: 'report', label: 'Vyúčtovanie' },
]

const vehicles = ref([])
const vehicleId = computed({
  get: () => Number(route.query.vozidlo ?? 0) || vehicles.value.find((v) => !v.archived)?.id || vehicles.value[0]?.id || 0,
  set: (v) => router.replace({ query: { ...route.query, vozidlo: v ? String(v) : undefined } }),
})
const vehicle = computed(() => vehicles.value.find((v) => v.id === vehicleId.value) ?? null)
const vehicleOptions = computed(() => vehicles.value.filter((v) => !v.archived || v.id === vehicleId.value).map((v) => ({ value: v.id, label: vehicleLabel(v) })))
const unit = computed(() => fuelUnit(vehicle.value?.fuel))

const trips = ref([])
const fuel = ref([])
const report = ref(null)
const loading = ref(true)
const loadError = ref('')

async function loadVehicles() {
  vehicles.value = await api.vehicles(session.companyId)
}
async function load() {
  if (!vehicleId.value) {
    trips.value = []
    fuel.value = []
    report.value = null
    loading.value = false
    return
  }
  loading.value = true
  try {
    const [t, f, r] = await Promise.all([
      api.trips(session.companyId, vehicleId.value, year.value),
      api.fuel(session.companyId, vehicleId.value, year.value),
      api.vehicleReport(session.companyId, vehicleId.value, year.value),
    ])
    trips.value = t
    fuel.value = f
    report.value = r
    loadError.value = ''
  } catch (e) {
    loadError.value = e.message
  } finally {
    loading.value = false
  }
}
async function reload() {
  await loadVehicles()
  await load()
}
watch([vehicleId, year], load)
onMounted(async () => {
  try {
    await loadVehicles()
  } catch (e) {
    loadError.value = e.message
  }
  await load()
})

// ---- jazdy ----
const search = ref('')
const kind = ref('all')
const kindFilter = [{ value: 'all', label: 'Všetky' }, ...tripKinds]
const costs = computed(() => new Map((report.value?.trips ?? []).map((c) => [c.tripId, c])))
const tripRows = computed(() => filterTrips(trips.value, search.value, kind.value).map((t) => ({ ...t, cost: costs.value.get(t.id) })).reverse())
const tripCols = [
  { field: 'date', headerName: 'Dátum', width: 100, valueFormatter: (p) => formatDate(p.value) },
  { colId: 'time', headerName: 'Čas', width: 100, valueGetter: (p) => tripTime(p.data) },
  { colId: 'route', headerName: 'Trasa', flex: 2, minWidth: 170, valueGetter: (p) => tripRoute(p.data), cellClass: 'cell-name' },
  { field: 'purpose', headerName: 'Účel', flex: 2, minWidth: 140 },
  {
    field: 'kind', headerName: 'Druh', width: 100,
    cellRenderer: (p) => `<span class="tag" data-tone="${p.value === 'private' ? 'draft' : 'issued'}"><span class="tag-text">${p.value === 'private' ? 'súkromná' : 'služobná'}</span></span>`,
  },
  { colId: 'odo', headerName: 'Tachometer', width: 130, type: 'rightAligned', valueGetter: (p) => (p.data.odoStart ? `${p.data.odoStart}–${p.data.odoEnd}` : '') },
  { field: 'km', headerName: 'Km', width: 80, type: 'rightAligned', cellClass: 'ag-right-aligned-cell cell-name' },
  {
    colId: 'cost', headerName: 'Náklady', width: 110, type: 'rightAligned',
    valueGetter: (p) => (p.data.cost ? Number(p.data.cost.total) : null), valueFormatter: (p) => (p.value == null ? '' : formatAmount(p.value)),
    tooltipValueGetter: (p) => (p.data.cost ? `PHM ${formatMoney(p.data.cost.fuel)}, opotrebenie ${formatMoney(p.data.cost.wear)}` : ''),
  },
]
const fuelRows = computed(() => filterFuel(fuel.value, search.value).slice().reverse())
const fuelCols = [
  { field: 'date', headerName: 'Dátum', width: 100, valueFormatter: (p) => formatDate(p.value) },
  { colId: 'qty', headerName: 'Množstvo', width: 110, type: 'rightAligned', valueGetter: (p) => Number(p.data.quantity), valueFormatter: (p) => `${formatAmount(p.value)} ${unit.value}` },
  { colId: 'amount', headerName: 'Cena s DPH', width: 120, type: 'rightAligned', valueGetter: (p) => Number(p.data.amount), valueFormatter: (p) => formatAmount(p.value), cellClass: 'ag-right-aligned-cell cell-name' },
  { field: 'odometer', headerName: 'Tachometer', width: 110, type: 'rightAligned', valueFormatter: (p) => p.value || '' },
  { field: 'station', headerName: 'Čerpacia stanica', flex: 1, minWidth: 140 },
  { field: 'docNumber', headerName: 'Doklad', width: 140, cellClass: 'cell-code' },
]
const gridApi = shallowRef(null)
const narrow = window.matchMedia('(max-width: 760px)')
function applyResponsive() {
  gridApi.value?.setColumnsVisible(['time', 'purpose', 'odo', 'cost', 'kind', 'odometer', 'station', 'docNumber'], !narrow.matches)
  gridApi.value?.sizeColumnsToFit()
}
narrow.addEventListener('change', applyResponsive)
watch(tab, () => setTimeout(applyResponsive))

// ---- formulár jazdy ----
const tripForm = ref(null)
const errors = ref({})
const busy = ref(false)
const purposes = computed(() => purposeSuggestions(trips.value))
const purposeHits = ref([])
const places = computed(() => [...new Set(trips.value.flatMap((t) => [t.from, t.to]).filter(Boolean))])
const placeHits = ref([])
function suggest(list, q) {
  const s = q.toLowerCase()
  return list.filter((x) => x.toLowerCase().includes(s)).slice(0, 10)
}
function openTrip(t) {
  errors.value = {}
  const x = t ?? blankTrip(vehicle.value, todayISO)
  tripForm.value = { ...x, date: fromISODate(x.date) }
}
const tripOpen = computed({ get: () => !!tripForm.value, set: (v) => !v && (tripForm.value = null) })
const tripKm = computed(() => {
  const f = tripForm.value
  return f?.odoStart && f?.odoEnd && f.odoEnd > f.odoStart ? f.odoEnd - f.odoStart : null
})
function applyError(e) {
  if (e instanceof ApiError && e.fields.length) {
    errors.value = e.fieldMap()
    toast.add({ severity: 'error', summary: 'Skontrolujte vyznačené polia', detail: e.fields[0].message, life: 5000 })
  } else {
    toast.add({ severity: 'error', summary: 'Uloženie zlyhalo', detail: e.message, life: 6000 })
  }
}
async function saveTrip(andReturn = false) {
  busy.value = true
  try {
    const f = tripForm.value
    const saved = await api.saveTrip(session.companyId, { ...f, date: toISODate(f.date), km: tripKm.value ?? f.km ?? 0, odoStart: f.odoStart || 0, odoEnd: f.odoEnd || 0 })
    toast.add({ severity: 'success', summary: f.id ? 'Jazda uložená' : 'Jazda zapísaná', detail: `${tripRoute(saved)}, ${saved.km} km`, life: 2500 })
    await reload()
    if (andReturn) {
      errors.value = {}
      const back = returnTrip(saved)
      tripForm.value = { ...back, date: fromISODate(back.date) }
    } else tripForm.value = null
  } catch (e) {
    applyError(e)
  } finally {
    busy.value = false
  }
}
function copyTrip(t) {
  const c = { ...t, id: 0, date: todayISO, depart: '', arrive: '', odoStart: vehicle.value?.lastOdometer || null, odoEnd: null }
  if (c.odoStart && t.km) c.odoEnd = c.odoStart + t.km
  openTrip(c)
}
function removeTrip(t) {
  confirm.require({
    header: 'Zmazať jazdu', message: `${formatDate(t.date)} ${tripRoute(t)}, ${t.km} km sa z knihy jázd odstráni.`,
    acceptProps: { label: 'Zmazať', severity: 'danger' }, rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await api.deleteTrip(session.companyId, t.id)
        tripForm.value = null
        await reload()
      } catch (e) {
        applyError(e)
      }
    },
  })
}

// ---- formulár tankovania ----
const fuelForm = ref(null)
const fuelDocs = ref([])
const fuelOpen = computed({ get: () => !!fuelForm.value, set: (v) => !v && (fuelForm.value = null) })
async function openFuel(f) {
  errors.value = {}
  const x = f ?? { id: 0, vehicleId: vehicleId.value, date: todayISO, quantity: null, amount: null, odometer: vehicle.value?.lastOdometer || null, full: true, station: '', docKind: '', docId: 0, note: '' }
  fuelForm.value = { ...x, date: fromISODate(x.date), quantity: x.quantity == null ? null : Number(x.quantity), amount: x.amount == null ? null : Number(x.amount), doc: x.docKind ? `${x.docKind}:${x.docId}` : null }
  try {
    fuelDocs.value = await api.fuelDocs(session.companyId, year.value)
  } catch {
    fuelDocs.value = []
  }
}
const docOptions = computed(() => {
  const list = fuelDocs.value.map((d) => ({ value: `${d.kind}:${d.id}`, label: `${d.number} · ${formatDate(d.date)} · ${d.partner || d.text} · ${formatMoney(d.amount)}`, doc: d }))
  const f = fuelForm.value
  if (f?.docKind && !list.some((o) => o.value === f.doc)) list.unshift({ value: f.doc, label: f.docNumber })
  return list
})
function pickDoc(v) {
  const f = fuelForm.value
  f.doc = v
  const o = docOptions.value.find((x) => x.value === v)
  if (!o?.doc) return
  f.amount = Number(o.doc.amount)
  f.date = fromISODate(o.doc.date)
  if (!f.station) f.station = o.doc.partner
}
async function saveFuel() {
  busy.value = true
  try {
    const f = fuelForm.value
    const [docKind, docId] = f.doc ? f.doc.split(':') : ['', '0']
    await api.saveFuel(session.companyId, {
      ...f, date: toISODate(f.date), quantity: String(f.quantity ?? 0), amount: String(f.amount ?? 0), odometer: f.odometer || 0,
      docKind, docId: Number(docId),
    })
    toast.add({ severity: 'success', summary: f.id ? 'Tankovanie uložené' : 'Tankovanie zapísané', life: 2500 })
    fuelForm.value = null
    await reload()
  } catch (e) {
    applyError(e)
  } finally {
    busy.value = false
  }
}
function removeFuel(f) {
  confirm.require({
    header: 'Zmazať tankovanie', message: `Tankovanie ${formatDate(f.date)} za ${formatMoney(f.amount)} sa odstráni. Doklad ostane vo výdavkoch.`,
    acceptProps: { label: 'Zmazať', severity: 'danger' }, rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await api.deleteFuel(session.companyId, f.id)
        fuelForm.value = null
        await reload()
      } catch (e) {
        applyError(e)
      }
    },
  })
}

// ---- vozidlo ----
const vehForm = ref(null)
const vehOpen = computed({ get: () => !!vehForm.value, set: (v) => !v && (vehForm.value = null) })
const assets = ref([])
async function openVehicle(v) {
  errors.value = {}
  const x = v ?? { id: 0, name: '', plate: '', moto: false, fuel: 'diesel', consumption: null, ownership: 'business', method: 'log', assetId: 0, driver: '', odometer: null, odometerDate: todayISO, archived: false, note: '' }
  vehForm.value = { ...x, consumption: x.consumption == null ? null : Number(x.consumption), odometerDate: x.odometerDate ? fromISODate(x.odometerDate) : null, assetId: x.assetId || null }
  try {
    const reg = await api.assets(session.companyId, year.value)
    assets.value = reg.items.filter((a) => a.kind === 'tangible').map((a) => ({ value: a.id, label: `${a.number} ${a.name}` }))
  } catch {
    assets.value = []
  }
}
async function saveVehicle() {
  busy.value = true
  try {
    const f = vehForm.value
    const saved = await api.saveVehicle(session.companyId, {
      ...f, consumption: String(f.consumption ?? 0), odometer: f.odometer || 0, odometerDate: f.odometerDate ? toISODate(f.odometerDate) : '', assetId: f.assetId || 0,
    })
    toast.add({ severity: 'success', summary: f.id ? 'Vozidlo uložené' : 'Vozidlo pridané', life: 2500 })
    vehForm.value = null
    await loadVehicles()
    vehicleId.value = saved.id
    await load()
  } catch (e) {
    applyError(e)
  } finally {
    busy.value = false
  }
}
function removeVehicle() {
  const v = vehForm.value
  confirm.require({
    header: 'Zmazať vozidlo', message: `${v.plate} ${v.name} sa odstráni z knihy jázd.`,
    acceptProps: { label: 'Zmazať', severity: 'danger' }, rejectProps: { label: 'Ponechať', severity: 'secondary', outlined: true },
    accept: async () => {
      try {
        await api.deleteVehicle(session.companyId, v.id)
        vehForm.value = null
        router.replace({ query: {} })
        await reload()
      } catch (e) {
        applyError(e)
      }
    },
  })
}

// ---- vyúčtovanie ----
const posting = computed(() => logbookPosting(report.value))
const postBusy = ref(false)
async function post() {
  postBusy.value = true
  try {
    const r = await api.postLogbook(session.companyId, vehicleId.value, year.value)
    toast.add({ severity: 'success', summary: `Kniha jázd ${year.value} zaúčtovaná`, detail: `${formatMoney(r.posted)} v uzávierke peňažného denníka (stĺpec e6)`, life: 4000 })
    await load()
  } catch (e) {
    applyError(e)
  } finally {
    postBusy.value = false
  }
}
const monthNames = ['jan', 'feb', 'mar', 'apr', 'máj', 'jún', 'júl', 'aug', 'sep', 'okt', 'nov', 'dec']
const activeMonths = computed(() => (report.value?.months ?? []).filter((m) => m.kmBusiness || m.kmPrivate || Number(m.fuelCost)))
const isPrivate = computed(() => vehicle.value?.ownership === 'private')
const fmtKm = (n) => `${new Intl.NumberFormat('sk-SK').format(n ?? 0)} km`
const fmtQty = (q) => `${formatAmount(q)} ${unit.value}`

function onKey(e) {
  const t = e.target
  const typing = t instanceof HTMLElement && /INPUT|TEXTAREA|SELECT/.test(t.tagName)
  if (typing || e.ctrlKey || e.metaKey || !canWrite.value || !vehicle.value || tripForm.value || fuelForm.value || vehForm.value) return
  if (e.key === 'n') {
    e.preventDefault()
    openTrip()
  } else if (e.key === 't') {
    e.preventDefault()
    openFuel()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  narrow.removeEventListener('change', applyResponsive)
})
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Kniha jázd</h1>
      <Select v-if="vehicleOptions.length" v-model="vehicleId" :options="vehicleOptions" option-label="label" option-value="value" size="small"
        aria-label="Vozidlo" class="veh-select" />
      <Select v-model="year" :options="years" option-label="label" option-value="value" size="small" aria-label="Rok" />
      <div class="spacer" />
      <template v-if="canWrite">
        <Button v-if="vehicle" icon="pi pi-cog" text severity="secondary" size="small" aria-label="Upraviť vozidlo" v-tooltip.bottom="'Upraviť vozidlo'" @click="openVehicle(vehicle)" />
        <Button icon="pi pi-plus" label="Vozidlo" text size="small" @click="openVehicle()" />
        <template v-if="vehicle && !vehicle.archived">
          <Button size="small" @click="openTrip()"><i class="pi pi-plus" /><span>Jazda</span><kbd class="hint">N</kbd></Button>
          <Button size="small" severity="secondary" outlined @click="openFuel()"><i class="pi pi-plus" /><span>Tankovanie</span><kbd class="hint dark">T</kbd></Button>
        </template>
      </template>
    </header>

    <p v-if="loadError" class="error">{{ loadError }}</p>

    <section v-if="!loading && !vehicles.length" class="empty">
      <h2>Zatiaľ žiadne vozidlo</h2>
      <p>Pridajte auto, ktorým jazdíte služobne — firemné z obchodného majetku, na leasing alebo vlastné súkromné. Kniha jázd potom z jázd a tankovaní vypočíta výdavky na PHM, cestovné náhrady a náklady na kilometer.</p>
      <Button v-if="canWrite" label="Pridať vozidlo" icon="pi pi-plus" size="small" @click="openVehicle()" />
    </section>

    <template v-else-if="vehicle">
      <section v-if="report" class="ledger" aria-label="Súhrn roka">
        <div class="figure">
          <span class="label">Služobne</span>
          <span class="value">{{ fmtKm(report.kmBusiness) }}</span>
          <span class="sub">{{ formatAmount(report.share) }} % z {{ fmtKm(report.kmTotal) }}</span>
        </div>
        <div class="figure">
          <span class="label">Súkromne</span>
          <span class="value">{{ fmtKm(report.kmPrivate) }}</span>
        </div>
        <div class="figure">
          <span class="label">PHM</span>
          <span class="value">{{ formatMoney(report.fuelCost) }}</span>
          <span class="sub">{{ fmtQty(report.quantity) }}<template v-if="Number(report.actual)">, {{ formatAmount(report.actual) }}/100 km (TP {{ formatAmount(report.norm) }})</template></span>
        </div>
        <div class="figure">
          <span class="label">{{ isPrivate ? 'Cestovné náhrady' : 'Výdavky na vozidlo' }}</span>
          <span class="value"><span class="grand">{{ formatMoney(isPrivate ? report.allowance : report.total) }}</span></span>
          <span class="sub">{{ formatMoney(report.perKm) }} na služobný km</span>
        </div>
      </section>

      <div class="toolbar">
        <SelectButton v-model="tab" :options="tabs" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobraziť" />
        <template v-if="tab !== 'report'">
          <SelectButton v-if="tab === 'trips'" v-model="kind" :options="kindFilter" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Druh jazdy" />
          <IconField class="search">
            <InputIcon class="pi pi-search" />
            <InputText v-model="search" :placeholder="tab === 'trips' ? 'Hľadať trasu, účel, vodiča, dátum' : 'Hľadať stanicu, doklad, dátum'" size="small" aria-label="Hľadať" />
          </IconField>
          <ExportButton v-if="tab === 'trips'" :name="`Kniha jázd ${vehicle.plate} ${year}`" :columns="tripColumns" :rows="() => gridRows(gridApi, tripRows)" />
          <ExportButton v-else :name="`Tankovanie ${vehicle.plate} ${year}`" :columns="fuelColumns" :rows="() => gridRows(gridApi, fuelRows)" />
        </template>
      </div>

      <div v-if="tab !== 'report'" class="grid-wrap">
        <AgGridVue
          :key="tab"
          class="grid"
          :theme="fiskusGridTheme"
          :row-data="tab === 'trips' ? tripRows : fuelRows"
          :column-defs="tab === 'trips' ? tripCols : fuelCols"
          :default-col-def="{ sortable: true, resizable: true }"
          :locale-text="skGridLocale"
          :loading="loading"
          :get-row-id="(p) => String(p.data.id)"
          :row-class-rules="{ 'row-private': (p) => p.data.kind === 'private' }"
          :tooltip-show-delay="300"
          :overlay-no-rows-template="tab === 'trips' ? '<span>Žiadne jazdy v tomto roku. Novú zapíšete klávesom N.</span>' : '<span>Žiadne tankovanie v tomto roku. Zapíšete ho klávesom T.</span>'"
          @grid-ready="(e) => { gridApi = e.api; applyResponsive() }"
          @grid-size-changed="(e) => e.api.sizeColumnsToFit()"
          @row-clicked="(e) => (tab === 'trips' ? openTrip(e.data) : openFuel(e.data))"
        />
      </div>

      <section v-else-if="report" class="report">
        <p class="method"><i class="pi pi-info-circle" /> {{ methodHint(vehicle) }}</p>
        <ul v-if="report.warnings.length" class="warnings">
          <li v-for="w in report.warnings" :key="w"><i class="pi pi-exclamation-triangle" /> {{ w }}</li>
        </ul>
        <div class="sheets">
          <table class="sheet">
            <caption>Výdavky {{ year }}</caption>
            <tbody>
              <tr><td>PHM spolu podľa dokladov</td><td class="num">{{ formatMoney(report.fuelCost) }}</td></tr>
              <template v-if="!isPrivate">
                <tr v-if="Number(report.fuelPrivate)"><td class="indent">súkromné jazdy (nie je výdavok)</td><td class="num neg">−{{ formatMoney(report.fuelPrivate) }}</td></tr>
                <tr v-if="Number(report.fuelOveruse)"><td class="indent">nadspotreba nad technický preukaz</td><td class="num neg">−{{ formatMoney(report.fuelOveruse) }}</td></tr>
                <tr class="sum"><td>PHM – daňový výdavok</td><td class="num">{{ formatMoney(report.fuelExpense) }}</td></tr>
                <tr><td>Odpis vozidla (register majetku)</td><td class="num">{{ formatMoney(report.depreciation) }}</td></tr>
              </template>
              <template v-else>
                <tr><td>Náhrada za PHM <span class="muted">{{ vehicle.method === 'flat' ? '50 % z dokladov' : `${fmtKm(report.kmBusiness)} × ${formatAmount(report.norm)}/100 × ${formatAmount(report.avgPrice)} €` }}</span></td><td class="num">{{ formatMoney(report.fuelExpense) }}</td></tr>
                <tr><td>Základná náhrada za km <span class="muted">opotrebenie vozidla</span></td><td class="num">{{ formatMoney(report.baseAllowance) }}</td></tr>
              </template>
              <tr class="grand-row"><td>Spolu</td><td class="num"><span class="grand">{{ formatMoney(report.total) }}</span></td></tr>
              <tr><td>Na služobný km</td><td class="num">{{ formatMoney(report.perKm) }}</td></tr>
            </tbody>
          </table>
          <table class="sheet months">
            <caption>Po mesiacoch</caption>
            <thead><tr><th>Mesiac</th><th class="num">Služobne</th><th class="num">Súkromne</th><th class="num">PHM</th></tr></thead>
            <tbody>
              <tr v-if="!activeMonths.length"><td colspan="4" class="muted">Žiadne jazdy ani tankovanie.</td></tr>
              <tr v-for="m in activeMonths" :key="m.month">
                <td>{{ monthNames[m.month - 1] }}</td><td class="num">{{ fmtKm(m.kmBusiness) }}</td><td class="num">{{ fmtKm(m.kmPrivate) }}</td><td class="num">{{ formatMoney(m.fuelCost) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="post">
          <div>
            <span class="tag" :data-tone="posting.tone"><span class="tag-text">{{ posting.label }}</span></span>
            <p class="muted small">
              <template v-if="isPrivate">Cestovné náhrady sa zapíšu do uzávierky ako výdavok {{ formatMoney(report.posting) }} (súkromné auto nemá doklady vo výdavkoch).</template>
              <template v-else-if="Number(report.posting)">Tankovanie je vo výdavkoch z dokladov — do uzávierky sa zapíše vylúčenie {{ formatMoney(String(report.posting).replace('-', '')) }} z výdavkov (súkromné jazdy a nadspotreba). DPH z PHM pri súkromnom používaní odpočítajte iba v pomere služobných jázd.</template>
              <template v-else>Tankovanie je vo výdavkoch z dokladov a nič netreba vylúčiť. Odpis je v uzávierke z registra majetku.</template>
            </p>
          </div>
          <Button v-if="canWrite && posting.action" :label="posting.action" icon="pi pi-book" size="small" :loading="postBusy"
            :outlined="posting.tone === 'paid'" :severity="posting.tone === 'paid' ? 'secondary' : undefined" @click="post" />
        </div>
      </section>
    </template>

    <!-- jazda -->
    <Drawer v-model:visible="tripOpen" position="right" class="lb-panel" :header="tripForm?.id ? 'Jazda' : 'Nová jazda'">
      <form v-if="tripForm" class="form-grid lb-form" @submit.prevent="saveTrip(false)">
        <FormField label="Druh jazdy" :span="2">
          <SelectButton v-model="tripForm.kind" :options="tripKinds" option-label="label" option-value="value" :allow-empty="false" size="small" :disabled="!canWrite" />
        </FormField>
        <FormField label="Dátum" :error="errors.date">
          <DatePicker v-model="tripForm.date" date-format="d. m. yy" size="small" fluid :disabled="!canWrite" />
        </FormField>
        <FormField label="Vodič" :error="errors.driver">
          <InputText v-model="tripForm.driver" size="small" :disabled="!canWrite" />
        </FormField>
        <FormField label="Odchod" :error="errors.depart">
          <InputText v-model="tripForm.depart" size="small" placeholder="07:30" inputmode="numeric" :disabled="!canWrite" />
        </FormField>
        <FormField label="Príchod" :error="errors.arrive">
          <InputText v-model="tripForm.arrive" size="small" placeholder="16:00" inputmode="numeric" :disabled="!canWrite" />
        </FormField>
        <FormField label="Odkiaľ" :error="errors.from">
          <AutoComplete v-model="tripForm.from" :suggestions="placeHits" size="small" fluid :disabled="!canWrite" @complete="(e) => (placeHits = suggest(places, e.query))" />
        </FormField>
        <FormField label="Kam" :error="errors.to">
          <AutoComplete v-model="tripForm.to" :suggestions="placeHits" size="small" fluid :disabled="!canWrite" @complete="(e) => (placeHits = suggest(places, e.query))" />
        </FormField>
        <FormField label="Účel jazdy" :span="2" :error="errors.purpose" :hint="tripForm.kind === 'private' ? 'nepovinné' : ''">
          <AutoComplete v-model="tripForm.purpose" :suggestions="purposeHits" size="small" fluid :disabled="!canWrite"
            placeholder="napr. Montáž rozvádzača – Hotel Lesná" @complete="(e) => (purposeHits = suggest(purposes, e.query))" />
        </FormField>
        <FormField label="Tachometer – začiatok" :error="errors.odoStart">
          <InputNumber v-model="tripForm.odoStart" :use-grouping="false" size="small" suffix=" km" :disabled="!canWrite" />
        </FormField>
        <FormField label="Tachometer – koniec" :error="errors.odoEnd">
          <InputNumber v-model="tripForm.odoEnd" :use-grouping="false" size="small" suffix=" km" :disabled="!canWrite" />
        </FormField>
        <FormField label="Najazdené" :error="errors.km" :hint="tripKm != null ? 'z tachometra' : 'bez tachometra zadajte km'">
          <InputNumber v-if="tripKm == null" v-model="tripForm.km" :min="0" size="small" suffix=" km" :disabled="!canWrite" />
          <strong v-else class="km">{{ fmtKm(tripKm) }}</strong>
        </FormField>
        <FormField v-if="tripForm.id && costs.get(tripForm.id)" label="Náklady jazdy">
          <span class="cost">{{ formatMoney(costs.get(tripForm.id).total) }}<small class="muted"> PHM {{ formatMoney(costs.get(tripForm.id).fuel) }}, opotrebenie {{ formatMoney(costs.get(tripForm.id).wear) }}</small></span>
        </FormField>
        <FormField label="Poznámka" :span="2">
          <InputText v-model="tripForm.note" size="small" :disabled="!canWrite" />
        </FormField>
        <div v-if="canWrite" class="actions">
          <Button v-if="tripForm.id" label="Zmazať" severity="danger" text size="small" @click="removeTrip(tripForm)" />
          <Button v-if="tripForm.id" label="Kopírovať" icon="pi pi-copy" severity="secondary" text size="small" @click="copyTrip(tripForm)" />
          <div class="spacer" />
          <Button label="Uložiť a spiatočná" severity="secondary" outlined size="small" :loading="busy" @click="saveTrip(true)" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Drawer>

    <!-- tankovanie -->
    <Drawer v-model:visible="fuelOpen" position="right" class="lb-panel" :header="fuelForm?.id ? 'Tankovanie' : 'Nové tankovanie'">
      <form v-if="fuelForm" class="form-grid lb-form" @submit.prevent="saveFuel">
        <FormField label="Doklad (pokladničný blok alebo faktúra)" :span="2" :error="errors.docId" hint="suma sa prevezme z dokladu — nákup ostáva vo výdavkoch">
          <Select :model-value="fuelForm.doc" :options="docOptions" option-label="label" option-value="value" filter show-clear size="small" fluid
            placeholder="bez dokladu vo Fiškuse" empty-message="Žiadne výdavkové doklady" empty-filter-message="Nič sa nenašlo" :disabled="!canWrite"
            @update:model-value="pickDoc" />
        </FormField>
        <FormField label="Dátum" :error="errors.date">
          <DatePicker v-model="fuelForm.date" date-format="d. m. yy" size="small" fluid :disabled="!canWrite" />
        </FormField>
        <FormField label="Čerpacia stanica" :error="errors.station">
          <InputText v-model="fuelForm.station" size="small" :disabled="!canWrite" />
        </FormField>
        <FormField :label="`Množstvo (${unit})`" :error="errors.quantity">
          <InputNumber v-model="fuelForm.quantity" :min-fraction-digits="2" :max-fraction-digits="2" locale="sk-SK" size="small" :suffix="` ${unit}`" :disabled="!canWrite" />
        </FormField>
        <FormField label="Cena s DPH" :error="errors.amount">
          <InputNumber v-model="fuelForm.amount" :min-fraction-digits="2" :max-fraction-digits="2" locale="sk-SK" size="small" suffix=" €" :disabled="!canWrite" />
        </FormField>
        <FormField label="Stav tachometra" :error="errors.odometer">
          <InputNumber v-model="fuelForm.odometer" :use-grouping="false" size="small" suffix=" km" :disabled="!canWrite" />
        </FormField>
        <label class="check-row"><Checkbox v-model="fuelForm.full" binary :disabled="!canWrite" /><span>Plná nádrž</span></label>
        <FormField label="Poznámka" :span="2">
          <InputText v-model="fuelForm.note" size="small" :disabled="!canWrite" />
        </FormField>
        <div v-if="canWrite" class="actions">
          <Button v-if="fuelForm.id" label="Zmazať" severity="danger" text size="small" @click="removeFuel(fuelForm)" />
          <div class="spacer" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Drawer>

    <!-- vozidlo -->
    <Drawer v-model:visible="vehOpen" position="right" class="lb-panel" :header="vehForm?.id ? `${vehForm.plate} ${vehForm.name}` : 'Nové vozidlo'">
      <form v-if="vehForm" class="form-grid lb-form" @submit.prevent="saveVehicle">
        <FormField label="Názov" :error="errors.name">
          <InputText v-model="vehForm.name" size="small" placeholder="Škoda Octavia" />
        </FormField>
        <FormField label="Evidenčné číslo" :error="errors.plate">
          <InputText v-model="vehForm.plate" size="small" placeholder="BB-123AB" />
        </FormField>
        <FormField label="Vzťah k podnikaniu" :span="2" :error="errors.ownership">
          <Select v-model="vehForm.ownership" :options="ownerships" option-label="label" option-value="value" size="small" fluid />
        </FormField>
        <FormField label="Výdavky na PHM" :span="2" :error="errors.method">
          <SelectButton v-model="vehForm.method" :options="methodOptions(vehForm.ownership)" option-label="label" option-value="value" :allow-empty="false" size="small" />
        </FormField>
        <p class="method span2">{{ methodHint(vehForm) }}</p>
        <FormField label="Pohonná látka" :error="errors.fuel">
          <Select v-model="vehForm.fuel" :options="fuelKinds" option-label="label" option-value="value" size="small" fluid />
        </FormField>
        <FormField :label="`Spotreba z TP (${fuelUnit(vehForm.fuel)}/100 km)`" :error="errors.consumption" hint="kombinovaná">
          <InputNumber v-model="vehForm.consumption" :min-fraction-digits="1" :max-fraction-digits="2" locale="sk-SK" size="small" />
        </FormField>
        <FormField v-if="vehForm.ownership === 'business'" label="Položka v registri majetku" :span="2" :error="errors.assetId" hint="odpis sa započíta do nákladov na km">
          <Select v-model="vehForm.assetId" :options="assets" option-label="label" option-value="value" show-clear size="small" fluid placeholder="vyberte auto z majetku" />
        </FormField>
        <FormField label="Predvolený vodič">
          <InputText v-model="vehForm.driver" size="small" />
        </FormField>
        <label class="check-row"><Checkbox v-model="vehForm.moto" binary /><span>Jednostopové vozidlo</span></label>
        <FormField label="Stav tachometra na začiatku" :error="errors.odometer">
          <InputNumber v-model="vehForm.odometer" :use-grouping="false" size="small" suffix=" km" />
        </FormField>
        <FormField label="ku dňu">
          <DatePicker v-model="vehForm.odometerDate" date-format="d. m. yy" size="small" fluid />
        </FormField>
        <FormField label="Poznámka" :span="2">
          <InputText v-model="vehForm.note" size="small" />
        </FormField>
        <label v-if="vehForm.id" class="check-row span2"><Checkbox v-model="vehForm.archived" binary /><span>Archivované — vozidlo už nepoužívate</span></label>
        <div class="actions">
          <Button v-if="vehForm.id && !vehForm.trips && !vehForm.fuels" label="Zmazať" severity="danger" text size="small" @click="removeVehicle" />
          <div class="spacer" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Drawer>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 14px 16px 12px;
  gap: 10px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.head h1 {
  font-size: var(--fs-xl);
}
.head :deep(.p-button) {
  gap: 6px;
}
.veh-select {
  min-width: 14rem;
}
.spacer {
  flex: 1;
}
.hint {
  margin-left: 6px;
  background: rgba(255, 255, 255, 0.16);
  border-color: rgba(255, 255, 255, 0.3);
  color: #fff;
}
.hint.dark {
  background: transparent;
  border-color: var(--rule);
  color: var(--ink-soft);
}
.error {
  color: var(--tone-overdue);
}
.empty {
  max-width: 60ch;
  padding: 24px;
  background: var(--sheet);
  border: 1px solid var(--rule);
}
.empty h2 {
  font-size: var(--fs-lg);
  margin: 0 0 6px;
}
.empty p {
  color: var(--ink-soft);
  margin: 0 0 14px;
}

/* súhrn roka ako riadok účtovnej knihy */
.ledger {
  display: flex;
  border-top: 2px solid var(--ink);
  border-bottom: 1px solid var(--rule);
  background: var(--sheet);
}
.figure {
  display: flex;
  flex-direction: column;
  padding: 8px 18px 9px 14px;
  border-right: 1px solid var(--rule-soft);
  min-width: 0;
}
.figure .label {
  font-size: var(--fs-xs);
  color: var(--ink-soft);
}
.figure .value {
  font-size: var(--fs-xl);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.figure .sub {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
  white-space: nowrap;
  margin-top: 4px;
}
.grand {
  border-bottom: 3px double currentColor;
  color: var(--blue-deep);
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.search {
  flex: 1;
  max-width: 340px;
}
.search :deep(input) {
  width: 100%;
}
.grid-wrap {
  flex: 1;
  min-height: 300px;
  border: 1px solid var(--rule);
  background: var(--sheet);
}
.grid {
  height: 100%;
}
.grid :deep(.ag-row) {
  cursor: pointer;
}
.grid :deep(.cell-name) {
  font-weight: 600;
}
.grid :deep(.cell-code) {
  color: var(--ink-soft);
}
.grid :deep(.row-private) {
  color: var(--ink-faint);
}

.report {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 980px;
}
.method {
  margin: 0;
  padding: 8px 10px;
  background: var(--blue-wash);
  border-left: 3px solid var(--blue);
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  max-width: 90ch;
}
.warnings {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.warnings li {
  padding: 6px 10px;
  background: var(--tone-overdue-bg);
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.sheets {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: 14px;
  align-items: start;
}
.sheet {
  width: 100%;
  border-collapse: collapse;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 2px solid var(--ink);
  font-size: var(--fs-md);
}
.sheet caption {
  text-align: left;
  font-weight: 600;
  padding: 0 0 6px;
}
.sheet th {
  text-align: left;
  font-weight: 600;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
}
.sheet td {
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule-soft);
}
.sheet .num {
  text-align: right;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.sheet .indent {
  padding-left: 24px;
  color: var(--ink-soft);
}
.sheet .neg {
  color: var(--ink-soft);
}
.sheet .sum td {
  font-weight: 600;
  border-top: 1px solid var(--rule);
}
.sheet .grand-row td {
  font-weight: 700;
  font-size: var(--fs-lg);
}
.sheet .muted {
  margin-left: 6px;
  font-size: var(--fs-xs);
}
.post {
  display: flex;
  align-items: center;
  gap: 16px;
  justify-content: space-between;
  padding: 10px 12px;
  background: var(--sheet);
  border: 1px solid var(--rule);
}
.post p {
  margin: 6px 0 0;
  max-width: 80ch;
}
.muted {
  color: var(--ink-faint);
}
.small {
  font-size: var(--fs-sm);
}
.lb-form {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.lb-form .span2 {
  grid-column: span 2;
}
.check-row {
  display: flex;
  align-items: center;
  gap: 8px;
  align-self: end;
  padding-bottom: 6px;
}
.km {
  font-size: var(--fs-lg);
  font-variant-numeric: tabular-nums;
  padding-top: 4px;
}
.cost {
  display: flex;
  flex-direction: column;
  font-weight: 600;
  padding-top: 4px;
}
.actions {
  grid-column: span 2;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 6px;
}

@media (max-width: 900px) {
  .sheets {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 760px) {
  .page {
    padding: 10px 10px 8px;
  }
  .head {
    flex-wrap: wrap;
  }
  .head h1 {
    font-size: var(--fs-lg);
  }
  .veh-select {
    order: 10;
    flex: 1 1 100%;
    min-width: 0;
  }
  .hint {
    display: none;
  }
  .ledger {
    overflow-x: auto;
  }
  .figure {
    flex: none;
    padding: 6px 12px 7px 10px;
  }
  .figure .value {
    font-size: var(--fs-lg);
  }
  .search {
    max-width: none;
  }
  .post {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>

<style>
.p-drawer.p-component.lb-panel {
  width: min(520px, 100vw);
}
</style>
