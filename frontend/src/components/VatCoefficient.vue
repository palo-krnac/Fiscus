<script setup>
// Pomerné odpočítanie dane (§ 50): predbežný koeficient roka, ročný
// koeficient z plnení a vysporiadanie v poslednom období. Výpočty robí server.
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import { useToast } from 'primevue/usetoast'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { session, can } from '../lib/session.js'
import { formatAmount, formatMoney } from '../lib/format.js'
import { coefPeriodLabel } from '../lib/coef.js'

const props = defineProps({ year: { type: Number, required: true } })
const emit = defineEmits(['saved'])
const toast = useToast()
const v = ref(null)
const open = ref(false)
const form = ref({ provisional: null, final: null, note: '' })
const errors = ref({})
const busy = ref(false)
const canWrite = computed(() => can('ledger.write'))

async function load() {
  try {
    v.value = await api.vatCoefficient(session.companyId, props.year)
    const s = v.value.settings
    form.value = { provisional: s.provisional ?? null, final: s.final ?? null, note: s.note ?? '' }
  } catch {
    v.value = null
  }
}
watch(() => props.year, load, { immediate: true })

// panel sa ukáže, keď sú doklady s pomerným odpočítaním alebo je koeficient zadaný
const relevant = computed(() => v.value && (v.value.docs > 0 || v.value.settings.provisional != null || v.value.settings.final != null))

async function save() {
  busy.value = true
  errors.value = {}
  try {
    v.value = await api.setVATCoefficient(session.companyId, props.year, {
      provisional: form.value.provisional ?? null, final: form.value.final ?? null, note: form.value.note,
    })
    toast.add({ severity: 'success', summary: 'Koeficient uložený', detail: `Rok ${props.year}`, life: 3000 })
    open.value = false
    emit('saved')
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) errors.value = e.fieldMap()
    else toast.add({ severity: 'error', summary: 'Koeficient sa neuložil', detail: e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section v-if="v" class="coef" :class="{ quiet: !relevant && !open }" aria-labelledby="coef-title">
    <div class="head">
      <h2 id="coef-title">Pomerné odpočítanie DPH (§ 50) — rok {{ year }}</h2>
      <div class="spacer" />
      <Button v-if="canWrite" :label="open ? 'Zavrieť' : relevant ? 'Upraviť koeficient' : 'Nastaviť koeficient'" text size="small" @click="open = !open" />
    </div>
    <p v-if="!relevant && !open" class="muted">
      Ak nákupy slúžia aj oslobodeným plneniam bez nároku na odpočet (napr. nájom bytu), označte ich pri prijatej faktúre
      ako spoločné — DPH sa odpočíta koeficientom.
    </p>
    <dl v-else class="figures">
      <div>
        <dt>Predbežný koeficient</dt>
        <dd v-if="v.provisionalOk">{{ v.provisional }} %<small>{{ v.provisionalSource }}</small></dd>
        <dd v-else class="missing">chýba<small>prvý rok: kvalifikovaný odhad</small></dd>
      </div>
      <div>
        <dt>Ročný koeficient</dt>
        <dd>{{ v.final }} %<small>{{ v.finalManual ? 'zadaný ručne' : `z plnení vo Fiškuse (zatiaľ ${v.computed} %)` }}</small></dd>
      </div>
      <div>
        <dt>Plnenia s nárokom / oslobodené</dt>
        <dd class="num">{{ formatAmount(v.turnover.withRight) }} / {{ formatAmount(v.turnover.exempt) }}</dd>
      </div>
      <div>
        <dt>Doklady s koeficientom</dt>
        <dd class="num">{{ v.docs }}</dd>
      </div>
      <div v-if="v.adjustment">
        <dt>Vysporiadanie v {{ coefPeriodLabel(v.lastPeriod) }}</dt>
        <dd class="num" :class="{ minus: Number(v.adjustment.total) < 0 }">{{ formatMoney(v.adjustment.total) }}<small>r. 18 – 19, {{ Number(v.adjustment.total) < 0 ? 'znižuje odpočet' : 'zvyšuje odpočet' }}</small></dd>
      </div>
    </dl>

    <form v-if="open" class="form-grid edit" @submit.prevent="save">
      <FormField label="Predbežný koeficient (%)" :error="errors.provisional" hint="Prázdne = ročný koeficient predchádzajúceho roka">
        <InputNumber v-model="form.provisional" :min="0" :max="100" :use-grouping="false" size="small" fluid suffix=" %" />
      </FormField>
      <FormField label="Ročný koeficient (%)" :error="errors.final" hint="Prázdne = vypočíta Fiškus z plnení roka">
        <InputNumber v-model="form.final" :min="0" :max="100" :use-grouping="false" size="small" fluid suffix=" %" />
      </FormField>
      <FormField label="Poznámka" :span="2" hint="Napr. vylúčený predaj majetku alebo príležitostný nájom">
        <InputText v-model="form.note" size="small" />
      </FormField>
      <p class="muted span4">
        Koeficient = plnenia s nárokom na odpočet / všetky plnenia, zaokrúhlený na dve desatinné miesta nahor. Do výpočtu sa
        nezahŕňa predaj majetku (okrem zásob), príležitostné finančné služby a príležitostný nájom nehnuteľnosti — ak ich máte,
        zadajte ročný koeficient ručne. Zmena predbežného koeficientu prepočíta doklady v obdobiach, za ktoré ešte nie je
        zaznamenané podanie.
      </p>
      <div class="actions span4">
        <Button type="submit" label="Uložiť koeficient" icon="pi pi-check" size="small" :loading="busy" />
      </div>
    </form>
  </section>
</template>

<style scoped>
.coef {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 8px 12px 10px;
  margin-top: 12px;
}
.coef.quiet {
  background: transparent;
  border-style: dashed;
}
.head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.head h2 {
  font-size: var(--fs-md);
  margin: 0;
}
.spacer {
  flex: 1;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  margin: 4px 0 0;
}
.figures {
  display: flex;
  flex-wrap: wrap;
  gap: 0;
  margin: 6px 0 0;
  border-top: 1px solid var(--rule-soft);
}
.figures > div {
  padding: 6px 18px 4px 0;
  min-width: 150px;
}
.figures dt {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.figures dd {
  margin: 0;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.figures dd small {
  display: block;
  font-weight: 400;
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.missing {
  color: var(--tone-draft);
}
.minus {
  color: var(--tone-overdue);
}
.edit {
  margin-top: 10px;
}
.span4 {
  grid-column: 1 / -1;
}
.actions {
  display: flex;
  justify-content: flex-end;
}
</style>
