<script setup>
// Odoslanie dokladu e-mailom: predvyplnená správa zo šablóny, PDF v prílohe
// (vytvorí ho server) a história odoslaní dokladu.
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useToast } from 'primevue/usetoast'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import FormField from './FormField.vue'
import { api } from '../lib/api.js'
import { can, session } from '../lib/session.js'
import { emailStatus } from '../lib/emails.js'

const props = defineProps({
  kind: { type: String, required: true }, // invoices | quotes | reminders
  id: { type: [String, Number], default: null },
})
const visible = defineModel('visible', { type: Boolean, default: false })
const emit = defineEmits(['sent'])
const router = useRouter()
const toast = useToast()

const draft = ref(null)
const form = ref({ to: '', cc: '', subject: '', body: '' })
const loading = ref(false)
const sending = ref(false)
const error = ref('')
const showCc = ref(false)

const fmt = new Intl.DateTimeFormat('sk-SK', { day: 'numeric', month: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
const when = (iso) => fmt.format(new Date(iso))
const pdfLink = computed(() => (props.id ? api.pdfURL(session.companyId, props.kind, props.id, true) : '#'))
const ready = computed(() => draft.value?.configured && draft.value?.sendable)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const d = await api.mailDraft(session.companyId, props.kind, props.id)
    draft.value = d
    form.value = { to: d.to, cc: d.cc, subject: d.subject, body: d.body }
    showCc.value = !!d.cc
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
watch(visible, (v) => v && props.id && load(), { immediate: true })

async function send() {
  sending.value = true
  error.value = ''
  try {
    const e = await api.sendMail(session.companyId, props.kind, props.id, form.value)
    toast.add({ severity: 'success', summary: 'E-mail odoslaný', detail: e.to, life: 3500 })
    emit('sent', e)
    visible.value = false
  } catch (e) {
    error.value = e.message
    // neúspešný pokus je v histórii
    try {
      draft.value = { ...draft.value, history: (await api.mailDraft(session.companyId, props.kind, props.id)).history }
    } catch {
      /* história sa nenačítala — chyba je už zobrazená */
    }
  } finally {
    sending.value = false
  }
}

function toSettings() {
  visible.value = false
  router.push('/emaily?nastavenie=1')
}
</script>

<template>
  <Dialog v-model:visible="visible" modal header="Poslať e-mailom" :style="{ width: '44rem' }" :breakpoints="{ '760px': '96vw' }">
    <p v-if="loading" class="muted">Načítavam…</p>
    <template v-else-if="draft">
      <div v-if="!draft.configured" class="notice">
        <i class="pi pi-info-circle" />
        <span>
          Odosielanie e-mailov ešte nie je nastavené.
          <button v-if="can('settings.write')" type="button" class="link" @click="toSettings">Nastaviť server SMTP</button>
          <template v-else>Požiadajte správcu firmy o nastavenie.</template>
        </span>
      </div>
      <div v-else-if="!draft.sendable" class="notice">
        <i class="pi pi-info-circle" />
        <span>Koncept sa neposiela — doklad najprv vystavte.</span>
      </div>

      <div class="form">
        <FormField label="Komu" hint="Viac adries oddeľte čiarkou">
          <div class="to-row">
            <InputText v-model="form.to" size="small" inputmode="email" autocomplete="off" :disabled="!ready" fluid />
            <button v-if="!showCc" type="button" class="link" :disabled="!ready" @click="showCc = true">Kópia</button>
          </div>
        </FormField>
        <FormField v-if="showCc" label="Kópia">
          <InputText v-model="form.cc" size="small" autocomplete="off" :disabled="!ready" fluid />
        </FormField>
        <FormField label="Predmet">
          <InputText v-model="form.subject" size="small" :disabled="!ready" fluid />
        </FormField>
        <FormField label="Správa">
          <Textarea v-model="form.body" rows="9" auto-resize :disabled="!ready" fluid />
        </FormField>
        <div class="attachment">
          <i class="pi pi-paperclip" />
          <a :href="pdfLink" target="_blank" rel="noopener">{{ draft.file }}</a>
        </div>
      </div>

      <p v-if="error" class="error">{{ error }}</p>

      <section v-if="draft.history?.length" class="history">
        <h3>Odoslané</h3>
        <ul>
          <li v-for="h in draft.history" :key="h.id">
            <span class="tag" :data-tone="emailStatus(h).tone"><span class="tag-text">{{ emailStatus(h).label }}</span></span>
            <span class="when">{{ when(h.sent) }}</span>
            <span class="to">{{ h.to }}</span>
            <span class="who">{{ h.user }}</span>
            <span v-if="h.error" class="err">{{ h.error }}</span>
          </li>
        </ul>
      </section>
    </template>
    <p v-else-if="error" class="error">{{ error }}</p>

    <template #footer>
      <Button label="Zrušiť" severity="secondary" text size="small" @click="visible = false" />
      <Button label="Odoslať" icon="pi pi-send" size="small" :loading="sending" :disabled="!ready || !form.to.trim()" @click="send" />
    </template>
  </Dialog>
</template>

<style scoped>
.form {
  display: grid;
  gap: 10px;
}
.to-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.notice {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  margin: 0 0 12px;
  padding: 8px 10px;
  background: var(--blue-wash);
  border-left: 3px solid var(--blue);
  font-size: var(--fs-sm);
}
.notice .pi {
  margin-top: 2px;
  color: var(--blue);
}
.link {
  font: inherit;
  font-size: var(--fs-sm);
  border: 0;
  background: none;
  padding: 0;
  color: var(--blue);
  cursor: pointer;
  white-space: nowrap;
}
.link:disabled {
  color: var(--ink-faint);
  cursor: default;
}
.attachment {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
}
.attachment .pi {
  color: var(--ink-soft);
}
.muted {
  color: var(--ink-soft);
}
.error {
  margin: 10px 0 0;
  color: var(--tone-overdue);
  font-size: var(--fs-sm);
}
.history {
  margin-top: 16px;
  border-top: 1px solid var(--rule);
  padding-top: 8px;
}
.history h3 {
  margin: 0 0 6px;
  font-size: var(--fs-sm);
  font-weight: 600;
  color: var(--ink-soft);
}
.history ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 4px;
  font-size: var(--fs-sm);
}
.history li {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 10px;
}
.when {
  font-variant-numeric: tabular-nums;
}
.to {
  font-weight: 600;
  overflow-wrap: anywhere;
}
.who {
  color: var(--ink-soft);
}
.err {
  flex-basis: 100%;
  color: var(--tone-overdue);
}
</style>
