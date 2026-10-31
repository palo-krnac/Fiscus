<script setup>
// Kalendár termínov: daňové lehoty (posunuté na pracovný deň), splatnosti
// faktúr, pravidelné faktúry, ponuky a vlastné termíny. Termíny zostavuje
// server (app.Calendar); tu sa iba zobrazujú, filtrujú a exportujú.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import SelectButton from 'primevue/selectbutton'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { useToast } from 'primevue/usetoast'
import ExportButton from '../components/ExportButton.vue'
import OwnEventsDrawer from '../components/OwnEventsDrawer.vue'
import CalendarFeedDialog from '../components/CalendarFeedDialog.vue'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatMoney } from '../lib/format.js'
import { matches, searchTerms } from '../lib/search.js'
import { calendarColumns } from '../lib/columns.js'
import { refreshCalendar } from '../lib/attention.js'
import {
  badgeRange, byDate, dayLabel, kinds, kindLabel, monthGrid, monthNames, monthRange, relative, searchText, shiftMonth, soonCount, stateOf, weekdays,
} from '../lib/calendar.js'

const router = useRouter()
const route = useRoute()
const toast = useToast()

const now = new Date()
const ym = ref({ y: now.getFullYear(), m: now.getMonth() + 1 })
const view = ref(null)
const loading = ref(false)
const mode = ref(window.matchMedia?.('(max-width: 760px)').matches ? 'list' : 'month')
const modes = [
  { value: 'month', label: 'Mesiac', icon: 'pi pi-calendar' },
  { value: 'list', label: 'Zoznam', icon: 'pi pi-list' },
]
const search = ref('')
const hidden = ref(new Set())
const selected = ref('')
const ownOpen = ref(false)
const ownEdit = ref(0)
const feedOpen = ref(false)

const refreshBadge = () => {
  const { from, to } = badgeRange(new Date())
  refreshCalendar(() => api.calendar(session.companyId, from, to), soonCount)
}

async function load() {
  loading.value = true
  const { from, to } = monthRange(ym.value.y, ym.value.m)
  try {
    view.value = await api.calendar(session.companyId, from, to)
    if (!selected.value || selected.value.slice(0, 7) !== from.slice(0, 7)) {
      selected.value = view.value.today.slice(0, 7) === from.slice(0, 7) ? view.value.today : from
    }
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Kalendár sa nenačítal', detail: e.message, life: 6000 })
  } finally {
    loading.value = false
  }
}
onMounted(load)
// návrat z prihlásenia do Google alebo Microsoftu
onMounted(() => {
  const r = route.query.sync
  if (!r) return
  if (r === 'ok') toast.add({ severity: 'success', summary: 'Kalendár prepojený', detail: 'Termíny sa prenášajú do kalendára „Fiškus“.', life: 5000 })
  else toast.add({ severity: 'error', summary: 'Kalendár sa neprepojil', detail: String(r), life: 9000 })
  feedOpen.value = true
  router.replace({ query: {} })
})
watch(ym, load, { deep: true })
watch(() => session.companyId, load)

function go(delta) {
  ym.value = delta === 0 ? { y: now.getFullYear(), m: now.getMonth() + 1 } : shiftMonth(ym.value.y, ym.value.m, delta)
}
const title = computed(() => `${monthNames[ym.value.m - 1]} ${ym.value.y}`)

const filtered = (es) => {
  const terms = searchTerms(search.value)
  return es.filter((e) => !hidden.value.has(e.kind) && matches(terms, searchText(e)))
}
const events = computed(() => filtered(view.value?.events ?? []))
const overdue = computed(() => filtered(view.value?.overdue ?? []))
const counts = computed(() => {
  const c = {}
  for (const e of view.value?.events ?? []) c[e.kind] = (c[e.kind] ?? 0) + 1
  return c
})
const days = computed(() => byDate(events.value))
const holidays = computed(() => Object.fromEntries((view.value?.holidays ?? []).map((h) => [h.date, h.title])))
const grid = computed(() => monthGrid(ym.value.y, ym.value.m))
const dayEvents = computed(() => days.value[selected.value] ?? [])
const overdueLimit = 4
const allOverdue = ref(false)
// najprv vlastné povinnosti (dane, úhrady, vlastné), potom pohľadávky
const shownOverdue = computed(() => {
  const mine = (e) => (['tax', 'own', 'pay'].includes(e.kind) ? 0 : 1)
  const sorted = [...overdue.value].sort((a, b) => mine(a) - mine(b) || a.date.localeCompare(b.date))
  return allOverdue.value ? sorted : sorted.slice(0, overdueLimit)
})
const exportRows = computed(() => [...overdue.value, ...events.value])
const agenda = computed(() => Object.entries(days.value).sort(([a], [b]) => a.localeCompare(b)))

