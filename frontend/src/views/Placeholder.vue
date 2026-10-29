<script setup>
defineProps({ title: String, stage: Number })
</script>

<template>
  <div class="placeholder">
    <h1>{{ title }}</h1>
    <p>Tento modul pribudne v etape {{ stage }}.</p>
    <RouterLink to="/faktury">Späť na vydané faktúry</RouterLink>
  </div>
</template>

<style scoped>
.placeholder {
  max-width: 520px;
  margin: 64px auto;
  padding: 0 16px;
}
h1 {
  font-size: var(--fs-xl);
  margin-bottom: 8px;
}
p {
  color: var(--ink-soft);
}
</style>
