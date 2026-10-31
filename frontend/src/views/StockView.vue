<script setup>
// Sklad: zásoby na kartách (vážený aritmetický priemer), skladové doklady
// a nastavenie skladov a skladových položiek. Ocenenie počíta server.
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import Drawer from 'primevue/drawer'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import FormField from '../components/FormField.vue'
import ExportButton from '../components/ExportButton.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, fromISODate, toISODate } from '../lib/format.js'
import { stockCardColumns, stockDocColumns } from '../lib/columns.js'
import { localOnly } from '../lib/calendar.js'
import { apiLines, blankLine, docLabel, docRows, filterCards, filterDocs, itemKindLabels, needsCost } from '../lib/stock.js'

const toast = useToast()
const confirm = useConfirm()
const canWrite = computed(() => can('purchases.write'))

const v = ref(null)
const tab = ref('cards')
const tabs = [
  { value: 'cards', label: 'Zásoby' },
  { value: 'docs', label: 'Doklady' },
  { value: 'setup', label: 'Nastavenie' },
]
const search = ref('')
const warehouse = ref('all')
const low = ref(false)
const kind = ref('all')

async function load() {
  try {
    v.value = await api.stock(session.companyId)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Sklad sa nenačítal', detail: e.message, life: 6000 })
  }
}
onMounted(load)
watch(() => session.companyId, load)
watch(tab, () => (search.value = ''))

const warehouseOptions = computed(() => [{ value: 'all', label: 'Všetky sklady' }, ...(v.value?.warehouses ?? []).map((w) => ({ value: w.code, label: `${w.code} – ${w.name}` }))])
const kindOptions = [{ value: 'all', label: 'Všetky doklady' }, { value: 'receipt', label: 'Príjemky' }, { value: 'issue', label: 'Výdajky' }, { value: 'count', label: 'Inventúrne rozdiely' }, { value: 'transfer', label: 'Presuny' }]
const wh = computed(() => (warehouse.value === 'all' ? '' : warehouse.value))
const cards = computed(() => (v.value ? filterCards(v.value.cards, { search: search.value, warehouse: wh.value, low: low.value }) : []))
const docs = computed(() => (v.value ? filterDocs(v.value.docs, { search: search.value, kind: kind.value === 'all' ? '' : kind.value, warehouse: wh.value }) : []))
const exportDocs = () => docRows(docs.value)
const lowCount = computed(() => (v.value?.cards ?? []).filter((c) => c.low).length)
const qty = (c) => `${formatAmount(c.quantity).replace(/,00$/, '')} ${c.unit}`
const multi = computed(() => (v.value?.warehouses.length ?? 0) > 1)

// ---- panel ----
const open = ref(false)
const mode = ref('') // doc | card
const form = ref(null)
const lines = ref([])
const errors = ref({})
const busy = ref(false)
const current = ref(null)
const moves = ref([])
const itemOptions = computed(() => {
  const names = new Map((v.value?.catalog ?? []).map((i) => [i.id, i]))
  return (v.value?.items ?? []).map((s) => {
    const it = names.get(s.itemId) ?? {}
    return { value: s.itemId, label: `${it.code ? it.code + ' · ' : ''}${it.name ?? '?'} (${it.unit ?? ''})` }
  })
})
const docKindOptions = computed(() => [
  { value: 'receipt', label: 'Príjemka' },
  { value: 'issue', label: 'Výdajka' },
  { value: 'count', label: 'Inventúrny rozdiel' },
  ...(multi.value || form.value?.kind === 'transfer' ? [{ value: 'transfer', label: 'Presun medzi skladmi' }] : []),
])
const methodLabel = computed(() => (v.value?.methods ?? []).find((m) => m.code === v.value.method)?.label ?? '')
// prijaté faktúry na väzbu príjemky (načítajú sa pri prvom otvorení formulára)
const purchases = ref(null)
async function loadPurchases() {
  if (purchases.value) return
  try {
    const y = new Date().getFullYear()
    const [a, b] = await Promise.all([api.purchases(session.companyId, { year: y }), api.purchases(session.companyId, { year: y - 1 })])
    purchases.value = [...a, ...b]
  } catch {
    purchases.value = []
  }
}
const purchaseOptions = computed(() => [
  { value: 0, label: 'Bez väzby' },
  ...(purchases.value ?? []).map((p) => ({ value: p.id, label: `${p.number} · ${p.supplier?.name ?? ''} · ${formatAmount(p.summary?.total)} €` })),
])
const title = computed(() => {
  if (mode.value === 'card') return current.value?.name ?? 'Skladová karta'
  return form.value?.id ? `${docLabel(form.value)} ${form.value.number}` : 'Nový skladový doklad'
})
const readonly = computed(() => !canWrite.value || form.value?.auto)

