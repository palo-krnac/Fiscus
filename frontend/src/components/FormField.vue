<script setup>
// Popis + pole + chybová hláška. Šírka sa riadi mriežkou rodiča (.form-grid).
defineProps({
  label: { type: String, required: true },
  error: { type: String, default: '' },
  span: { type: Number, default: 1 },
  hint: { type: String, default: '' },
})
</script>

<template>
  <label class="ff" :class="{ wide: span > 1 }" :style="{ '--span': span }">
    <span class="ff-label">{{ label }}</span>
    <slot />
    <small v-if="error" class="ff-error">{{ error }}</small>
    <small v-else-if="hint" class="ff-hint">{{ hint }}</small>
  </label>
</template>

<style scoped>
.ff {
  grid-column: span var(--span, 1);
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}
.ff-label {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.ff :deep(input),
.ff :deep(.p-inputnumber),
.ff :deep(.p-textarea) {
  width: 100%;
}
/* na mobile (mriežka s 2 stĺpcami) je široké pole cez celý riadok */
@media (max-width: 760px) {
  .ff.wide {
    grid-column: 1 / -1;
  }
}
.ff-error {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.ff-hint {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
@media (max-width: 760px) {
  .ff.wide {
    grid-column: 1 / -1;
  }
}
</style>
