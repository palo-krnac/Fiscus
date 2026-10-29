<script setup>
// Používatelia firmy a ich roly (iba serverová verzia, právo users.manage).
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Select from 'primevue/select'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Dialog from 'primevue/dialog'
import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { api } from '../lib/api.js'
import { codes } from '../lib/codes.js'
import { session } from '../lib/session.js'
import FormField from '../components/FormField.vue'

const toast = useToast()
const confirm = useConfirm()
const members = ref([])
const loading = ref(true)
const adding = ref(false)
const form = ref(null)
const errors = ref({})
const busy = ref(false)

const cid = computed(() => session.companyId)
const meId = computed(() => session.me?.user?.id)
const server = computed(() => session.caps.mode === 'server')
const roleInfo = (code) => codes.roles?.find((r) => r.code === code)

async function load() {
  loading.value = true
  try {
    members.value = await api.members(cid.value)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Používatelia', detail: e.message, life: 5000 })
  } finally {
    loading.value = false
  }
}
onMounted(load)

function openAdd() {
  form.value = { email: '', name: '', password: '', role: 'accountant' }
  errors.value = {}
  adding.value = true
}

async function add() {
  busy.value = true
  errors.value = {}
  try {
    await api.addMember(cid.value, form.value)
    adding.value = false
    toast.add({ severity: 'success', summary: 'Prístup pridaný', detail: form.value.email, life: 3000 })
    await load()
  } catch (e) {
    errors.value = e.fieldMap?.() ?? {}
    if (!Object.keys(errors.value).length) toast.add({ severity: 'error', summary: 'Chyba', detail: e.message, life: 5000 })
  } finally {
    busy.value = false
  }
}

async function changeRole(m, role) {
  const old = m.role
  m.role = role
  try {
    await api.setRole(cid.value, m.user.id, role)
    toast.add({ severity: 'success', summary: 'Rola zmenená', detail: `${m.user.name}: ${roleInfo(role)?.label}`, life: 3000 })
  } catch (e) {
    m.role = old
    toast.add({ severity: 'error', summary: 'Rolu nemožno zmeniť', detail: e.message, life: 5000 })
  }
}

function remove(m) {
  confirm.require({
    header: 'Odobrať prístup',
    message: `Odobrať používateľovi ${m.user.name} prístup do firmy ${session.company?.name}? Účet zostane, iba stratí prístup k tejto firme.`,
    acceptLabel: 'Odobrať',
    rejectLabel: 'Zrušiť',
    acceptProps: { severity: 'danger', size: 'small' },
    rejectProps: { severity: 'secondary', text: true, size: 'small' },
    accept: async () => {
      try {
        await api.removeMember(cid.value, m.user.id)
        await load()
      } catch (e) {
        toast.add({ severity: 'error', summary: 'Prístup nemožno odobrať', detail: e.message, life: 5000 })
      }
    },
  })
}
</script>

