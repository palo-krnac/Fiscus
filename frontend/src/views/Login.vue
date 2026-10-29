<script setup>
// Prihlásenie do serverovej verzie.
import { nextTick, ref } from 'vue'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import { api } from '../lib/api.js'
import { loadSession, session } from '../lib/session.js'

const email = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)
// druhý krok: kód z aplikácie alebo záložný kód
const ticket = ref('')
const code = ref('')
const useRecovery = ref(false)
const codeEl = ref(null)
// desktop chránený PIN-om (bez e-mailu a hesla)
const pinMode = session.caps.mode === 'desktop'
const pin = ref('')

async function submit() {
  error.value = ''
  if (pinMode) return submitPIN()
  if (ticket.value) return submitCode()
  if (!email.value.trim() || !password.value) {
    error.value = 'Zadajte e-mail a heslo'
    return
  }
  busy.value = true
  try {
    const res = await api.login(email.value.trim(), password.value)
    password.value = ''
    if (res?.mfa) {
      ticket.value = res.ticket
      await nextTick()
      codeEl.value?.$el?.focus()
      return
    }
    await loadSession()
  } catch (e) {
    error.value = e.message
  } finally {
    busy.value = false
  }
}

async function submitPIN() {
  if (!pin.value) {
    error.value = 'Zadajte PIN'
    return
  }
  busy.value = true
  try {
    await api.unlockPIN(pin.value)
    pin.value = ''
    await loadSession()
  } catch (e) {
    error.value = e.message
    pin.value = ''
  } finally {
    busy.value = false
  }
}

async function submitCode() {
  if (!code.value.trim()) {
    error.value = useRecovery.value ? 'Zadajte záložný kód' : 'Zadajte 6-miestny kód z aplikácie'
    return
  }
  busy.value = true
  try {
    await api.loginMFA(ticket.value, code.value.trim())
    await loadSession()
  } catch (e) {
    error.value = e.message
    code.value = ''
    if (e.status === 401 && /vypršalo/.test(e.message)) back()
  } finally {
    busy.value = false
  }
}

function back() {
  ticket.value = ''
  code.value = ''
  useRecovery.value = false
}

async function toggleRecovery() {
  useRecovery.value = !useRecovery.value
  code.value = ''
  error.value = ''
  await nextTick()
  codeEl.value?.$el?.focus()
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
          <p>{{ pinMode ? 'Zamknuté' : ticket ? 'Dvojfázové overenie' : 'Prihlásenie' }}</p>
        </div>
      </div>

      <label v-if="pinMode" class="field">
        <span>PIN</span>
        <Password v-model="pin" :feedback="false" toggle-mask autocomplete="current-password" fluid autofocus input-id="pin" input-class="code" />
      </label>
      <template v-else-if="!ticket">
        <label class="field">
          <span>E-mail</span>
          <InputText v-model="email" type="email" autocomplete="username" autofocus fluid />
        </label>
        <label class="field">
          <span>Heslo</span>
          <Password v-model="password" :feedback="false" toggle-mask autocomplete="current-password" fluid input-id="login-password" />
        </label>
      </template>
      <template v-else>
        <p class="hint">
          {{ useRecovery
            ? 'Zadajte jeden zo záložných kódov. Každý platí iba raz.'
            : 'Otvorte v telefóne aplikáciu na overovanie a zadajte kód pre Fiškus.' }}
        </p>
        <label class="field">
          <span>{{ useRecovery ? 'Záložný kód' : 'Kód z aplikácie' }}</span>
          <InputText
            ref="codeEl"
            v-model="code"
            :inputmode="useRecovery ? 'text' : 'numeric'"
            autocomplete="one-time-code"
            :maxlength="useRecovery ? 14 : 7"
            :placeholder="useRecovery ? 'abcde-fghjk' : '123 456'"
            class="code"
            fluid
          />
        </label>
      </template>

      <p v-if="error" class="error" role="alert"><i class="pi pi-exclamation-circle" /> {{ error }}</p>

      <Button
        type="submit"
        :label="pinMode ? 'Odomknúť' : ticket ? 'Overiť' : 'Prihlásiť sa'"
        :icon="pinMode ? 'pi pi-lock-open' : ticket ? 'pi pi-check' : 'pi pi-sign-in'"
        :loading="busy"
        fluid
      />
      <div v-if="ticket" class="alt">
        <button type="button" class="link" @click="toggleRecovery">
          {{ useRecovery ? 'Použiť kód z aplikácie' : 'Nemám telefón — použiť záložný kód' }}
        </button>
        <button type="button" class="link" @click="back">Späť</button>
      </div>
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
.hint {
  margin: 0;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.code {
  font-size: var(--fs-lg);
  letter-spacing: 0.08em;
  font-variant-numeric: tabular-nums;
}
.alt {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}
.link {
  border: 0;
  background: none;
  padding: 0;
  font: inherit;
  font-size: var(--fs-sm);
  color: var(--blue);
  cursor: pointer;
  text-align: left;
}
.link:hover {
  text-decoration: underline;
}
.error {
  margin: 0;
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
</style>
