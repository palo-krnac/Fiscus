<script setup>
// Prihlásenie do serverovej verzie.
import { ref } from 'vue'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { loadSession } from '../lib/session.js'

const email = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  if (!email.value.trim() || !password.value) {
    error.value = 'Zadajte e-mail a heslo'
    return
  }
  busy.value = true
  try {
    await api.login(email.value.trim(), password.value)
    password.value = ''
    await loadSession()
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <form class="login" novalidate @submit.prevent="submit">
      <div class="brand">
        <svg viewBox="0 0 32 32" width="34" height="34" aria-hidden="true">
          <rect width="32" height="32" rx="7" fill="#1f3a5c" />
          <path d="M10 8h13v3.4h-9v3.6h8v3.3h-8V24h-4z" fill="#f6f7f9" />
          <rect x="18" y="20.5" width="5" height="3.5" fill="#d9a441" />
        </svg>
        <div>
          <h1>Fiškus</h1>
          <p>Prihlásenie</p>
        </div>
      </div>

      <label class="field">
        <span>E-mail</span>
        <InputText v-model="email" type="email" autocomplete="username" autofocus fluid />
      </label>
      <label class="field">
        <span>Heslo</span>
        <Password v-model="password" :feedback="false" toggle-mask autocomplete="current-password" fluid input-id="login-password" />
      </label>

      <p v-if="error" class="error" role="alert"><i class="pi pi-exclamation-circle" /> {{ error }}</p>

      <Button type="submit" label="Prihlásiť sa" icon="pi pi-sign-in" :loading="busy" fluid />
    </form>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100%;
  display: grid;
  place-items: center;
  padding: 24px 16px;
  background: var(--paper);
}
.login {
  width: 100%;
  max-width: 360px;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 3px solid var(--blue-deep);
  border-radius: var(--radius);
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 6px;
}
.brand h1 {
  font-size: var(--fs-xl);
  margin: 0;
  color: var(--blue-deep);
  line-height: 1.1;
}
.brand p {
  margin: 0;
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.field {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.field > span {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.error {
  margin: 0;
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
</style>
