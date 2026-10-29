<script setup>
// Audit: kto, kedy, cez aký kanál (web, API, AI/MCP) a čo zmenil.
// Rozdelenie podľa kategórií a podľa firiem; správca inštalácie vidí všetky firmy.
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Select from 'primevue/select'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { codes } from '../lib/codes.js'
import { session } from '../lib/session.js'
import { actionLabel, channelLabel, diffFields, isAlert } from '../lib/audit.js'

const PAGE = 100
const toast = useToast()
const entries = ref([])
const total = ref(0)
const loading = ref(false)
const open = ref(null) // id rozbaleného záznamu
const verify = ref(null)

const superAdmin = computed(() => !!session.me?.user?.superAdmin)
const filter = ref({ category: null, from: '', to: '', q: '', scope: 'company' })

const categories = computed(() => codes.auditCategories ?? [])
const scopes = computed(() => [
  { value: 'company', label: `Táto firma (${session.company?.name ?? ''})` },
  { value: 'all', label: 'Všetky firmy a prihlásenia' },
  { value: 'none', label: 'Mimo firmy (prihlásenia, systém)' },
  ...session.companies.filter((c) => c.id !== session.companyId).map((c) => ({ value: String(c.id), label: c.name })),
])
const showCompany = computed(() => filter.value.scope === 'all')
const catLabel = (c) => codes.auditCategories?.find((x) => x.code === c)?.label ?? c
const companyName = (id) => (id ? (session.companies.find((c) => c.id === id)?.name ?? `#${id}`) : '—')

const fmtTime = new Intl.DateTimeFormat('sk-SK', {
  timeZone: 'Europe/Bratislava',
  day: 'numeric',
  month: 'numeric',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})

function params(offset) {
  const f = filter.value
  const p = { category: f.category, from: f.from, to: f.to, q: f.q.trim(), limit: PAGE, offset }
  if (f.scope === 'none') p.company = 0
  else if (f.scope !== 'company' && f.scope !== 'all') p.company = f.scope
  return p
}

async function load(more = false) {
  loading.value = true
  try {
    const offset = more ? entries.value.length : 0
    const cid = filter.value.scope === 'company' ? session.companyId : 0
    const res = await api.audit(cid, params(offset))
    entries.value = more ? [...entries.value, ...res.entries] : res.entries
    total.value = res.total
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Audit', detail: e.message, life: 5000 })
  } finally {
    loading.value = false
  }
}

let timer
watch(
  filter,
  () => {
    clearTimeout(timer)
    timer = setTimeout(() => load(), 250)
  },
  { deep: true },
)
onMounted(() => load())

function toggle(e) {
  open.value = open.value === e.id ? null : e.id
}

async function runVerify() {
  try {
    verify.value = await api.verifyAudit()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Overenie auditu', detail: e.message, life: 5000 })
  }
}