function toggle(kind) {
  const s = new Set(hidden.value)
  s.has(kind) ? s.delete(kind) : s.add(kind)
  hidden.value = s
}

function open(e) {
  if (e.kind === 'own') {
    ownEdit.value = Number(e.id.split('-')[1])
    ownOpen.value = true
    return
  }
  if (e.link) router.push(e.link)
}

// pôvodný dátum výskytu vlastného termínu (pred posunom) je v ID: own-<id>-<dátum>
async function markDone(e) {
  const [, id, ...rest] = e.id.split('-')
  try {
    await api.markOwnEvent(session.companyId, Number(id), rest.join('-'), !e.done)
    await load()
    refreshBadge()
  } catch (err) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: err.message, life: 6000 })
  }
}

function onOwnChanged() {
  ownEdit.value = 0
  load()
  refreshBadge()
}
watch(ownOpen, (v) => {
  if (!v) ownEdit.value = 0
})

const amount = (e) => (e.amount ? formatMoney(e.amount, e.currency || 'EUR') : '')
const fmtShort = (iso) => {
  const [, m, d] = iso.split('-').map(Number)
  return `${d}. ${m}.`
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Kalendár</h1>
      <nav class="month" aria-label="Mesiac">
        <Button icon="pi pi-chevron-left" text size="small" aria-label="Predchádzajúci mesiac" @click="go(-1)" />
        <span class="month-name" aria-live="polite">{{ title }}</span>
        <Button icon="pi pi-chevron-right" text size="small" aria-label="Nasledujúci mesiac" @click="go(1)" />
        <Button label="Dnes" severity="secondary" outlined size="small" @click="go(0)" />
      </nav>
      <div class="spacer" />
      <SelectButton v-model="mode" :options="modes" option-value="value" :allow-empty="false" size="small" aria-label="Zobrazenie">
        <template #option="{ option }"><i :class="option.icon" /><span class="mode-label">{{ option.label }}</span></template>
      </SelectButton>
      <Button label="Vlastné termíny" icon="pi pi-bookmark" severity="secondary" outlined size="small" @click="ownOpen = true" />
      <Button label="Google, Outlook, mobil" icon="pi pi-calendar-plus" size="small" @click="feedOpen = true" />
    </header>

    <section v-if="overdue.length" class="overdue" aria-labelledby="od-title">
      <h2 id="od-title">Po termíne z predchádzajúcich mesiacov <span class="od-count">{{ overdue.length }}</span></h2>
      <ul>
        <li v-for="e in shownOverdue" :key="e.id">
          <span class="od-date">{{ fmtShort(e.date) }}</span>
          <a href="#" class="ev-title" @click.prevent="open(e)">{{ e.title }}</a>
          <span class="num">{{ amount(e) }}</span>
          <button v-if="e.kind === 'own' && view?.canEdit" type="button" class="link" @click="markDone(e)">Splnené</button>
          <span v-else />
        </li>
      </ul>
      <button v-if="overdue.length > overdueLimit" type="button" class="link more-od" @click="allOverdue = !allOverdue">
        {{ allOverdue ? 'Zobraziť menej' : `Zobraziť všetky (${overdue.length})` }}
      </button>
    </section>

    <div class="toolbar">
      <div class="chips" role="group" aria-label="Druhy termínov">
        <button v-for="k in kinds" :key="k.id" type="button" class="chip" :class="['k-' + k.id, { off: hidden.has(k.id) }]"
          :aria-pressed="!hidden.has(k.id)" @click="toggle(k.id)">
          <span class="swatch" aria-hidden="true" />{{ k.label }}<span class="cnt">{{ counts[k.id] ?? 0 }}</span>
        </button>
      </div>
      <div class="spacer" />
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať termín" size="small" aria-label="Hľadať termín" />
      </IconField>
      <ExportButton :name="`Termíny ${title}`" :columns="calendarColumns" :rows="exportRows" />
    </div>

    <div v-if="mode === 'month'" class="month-view">
      <table class="grid" :class="{ busy: loading }">
        <thead>
          <tr><th v-for="w in weekdays" :key="w" scope="col">{{ w }}</th></tr>
        </thead>
        <tbody>
          <tr v-for="(week, i) in grid" :key="i">
            <td v-for="d in week" :key="d.date"
              :class="{ out: !d.inMonth, weekend: d.weekend, holiday: holidays[d.date], today: d.date === view?.today, sel: d.date === selected }"
              @click="selected = d.date">
              <button type="button" class="day" :aria-label="`${d.day}. ${title}`" :aria-pressed="d.date === selected">{{ d.day }}</button>
              <span v-if="holidays[d.date]" class="hol" :title="holidays[d.date]">{{ holidays[d.date] }}</span>
              <ul class="cell-events">
                <li v-for="e in (days[d.date] ?? []).slice(0, 3)" :key="e.id" class="ev" :class="['k-' + e.kind, { done: e.done, late: e.overdue }]" :title="e.title">
                  {{ e.title }}
                </li>
                <li v-if="(days[d.date] ?? []).length > 3" class="more">+ ďalšie {{ days[d.date].length - 3 }}</li>
              </ul>
            </td>
          </tr>
        </tbody>
      </table>

      <aside class="day-panel" aria-live="polite">
        <h2>{{ selected ? dayLabel(selected) : '' }} <small v-if="view && selected">{{ relative(view.today, selected) }}</small></h2>
        <p v-if="holidays[selected]" class="hol-line">{{ holidays[selected] }} — deň pracovného pokoja</p>
        <p v-if="!dayEvents.length" class="empty">Žiadne termíny.</p>
        <ul class="agenda-list">
          <li v-for="e in dayEvents" :key="e.id" class="row" :class="['k-' + e.kind, { done: e.done, late: e.overdue }]">
            <span class="bar" aria-hidden="true" />
            <div class="body">
              <span class="kind">{{ kindLabel(e.kind) }}<template v-if="stateOf(e).text"> · <b :class="'t-' + stateOf(e).tone">{{ stateOf(e).text }}</b></template></span>
              <a href="#" class="ev-title" @click.prevent="open(e)">{{ e.title }}</a>
              <span v-if="e.detail" class="detail">{{ e.detail }}</span>
              <span v-if="e.shifted" class="detail">Lehota {{ fmtShort(e.shifted) }} pripadla na deň pracovného pokoja.</span>
            </div>
            <span class="num">{{ amount(e) }}</span>
            <Button v-if="e.kind === 'own' && view?.canEdit" :icon="e.done ? 'pi pi-undo' : 'pi pi-check'" :aria-label="e.done ? 'Vrátiť' : 'Splnené'"
              v-tooltip.left="e.done ? 'Vrátiť ako nesplnené' : 'Označiť ako splnené'" text rounded size="small" @click="markDone(e)" />
          </li>
        </ul>
      </aside>
    </div>

    <section v-else class="list-view" :class="{ busy: loading }">
      <p v-if="!agenda.length" class="empty">V mesiaci {{ title }} nie sú žiadne termíny{{ search ? ' pre hľadaný text' : '' }}.</p>
      <div v-for="[date, es] in agenda" :key="date" class="agenda-day" :class="{ today: date === view?.today }">
        <h3>
          <span>{{ dayLabel(date) }}</span>
          <small>{{ relative(view.today, date) }}</small>
          <small v-if="holidays[date]" class="hol-line">{{ holidays[date] }}</small>
        </h3>
        <ul class="agenda-list">
          <li v-for="e in es" :key="e.id" class="row" :class="['k-' + e.kind, { done: e.done, late: e.overdue }]">
            <span class="bar" aria-hidden="true" />
            <div class="body">
              <span class="kind">{{ kindLabel(e.kind) }}<template v-if="stateOf(e).text"> · <b :class="'t-' + stateOf(e).tone">{{ stateOf(e).text }}</b></template></span>
              <a href="#" class="ev-title" @click.prevent="open(e)">{{ e.title }}</a>
              <span v-if="e.detail" class="detail">{{ e.detail }}</span>
            </div>
            <span class="num">{{ amount(e) }}</span>
            <Button v-if="e.kind === 'own' && view?.canEdit" :icon="e.done ? 'pi pi-undo' : 'pi pi-check'" :aria-label="e.done ? 'Vrátiť' : 'Splnené'"
              text rounded size="small" @click="markDone(e)" />
          </li>
        </ul>
      </div>
    </section>

    <OwnEventsDrawer v-model:visible="ownOpen" :can-edit="!!view?.canEdit" :edit-id="ownEdit" @changed="onOwnChanged" />
    <CalendarFeedDialog v-model:visible="feedOpen" />
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  /* farby druhov termínov */
  --k-tax: var(--blue-deep);
  --k-own: var(--blue);
  --k-pay: var(--tone-draft);
  --k-receive: var(--tone-paid);
  --k-recurring: var(--ink-faint);
  --k-quote: var(--ink-faint);
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
.month {
  display: flex;
  align-items: center;
  gap: 2px;
}
.month-name {
  font-size: var(--fs-lg);
  font-weight: 600;
  min-width: 132px;
  text-align: center;
}
.spacer {
  flex: 1;
}
.mode-label {
  margin-left: 5px;
}
.k-tax { --k: var(--k-tax); }
.k-own { --k: var(--k-own); }
.k-pay { --k: var(--k-pay); }
.k-receive { --k: var(--k-receive); }
.k-recurring { --k: var(--k-recurring); }
.k-quote { --k: var(--k-quote); }

.overdue {
  background: var(--tone-overdue-bg);
  border-left: 3px solid var(--tone-overdue);
  padding: 8px 12px;
}
.overdue h2 {
  font-size: var(--fs-md);
  color: var(--tone-overdue);
  margin: 0 0 4px;
}
.od-count {
  font-weight: 400;
  margin-left: 4px;
}
.overdue ul {
  list-style: none;
  margin: 0;
  padding: 0;
}
.overdue li {
  display: grid;
  grid-template-columns: 48px minmax(0, 1fr) auto 64px;
  align-items: baseline;
  gap: 10px;
  padding: 2px 0;
}
.od-date {
  font-variant-numeric: tabular-nums;
  color: var(--tone-overdue);
}
.link {
  all: unset;
  color: var(--blue);
  font-size: var(--fs-sm);
  cursor: pointer;
}
.link:hover {
  text-decoration: underline;
}
.link:focus-visible {
  outline: 2px solid var(--blue);
}
.more-od {
  margin-top: 4px;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid var(--rule);
  background: var(--sheet);
  color: var(--ink);
  font: inherit;
  font-size: var(--fs-sm);
  padding: 3px 8px;
  border-radius: var(--radius);
  cursor: pointer;
}
.chip:focus-visible {
  outline: 2px solid var(--blue);
  outline-offset: 1px;
}
.chip .swatch {
  width: 10px;
  height: 10px;
  background: var(--k);
}
.chip.off {
  color: var(--ink-faint);
  background: var(--paper);
}
.chip.off .swatch {
  background: transparent;
  outline: 1px solid var(--k);
}
.cnt {
  color: var(--ink-faint);
  font-variant-numeric: tabular-nums;
}
.search {
  width: 220px;
}
.search :deep(input) {
  width: 100%;
}

.month-view {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 320px;
  gap: 12px;
  align-items: start;
}
.grid {
  width: 100%;
  table-layout: fixed;
  border-collapse: collapse;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 2px solid var(--ink);
}
.busy {
  opacity: 0.6;
}
.grid th {
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 4px 6px;
  text-align: left;
  border-bottom: 1px solid var(--rule);
}
.grid td {
  height: 92px;
  vertical-align: top;
  padding: 3px 4px 4px;
  border: 1px solid var(--rule-soft);
  cursor: pointer;
  overflow: hidden;
}
.grid td.weekend,
.grid td.holiday {
  background: #fafbfc;
}
.grid td.out {
  background: var(--paper);
}
.grid td.out .day {
  color: var(--ink-faint);
}
.grid td.sel {
  box-shadow: inset 0 0 0 2px var(--blue);
}
.day {
  all: unset;
  font-size: var(--fs-sm);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  padding: 0 3px;
  border-radius: 2px;
}
.day:focus-visible {
  outline: 2px solid var(--blue);
}
.today .day {
  background: var(--blue-deep);
  color: #fff;
}
.hol {
  display: block;
  font-size: var(--fs-xs);
  color: var(--tone-overdue);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cell-events {
  list-style: none;
  margin: 2px 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.ev {
  font-size: var(--fs-xs);
  line-height: 1.35;
  padding: 0 4px;
  border-left: 3px solid var(--k);
  background: color-mix(in srgb, var(--k) 9%, white);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ev.k-tax {
  background: var(--k);
  color: #fff;
}
.ev.late {
  border-left-color: var(--tone-overdue);
  color: var(--tone-overdue);
  background: var(--tone-overdue-bg);
}
.ev.done {
  text-decoration: line-through;
  opacity: 0.55;
}
.more {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}

.day-panel {
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 2px solid var(--ink);
  padding: 8px 10px;
  position: sticky;
  top: 8px;
}
.day-panel h2 {
  font-size: var(--fs-lg);
  margin: 0 0 4px;
}
.day-panel h2 small {
  font-size: var(--fs-sm);
  font-weight: 400;
  color: var(--ink-faint);
  margin-left: 6px;
}
.hol-line {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
  margin: 0 0 4px;
}
.empty {
  color: var(--ink-faint);
  margin: 4px 0;
}
.agenda-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid var(--rule-soft);
}
.row:last-child {
  border-bottom: 0;
}
.bar {
  width: 3px;
  align-self: stretch;
  background: var(--k);
  flex: none;
}
.row.late .bar {
  background: var(--tone-overdue);
}
.body {
  display: flex;
  flex-direction: column;
  gap: 1px;
  flex: 1;
  min-width: 0;
}
.kind {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.t-overdue {
  color: var(--tone-overdue);
}
.t-paid {
  color: var(--tone-paid);
}
.ev-title {
  color: var(--ink);
  text-decoration: none;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.ev-title:hover {
  color: var(--blue);
  text-decoration: underline;
}
.row.done .ev-title {
  text-decoration: line-through;
  color: var(--ink-faint);
  font-weight: 400;
}
.row.late .ev-title {
  color: var(--tone-overdue);
}
.detail {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.num {
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  text-align: right;
}

.list-view {
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 2px solid var(--ink);
  padding: 4px 12px 8px;
}
.agenda-day h3 {
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-size: var(--fs-md);
  margin: 10px 0 2px;
  padding-bottom: 2px;
  border-bottom: 1px solid var(--rule);
}
.agenda-day h3 small {
  font-weight: 400;
  color: var(--ink-faint);
}
.agenda-day.today h3 span {
  background: var(--blue-deep);
  color: #fff;
  padding: 0 5px;
}

@media (max-width: 1100px) {
  .month-view {
    grid-template-columns: 1fr;
  }
  .day-panel {
    position: static;
  }
}
@media (max-width: 760px) {
  .page {
    padding: 10px 10px 20px;
  }
  .overdue li {
    grid-template-columns: 40px minmax(0, 1fr) auto;
  }
  .overdue li > :last-child:empty {
    display: none;
  }
  .mode-label {
    display: none;
  }
  .search {
    flex: 1;
    width: auto;
  }
  .grid td {
    height: 58px;
    padding: 2px;
  }
  .grid .hol,
  .grid .more {
    display: none;
  }
  /* v mriežke na mobile iba farebné značky, text je v paneli dňa */
  .ev {
    height: 5px;
    padding: 0;
    font-size: 0;
    border-left-width: 0;
    background: var(--k);
  }
}
</style>
