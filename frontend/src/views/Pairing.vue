<script setup>
// Spárovanie telefónu s počítačom (Fiškus desktop v domácej sieti).
import { ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import { api } from '../lib/api.js'
import { deviceName, pairCode } from '../lib/mobile.js'

const code = ref(pairCode(location.search))
const name = ref(deviceName(navigator.userAgent))
const busy = ref(false)
const error = ref('')

async function pair() {
  busy.value = true
  error.value = ''
  try {
    await api.pairMobile(code.value, name.value)
    location.replace('/m')
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="pair">
    <svg viewBox="0 0 32 32" width="48" height="48" aria-hidden="true">
      <rect width="32" height="32" rx="7" fill="#1f3a5c" />
      <path d="M10 8h13v3.4h-9v3.6h8v3.3h-8V24h-4z" fill="#f6f7f9" />
      <rect x="18" y="20.5" width="5" height="3.5" fill="#d9a441" />
    </svg>
    <h1>Spárovať telefón</h1>
    <p>Telefón získa trvalý prístup k Fiškusu na tomto počítači. Odobrať ho môžete kedykoľvek v Nastaveniach → Mobil v sieti.</p>
    <form @submit.prevent="pair">
      <label>
        <span>Kód z počítača</span>
        <InputText v-model="code" maxlength="6" autocapitalize="characters" spellcheck="false" class="code" />
      </label>
      <label>
        <span>Meno telefónu</span>
        <InputText v-model="name" maxlength="40" />
      </label>
      <Button type="submit" label="Spárovať" icon="pi pi-link" :loading="busy" :disabled="code.length !== 6" />
    </form>
    <p v-if="error" class="err" role="alert">{{ error }}</p>
  </main>
</template>

<style scoped>
.pair {
  max-width: 420px;
  margin: 0 auto;
  padding: 32px 16px;
}
h1 {
  margin: 14px 0 6px;
  font-size: var(--fs-xl);
  color: var(--blue-deep);
}
p {
  color: var(--ink-soft);
}
form {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 18px;
}
label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.code {
  font-size: 1.4rem;
  letter-spacing: 0.3em;
  text-align: center;
  font-variant-numeric: tabular-nums;
}
.err {
  margin-top: 12px;
  padding: 10px 12px;
  background: var(--tone-overdue-bg);
  color: var(--tone-overdue);
}
</style>
