<script setup>
// Dvojfázové overenie prihláseného používateľa: zapnutie (heslo → QR kód →
// prvý kód → záložné kódy), nové záložné kódy a vypnutie.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Password from 'primevue/password'
import InputText from 'primevue/inputtext'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'
import { formatSecret, mfaQR, recoveryFile } from '../lib/mfa.js'

const visible = defineModel('visible', { type: Boolean, default: false })
// forced: správca vyžaduje 2FA — okno sa nedá zavrieť, po zapnutí ohlási „done“
const props = defineProps({ forced: { type: Boolean, default: false } })
const emit = defineEmits(['done'])
const toast = useToast()

// krok: status | password | scan | codes | disable | recovery
const step = ref('status')
const status = ref(null)
const password = ref('')
const code = ref('')
const setup = ref(null)
const qr = ref('')
const codes = ref([])
const saved = ref(false)
const error = ref('')
const busy = ref(false)

const email = computed(() => status.value?.account || session.me?.user?.name || '')

watch(visible, async (v) => {
  if (!v) return
  reset('status')
  try {
    status.value = await api.mfa()
  } catch (e) {
    error.value = e.message
  }
}, { immediate: true })

function reset(s) {
  step.value = s
  password.value = ''
  code.value = ''
  error.value = ''
}

const fieldMsg = (e) => e.fields?.[0]?.message ?? e.message

async function run(fn) {
  error.value = ''
  busy.value = true
  try {
    await fn()
  } catch (e) {
    error.value = fieldMsg(e)
  } finally {
    busy.value = false
  }
}

const startSetup = () =>
  run(async () => {
    setup.value = await api.mfaSetup(password.value)
    qr.value = await mfaQR(setup.value.uri)
    reset('scan')
  })

const confirmCode = () =>
  run(async () => {
    const out = await api.mfaEnable(code.value.trim())
    codes.value = out.recoveryCodes
    saved.value = false
    setup.value = null
    status.value = { ...status.value, enabled: true, recoveryLeft: codes.value.length }
    reset('codes')
  })

const newCodes = () =>
  run(async () => {
    const out = await api.mfaRecovery(password.value)
    codes.value = out.recoveryCodes
    saved.value = false
    status.value = { ...status.value, enabled: true, recoveryLeft: codes.value.length }
    reset('codes')
  })

const disable = () =>
  run(async () => {
    await api.mfaDisable(password.value, code.value.trim())
    status.value = { ...status.value, enabled: false, recoveryLeft: 0 }
    toast.add({ severity: 'success', summary: 'Dvojfázové overenie vypnuté', detail: 'Prihlásite sa iba heslom.', life: 4000 })
    reset('status')
  })

