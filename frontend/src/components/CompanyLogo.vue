<script setup>
// Logo aktuálnej firmy; ak firma logo nemá, nezobrazí nič.
import { computed, ref, watch } from 'vue'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'

const props = defineProps({ height: { type: String, default: '16mm' } })
const failed = ref(false)
const src = computed(() => (session.companyId ? api.logoURL(session.companyId, session.logoVersion) : ''))
watch(src, () => (failed.value = false))
defineExpose({ failed })
</script>

<template>
  <img v-if="src && !failed" :src="src" alt="Logo" class="company-logo" :style="{ maxHeight: props.height }" @error="failed = true" />
</template>

<style scoped>
.company-logo {
  display: block;
  max-width: 70mm;
  width: auto;
  object-fit: contain;
}
</style>