function reset() {
  filter.value = { category: null, from: '', to: '', q: '', scope: filter.value.scope }
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Audit</h1>
      <span class="count">{{ total.toLocaleString('sk-SK') }} záznamov</span>
      <div class="spacer" />
      <span v-if="verify" class="verify" :class="verify.ok ? 'ok' : 'bad'" role="status">
        <i class="pi" :class="verify.ok ? 'pi-verified' : 'pi-exclamation-triangle'" />
        {{ verify.ok ? `Reťazec neporušený (${verify.entries})` : verify.error }}
      </span>
      <Button
        v-if="superAdmin"
        label="Overiť celistvosť"
        icon="pi pi-shield"
        severity="secondary"
        outlined
        size="small"
        v-tooltip.bottom="'Každý záznam obsahuje odtlačok predchádzajúceho — zmena alebo zmazanie sa prejaví'"
        @click="runVerify"
      />
    </header>

    <div class="filters">
      <Select v-if="superAdmin && session.caps.multiCompany" v-model="filter.scope" :options="scopes" option-label="label" option-value="value" size="small" class="scope" aria-label="Firma" />
      <Select v-model="filter.category" :options="categories" option-label="label" option-value="code" placeholder="Všetky kategórie" show-clear size="small" class="cat" aria-label="Kategória" />
      <label class="date"><span>od</span><InputText v-model="filter.from" type="date" size="small" aria-label="Od dátumu" /></label>
      <label class="date"><span>do</span><InputText v-model="filter.to" type="date" size="small" aria-label="Do dátumu" /></label>
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="filter.q" placeholder="Hľadať popis, používateľa" size="small" aria-label="Hľadať v audite" />
      </IconField>
      <Button v-if="filter.category || filter.from || filter.to || filter.q" label="Zrušiť filter" text size="small" @click="reset" />
    </div>

    <div class="table-wrap">
      <table class="log">
        <thead>
          <tr>
            <th class="time">Čas</th>
            <th v-if="showCompany">Firma</th>
            <th>Kategória</th>
            <th>Používateľ</th>
            <th>Akcia</th>
            <th class="sum">Popis</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!entries.length && !loading"><td :colspan="showCompany ? 6 : 5" class="empty">Žiadne záznamy pre zvolený filter.</td></tr>
          <template v-for="e in entries" :key="e.id">
            <tr class="row" :class="{ alert: isAlert(e), open: open === e.id }" tabindex="0" @click="toggle(e)" @keydown.enter="toggle(e)">
              <td class="time">{{ fmtTime.format(new Date(e.time)) }}</td>
              <td v-if="showCompany" class="company">{{ companyName(e.companyId) }}</td>
              <td><span class="cat-tag" :data-cat="e.category">{{ catLabel(e.category) }}</span></td>
              <td class="user">
                {{ e.userName || '—' }}
                <span v-if="e.channel !== 'web' && e.channel !== 'system'" class="chan" :data-chan="e.channel">{{ channelLabel(e.channel) }}</span>
              </td>
              <td class="action">{{ actionLabel(e.action) }}</td>
              <td class="sum">{{ e.summary }}</td>
            </tr>
            <tr v-if="open === e.id" class="detail">
              <td :colspan="showCompany ? 6 : 5">
                <dl class="meta">
                  <dt>Kanál</dt><dd>{{ channelLabel(e.channel) }}</dd>
                  <dt v-if="e.ip">IP adresa</dt><dd v-if="e.ip">{{ e.ip }}</dd>
                  <dt v-if="e.entityType">Objekt</dt><dd v-if="e.entityType">{{ e.entityType }} #{{ e.entityId }}</dd>
                  <dt>Odtlačok</dt><dd class="hash" :title="e.hash">{{ e.hash.slice(0, 16) }}…</dd>
                </dl>
                <table v-if="diffFields(e.before, e.after).length" class="diff">
                  <thead><tr><th>Pole</th><th>Pred</th><th>Po</th></tr></thead>
                  <tbody>
                    <tr v-for="d in diffFields(e.before, e.after)" :key="d.field">
                      <td class="f">{{ d.field }}</td>
                      <td :class="{ b: d.before !== '—' }">{{ d.before }}</td>
                      <td class="a">{{ d.after }}</td>
                    </tr>
                  </tbody>
                </table>
                <p v-else class="nodiff">Bez zmeny údajov.</p>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <div class="more">
      <Button v-if="entries.length < total" :label="`Načítať ďalšie (${total - entries.length})`" severity="secondary" text size="small" :loading="loading" @click="load(true)" />
    </div>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
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
.count {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.spacer {
  flex: 1;
}
.verify {
  font-size: var(--fs-sm);
}
.verify.ok {
  color: var(--tone-paid);
}
.verify.bad {
  color: var(--tone-overdue);
  font-weight: 600;
}
.filters {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}
.scope {
  min-width: 220px;
}
.cat {
  min-width: 170px;
}
.date {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.date input {
  width: 140px;
}
.search {
  width: 240px;
}
.table-wrap {
  overflow-x: auto;
  border: 1px solid var(--rule);
  background: var(--sheet);
}
.log {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-md);
}
.log th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
  white-space: nowrap;
}
.log td {
  padding: 4px 10px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: top;
}
.row {
  cursor: pointer;
}
.row:hover,
.row.open {
  background: var(--blue-wash);
}
.row:focus-visible {
  outline: 2px solid var(--blue);
  outline-offset: -2px;
}
.row.alert .action,
.row.alert .sum {
  color: var(--tone-overdue);
}
.time {
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
  color: var(--ink-soft);
}
.company,
.user,
.action {
  white-space: nowrap;
}
.chan {
  margin-left: 4px;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
  border: 1px solid var(--rule);
  border-radius: 3px;
  padding: 0 4px;
}
.chan[data-chan='mcp'] {
  color: var(--blue-deep);
  border-color: var(--blue);
  background: var(--blue-wash);
}
.cat-tag {
  font-size: var(--fs-sm);
  white-space: nowrap;
}
.empty {
  color: var(--ink-faint);
  padding: 14px 10px !important;
}
.detail td {
  background: #fafbfc;
  padding: 8px 12px 12px;
}
.meta {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 2px 12px;
  margin: 0 0 8px;
  font-size: var(--fs-sm);
}
.meta dt {
  color: var(--ink-faint);
}
.meta dd {
  margin: 0;
}
.hash {
  font-family: ui-monospace, monospace;
  font-size: var(--fs-xs);
}
.diff {
  border-collapse: collapse;
  font-size: var(--fs-sm);
  max-width: 100%;
}
.diff th {
  text-align: left;
  font-weight: 600;
  color: var(--ink-soft);
  padding: 3px 10px 3px 0;
  border-bottom: 1px solid var(--rule);
}
.diff td {
  padding: 2px 10px 2px 0;
  border: 0;
  word-break: break-word;
}
.diff .f {
  color: var(--ink-soft);
  white-space: nowrap;
}
.diff .b {
  color: var(--tone-overdue);
  text-decoration: line-through;
  text-decoration-color: rgba(176, 58, 38, 0.4);
}
.diff .a {
  color: var(--tone-paid);
}
.nodiff {
  margin: 0;
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.more {
  display: flex;
  justify-content: center;
}

@media (max-width: 640px) {
  .scope,
  .cat,
  .search {
    width: 100%;
    min-width: 0;
  }
  .search :deep(input) {
    width: 100%;
  }
  .date {
    flex: 1;
  }
  .date input {
    width: 100%;
  }
  .log thead {
    display: none;
  }
  .log,
  .log tbody,
  .log tr,
  .log td {
    display: block;
  }
  .row {
    display: grid !important;
    grid-template-columns: 1fr auto;
    padding: 6px 10px;
    border-bottom: 1px solid var(--rule-soft);
  }
  .row td {
    border: 0;
    padding: 0;
  }
  .row .time {
    font-size: var(--fs-xs);
  }
  .row .sum {
    grid-column: 1 / -1;
    order: 5;
  }
  .row .action {
    font-size: var(--fs-xs);
    color: var(--ink-faint);
  }
}
</style>
