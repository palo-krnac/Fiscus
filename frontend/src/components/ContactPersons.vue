<script setup>
// Kontaktné osoby firmy v adresári: meno, funkcia, e-mail, telefón;
// hviezdička = predvolená osoba na doklady.
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import { blankPerson, setPrimary } from '../lib/contacts.js'

const list = defineModel({ type: Array, default: () => [] })
defineProps({ errors: { type: Object, default: () => ({}) }, readonly: Boolean })

const add = () => (list.value = [...list.value, blankPerson()])
const remove = (i) => (list.value = list.value.filter((_, j) => j !== i))
const star = (i) => (list.value = setPrimary(list.value, i))
</script>

<template>
  <section class="sub">
    <header>
      <h3>Kontaktné osoby <span v-if="list.length" class="count">{{ list.length }}</span></h3>
      <Button v-if="!readonly" label="Osoba" icon="pi pi-plus" text size="small" @click="add" />
    </header>
    <p v-if="!list.length" class="empty">Konateľ, účtovníčka, nákup… Osobu označenú hviezdičkou Fiškus predvyplní na faktúru.</p>
    <div v-for="(p, i) in list" :key="i" class="row">
      <InputText v-model="p.name" size="small" placeholder="Meno a priezvisko" aria-label="Meno" :invalid="!!errors[`persons[${i}].name`]" :readonly="readonly" class="c-name" />
      <InputText v-model="p.role" size="small" placeholder="Funkcia" aria-label="Funkcia" :readonly="readonly" class="c-role" />
      <InputText v-model="p.email" size="small" placeholder="E-mail" aria-label="E-mail" autocomplete="off" :invalid="!!errors[`persons[${i}].email`]" :readonly="readonly" class="c-email" />
      <InputText v-model="p.phone" size="small" placeholder="Telefón" aria-label="Telefón" inputmode="tel" :readonly="readonly" class="c-phone" />
      <div class="c-act">
        <Button :icon="p.primary ? 'pi pi-star-fill' : 'pi pi-star'" text rounded size="small" :class="{ on: p.primary }"
          :aria-label="p.primary ? 'Predvolená osoba' : 'Nastaviť ako predvolenú'" v-tooltip.bottom="'Predvolená osoba na doklady'" :disabled="readonly" @click="star(i)" />
        <Button v-if="!readonly" icon="pi pi-times" text rounded size="small" severity="secondary" aria-label="Odstrániť osobu" @click="remove(i)" />
      </div>
      <small v-if="errors[`persons[${i}].name`] || errors[`persons[${i}].email`]" class="err">
        {{ errors[`persons[${i}].name`] || errors[`persons[${i}].email`] }}
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
  grid-template-columns: 1.4fr 1fr 1.4fr 1fr auto;
  gap: 6px;
  align-items: center;
  padding: 4px 0;
  border-bottom: 1px solid var(--rule-soft);
}
.row :deep(input) {
  min-width: 0;
}
.c-act {
  display: flex;
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
  .c-name {
    grid-column: 1 / 3;
  }
  .c-act {
    grid-column: 3;
    grid-row: 1;
  }
}
</style>
