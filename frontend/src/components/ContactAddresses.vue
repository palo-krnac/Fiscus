<script setup>
// Ďalšie adresy firmy: dodacia, korešpondenčná, prevádzka, iná.
// Hviezdička = predvolená adresa svojho druhu (dodacia ide na faktúru).
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import { addressKinds, blankAddress, setPrimary } from '../lib/contacts.js'
import { countries } from '../lib/codes.js'

const list = defineModel({ type: Array, default: () => [] })
defineProps({ errors: { type: Object, default: () => ({}) }, readonly: Boolean })

const add = () => (list.value = [...list.value, blankAddress(list.value.some((a) => a.kind === 'delivery') ? 'other' : 'delivery')])
const remove = (i) => (list.value = list.value.filter((_, j) => j !== i))
const star = (i) => (list.value = setPrimary(list.value, i, 'default'))
</script>

<template>
  <section class="sub">
    <header>
      <h3>Ďalšie adresy <span v-if="list.length" class="count">{{ list.length }}</span></h3>
      <Button v-if="!readonly" label="Adresa" icon="pi pi-plus" text size="small" @click="add" />
    </header>
    <p v-if="!list.length" class="empty">Hlavná adresa hore je fakturačná. Tu pridáte dodaciu, korešpondenčnú adresu alebo prevádzku.</p>
    <div v-for="(a, i) in list" :key="i" class="row">
      <Select v-model="a.kind" :options="addressKinds" option-label="label" option-value="value" size="small" aria-label="Druh adresy" :disabled="readonly" class="c-kind" />
      <InputText v-model="a.name" size="small" placeholder="Príjemca / názov" aria-label="Príjemca alebo názov" :readonly="readonly" class="c-name" />
      <div class="c-act">
        <Button :icon="a.default ? 'pi pi-star-fill' : 'pi pi-star'" text rounded size="small" :class="{ on: a.default }"
          :aria-label="a.default ? 'Predvolená adresa' : 'Nastaviť ako predvolenú'" v-tooltip.bottom="'Predvolená adresa tohto druhu (dodacia sa predvyplní na faktúru)'"
          :disabled="readonly" @click="star(i)" />
        <Button v-if="!readonly" icon="pi pi-times" text rounded size="small" severity="secondary" aria-label="Odstrániť adresu" @click="remove(i)" />
      </div>
      <InputText v-model="a.street" size="small" placeholder="Ulica a číslo" aria-label="Ulica" :invalid="!!errors[`addresses[${i}].street`]" :readonly="readonly" class="c-street" />
      <InputText v-model="a.zip" size="small" placeholder="PSČ" aria-label="PSČ" :readonly="readonly" class="c-zip" />
      <InputText v-model="a.city" size="small" placeholder="Obec" aria-label="Obec" :invalid="!!errors[`addresses[${i}].street`]" :readonly="readonly" class="c-city" />
      <Select v-model="a.country" :options="countries" option-label="label" option-value="code" filter size="small" aria-label="Krajina" :disabled="readonly" class="c-country" />
      <small v-if="errors[`addresses[${i}].street`] || errors[`addresses[${i}].kind`]" class="err">
        {{ errors[`addresses[${i}].street`] || errors[`addresses[${i}].kind`] }}
      </small>
    </div>
  </section>
</template>

<style scoped>
.sub {
  border-top: 1px solid var(--rule);
  padding-top: 8px;
  margin-top: 4px;
}
header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
h3 {
  margin: 0;
  font-size: var(--fs-md);
  font-weight: 600;
  color: var(--blue-deep);
}
.count {
  color: var(--ink-faint);
  font-weight: 400;
}
.empty {
  margin: 2px 0 6px;
  font-size: var(--fs-sm);
  color: var(--ink-faint);
}
.row {
  display: grid;
  grid-template-columns: 9rem 1fr 6rem 1fr auto;
  gap: 6px;
  align-items: center;
  padding: 6px 0;
  border-bottom: 1px solid var(--rule-soft);
}
.row :deep(input) {
  min-width: 0;
}
.c-kind {
  grid-column: 1;
}
.c-name {
  grid-column: 2 / 5;
}
.c-act {
  grid-column: 5;
  display: flex;
}
.c-street {
  grid-column: 1 / 3;
}
.c-zip {
  grid-column: 3;
}
.c-city {
  grid-column: 4;
}
.c-country {
  grid-column: 5;
  width: 7rem;
}
.on :deep(.pi) {
  color: var(--brass);
}
.err {
  grid-column: 1 / -1;
  color: var(--tone-overdue);
  font-size: var(--fs-xs);
}
@container (max-width: 560px) {
  .row {
    grid-template-columns: 1fr 1fr auto;
  }
  .c-kind {
    grid-column: 1;
  }
  .c-name {
    grid-column: 2;
  }
  .c-act {
    grid-column: 3;
  }
  .c-street {
    grid-column: 1 / -1;
  }
  .c-zip {
    grid-column: 1;
  }
  .c-city {
    grid-column: 2 / -1;
  }
  .c-country {
    grid-column: 1 / -1;
    width: auto;
  }
}
</style>
