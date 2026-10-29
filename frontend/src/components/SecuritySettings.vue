<script setup>
// Zabezpečenie inštalácie: PIN desktopu alebo povinné 2FA na serveri.
import { onMounted, ref } from 'vue'
import { useToast } from 'primevue/usetoast'
import Button from 'primevue/button'
import Password from 'primevue/password'
import ToggleSwitch from 'primevue/toggleswitch'
import FormField from './FormField.vue'
import { api, ApiError } from '../lib/api.js'
import { session, isSuperAdmin, loadSession } from '../lib/session.js'

const toast = useToast()
const st = ref(null)
const form = ref({ current: '', pin: '', again: '' })
const errors = ref({})
const busy = ref(false)

onMounted(async () => {
  if (!isSuperAdmin()) return
  try {
    st.value = await api.security()
  } catch {
    st.value = null
  }
})

async function savePIN(remove = false) {
  errors.value = {}
  if (!remove && form.value.pin !== form.value.again) {
    errors.value = { again: 'PIN-y sa nezhodujú' }
    return
  }
  busy.value = true
  try {
    await api.setPIN(form.value.current, remove ? '' : form.value.pin)
    const had = st.value.pin
    st.value.pin = !remove
    form.value = { current: '', pin: '', again: '' }
    toast.add({ severity: 'success', summary: remove ? 'PIN zrušený' : had ? 'PIN zmenený' : 'PIN nastavený', life: 3000 })
    await loadSession()
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'PIN sa neuložil', detail: e.message, life: 5000 })
  } finally {
    busy.value = false
  }
}

async function setRequire(on) {
  busy.value = true
  errors.value = {}
  try {
    await api.setRequireMFA(on)
    st.value.requireMfa = on
    toast.add({ severity: 'success', summary: on ? 'Dvojfázové overenie je povinné' : 'Dvojfázové overenie je dobrovoľné', life: 3000 })
  } catch (e) {
    errors.value = e instanceof ApiError ? e.fieldMap() : {}
    toast.add({ severity: 'error', summary: 'Nastavenie sa neuložilo', detail: e.fields?.[0]?.message ?? e.message, life: 6000 })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <fieldset v-if="st" class="form-section sec">
    <legend>Zabezpečenie</legend>

    <template v-if="session.caps.mode === 'desktop'">
      <p class="help">
        PIN zamkne Fiškus v prehliadači, aby doň nenazrel nikto, kto si sadne k vášmu počítaču. Pýta sa po spustení programu
        a po 30 minútach nečinnosti. AI asistent s API kľúčom funguje aj pri zamknutom okne.
      </p>
      <p class="state" :class="{ on: st.pin }"><i class="pi" :class="st.pin ? 'pi-lock' : 'pi-lock-open'" /> {{ st.pin ? 'PIN je nastavený' : 'Bez PIN-u' }}</p>
      <form class="form-grid" @submit.prevent="savePIN(false)">
        <FormField v-if="st.pin" label="Súčasný PIN" :error="errors.current">
          <Password v-model="form.current" :feedback="false" size="small" fluid autocomplete="current-password" :invalid="!!errors.current" />
        </FormField>
        <FormField :label="st.pin ? 'Nový PIN' : 'PIN'" :error="errors.pin" hint="Aspoň 4 znaky">
          <Password v-model="form.pin" :feedback="false" size="small" fluid autocomplete="new-password" :invalid="!!errors.pin" />
        </FormField>
        <FormField label="PIN znova" :error="errors.again">
          <Password v-model="form.again" :feedback="false" size="small" fluid autocomplete="new-password" :invalid="!!errors.again" />
        </FormField>
        <div class="actions">
          <Button type="submit" :label="st.pin ? 'Zmeniť PIN' : 'Nastaviť PIN'" icon="pi pi-lock" severity="secondary" outlined size="small" :loading="busy" :disabled="!form.pin" />
          <Button v-if="st.pin" label="Zrušiť PIN" severity="danger" text size="small" :disabled="!form.current || busy" @click="savePIN(true)" />
        </div>
      </form>
    </template>

    <template v-else>
      <p class="help">
        Keď je dvojfázové overenie povinné, každý používateľ si ho musí pri najbližšom prihlásení zapnúť (aplikácia v telefóne),
        inak sa k dokladom nedostane. Vy ho musíte mať zapnuté skôr, než ho zapnete pre ostatných.
      </p>
      <label class="switch">
        <ToggleSwitch :model-value="st.requireMfa" :disabled="busy" input-id="require-mfa" @update:model-value="setRequire" />
        <span>
          <strong>Povinné dvojfázové overenie pre všetkých</strong>
          <small v-if="errors.requireMfa" class="err">{{ errors.requireMfa }}</small>
        </span>
      </label>
    </template>
  </fieldset>
</template>

<style scoped>
.sec {
  display: grid;
  gap: 10px;
}
.help {
  margin: 0;
  max-width: 70ch;
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.state {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  color: var(--ink-soft);
}
.state.on {
  color: var(--tone-paid);
}
.actions {
  display: flex;
  align-items: flex-end;
  gap: 8px;
}
.switch {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
}
.switch span {
  display: flex;
  flex-direction: column;
}
.err {
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
</style>
