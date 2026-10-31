<script setup>
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { canSee } from '../lib/session.js'
import { visibleSettings } from '../lib/settingsNav.js'

// /nastavenia: presmeruje na prvú sekciu, ktorú používateľ smie vidieť.
const router = useRouter()
const first = visibleSettings(canSee)[0]?.items[0]
onMounted(() => first && router.replace(first.path))
</script>

<template>
  <p v-if="!first" class="muted-box empty">Na nastavenia nemáte oprávnenie.</p>
</template>

<style scoped>
.empty {
  margin: 48px auto;
  max-width: 440px;
  text-align: center;
}
</style>
