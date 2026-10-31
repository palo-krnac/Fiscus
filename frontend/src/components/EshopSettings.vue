<script setup>
// Napojenie e-shopov: objednávky → koncepty faktúr, sklad → stav zásob v obchode.
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { formatMoney } from '../lib/format.js'
import { blankConn, catalogSummary, connErrors, formatWhen, importSummary, orderStatus, splitError, stockSummary, toSave } from '../lib/eshops.js'

const toast = useToast()
const confirm = useConfirm()
const platforms = ref([])
const conns = ref([])
const saved = ref('[]')
const errors = ref({})
const busy = ref('')
const results = ref({}) // id → výsledok načítania objednávok
const stockResults = ref({})
const dirty = computed(() => JSON.stringify(toSave(conns.value)) !== saved.value)
const info = (p) => platforms.value.find((x) => x.platform === p) ?? {}

function apply(out) {
  platforms.value = out.platforms
  conns.value = out.conns.map((c) => ({ ...c, key: '', secret: '' }))
  saved.value = JSON.stringify(toSave(conns.value))
}

async function load() {
  try {
    apply(await api.eshops(session.companyId))
  } catch (e) {
    toast.add({ severity: 'error', summary: 'E-shopy sa nenačítali', detail: e.message, life: 6000 })
  }
}
onMounted(load)
watch(() => session.companyId, load)

function add() {
  conns.value.push(blankConn(platforms.value[0]?.platform))
}

function remove(i) {
  conns.value.splice(i, 1)
}

async function save() {
  busy.value = 'save'
  try {
    apply(await api.saveEshops(session.companyId, toSave(conns.value)))
    errors.value = {}
    toast.add({ severity: 'success', summary: 'Napojenie uložené', life: 2500 })
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) {
      errors.value = e.fieldMap()
      toast.add({ severity: 'error', summary: 'Skontrolujte vyznačené polia', life: 4000 })
    } else {
      toast.add({ severity: 'error', summary: 'Uloženie zlyhalo', detail: e.message, life: 6000 })
    }
  } finally {
    busy.value = ''
  }
}

