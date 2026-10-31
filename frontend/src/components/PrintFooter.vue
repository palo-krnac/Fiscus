<script setup>
// Päta tlače: dole na stred malé logo a „Vytvorené v programe Fiškus“
// (rovnako ako PDF zo servera, internal/printout). Pri tlači sa opakuje
// na každej strane.
</script>

<template>
  <footer class="print-footer" aria-label="Vytvorené v programe Fiškus">
    <svg viewBox="0 0 32 32" aria-hidden="true">
      <rect width="32" height="32" rx="7" fill="#1f3a5c" />
      <path d="M10 8h13v3.4h-9v3.6h8v3.3h-8V24h-4z" fill="#f6f7f9" />
      <rect x="18" y="20.5" width="5" height="3.5" fill="#d9a441" />
    </svg>
    <span>Vytvorené v programe Fiškus</span>
  </footer>
</template>

<style scoped>
.print-footer {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 7mm;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1.5mm;
  font-size: 7pt;
  color: #6b7685;
}
svg {
  width: 3.2mm;
  height: 3.2mm;
}
@media print {
  .print-footer {
    position: fixed;
  }
}
</style>
