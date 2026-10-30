<script setup>
// Kópia záloh v cloude (WebDAV alebo S3). Server spojenie overí skúšobným
// súborom; tajný údaj sa do prehliadača nevracia — prázdne pole ho ponechá.
import { computed, reactive, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import { useToast } from 'primevue/usetoast'
import { api, ApiError } from '../lib/api.js'
import { s3Endpoint, s3ProviderOf, s3Providers } from '../lib/backups.js'
import FormField from './FormField.vue'

const props = defineProps({ remote: { type: Object, required: true } })
const emit = defineEmits(['saved', 'cancel'])
const toast = useToast()

const kinds = [
  { label: 'WebDAV', value: 'webdav' },
  { label: 'S3', value: 's3' },
]
const form = reactive({ kind: 'webdav', url: '', user: '', secret: '', bucket: '', region: '', prefix: 'fiskus' })
const provider = ref('aws')
const errors = ref({})
const saving = ref(false)

watch(
  () => props.remote,
  (r) => {
    Object.assign(form, { kind: r.kind || 'webdav', url: r.url, user: r.user, secret: '', bucket: r.bucket, region: r.region, prefix: r.kind ? r.prefix : 'fiskus' })
    provider.value = r.kind === 's3' ? s3ProviderOf(r.url, r.region) : 'aws'
    if (!r.kind) form.region = s3Providers[0].region
  },
  { immediate: true },
)

// adresa S3 sa doplní podľa poskytovateľa a regiónu
const fixedURL = computed(() => form.kind === 's3' && provider.value !== 'other')
const providerRegion = (id) => s3Providers.find((x) => x.id === id)?.region ?? ''
watch(
  () => form.kind,
  (k, old) => {
    if (k === old) return
    form.user = form.secret = form.url = ''
    if (k === 's3' && !form.region) form.region = providerRegion(provider.value)
  },
)
watch(provider, (p) => {
  if (p !== 'other') form.region = providerRegion(p)
})
watch([provider, () => form.region, () => form.kind], () => {
  if (fixedURL.value) form.url = s3Endpoint(provider.value, form.region)
})

const sameLogin = computed(() => props.remote.secretSet && form.kind === props.remote.kind && form.user === props.remote.user)
const labels = computed(() =>
  form.kind === 's3'
    ? { user: 'Prístupový kľúč (access key)', secret: 'Tajný kľúč (secret key)' }
    : { user: 'Používateľ', secret: 'Heslo (pri Nextcloude heslo aplikácie)' },
)

async function save() {
  saving.value = true
  errors.value = {}
  try {
    const st = await api.setBackupRemote({ ...form, url: form.url.trim() })
    form.secret = ''
    const s = st.remote.state
    toast.add({
      severity: s.lastError ? 'warn' : 'success',
      summary: 'Cloud nastavený',
      detail: s.lastError ? `Spojenie funguje, ale kópia zlyhala: ${s.lastError}` : 'Spojenie overené, posledná záloha je skopírovaná.',
      life: 6000,
    })
    emit('saved', st)
  } catch (e) {
    if (e instanceof ApiError && e.fields.length) errors.value = e.fieldMap()
    else toast.add({ severity: 'error', summary: 'Cloud sa nenastavil', detail: e.message, life: 8000 })
  } finally {
    saving.value = false
  }
}

async function disable() {
  saving.value = true
  try {
    const st = await api.setBackupRemote({ kind: '' })
    toast.add({ severity: 'info', summary: 'Kópia do cloudu vypnutá', detail: 'Zálohy, ktoré už v cloude sú, ostávajú.', life: 5000 })
    emit('saved', st)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nepodarilo sa', detail: e.message, life: 6000 })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="card" aria-labelledby="br-title">
    <h2 id="br-title">Kópia záloh v cloude</h2>
    <p class="muted lead">
      Po každej zálohe sa zašifrovaný súbor skopíruje aj do cloudu a staré kópie sa mažú podľa rovnakého pravidla ako na disku.
      Poskytovateľ zálohy neprečíta — sú zašifrované heslom záloh.
    </p>
    <div class="form-grid">
      <FormField label="Úložisko" :span="4" :error="errors.kind">
        <SelectButton v-model="form.kind" :options="kinds" option-label="label" option-value="value" :allow-empty="false" size="small" aria-label="Úložisko" />
      </FormField>

      <template v-if="form.kind === 'webdav'">
        <FormField label="Adresa priečinka" :span="4" :error="errors.url"
          hint="Napr. https://cloud.firma.sk/remote.php/dav/files/jana/Fiškus — Nextcloud, ownCloud, Synology, QNAP…">
          <InputText v-model="form.url" size="small" spellcheck="false" inputmode="url" :invalid="!!errors.url" placeholder="https://" />
        </FormField>
      </template>
      <template v-else>
        <FormField label="Poskytovateľ" :span="2">
          <Select v-model="provider" :options="s3Providers" option-label="label" option-value="id" size="small" fluid />
        </FormField>
        <FormField label="Región" :span="2" :error="errors.region">
          <InputText v-model="form.region" size="small" spellcheck="false" :invalid="!!errors.region" />
        </FormField>
        <FormField label="Adresa (endpoint)" :span="4" :error="errors.url" :hint="fixedURL ? 'Doplnená podľa poskytovateľa a regiónu.' : 'Napr. http://127.0.0.1:9000 pre MinIO v sieti firmy.'">
          <InputText v-model="form.url" size="small" spellcheck="false" inputmode="url" :disabled="fixedURL" :invalid="!!errors.url" />
        </FormField>
        <FormField label="Bucket" :span="2" :error="errors.bucket">
          <InputText v-model="form.bucket" size="small" spellcheck="false" :invalid="!!errors.bucket" />
        </FormField>
        <FormField label="Priečinok v buckete" :span="2" hint="Nepovinné">
          <InputText v-model="form.prefix" size="small" spellcheck="false" />
        </FormField>
      </template>

      <FormField :label="labels.user" :span="2" :error="errors.user">
        <InputText v-model="form.user" size="small" spellcheck="false" autocomplete="off" :invalid="!!errors.user" />
      </FormField>
      <FormField :label="labels.secret" :span="2" :error="errors.secret" :hint="sameLogin ? 'Uložené — vyplňte iba pri zmene' : ''">
        <Password v-model="form.secret" :feedback="false" toggle-mask size="small" autocomplete="new-password" :invalid="!!errors.secret" fluid />
      </FormField>
    </div>
    <p class="muted">
      Odporúčame samostatného používateľa alebo kľúč s prístupom iba k tomuto priečinku či bucketu. Údaje sú uložené v súbore
      nastavenia záloh vedľa databázy, rovnako ako heslo záloh.
    </p>
    <div class="actions">
      <Button v-if="remote.kind" label="Vypnúť kópiu do cloudu" severity="danger" text size="small" :disabled="saving" @click="disable" />
      <div class="spacer" />
      <Button label="Zrušiť" severity="secondary" text size="small" @click="emit('cancel')" />
      <Button label="Overiť a uložiť" icon="pi pi-check" size="small" :loading="saving" @click="save" />
    </div>
  </section>
</template>

<style scoped>
.card {
  background: var(--sheet);
  border: 1px solid var(--rule);
  padding: 12px 14px;
}
.card h2 {
  font-size: var(--fs-md);
  margin: 0 0 6px;
}
.muted {
  color: var(--ink-soft);
  font-size: var(--fs-sm);
  margin: 10px 0 0;
}
.muted.lead {
  margin: 0 0 10px;
}
.actions {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 10px;
  flex-wrap: wrap;
}
.spacer {
  flex: 1;
}
</style>