async function run(c, what) {
  busy.value = what + c.id
  try {
    if (what === 'test') {
      const { name } = await api.testEshop(session.companyId, c.id)
      toast.add({ severity: 'success', summary: 'Spojenie funguje', detail: name, life: 4000 })
    } else if (what === 'orders') {
      const r = await api.syncEshopOrders(session.companyId, c.id)
      results.value = { ...results.value, [c.id]: r }
      toast.add({ severity: r.failed ? 'warn' : 'success', summary: `Nové koncepty faktúr: ${r.created}`, life: 4000 })
      await load()
    } else if (what === 'import') {
      const preview = await api.importEshopCatalog(session.companyId, c.id, false)
      busy.value = ''
      confirm.require({
        header: 'Prevziať z obchodu',
        message: `${importSummary(preview)}. Položky a kontakty, ktoré už vo Fiškuse sú, ostanú bez zmeny.`,
        acceptLabel: 'Prevziať',
        rejectLabel: 'Zrušiť',
        accept: async () => {
          busy.value = 'import' + c.id
          try {
            const r = await api.importEshopCatalog(session.companyId, c.id, true)
            toast.add({ severity: 'success', summary: 'Prevzaté z obchodu', detail: importSummary(r), life: 6000 })
          } catch (e) {
            toast.add({ severity: 'error', summary: 'Prenos zlyhal', detail: e.message, life: 8000 })
          } finally {
            busy.value = ''
          }
        },
      })
      return
    } else if (what === 'catalog') {
      const r = await api.pushEshopCatalog(session.companyId, c.id)
      stockResults.value = { ...stockResults.value, [c.id]: { text: catalogSummary(r) } }
      toast.add({ severity: r.errors.length ? 'warn' : 'success', summary: 'Cenník odoslaný', detail: catalogSummary(r), life: 6000 })
      await load()
    } else {
      const r = await api.pushEshopStock(session.companyId, c.id)
      stockResults.value = { ...stockResults.value, [c.id]: { text: stockSummary(r) } }
      toast.add({ severity: r.missing.length || r.errors.length ? 'warn' : 'success', summary: 'Zásoby odoslané', detail: stockSummary(r), life: 6000 })
      await load()
    }
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Obchod neodpovedal', detail: e.message, life: 8000 })
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <fieldset class="form-section">
    <legend>Napojené obchody</legend>
    <p class="lead">
      Fiškus načíta objednávky z e-shopu ako <strong>koncepty faktúr</strong> — skontrolujete ich a vystavíte. Položky s rovnakým kódom
      ako v cenníku sa naviažu na cenník a pri vystavení sa vydajú zo skladu. Obchod sa dá riadiť z Fiškusu: cenník (názvy, ceny, DPH) a zásoby idú z Fiškusu do obchodu, produkty a zákazníkov z obchodu prevezmete jedným tlačidlom.
    </p>
    <p class="unverified">
      <i class="pi pi-info-circle" aria-hidden="true" />
      <span>Konektory sú napísané podľa verejnej dokumentácie platforiem a zatiaľ <strong>neboli overené na živom obchode</strong>.
      Prvé načítanie si dôkladne skontrolujte.</span>
    </p>

    <div v-for="(c, i) in conns" :key="c.id || 'new' + i" class="conn">
      <header class="conn-head">
        <strong>{{ c.name || info(c.platform).name || 'Nový obchod' }}</strong>
        <span class="tag">neoverené</span>
        <span v-if="c.lastSync" class="when">Objednávky {{ formatWhen(c.lastSync) }} — {{ c.lastResult }}</span>
        <div class="spacer" />
        <Button v-if="can('settings.write')" icon="pi pi-trash" text size="small" severity="secondary" aria-label="Odpojiť obchod" v-tooltip.bottom="'Odpojiť (po uložení)'" @click="remove(i)" />
      </header>
      <div class="form-grid">
        <FormField label="Platforma" :error="connErrors(errors, i).platform">
          <Select v-model="c.platform" :options="platforms" option-label="name" option-value="platform" size="small" :disabled="!!c.id" />
        </FormField>
        <FormField label="Názov" hint="Ako sa obchod zobrazí vo Fiškuse">
          <InputText v-model="c.name" size="small" :placeholder="info(c.platform).name" />
        </FormField>
        <FormField label="Adresa obchodu" :span="2" :error="connErrors(errors, i).url">
          <InputText v-model="c.url" size="small" inputmode="url" spellcheck="false" :placeholder="info(c.platform).urlHint" :invalid="!!connErrors(errors, i).url" />
        </FormField>
        <FormField v-if="info(c.platform).keyLabel" :label="info(c.platform).keyLabel" :span="2" :error="connErrors(errors, i).key" :hint="c.hasKey ? 'Uložené — vyplňte iba pri zmene' : ''">
          <InputText v-model="c.key" size="small" spellcheck="false" autocomplete="off" :invalid="!!connErrors(errors, i).key" />
        </FormField>
        <FormField :label="info(c.platform).secretLabel || 'Prístupový kľúč'" :span="2" :error="connErrors(errors, i).secret" :hint="c.hasSecret ? 'Uložené — vyplňte iba pri zmene' : ''">
          <Password v-model="c.secret" :feedback="false" toggle-mask size="small" autocomplete="new-password" :invalid="!!connErrors(errors, i).secret" fluid />
        </FormField>
        <FormField label="Objednávky od" :error="connErrors(errors, i).since" hint="Prázdne = posledných 30 dní">
          <input v-model="c.since" type="date" class="p-inputtext p-component p-inputtext-sm" />
        </FormField>
        <label class="check">
          <Checkbox v-model="c.onlyPaid" binary />
          <span>Faktúra až z uhradenej objednávky<small>Neuhradené počkajú na ďalšie načítanie</small></span>
        </label>
        <label class="check">
          <Checkbox v-model="c.auto" binary />
          <span>Načítavať automaticky každú hodinu<small>Kým beží Fiškus; koncepty počkajú na kontrolu</small></span>
        </label>
        <label v-if="info(c.platform).catalog" class="check">
          <Checkbox v-model="c.manage" binary />
          <span>Riadiť obchod z Fiškusu<small>Každú hodinu pošle zmeny cenníka (názov, cena, DPH, vyradenie) a zásoby; novú skladovú položku založí ako koncept, služby nie</small></span>
        </label>
      </div>
      <p v-if="splitError(c.lastError)" class="auto-error">
        <i class="pi pi-exclamation-triangle" aria-hidden="true" />
        Automatické načítanie {{ splitError(c.lastError).when }} zlyhalo: {{ splitError(c.lastError).text }}
      </p>
      <p class="help">
        {{ info(c.platform).help }}
        <a v-if="info(c.platform).docs" :href="info(c.platform).docs" target="_blank" rel="noopener">Dokumentácia API</a>
      </p>
      <div class="actions">
        <span v-if="!c.id" class="muted">Akcie budú dostupné po uložení.</span>
        <template v-else>
          <Button label="Otestovať spojenie" icon="pi pi-wifi" size="small" outlined :loading="busy === 'test' + c.id" :disabled="dirty || !!busy" @click="run(c, 'test')" />
          <Button v-if="can('invoices.write')" label="Načítať objednávky" icon="pi pi-download" size="small" :loading="busy === 'orders' + c.id" :disabled="dirty || !!busy" @click="run(c, 'orders')" />
          <Button v-if="can('purchases.write') && info(c.platform).stock" label="Poslať zásoby" icon="pi pi-upload" size="small" outlined :loading="busy === 'stock' + c.id" :disabled="dirty || !!busy" @click="run(c, 'stock')" />
          <Button v-if="can('invoices.write') && info(c.platform).catalog" label="Prevziať produkty a zákazníkov" icon="pi pi-file-import" size="small" outlined :loading="busy === 'import' + c.id" :disabled="dirty || !!busy" @click="run(c, 'import')" />
          <Button v-if="can('invoices.write') && info(c.platform).catalog" label="Poslať cenník" icon="pi pi-tags" size="small" outlined :loading="busy === 'catalog' + c.id" :disabled="dirty || !!busy" @click="run(c, 'catalog')" />
          <span v-if="dirty" class="muted">Najprv uložte zmeny.</span>
          <span v-else-if="!info(c.platform).stock" class="muted">Tento obchod stav zásob cez API neprijíma.</span>
          <span v-else-if="c.lastStock" class="muted">Zásoby odoslané {{ formatWhen(c.lastStock) }}</span>
        </template>
      </div>

      <div v-if="stockResults[c.id]" class="result">{{ stockResults[c.id].text }}</div>
      <div v-if="results[c.id]" class="result">
        <p v-if="!results[c.id].orders.length" class="muted">Žiadne nové objednávky.</p>
        <table v-else class="orders">
          <thead>
            <tr><th>Objednávka</th><th>Zákazník</th><th class="num">Suma</th><th>Výsledok</th></tr>
          </thead>
          <tbody>
            <tr v-for="o in results[c.id].orders" :key="o.order">
              <td>{{ o.order }}</td>
              <td>{{ o.customer }}</td>
              <td class="num">{{ formatMoney(o.total) }}</td>
              <td>
                <span class="status" :class="'tone-' + orderStatus(o.status).tone">{{ orderStatus(o.status).label }}</span>
                <RouterLink v-if="o.invoiceId" :to="`/faktury/${o.invoiceId}`" class="open">otvoriť</RouterLink>
                <small v-if="o.message" class="msg">{{ o.message }}</small>
              </td>
            </tr>
          </tbody>
        </table>
        <p v-for="w in results[c.id].warnings" :key="w" class="muted">{{ w }}</p>
      </div>
    </div>

    <p v-if="!conns.length" class="muted empty">Zatiaľ nie je napojený žiadny obchod.</p>
    <div v-if="can('settings.write')" class="bottom">
      <Button label="Pridať obchod" icon="pi pi-plus" size="small" outlined :disabled="conns.length >= 10" @click="add" />
      <div class="spacer" />
      <Button label="Uložiť napojenie" icon="pi pi-check" size="small" :loading="busy === 'save'" :disabled="!dirty || !!busy" @click="save" />
    </div>
  </fieldset>
</template>

<style scoped>
.lead {
  max-width: 68ch;
  margin-bottom: 8px;
}
.unverified {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  max-width: 68ch;
  padding: 8px 10px;
  margin-bottom: 12px;
  background: var(--tone-draft-bg);
  color: var(--tone-draft);
  font-size: var(--fs-sm);
}
.unverified .pi {
  margin-top: 2px;
}
.conn {
  padding: 12px 0 14px;
  border-top: 1px solid var(--rule);
}
.conn-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.tag {
  padding: 0 6px;
  border: 1px solid var(--tone-draft);
  border-radius: var(--radius);
  color: var(--tone-draft);
  font-size: var(--fs-xs);
}
.when {
  order: 3;
  flex-basis: 100%;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.spacer {
  flex: 1;
}
.check {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  align-self: center;
  cursor: pointer;
}
.check small {
  display: block;
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.auto-error {
  margin-top: 8px;
  font-size: var(--fs-sm);
  color: var(--tone-overdue);
}
.help {
  margin-top: 8px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  max-width: 80ch;
}
.actions,
.bottom {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 10px;
}
.bottom {
  padding-top: 12px;
  border-top: 1px solid var(--rule);
}
.muted {
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.result {
  margin-top: 10px;
  font-size: var(--fs-sm);
}
.orders {
  width: 100%;
  border-collapse: collapse;
}
.orders th,
.orders td {
  padding: 4px 8px 4px 0;
  border-bottom: 1px solid var(--rule-soft);
  text-align: left;
  vertical-align: top;
}
.orders th {
  font-weight: 600;
  color: var(--ink-soft);
}
.num {
  text-align: right !important;
  font-variant-numeric: tabular-nums;
}
.status {
  padding: 0 6px;
  border-radius: var(--radius);
}
.tone-paid {
  background: var(--tone-paid-bg);
  color: var(--tone-paid);
}
.tone-draft {
  background: var(--tone-draft-bg);
  color: var(--tone-draft);
}
.tone-cancelled {
  background: var(--tone-cancelled-bg);
  color: var(--tone-cancelled);
}
.tone-overdue {
  background: var(--tone-overdue-bg);
  color: var(--tone-overdue);
}
.open {
  margin-left: 6px;
}
.msg {
  display: block;
  color: var(--ink-faint);
}
.empty {
  padding: 8px 0;
}
</style>
