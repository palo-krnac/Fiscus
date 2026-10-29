<script setup>
// Prihlásený používateľ v hornej lište: rola vo firme, zmena hesla, odhlásenie.
import { computed, ref } from 'vue'
import Menu from 'primevue/menu'
import Dialog from 'primevue/dialog'
import Password from 'primevue/password'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { codes } from '../lib/codes.js'
import { session, currentRole, logout } from '../lib/session.js'

const toast = useToast()
const menu = ref()
const pwOpen = ref(false)
const pw = ref({ current: '', next: '', again: '' })
const pwError = ref('')
const busy = ref(false)

const name = computed(() => session.me?.user?.name ?? '')
const roleLabel = computed(() => {
  if (session.me?.user?.superAdmin && !currentRole()) return 'Správca inštalácie'
  const r = currentRole()
  return codes.roles?.find((x) => x.code === r)?.label ?? ''
})
const initials = computed(() =>
  name.value
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0].toUpperCase())
    .join(''),
)

const items = [
  { label: 'Zmeniť heslo', icon: 'pi pi-key', command: () => openPassword() },
  { separator: true },
  { label: 'Odhlásiť sa', icon: 'pi pi-sign-out', command: () => logout() },
]

function openPassword() {
  pw.value = { current: '', next: '', again: '' }
  pwError.value = ''
  pwOpen.value = true
}

async function savePassword() {
  pwError.value = ''
  if (pw.value.next !== pw.value.again) {
    pwError.value = 'Nové heslá sa nezhodujú'
    return
  }
  busy.value = true
  try {
    await api.changePassword(pw.value.current, pw.value.next)
    pwOpen.value = false
    toast.add({ severity: 'success', summary: 'Heslo zmenené', detail: 'Prihláste sa novým heslom.', life: 4000 })
    await logout()
  } catch (e) {
    pwError.value = e.fields?.[0]?.message ?? e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="session.caps.mode === 'server' && session.me" class="user">
    <button class="who" aria-haspopup="true" aria-controls="user-menu" @click="menu.toggle($event)">
      <span class="avatar" aria-hidden="true">{{ initials }}</span>
      <span class="text">
        <span class="name">{{ name }}</span>
        <span class="role">{{ roleLabel }}</span>
      </span>
      <i class="pi pi-angle-down" aria-hidden="true" />
    </button>
    <Menu id="user-menu" ref="menu" :model="items" popup />

    <Dialog v-model:visible="pwOpen" header="Zmena hesla" modal :style="{ width: '22rem' }" :breakpoints="{ '480px': '94vw' }">
      <form class="pw" @submit.prevent="savePassword">
        <label><span>Súčasné heslo</span><Password v-model="pw.current" :feedback="false" fluid autocomplete="current-password" /></label>
        <label><span>Nové heslo (aspoň 10 znakov)</span><Password v-model="pw.next" :feedback="false" fluid autocomplete="new-password" /></label>
        <label><span>Nové heslo znova</span><Password v-model="pw.again" :feedback="false" fluid autocomplete="new-password" /></label>
        <p v-if="pwError" class="err">{{ pwError }}</p>
        <div class="actions">
          <Button label="Zrušiť" severity="secondary" text size="small" @click="pwOpen = false" />
          <Button type="submit" label="Zmeniť heslo" size="small" :loading="busy" />
        </div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.user {
  margin-left: auto;
}
.who {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 0;
  background: none;
  padding: 4px 6px;
  border-radius: var(--radius);
  cursor: pointer;
  color: var(--ink);
  font: inherit;
}
.who:hover {
  background: var(--rule-soft);
}
.avatar {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: var(--blue-deep);
  color: #fff;
  font-size: var(--fs-xs);
  font-weight: 700;
  display: grid;
  place-items: center;
}
.text {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  line-height: 1.15;
}
.name {
  font-size: var(--fs-sm);
  font-weight: 600;
}
.role {
  font-size: var(--fs-xs);
  color: var(--ink-faint);
}
.pi-angle-down {
  font-size: 0.75rem;
  color: var(--ink-faint);
}
.pw {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.pw label {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.pw label > span {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.err {
  margin: 0;
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
@media (max-width: 860px) {
  .text {
    display: none;
  }
}
</style>
