<script setup>
// Pole IČ DPH s overením vo VIES (register platiteľov DPH v EÚ): platnosť,
// pri zahraničnom partnerovi aj názov a adresa.
import { ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import InputGroup from 'primevue/inputgroup'
import InputText from 'primevue/inputtext'
import Button from 'primevue/button'
import { api } from '../lib/api.js'

const props = defineProps({
  modelValue: { type: String, default: '' },
  disabled: Boolean,
  invalid: Boolean,
})
const emit = defineEmits(['update:modelValue', 'found'])
const toast = useToast()
const loading = ref(false)

async function check() {
  const id = (props.modelValue ?? '').replace(/\s/g, '')
  if (!/^[A-Za-z]{2}\w{2,}$/.test(id)) {
    toast.add({ severity: 'warn', summary: 'Zadajte IČ DPH s kódom krajiny', detail: 'napr. SK2020372640 alebo CZ12345678', life: 4000 })
    return
  }
  loading.value = true
  try {
    const c = await api.vies(id)
    if (c.valid) {
      emit('found', c)
      toast.add({ severity: 'success', summary: `IČ DPH ${c.vatId} je platné`, detail: [c.name, c.city].filter(Boolean).join(', ') || 'VIES nezverejňuje názov', life: 4000 })
    } else {
      toast.add({ severity: 'warn', summary: `IČ DPH ${id.toUpperCase()} nie je platné`, detail: 'Podľa VIES nie je registrované pre DPH — prenesenie daňovej povinnosti sa naň nevzťahuje.', life: 7000 })
    }
  } catch (e) {
    toast.add({ severity: 'error', summary: e.message, life: 6000 })
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <InputGroup class="vat-input">
    <InputText
      :model-value="modelValue"
      :disabled="disabled"
      :invalid="invalid"
      size="small"
      aria-label="IČ DPH"
      placeholder="SK…"
      @update:model-value="emit('update:modelValue', $event)"
      @keydown.enter.prevent="check"
    />
    <Button
      v-if="!disabled"
      icon="pi pi-verified"
      severity="secondary"
      outlined
      size="small"
      :loading="loading"
      aria-label="Overiť vo VIES"
      v-tooltip.bottom="'Overiť IČ DPH vo VIES (EÚ) — pri zahraničnom partnerovi doplní názov a adresu (Enter)'"
      @click="check"
    />
  </InputGroup>
</template>

<style scoped>
.vat-input :deep(input) {
  min-width: 0;
}
</style>
