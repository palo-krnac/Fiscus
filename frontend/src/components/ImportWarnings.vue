<script setup>
// Upozornenia prenosu dát: zoznam s hľadaním a exportom (Prenos dát, import adresára).
import { computed, ref } from 'vue'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import ExportButton from './ExportButton.vue'
import { importWarningColumns } from '../lib/columns.js'
import { filterWarnings } from '../lib/migrate.js'

const props = defineProps({
  warnings: { type: Array, default: () => [] },
  name: { type: String, default: 'Prenos dát – upozornenia' },
})
const search = ref('')
const rows = computed(() => filterWarnings(props.warnings, search.value))
</script>

<template>
  <section v-if="warnings.length" class="warnings">
    <div class="warn-head">
      <h3>Upozornenia <span class="count">{{ warnings.length }}</span></h3>
      <IconField class="search">
        <InputIcon class="pi pi-search" />
        <InputText v-model="search" placeholder="Hľadať v upozorneniach" size="small" aria-label="Hľadať v upozorneniach" />
      </IconField>
      <ExportButton :name="name" :columns="importWarningColumns" :rows="rows" />
    </div>
    <ol>
      <li v-for="w in rows" :key="w.n" :value="w.n">{{ w.text }}</li>
    </ol>
    <p v-if="!rows.length" class="empty">Nič sa nenašlo.</p>
  </section>
</template>

<style scoped>
.warnings {
  margin-top: 16px;
  border-top: 1px solid var(--rule);
  padding-top: 12px;
}
.warn-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.warn-head h3 {
  margin: 0;
  font-size: var(--fs-md);
  font-weight: 600;
  margin-right: auto;
}
.count {
  color: var(--ink-faint);
  font-weight: 400;
}
ol {
  margin: 10px 0 0;
  padding-left: 2.2em;
  font-size: var(--fs-sm);
  max-height: 320px;
  overflow: auto;
}
li {
  padding: 2px 0;
}
.empty {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
@media (max-width: 720px) {
  .warn-head .search {
    flex: 1 1 100%;
    order: 3;
  }
}
</style>
