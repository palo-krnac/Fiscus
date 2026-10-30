<script setup>
// Riadky zápisu na úpravu: účet, suma na strane MD alebo D. Vyrovnanie
// MD = D kontroluje server pri uložení.
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'

const lines = defineModel({ type: Array, required: true })
defineProps({
  accounts: { type: Array, required: true }, // [{ value, label }]
  disabled: { type: Boolean, default: false },
})

function add() {
  lines.value.push({ account: '', debit: '', credit: '' })
}
function remove(i) {
  lines.value.splice(i, 1)
}
</script>

<template>
  <div class="ledger-lines">
    <div class="ll-head"><span>Účet</span><span class="num">MD</span><span class="num">D</span><span /></div>
    <div v-for="(l, i) in lines" :key="i" class="ll-row">
      <Select v-model="l.account" :options="accounts" option-label="label" option-value="value" filter size="small" fluid
        placeholder="Účet" :disabled="disabled" :aria-label="`Účet riadku ${i + 1}`" />
      <InputText v-model="l.debit" size="small" inputmode="decimal" class="amt" :disabled="disabled || !!l.credit" :aria-label="`MD riadku ${i + 1}`" />
      <InputText v-model="l.credit" size="small" inputmode="decimal" class="amt" :disabled="disabled || !!l.debit" :aria-label="`D riadku ${i + 1}`" />
      <Button v-if="!disabled" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="`Odstrániť riadok ${i + 1}`"
        :disabled="lines.length <= 2" @click="remove(i)" />
    </div>
    <Button v-if="!disabled" label="Riadok" icon="pi pi-plus" text size="small" class="ll-add" @click="add" />
  </div>
</template>

<style scoped>
.ledger-lines {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ll-head,
.ll-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 96px 96px 30px;
  gap: 6px;
  align-items: center;
}
.ll-head {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.num {
  text-align: right;
}
.amt :deep(input),
.amt {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.ll-add {
  align-self: flex-start;
}
@media (max-width: 640px) {
  .ll-head,
  .ll-row {
    grid-template-columns: minmax(0, 1fr) 76px 76px 28px;
    gap: 4px;
  }
}
</style>
