<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { formatAmount, formatDate, formatMoney, toISODate } from '../lib/format.js'
import { saveBlob } from '../lib/export.js'
import { defaultSelection, selectedTotal } from '../lib/paymentorder.js'

const visible = defineModel('visible', { type: Boolean, default: false })
const router = useRouter()
const toast = useToast()

const cands = ref([])
const selected = ref([])
const date = ref(new Date())
const loading = ref(false)
const busy = ref(false)
const error = ref('')

const companyIBAN = computed(() => session.company?.iban ?? '')
const eligible = computed(() => cands.value.filter((c) => !c.problem))
const total = computed(() => selectedTotal(cands.value, selected.value))
const allOn = computed(() => eligible.value.length > 0 && selected.value.length === eligible.value.length)

async function load() {
  loading.value = true
  error.value = ''
  try {
    cands.value = await api.paymentCandidates(session.companyId)
    selected.value = defaultSelection(cands.value, toISODate(new Date()))
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
watch(visible, (v) => v && load())

function toggleAll() {
  selected.value = allOn.value ? [] : eligible.value.map((c) => c.id)
}

async function downloadOrder() {
  busy.value = true
  error.value = ''
  try {
    const { blob, name } = await api.paymentOrder(session.companyId, selected.value, toISODate(date.value))
    saveBlob(blob, name)
    toast.add({ severity: 'success', summary: 'Príkaz stiahnutý', detail: `${name} nahrajte v internet bankingu. Faktúry sa označia ako uhradené po načítaní výpisu.`, life: 8000 })
    visible.value = false
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" header="Platobný príkaz" modal :style="{ width: '58rem' }" :breakpoints="{ '760px': '96vw' }">
    <div class="po-head">
      <div class="from">
        <span class="label">Z účtu</span>
        <span v-if="companyIBAN" class="iban">{{ companyIBAN }}</span>
        <span v-else class="missing">
          Firma nemá IBAN.
          <Button v-if="can('settings.write')" label="Doplniť v nastaveniach" text size="small" @click="(visible = false), router.push('/nastavenia')" />
        </span>
      </div>
      <label class="when">
        <span class="label">Dátum splatnosti</span>
        <DatePicker v-model="date" :min-date="new Date()" date-format="d. m. yy" show-icon icon-display="input" size="small" />
      </label>
    </div>

    <p v-if="loading" class="muted">Načítavam neuhradené faktúry…</p>
    <p v-else-if="!cands.length" class="muted">Žiadne neuhradené prijaté faktúry.</p>
    <div v-else class="po-wrap">
      <table class="po">
        <thead>
          <tr>
            <th class="chk"><Checkbox :model-value="allOn" binary aria-label="Vybrať všetky" @update:model-value="toggleAll" /></th>
            <th>Faktúra</th>
            <th class="due">Splatná</th>
            <th class="vs">VS</th>
            <th class="acc">IBAN</th>
            <th class="num">Suma</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in cands" :key="c.id" :class="{ off: c.problem, overdue: c.overdue && !c.problem }">
            <td class="chk">
              <Checkbox v-model="selected" :value="c.id" :disabled="!!c.problem" :aria-label="`Zaradiť ${c.number}`" />
            </td>
            <td>
              <div class="who">{{ c.supplier }}</div>
              <div class="sub">{{ c.number }}<template v-if="c.supplierNumber"> · {{ c.supplierNumber }}</template></div>
              <div v-if="c.problem" class="problem">{{ c.problem }}</div>
            </td>
            <td class="due">{{ formatDate(c.dueDate) }}</td>
            <td class="vs">{{ c.variableSymbol }}</td>
            <td class="acc">{{ c.iban }}</td>
            <td class="num">{{ formatAmount(c.amount) }}<span v-if="c.currency && c.currency !== 'EUR'" class="cur">&nbsp;{{ c.currency }}</span></td>
          </tr>
        </tbody>
      </table>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <template #footer>
      <div class="po-footer">
        <div class="po-foot">
          <span class="sum">Vybrané {{ selected.length }} · spolu <strong>{{ formatMoney(total) }}</strong></span>
          <Button label="Zrušiť" severity="secondary" text size="small" @click="visible = false" />
          <Button icon="pi pi-download" label="Stiahnuť príkaz (XML)" size="small" :loading="busy" :disabled="!selected.length || !companyIBAN" @click="downloadOrder" />
        </div>
        <p class="hint">Súbor SEPA (pain.001) nahrajte v internet bankingu ako hromadný príkaz. Platby podpíšete v banke.</p>
      </div>
    </template>
  </Dialog>
</template>

<style scoped>
.po-head {
  display: flex;
  gap: 24px;
  align-items: flex-end;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.from,
.when {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.iban {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.missing {
  color: var(--tone-overdue);
}
.po-wrap {
  max-height: 52vh;
  overflow: auto;
  border-top: 2px solid var(--ink);
}
.po {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.po th {
  position: sticky;
  top: 0;
  background: var(--sheet);
  text-align: left;
  font-weight: 500;
  color: var(--ink-soft);
  padding: 5px 6px;
  border-bottom: 1px solid var(--rule);
}
.po td {
  padding: 6px;
  border-bottom: 1px solid var(--rule);
  vertical-align: top;
}
.po .chk {
  width: 32px;
}
.po .num {
  text-align: right;
  white-space: nowrap;
}
.po .due,
.po .vs {
  white-space: nowrap;
}
.po .acc {
  white-space: nowrap;
  color: var(--ink-soft);
}
.who {
  font-weight: 600;
}
.sub {
  color: var(--ink-soft);
}
.problem {
  color: var(--tone-overdue);
}
tr.off td:not(:first-child) {
  color: var(--ink-faint);
}
tr.off .problem {
  color: var(--tone-overdue);
}
tr.overdue .due {
  color: var(--tone-overdue);
  font-weight: 600;
}
.cur {
  font-size: var(--fs-xs);
}
.po-footer {
  width: 100%;
}
.po-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.sum {
  margin-right: auto;
}
.hint {
  margin: 8px 0 0;
  text-align: right;
  font-size: var(--fs-xs);
  color: var(--ink-soft);
}
.muted {
  color: var(--ink-soft);
}
.error {
  color: var(--tone-overdue);
  margin: 8px 0 0;
}
@media (max-width: 760px) {
  .po .vs,
  .po .acc {
    display: none;
  }
  .po-foot {
    flex-wrap: wrap;
  }
  .sum {
    width: 100%;
  }
}
</style>