function editDoc(d) {
  loadPurchases()
  errors.value = {}
  form.value = d
    ? { ...d, date: fromISODate(d.date) }
    : { id: 0, kind: 'receipt', date: new Date(), warehouse: v.value.warehouses[0].code, to: '', partner: '', note: '', return: false, linkId: 0 }
  lines.value = d
    ? d.lines.map((l) => ({ itemId: l.itemId, quantity: String(l.quantity), unitCost: Number(l.unitCost) ? String(l.unitCost) : '', name: l.name, unit: l.unit, total: l.total }))
    : [blankLine()]
  mode.value = 'doc'
  open.value = true
}

async function openCard(c) {
  current.value = c
  mode.value = 'card'
  moves.value = []
  open.value = true
  try {
    moves.value = await api.stockCard(session.companyId, c.warehouse, c.itemId)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Pohyby sa nenačítali', detail: e.message, life: 6000 })
  }
}

async function saveDoc() {
  busy.value = true
  errors.value = {}
  try {
    const f = form.value
    const saved = await api.saveStockDoc(session.companyId, {
      id: f.id || undefined, kind: f.kind, date: toISODate(f.date), warehouse: f.warehouse, partner: f.partner, note: f.note,
      return: f.kind === 'receipt' && f.return, lines: apiLines(lines.value), to: f.kind === 'transfer' ? f.to : '',
      ...(f.kind === 'receipt' && !f.return && f.linkId ? { linkKind: 'purchase', linkId: f.linkId } : {}),
    })
    toast.add({ severity: 'success', summary: f.id ? 'Doklad upravený' : 'Doklad vytvorený', detail: saved.number, life: 3000 })
    open.value = false
    await load()
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) errors.value = e.fieldMap()
    else toast.add({ severity: 'error', summary: 'Neuložilo sa', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}

function removeDoc() {
  confirm.require({
    header: 'Zmazať doklad',
    message: `Zmazať ${docLabel(form.value).toLowerCase()} ${form.value.number}? Ocenenie neskorších výdajov sa prepočíta.`,
    acceptLabel: 'Zmazať',
    rejectLabel: 'Späť',
    acceptProps: { severity: 'danger', size: 'small' },
    rejectProps: { severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.deleteStockDoc(session.companyId, form.value.id)
        open.value = false
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
      }
    },
  })
}

// ---- nastavenie ----
const setup = ref(null)
const setupErrors = ref({})
let keepSetup = false // po vytvorení odkazu ostanú neuložené zmeny nastavenia
watch([tab, v], () => {
  if (keepSetup) {
    keepSetup = false
    return
  }
  if (tab.value === 'setup' && v.value) {
    setup.value = {
      warehouses: v.value.warehouses.map((w) => ({ ...w })),
      items: v.value.items.map((i) => ({ ...i, min: Number(i.min) ? String(i.min) : '' })),
      method: v.value.method,
      orders: v.value.orders,
    }
  }
})
const catalogOptions = computed(() => {
  const used = new Set((setup.value?.items ?? []).map((i) => i.itemId))
  return (v.value?.catalog ?? []).filter((i) => !i.archived && !used.has(i.id)).map((i) => ({ value: i.id, label: `${i.code ? i.code + ' · ' : ''}${i.name}` }))
})
const catalogName = (id) => (v.value?.catalog ?? []).find((i) => i.id === id)?.name ?? '?'
const itemKindOptions = Object.entries(itemKindLabels).map(([value, label]) => ({ value, label }))
const addItem = ref(null)
function pickItem(id) {
  if (!id) return
  setup.value.items.push({ itemId: id, kind: 'goods', min: '' })
  addItem.value = null
}
async function saveSetup() {
  busy.value = true
  setupErrors.value = {}
  try {
    await api.saveStockConfig(session.companyId, {
      warehouses: setup.value.warehouses,
      items: setup.value.items.map((i) => ({ itemId: i.itemId, kind: i.kind, min: String(i.min || '0').replace(/\s/g, '').replace(',', '.') })),
      method: setup.value.method,
      orders: setup.value.orders,
    })
    toast.add({ severity: 'success', summary: 'Nastavenie skladu uložené', life: 2500 })
    await load()
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) setupErrors.value = e.fieldMap()
    else toast.add({ severity: 'error', summary: 'Neuložilo sa', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
const setupError = computed(() => Object.values(setupErrors.value)[0])

// odkaz pre e-shop
const feedToken = ref('')
const local = localOnly(window.location.origin)
const feedBusy = ref(false)
const feedURLs = computed(() => {
  const base = `${window.location.origin}/api/public/stock/${session.companyId}/${feedToken.value}`
  return [
    { label: 'Zásoby JSON', url: `${base}/zasoby.json` },
    { label: 'Zásoby CSV', url: `${base}/zasoby.csv` },
    { label: 'Objednávky (POST)', url: `${base}/objednavky` },
  ]
})
async function createFeed() {
  feedBusy.value = true
  try {
    feedToken.value = (await api.createStockFeed(session.companyId)).token
    keepSetup = true
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Odkaz sa nevytvoril', detail: e.message, life: 6000 })
  } finally {
    feedBusy.value = false
  }
}
async function revokeFeed() {
  try {
    await api.revokeStockFeed(session.companyId)
    feedToken.value = ''
    keepSetup = true
    await load()
    toast.add({ severity: 'info', summary: 'Odkaz zrušený', life: 3000 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  }
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Sklad</h1>
      <SelectButton v-model="tab" :options="tabs" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Zobrazenie" />
      <div class="spacer" />
      <Button v-if="canWrite && v?.items.length" label="Skladový doklad" icon="pi pi-plus" size="small" @click="editDoc(null)" />
    </header>

    <template v-if="v">
      <section class="summary" aria-label="Súhrn skladu">
        <div class="cell"><span class="label">Hodnota zásob</span><span class="value"><span class="grand">{{ formatMoney(v.value) }}</span></span></div>
        <div class="cell"><span class="label">Skladové položky</span><span class="value">{{ v.items.length }}</span></div>
        <div class="cell">
          <span class="label">Pod minimom</span>
          <span class="value" :class="{ bad: lowCount }">{{ lowCount }}</span>
        </div>
        <div class="cell"><span class="label">Ocenenie</span><span class="value small">{{ methodLabel }}</span></div>
      </section>

      <p v-if="v.problems.length" class="warn">
        <i class="pi pi-exclamation-triangle" />
        {{ v.problems.map((p) => `${p.number}: ${p.message}`).join(' · ') }}
      </p>

      <p v-if="!v.items.length" class="empty-state">
        Zatiaľ žiadne skladové položky. V záložke <b>Nastavenie</b> vyberte z cenníka tovar alebo materiál, ktorý vediete na sklade.
        Pri vystavení faktúry s takou položkou (vloženou z cenníka) sa výdajka vytvorí sama.
      </p>

      <template v-if="tab !== 'setup'">
        <div class="toolbar">
          <IconField class="search">
            <InputIcon class="pi pi-search" />
            <InputText v-model="search" :placeholder="tab === 'cards' ? 'Hľadať položku' : 'Hľadať doklad, partnera, položku'" size="small" aria-label="Hľadať" />
          </IconField>
          <Select v-if="multi" v-model="warehouse" :options="warehouseOptions" option-label="label" option-value="value" size="small" class="sel" aria-label="Sklad" />
          <label v-if="tab === 'cards'" class="check"><Checkbox v-model="low" binary /> <span>iba pod minimom</span></label>
          <Select v-else v-model="kind" :options="kindOptions" option-label="label" option-value="value" size="small" class="sel" aria-label="Druh dokladu" />
          <div class="spacer" />
          <ExportButton v-if="tab === 'cards'" name="Skladové zásoby" :columns="stockCardColumns" :rows="cards" />
          <ExportButton v-else name="Skladové doklady" :columns="stockDocColumns" :rows="exportDocs" />
        </div>

        <section v-if="tab === 'cards'" class="sheet scroll">
          <table class="list">
            <thead>
              <tr><th>Položka</th><th v-if="multi">Sklad</th><th class="num">Množstvo</th><th class="num hide-m">Priemerná cena</th><th class="num">Hodnota</th></tr>
            </thead>
            <tbody>
              <tr v-if="!cards.length"><td :colspan="multi ? 5 : 4" class="empty">{{ v.cards.length ? 'Nič sa nenašlo.' : 'Žiadne zásoby.' }}</td></tr>
              <tr v-for="c in cards" :key="c.warehouse + c.itemId" tabindex="0" @click="openCard(c)" @keydown.enter="openCard(c)">
                <td>
                  <span v-if="c.code" class="code">{{ c.code }}</span> {{ c.name }}
                  <span class="kind">{{ itemKindLabels[c.kind] }}</span>
                  <span v-if="c.low" class="tag low">pod minimom {{ formatAmount(c.min).replace(/,00$/, '') }}</span>
                </td>
                <td v-if="multi">{{ c.warehouse }}</td>
                <td class="num" :class="{ neg: Number(c.quantity) < 0 }">{{ qty(c) }}</td>
                <td class="num hide-m">{{ formatAmount(c.avgCost) }}</td>
                <td class="num">{{ formatAmount(c.value) }}</td>
              </tr>
            </tbody>
          </table>
        </section>

        <section v-else class="sheet scroll">
          <table class="list">
            <thead><tr><th>Dátum</th><th>Doklad</th><th>Partner / položky</th><th class="num">Hodnota</th></tr></thead>
            <tbody>
              <tr v-if="!docs.length"><td colspan="4" class="empty">{{ v.docs.length ? 'Nič sa nenašlo.' : 'Zatiaľ žiadne skladové doklady.' }}</td></tr>
              <tr v-for="d in docs" :key="d.id" tabindex="0" @click="editDoc(d)" @keydown.enter="editDoc(d)">
                <td class="date">{{ formatDate(d.date) }}</td>
                <td>
                  <b>{{ d.number }}</b> <span class="kind">{{ docLabel(d) }}<template v-if="multi"> · {{ d.warehouse }}<template v-if="d.to"> → {{ d.to }}</template></template></span>
                  <span v-if="d.auto" class="tag">z faktúry</span>
                </td>
                <td class="text">
                  {{ [d.partner, d.linkNumber].filter(Boolean).join(' · ') }}
                  <span class="lines-sum">{{ d.lines.map((l) => `${l.name} ${formatAmount(l.quantity).replace(/,00$/, '')} ${l.unit}`).join(', ') }}</span>
                </td>
                <td class="num" :class="{ neg: d.kind === 'issue' }">{{ formatAmount(d.total) }}</td>
              </tr>
            </tbody>
          </table>
        </section>
      </template>

      <section v-else-if="setup" class="setup">
        <div class="sheet pad">
          <h2>Sklady</h2>
          <div v-for="(w, i) in setup.warehouses" :key="i" class="row">
            <InputText v-model="w.code" size="small" class="w-code" placeholder="Kód" :disabled="!canWrite" :aria-label="`Kód skladu ${i + 1}`" />
            <InputText v-model="w.name" size="small" class="w-name" placeholder="Názov" :disabled="!canWrite" :aria-label="`Názov skladu ${i + 1}`" />
            <Button v-if="canWrite" icon="pi pi-times" text rounded size="small" severity="secondary" :disabled="setup.warehouses.length < 2"
              :aria-label="`Odstrániť sklad ${i + 1}`" @click="setup.warehouses.splice(i, 1)" />
          </div>
          <Button v-if="canWrite" label="Sklad" icon="pi pi-plus" text size="small" @click="setup.warehouses.push({ code: '', name: '' })" />
          <p class="hint">Výdajky z faktúr idú z prvého skladu.</p>
        </div>
        <div class="sheet pad">
          <h2>Ocenenie zásob</h2>
          <Select v-model="setup.method" :options="v.methods" option-label="label" option-value="code" size="small" class="method" :disabled="!canWrite" aria-label="Spôsob ocenenia" />
          <p v-if="setup.method !== v.method" class="warn">
            Zmena spôsobu prepočíta ocenenie všetkých výdajov od začiatku. Spôsob sa podľa zákona o účtovníctve mení iba výnimočne
            a od začiatku účtovného obdobia — poraďte sa s účtovníkom.
          </p>
          <p v-else class="hint">Vážený priemer sa prepočíta pri každom príjme; pri FIFO sa vydáva najstaršia zásoba za jej cenu.</p>
        </div>
        <div class="sheet pad">
          <h2>Skladové položky</h2>
          <p class="hint">
            Položky cenníka, ktoré vediete na sklade. Druh určuje účty v podvojnom účtovníctve (tovar 132, materiál 112, výrobky 123).
            Nové položky pridáte v <RouterLink to="/cennik">cenníku</RouterLink>.
          </p>
          <div class="items-head"><span>Položka</span><span>Druh</span><span>Minimum</span><span /></div>
          <div v-for="(it, i) in setup.items" :key="it.itemId" class="items-row">
            <span class="name">{{ catalogName(it.itemId) }}</span>
            <Select v-model="it.kind" :options="itemKindOptions" option-label="label" option-value="value" size="small" fluid :disabled="!canWrite" :aria-label="`Druh ${catalogName(it.itemId)}`" />
            <InputText v-model="it.min" size="small" inputmode="decimal" class="num-in" :disabled="!canWrite" :aria-label="`Minimum ${catalogName(it.itemId)}`" />
            <Button v-if="canWrite" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="`Vyradiť ${catalogName(it.itemId)}`" @click="setup.items.splice(i, 1)" />
          </div>
          <Select v-if="canWrite" v-model="addItem" :options="catalogOptions" option-label="label" option-value="value" filter size="small" class="add-item"
            placeholder="Pridať položku z cenníka" :empty-message="'Všetky položky cenníka sú na sklade'" @update:model-value="pickItem" />
        </div>
        <div class="sheet pad">
          <h2>E-shop</h2>
          <p class="hint">
            E-shop si stiahne zásoby a ceny z tajného odkazu (JSON alebo CSV, kód položky z cenníka) a môže posielať objednávky,
            z ktorých vzniknú výdajky. Konektory pre konkrétne e-shopy (Shoptet, WooCommerce…) pribudnú.
          </p>
          <p v-if="local" class="warn">
            Fiškus beží iba na tomto počítači — e-shop na internete sa k odkazu nedostane. Použite serverovú verziu Fiškusa
            alebo konektor e-shopu (pribudne), ktorý zásoby posiela sám.
          </p>
          <template v-if="feedToken">
            <p class="once"><i class="pi pi-key" /> Odkaz si skopírujte teraz — znova sa neukáže.</p>
            <label v-for="u in feedURLs" :key="u.label" class="url">
              <span>{{ u.label }}</span>
              <InputText :model-value="u.url" readonly size="small" fluid @focus="$event.target.select()" />
            </label>
            <p class="hint">
              Objednávka: <code>POST {{ feedURLs[2].url }}</code> s hlavičkou <code>X-Fiskus: 1</code> a telom
              <code>{"order":"2026-123","customer":"…","lines":[{"code":"K5","quantity":"2"}]}</code>; zrušenie <code>"cancel":true</code>.
            </p>
          </template>
          <p v-else-if="v.feedActive" class="hint"><i class="pi pi-check-circle ok" /> Odkaz je aktívny od {{ new Date(v.feedCreated * 1000).toLocaleDateString('sk-SK') }}.</p>
          <label class="check"><Checkbox v-model="setup.orders" binary :disabled="!canWrite" /> <span>Povoliť e-shopu posielať objednávky (výdajky)</span></label>
          <div v-if="canWrite" class="actions">
            <Button :label="v.feedActive ? 'Nový odkaz' : 'Vytvoriť odkaz pre e-shop'" icon="pi pi-link" size="small" severity="secondary" outlined :loading="feedBusy" @click="createFeed" />
            <Button v-if="v.feedActive" label="Zrušiť odkaz" severity="danger" text size="small" @click="revokeFeed" />
          </div>
        </div>
        <p v-if="setupError" class="warn">{{ setupError }}</p>
        <div v-if="canWrite" class="actions">
          <span class="spacer" />
          <Button label="Uložiť nastavenie" icon="pi pi-check" size="small" :loading="busy" @click="saveSetup" />
        </div>
      </section>
    </template>

    <Drawer v-model:visible="open" position="right" class="stock-panel" :header="title">
      <!-- skladová karta -->
      <div v-if="mode === 'card' && current" class="dform">
        <dl class="facts">
          <dt>Sklad</dt><dd>{{ current.warehouse }}</dd>
          <dt>Stav</dt><dd class="numl">{{ qty(current) }}</dd>
          <dt>Priemerná cena</dt><dd class="numl">{{ formatMoney(current.avgCost) }}</dd>
          <dt>Hodnota</dt><dd class="numl"><span class="grand">{{ formatMoney(current.value) }}</span></dd>
        </dl>
        <p v-if="!moves.length" class="hint">Na karte zatiaľ nie sú pohyby.</p>
        <div v-else class="moves">
          <div class="move head"><span>Dátum / doklad</span><span class="num">Pohyb</span><span class="num">Stav</span></div>
          <div v-for="(mv, i) in moves" :key="i" class="move">
            <span>{{ formatDate(mv.date) }} <b>{{ mv.number }}</b><small>{{ mv.partner }}</small></span>
            <span class="num" :class="{ neg: Number(mv.quantity) < 0 }">{{ Number(mv.quantity) > 0 ? '+' : '' }}{{ formatAmount(mv.quantity).replace(/,00$/, '') }}<small>{{ formatAmount(mv.total) }} €</small></span>
            <span class="num">{{ formatAmount(mv.balance).replace(/,00$/, '') }}<small>{{ formatAmount(mv.value) }} €</small></span>
          </div>
        </div>
      </div>

      <!-- skladový doklad -->
      <form v-else-if="mode === 'doc' && form" class="dform" @submit.prevent="saveDoc">
        <p v-if="form.auto" class="hint">
          Doklad vytvoril Fiškus z faktúry <RouterLink :to="`/faktury/${form.linkId}`">{{ form.linkNumber }}</RouterLink> — mení sa a ruší spolu s ňou.
        </p>
        <div class="grid2">
          <FormField label="Druh" :error="errors.kind">
            <Select v-model="form.kind" :options="docKindOptions" option-label="label" option-value="value" size="small" fluid :disabled="readonly || !!form.id" />
          </FormField>
          <FormField label="Dátum" :error="errors.date">
            <DatePicker v-model="form.date" date-format="d. m. yy" size="small" fluid :disabled="readonly" :invalid="!!errors.date" />
          </FormField>
          <FormField v-if="multi" label="Sklad" :error="errors.warehouse">
            <Select v-model="form.warehouse" :options="warehouseOptions.slice(1)" option-label="label" option-value="value" size="small" fluid :disabled="readonly" />
          </FormField>
          <FormField v-if="form.kind === 'transfer'" label="Do skladu" :error="errors.to">
            <Select v-model="form.to" :options="warehouseOptions.slice(1).filter((w) => w.value !== form.warehouse)" option-label="label" option-value="value"
              size="small" fluid :disabled="readonly" :invalid="!!errors.to" />
          </FormField>
          <FormField :label="{ receipt: 'Dodávateľ', issue: 'Odberateľ / komu', count: 'Inventúru vykonal', transfer: 'Presunul' }[form.kind]" :span="multi ? 1 : 2">
            <InputText v-model="form.partner" size="small" :disabled="readonly" />
          </FormField>
        </div>
        <FormField v-if="form.kind === 'receipt' && !form.return" label="K prijatej faktúre" :error="errors.linkId" hint="Nepovinné — dodávateľa doplní z faktúry">
          <Select v-model="form.linkId" :options="purchaseOptions" option-label="label" option-value="value" filter size="small" fluid :disabled="readonly"
            :loading="purchases === null" />
        </FormField>
        <label v-if="form.kind === 'receipt'" class="check"><Checkbox v-model="form.return" binary :disabled="readonly" /> <span>Vrátenie od odberateľa (ocení sa cenou zo skladu)</span></label>
        <p v-if="form.kind === 'count'" class="hint">Prebytok zadajte kladným množstvom (s cenou), manko záporným — ocení sa priemernou cenou.</p>

        <div class="doc-lines">
          <div class="dl-head"><span>Položka</span><span class="num">Množstvo</span><span class="num">{{ form.kind === 'issue' || form.kind === 'transfer' || form.return ? 'Ocenenie' : 'Cena za MJ' }}</span><span /></div>
          <div v-for="(l, i) in lines" :key="i" class="dl-row">
            <Select v-model="l.itemId" :options="itemOptions" option-label="label" option-value="value" filter size="small" fluid placeholder="Položka"
              :disabled="readonly" :invalid="!!errors[`lines[${i}].itemId`]" :aria-label="`Položka ${i + 1}`" />
            <InputText v-model="l.quantity" size="small" inputmode="decimal" class="num-in" :disabled="readonly" :invalid="!!errors[`lines[${i}].quantity`]" :aria-label="`Množstvo ${i + 1}`" />
            <InputText v-if="needsCost(form.kind, l.quantity, form.return)" v-model="l.unitCost" size="small" inputmode="decimal" class="num-in" :disabled="readonly" :aria-label="`Cena ${i + 1}`" />
            <span v-else class="auto-cost">{{ l.total !== undefined && form.id ? formatAmount(l.total) + ' €' : (v.method === 'fifo' ? 'podľa FIFO' : 'priemerom') }}</span>
            <Button v-if="!readonly" icon="pi pi-times" text rounded size="small" severity="secondary" :disabled="lines.length < 2" :aria-label="`Odstrániť riadok ${i + 1}`" @click="lines.splice(i, 1)" />
          </div>
          <small v-for="(msg, k) in errors" v-show="k.startsWith('lines')" :key="k" class="err">{{ msg }}</small>
          <Button v-if="!readonly" label="Položka" icon="pi pi-plus" text size="small" class="add" @click="lines.push(blankLine())" />
        </div>
        <FormField label="Poznámka">
          <Textarea v-model="form.note" rows="2" auto-resize :disabled="readonly" />
        </FormField>
        <p v-if="form.id" class="hint">Hodnota dokladu: <b>{{ formatMoney(form.total) }}</b></p>
        <div v-if="!readonly" class="actions">
          <Button v-if="form.id" label="Zmazať" severity="danger" text size="small" @click="removeDoc" />
          <span class="spacer" />
          <Button type="submit" label="Uložiť" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Drawer>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  max-width: 1080px;
  display: flex;
  flex-direction: column;
  gap: 10px;
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
.spacer {
  flex: 1;
}
.summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  background: var(--sheet);
  border: 1px solid var(--rule);
}
.cell {
  display: flex;
  flex-direction: column;
  padding: 8px 12px;
  border-right: 1px solid var(--rule-soft);
}
.cell:last-child {
  border-right: 0;
}
.label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.value {
  font-size: var(--fs-lg);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.value.small {
  font-size: var(--fs-md);
  font-weight: 400;
}
.value.bad,
.neg {
  color: var(--tone-overdue);
}
.grand {
  border-bottom: 3px double currentColor;
  line-height: 1.15;
}
.warn {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
  margin: 0;
}
.empty-state {
  margin: 0;
  padding: 10px 12px;
  background: var(--tone-draft-bg);
  border-left: 3px solid var(--tone-draft);
  font-size: var(--fs-sm);
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.search {
  width: 280px;
}
.search :deep(input) {
  width: 100%;
}
.sel {
  width: 190px;
}
.check {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.sheet {
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 2px solid var(--ink);
}
.scroll {
  overflow-x: auto;
}
.pad {
  padding: 10px 12px;
}
.pad h2 {
  font-size: var(--fs-md);
  margin: 0 0 8px;
}
.list {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.list th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 5px 8px;
  border-bottom: 1px solid var(--rule);
  white-space: nowrap;
}
.list th.num,
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.list td {
  padding: 4px 8px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: top;
}
.list td.num {
  white-space: nowrap;
}
.list tbody tr {
  cursor: pointer;
}
.list tbody tr:hover,
.list tbody tr:focus-visible {
  background: var(--blue-wash);
  outline: none;
}
.empty {
  color: var(--ink-faint);
}
.code {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.kind {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
  margin-left: 4px;
}
.tag {
  font-size: var(--fs-xs);
  border: 1px solid var(--rule);
  color: var(--ink-faint);
  padding: 0 4px;
  margin-left: 4px;
  border-radius: 2px;
  white-space: nowrap;
}
.tag.low {
  border-color: var(--tone-overdue);
  color: var(--tone-overdue);
}
.date {
  white-space: nowrap;
  color: var(--ink-soft);
  font-variant-numeric: tabular-nums;
}
.text {
  color: var(--ink-soft);
}
.lines-sum {
  display: block;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.setup {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.row {
  display: flex;
  gap: 6px;
  align-items: center;
  margin-bottom: 4px;
}
.w-code {
  width: 90px;
}
.w-name {
  flex: 1;
  max-width: 360px;
}
.items-head,
.items-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 130px 100px 30px;
  gap: 6px;
  align-items: center;
}
.items-head {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
  margin-bottom: 2px;
}
.items-row {
  padding: 3px 0;
  border-bottom: 1px solid var(--rule-soft);
}
.num-in :deep(input),
.num-in {
  text-align: right;
}
.method {
  width: min(420px, 100%);
}
.once {
  font-weight: 600;
  margin: 6px 0;
  font-size: var(--fs-sm);
}
.url {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  margin-bottom: 6px;
}
.url :deep(input),
code {
  font-family: ui-monospace, monospace;
  font-size: var(--fs-xs);
  word-break: break-all;
}
.ok {
  color: var(--tone-paid);
}
.add-item {
  margin-top: 8px;
  width: min(420px, 100%);
}
.hint {
  margin: 4px 0 0;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.actions {
  display: flex;
  align-items: center;
  gap: 6px;
}
/* panel */
.dform {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.grid2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}
.facts {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 2px 12px;
  margin: 0;
}
.facts dt {
  color: var(--ink-soft);
}
.facts dd {
  margin: 0;
}
.numl {
  font-variant-numeric: tabular-nums;
}
.moves {
  border-top: 1px solid var(--rule);
}
.move {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 110px 110px;
  gap: 6px;
  padding: 4px 0;
  border-bottom: 1px solid var(--rule-soft);
  font-size: var(--fs-sm);
}
.move.head {
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.move small {
  display: block;
  color: var(--ink-faint);
}
.doc-lines {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.dl-head,
.dl-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 90px 100px 30px;
  gap: 6px;
  align-items: center;
}
.dl-head {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.auto-cost {
  font-size: var(--fs-sm);
  color: var(--ink-faint);
  text-align: right;
}
.err {
  color: var(--tone-overdue);
}
.add {
  align-self: flex-start;
}
@media (max-width: 640px) {
  .page {
    padding: 10px 10px 20px;
  }
  .summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .cell:nth-child(2) {
    border-right: 0;
  }
  .cell:nth-child(-n + 2) {
    border-bottom: 1px solid var(--rule-soft);
  }
  .search,
  .sel {
    flex: 1 1 100%;
    width: auto;
  }
  .hide-m {
    display: none;
  }
  .items-head,
  .items-row {
    grid-template-columns: minmax(0, 1fr) 100px 70px 28px;
  }
  .dl-head,
  .dl-row {
    grid-template-columns: minmax(0, 1fr) 70px 80px 28px;
    gap: 4px;
  }
}
</style>

<style>
.p-drawer.p-component.stock-panel {
  width: min(560px, 100vw);
}
</style>