<template>
  <div class="page">
    <header class="head">
      <h1>Používatelia</h1>
      <span class="count">{{ members.length }}</span>
      <div class="spacer" />
      <Button v-if="server" size="small" icon="pi pi-user-plus" label="Pridať používateľa" @click="openAdd" />
    </header>

    <p v-if="!server" class="note">
      <i class="pi pi-info-circle" />
      Desktopová verzia má jedného lokálneho používateľa so všetkými právami. Používateľov a roly pre účtovnú kanceláriu
      spravuje serverová verzia.
    </p>

    <table class="users">
      <thead>
        <tr>
          <th>Meno</th>
          <th>E-mail</th>
          <th class="role-col">Rola vo firme</th>
          <th class="act" aria-label="Akcie" />
        </tr>
      </thead>
      <tbody>
        <tr v-if="loading"><td colspan="4" class="empty">Načítavam…</td></tr>
        <tr v-else-if="!members.length"><td colspan="4" class="empty">Vo firme zatiaľ nie je nikto ďalší.</td></tr>
        <tr v-for="m in members" :key="m.user.id">
          <td class="name" data-label="Meno">
            {{ m.user.name }}
            <span v-if="m.user.id === meId" class="badge">vy</span>
            <span v-if="m.user.superAdmin" class="badge">správca inštalácie</span>
            <span v-if="!m.user.active" class="badge off">zablokovaný</span>
          </td>
          <td class="email" data-label="E-mail">{{ m.user.email }}</td>
          <td class="role-col" data-label="Rola">
            <Select
              :model-value="m.role"
              :options="codes.roles"
              option-label="label"
              option-value="code"
              size="small"
              :disabled="m.user.id === meId"
              :aria-label="`Rola – ${m.user.name}`"
              @update:model-value="(r) => changeRole(m, r)"
            >
              <template #option="{ option }">
                <div class="opt">
                  <strong>{{ option.label }}</strong>
                  <small>{{ option.info }}</small>
                </div>
              </template>
            </Select>
          </td>
          <td class="act">
            <Button
              v-if="m.user.id !== meId"
              icon="pi pi-times"
              severity="danger"
              text
              rounded
              size="small"
              :aria-label="`Odobrať prístup – ${m.user.name}`"
              v-tooltip.left="'Odobrať prístup'"
              @click="remove(m)"
            />
          </td>
        </tr>
      </tbody>
    </table>

    <section class="legend" aria-label="Roly">
      <h2>Roly</h2>
      <dl>
        <template v-for="r in codes.roles" :key="r.code">
          <dt>{{ r.label }}</dt>
          <dd>{{ r.info }}</dd>
        </template>
      </dl>
    </section>

    <Dialog v-model:visible="adding" header="Pridať používateľa" modal :style="{ width: '30rem' }" :breakpoints="{ '520px': '94vw' }">
      <form v-if="form" class="form-grid add" @submit.prevent="add">
        <FormField label="E-mail" :error="errors.email" :span="2" hint="Ak účet už existuje, iba sa mu pridá prístup do firmy">
          <InputText v-model="form.email" type="email" autocomplete="off" fluid />
        </FormField>
        <FormField label="Meno" :error="errors.name" :span="2">
          <InputText v-model="form.name" autocomplete="off" fluid />
        </FormField>
        <FormField label="Počiatočné heslo" :error="errors.password" :span="2" hint="Aspoň 10 znakov; používateľ si ho zmení">
          <Password v-model="form.password" :feedback="false" toggle-mask autocomplete="new-password" fluid />
        </FormField>
        <FormField label="Rola" :error="errors.role" :span="2" :hint="roleInfo(form.role)?.info">
          <Select v-model="form.role" :options="codes.roles" option-label="label" option-value="code" fluid />
        </FormField>
        <div class="actions">
          <Button label="Zrušiť" severity="secondary" text size="small" @click="adding = false" />
          <Button type="submit" label="Pridať" icon="pi pi-check" size="small" :loading="busy" />
        </div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  padding: 14px 16px 24px;
  max-width: 980px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.head h1 {
  font-size: var(--fs-xl);
  margin: 0;
}
.count {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.spacer {
  flex: 1;
}
.note {
  margin: 0;
  padding: 8px 12px;
  background: var(--blue-wash);
  border-left: 3px solid var(--blue);
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.users {
  width: 100%;
  border-collapse: collapse;
  background: var(--sheet);
  border: 1px solid var(--rule);
  font-size: var(--fs-md);
}
.users th {
  text-align: left;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
  background: var(--paper);
  padding: 6px 10px;
  border-bottom: 1px solid var(--rule);
}
.users td {
  padding: 4px 10px;
  border-bottom: 1px solid var(--rule-soft);
  vertical-align: middle;
}
.role-col {
  width: 200px;
}
.role-col :deep(.p-select) {
  width: 100%;
}
.act {
  width: 44px;
  text-align: right;
}
.empty {
  color: var(--ink-faint);
  padding: 14px 10px !important;
}
.email {
  color: var(--ink-soft);
}
.badge {
  margin-left: 6px;
  font-size: var(--fs-xs);
  padding: 0 6px;
  border-radius: 8px;
  background: var(--blue-wash);
  color: var(--blue-deep);
}
.badge.off {
  background: var(--tone-overdue-bg);
  color: var(--tone-overdue);
}
.opt {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}
.opt small {
  color: var(--ink-faint);
  font-size: var(--fs-xs);
}
.legend h2 {
  font-size: var(--fs-md);
  margin: 6px 0 4px;
  color: var(--ink-soft);
}
.legend dl {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 2px 14px;
  margin: 0;
  font-size: var(--fs-sm);
}
.legend dt {
  font-weight: 600;
}
.legend dd {
  margin: 0;
  color: var(--ink-soft);
}
.add {
  grid-template-columns: 1fr 1fr;
}
.actions {
  grid-column: 1 / -1;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@media (max-width: 640px) {
  .users thead {
    display: none;
  }
  .users,
  .users tbody,
  .users tr,
  .users td {
    display: block;
    width: auto;
  }
  .users tr {
    position: relative;
    padding: 8px 44px 8px 10px;
    border-bottom: 1px solid var(--rule);
  }
  .users td {
    border: 0;
    padding: 1px 0;
  }
  .users td.act {
    position: absolute;
    top: 6px;
    right: 4px;
  }
  .name {
    font-weight: 600;
  }
  .role-col {
    margin-top: 4px;
  }
  .legend dl {
    grid-template-columns: 1fr;
  }
  .legend dd {
    margin-bottom: 4px;
  }
}
</style>
