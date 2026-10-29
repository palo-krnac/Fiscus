<script setup>
// Tlačidlo Export pre každý zoznam: Excel (.xlsx) alebo CSV.
// rows môže byť pole alebo async funkcia (napr. audit načíta všetky strany).
import { ref } from 'vue'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import { useToast } from 'primevue/usetoast'
import { downloadCSV, downloadXLSX } from '../lib/export.js'

const props = defineProps({
  name: { type: String, required: true }, // základ názvu súboru a hárku
  columns: { type: Array, required: true },
  rows: { type: [Array, Function], required: true },
})
const toast = useToast()
const menu = ref()
const busy = ref(false)

async function run(kind) {
  busy.value = true
  try {
    const rows = typeof props.rows === 'function' ? await props.rows() : props.rows
    if (!rows.length) {
      toast.add({ severity: 'warn', summary: 'Nie je čo exportovať', detail: 'Zoznam je po filtri prázdny.', life: 3000 })
      return
    }
    if (kind === 'xlsx') await downloadXLSX(props.name, props.columns, rows, props.name)
    else downloadCSV(props.name, props.columns, rows)
    toast.add({ severity: 'success', summary: `Exportovaných ${rows.length} riadkov`, life: 2500 })
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Export zlyhal', detail: e.message, life: 5000 })
  } finally {
    busy.value = false
  }
}

const items = [
  { label: 'Excel (.xlsx)', icon: 'pi pi-file-excel', command: () => run('xlsx') },
  { label: 'CSV (bodkočiarka)', icon: 'pi pi-file', command: () => run('csv') },
]
</script>

<template>
  <Button
    icon="pi pi-download"
    label="Export"
    severity="secondary"
    outlined
    size="small"
    class="export-btn"
    :loading="busy"
    aria-haspopup="true"
    v-tooltip.bottom="'Exportuje zoznam po filtri a hľadaní'"
    @click="menu.toggle($event)"
  />
  <Menu ref="menu" :model="items" popup />
</template>

<style scoped>
@media (max-width: 640px) {
  .export-btn :deep(.p-button-label) {
    display: none;
  }
}
</style>