function download() {
  const blob = new Blob([recoveryFile(codes.value, email.value)], { type: 'text/plain;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = 'fiskus-zalozne-kody.txt'
  a.click()
  URL.revokeObjectURL(a.href)
  saved.value = true
}

async function copy() {
  try {
    await navigator.clipboard.writeText(codes.value.join('\n'))
    saved.value = true
    toast.add({ severity: 'success', summary: 'Skopírované', life: 2000 })
  } catch {
    toast.add({ severity: 'warn', summary: 'Kopírovanie nefunguje', detail: 'Stiahnite si súbor.', life: 4000 })
  }
}

function finish() {
  codes.value = []
  toast.add({ severity: 'success', summary: 'Dvojfázové overenie je zapnuté', detail: 'Pri ďalšom prihlásení zadáte aj kód z aplikácie.', life: 5000 })
  reset('status')
  visible.value = false
  emit('done')
}

const header = computed(
  () =>
    ({
      scan: 'Naskenujte QR kód',
      codes: 'Záložné kódy',
      disable: 'Vypnúť dvojfázové overenie',
      recovery: 'Nové záložné kódy',
    })[step.value] ?? 'Dvojfázové overenie',
)
</script>

<template>
  <Dialog v-model:visible="visible" :header="header" modal :closable="step !== 'codes' && !props.forced" :style="{ width: '28rem' }" :breakpoints="{ '520px': '94vw' }">
    <!-- stav -->
    <div v-if="step === 'status'" class="mfa">
      <p v-if="!status" class="muted">Načítavam…</p>
      <template v-else-if="status.enabled">
        <p class="state on"><i class="pi pi-shield" /> Zapnuté</p>
        <p>Pri prihlásení zadávate okrem hesla aj kód z aplikácie v telefóne.</p>
        <p class="muted">
          Nepoužité záložné kódy: <strong :class="{ low: status.recoveryLeft < 3 }">{{ status.recoveryLeft }}</strong>
          <template v-if="status.recoveryLeft < 3"> — vytvorte si nové.</template>
        </p>
        <div class="actions">
          <Button label="Vypnúť" severity="danger" text size="small" @click="reset('disable')" />
          <Button label="Nové záložné kódy" severity="secondary" size="small" @click="reset('recovery')" />
        </div>
      </template>
      <template v-else>
        <p class="state">Vypnuté</p>
        <p v-if="props.forced" class="forced">
          <i class="pi pi-shield" /> Správca vyžaduje dvojfázové overenie. Kým si ho nezapnete, k dokladom sa nedostanete.
        </p>
        <p>
          S dvojfázovým overením nestačí na prihlásenie iba heslo. Treba aj kód z aplikácie v telefóne, napríklad Google
          Authenticator, Microsoft Authenticator alebo Aegis.
        </p>
        <form class="mfa" @submit.prevent="startSetup">
          <label><span>Na zapnutie zadajte svoje heslo</span><Password v-model="password" :feedback="false" fluid autocomplete="current-password" /></label>
          <p v-if="error" class="err">{{ error }}</p>
          <div class="actions">
            <Button type="submit" label="Zapnúť" icon="pi pi-shield" size="small" :loading="busy" :disabled="!password" />
          </div>
        </form>
      </template>
    </div>

    <!-- QR kód a prvý kód -->
    <form v-else-if="step === 'scan'" class="mfa" @submit.prevent="confirmCode">
      <ol class="steps">
        <li>V aplikácii zvoľte pridanie účtu a naskenujte kód.</li>
        <li>Opíšte 6-miestny kód, ktorý aplikácia ukáže pri Fiškuse.</li>
      </ol>
      <!-- eslint-disable-next-line vue/no-v-html — SVG z knižnice qrcode, bez vstupu používateľa okrem kľúča -->
      <div class="qr" role="img" aria-label="QR kód pre aplikáciu na overovanie" v-html="qr" />
      <p class="secret">
        Nedá sa skenovať? Zadajte kľúč ručne:<br />
        <code>{{ formatSecret(setup?.secret) }}</code>
      </p>
      <label>
        <span>Kód z aplikácie</span>
        <InputText v-model="code" inputmode="numeric" autocomplete="one-time-code" maxlength="7" placeholder="123 456" class="code" fluid autofocus />
      </label>
      <p v-if="error" class="err">{{ error }}</p>
      <div class="actions">
        <Button label="Zrušiť" severity="secondary" text size="small" @click="reset('status')" />
        <Button type="submit" label="Overiť a zapnúť" size="small" :loading="busy" :disabled="!code" />
      </div>
    </form>

    <!-- záložné kódy (zobrazia sa iba raz) -->
    <div v-else-if="step === 'codes'" class="mfa">
      <p>
        Keď nebudete mať telefón, prihlásite sa jedným z týchto kódov. Každý platí iba raz. <strong>Zobrazia sa iba teraz</strong> —
        stiahnite si ich alebo vytlačte.
      </p>
      <ul class="codes">
        <li v-for="c in codes" :key="c"><code>{{ c }}</code></li>
      </ul>
      <div class="actions left">
        <Button label="Stiahnuť" icon="pi pi-download" severity="secondary" size="small" @click="download" />
        <Button label="Kopírovať" icon="pi pi-copy" severity="secondary" text size="small" @click="copy" />
      </div>
      <label class="check"><input v-model="saved" type="checkbox" /> Kódy mám uložené</label>
      <div class="actions">
        <Button label="Hotovo" size="small" :disabled="!saved" @click="finish" />
      </div>
    </div>

    <!-- vypnutie -->
    <form v-else-if="step === 'disable'" class="mfa" @submit.prevent="disable">
      <p>Po vypnutí bude na prihlásenie stačiť heslo.</p>
      <label><span>Heslo</span><Password v-model="password" :feedback="false" fluid autocomplete="current-password" /></label>
      <label>
        <span>Kód z aplikácie alebo záložný kód</span>
        <InputText v-model="code" autocomplete="one-time-code" class="code" fluid />
      </label>
      <p v-if="error" class="err">{{ error }}</p>
      <div class="actions">
        <Button label="Späť" severity="secondary" text size="small" @click="reset('status')" />
        <Button type="submit" label="Vypnúť" severity="danger" size="small" :loading="busy" :disabled="!password || !code" />
      </div>
    </form>

    <!-- nové záložné kódy -->
    <form v-else-if="step === 'recovery'" class="mfa" @submit.prevent="newCodes">
      <p>Vytvorí sa 10 nových kódov a doterajšie prestanú platiť.</p>
      <label><span>Heslo</span><Password v-model="password" :feedback="false" fluid autocomplete="current-password" /></label>
      <p v-if="error" class="err">{{ error }}</p>
      <div class="actions">
        <Button label="Späť" severity="secondary" text size="small" @click="reset('status')" />
        <Button type="submit" label="Vytvoriť kódy" size="small" :loading="busy" :disabled="!password" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.mfa {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.mfa p {
  margin: 0;
  font-size: var(--fs-md);
}
.mfa label {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.mfa label > span {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
}
.forced {
  padding: 8px 10px;
  background: var(--tone-draft-bg);
  color: var(--tone-draft);
  border-radius: var(--radius);
}
.muted {
  color: var(--ink-faint);
}
.state {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  color: var(--ink-soft);
}
.state.on {
  color: var(--tone-paid);
}
.low {
  color: var(--tone-overdue);
}
.steps {
  margin: 0;
  padding-left: 1.2em;
  font-size: var(--fs-md);
}
.qr {
  align-self: center;
  width: 180px;
  height: 180px;
  padding: 10px;
  background: #fff;
  border: 1px solid var(--rule);
}
.qr :deep(svg) {
  width: 100%;
  height: 100%;
  display: block;
}
.secret {
  font-size: var(--fs-sm) !important;
  color: var(--ink-soft);
}
.secret code,
.codes code {
  font-family: var(--font);
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.06em;
  font-weight: 600;
  color: var(--ink);
}
.code {
  font-size: var(--fs-lg);
  letter-spacing: 0.08em;
  font-variant-numeric: tabular-nums;
}
.codes {
  list-style: none;
  margin: 0;
  padding: 10px 12px;
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 4px 16px;
  background: var(--paper);
  border: 1px solid var(--rule);
  border-radius: var(--radius);
}
.check {
  flex-direction: row !important;
  align-items: center;
  gap: 8px !important;
  font-size: var(--fs-md);
}
.err {
  color: var(--tone-overdue);
  font-size: var(--fs-sm) !important;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.actions.left {
  justify-content: flex-start;
}
</style>
