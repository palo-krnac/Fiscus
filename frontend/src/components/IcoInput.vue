<script setup>
// Pole IČO s tlačidlom „Z registra“: doplní názov a adresu z RPO.
// auto = vyhľadať hneď, ako je zadané platné IČO (pri novom zázname bez názvu).
import { ref, watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import InputGroup from 'primevue/inputgroup'
import InputText from 'primevue/inputtext'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { normalizeICO, validICO } from '../lib/registry.js'

const props = defineProps({
  modelValue: { type: String, default: '' },
  disabled: Boolean,
  invalid: Boolean,
  auto: Boolean,
})
const emit = defineEmits(['update:modelValue', 'found'])
const toast = useToast()
const loading = ref(false)
let lastAuto = ''

async function lookup() {
  const ico = normalizeICO(props.modelValue)
  if (!validICO(ico)) {
    toast.add({ severity: 'warn', summary: 'Zadajte platné IČO (8 číslic)', life: 3000 })
    return
  }
  loading.value = true
  try {
    const c = await api.registry(ico)
    emit('found', c)
    toast.add({
      severity: c.terminated ? 'warn' : 'success',
      summary: c.terminated ? `${c.name} zanikla ${c.terminated}` : `Doplnené z registra: ${c.name}`,
      life: c.terminated ? 6000 : 2500,
    })
  } catch (e) {
    toast.add({ severity: e.status === 404 ? 'warn' : 'error', summary: e.message, life: 5000 })
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (v) => {
    const ico = normalizeICO(v)
    if (props.auto && ico !== lastAuto && validICO(ico)) {
      lastAuto = ico
      lookup()
    }
  },
)
</script>

<template>
  <InputGroup class="ico-input">
    <InputText
      :model-value="modelValue"
      :disabled="disabled"
      :invalid="invalid"
      inputmode="numeric"
      size="small"
      aria-label="IČO"
      @update:model-value="emit('update:modelValue', $event)"
      @keydown.enter.prevent="lookup"
    />
    <Button
      v-if="!disabled"
      icon="pi pi-search"
      severity="secondary"
      outlined
      size="small"
      :loading="loading"
      aria-label="Doplniť z registra"
      v-tooltip.bottom="'Doplniť názov a adresu z registra (Enter)'"
      @click="lookup"
    />
  </InputGroup>
</template>

<style scoped>
.ico-input :deep(input) {
  min-width: 0;
}
</style>
