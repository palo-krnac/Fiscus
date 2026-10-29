<script setup>
// Súhlas s pripojením aplikácie (napr. Claude na webe) cez OAuth.
// Sem presmeruje /oauth/authorize; prihlásenie zariadi App.vue.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import RadioButton from 'primevue/radiobutton'
import { api } from '../lib/api.js'
import { session } from '../lib/session.js'

const query = window.location.search.replace(/^\?/, '')
const info = ref(null)
const error = ref('')
const readOnly = ref(false)
const busy = ref('')

const userName = computed(() => info.value?.user || session.me?.user?.name || '')

onMounted(async () => {
  try {
    info.value = await api.oauthRequest(query)
  } catch (e) {
    error.value = e.message
  }
})

async function decide(allow) {
  busy.value = allow ? 'allow' : 'deny'
  try {
    const out = await api.oauthDecision(query, allow, readOnly.value)
    window.location.assign(out.redirect)
  } catch (e) {
    error.value = e.message
    busy.value = ''
  }
}
</script>

<template>
  <div class="consent-page">
    <section class="consent" aria-labelledby="consent-title">
      <div class="brand">
        <svg viewBox="0 0 32 32" width="34" height="34" aria-hidden="true">
          <rect width="32" height="32" rx="7" fill="#1f3a5c" />
          <path d="M10 8h13v3.4h-9v3.6h8v3.3h-8V24h-4z" fill="#f6f7f9" />
          <rect x="18" y="20.5" width="5" height="3.5" fill="#d9a441" />
        </svg>
        <div>
          <h1 id="consent-title">Pripojenie aplikácie</h1>
          <p v-if="userName">Prihlásený: {{ userName }}</p>
        </div>
      </div>

      <p v-if="error" class="error" role="alert"><i class="pi pi-exclamation-circle" /> {{ error }}</p>
      <p v-else-if="!info" class="muted">Načítavam…</p>

      <template v-else>
        <p class="ask"><strong>{{ info.client }}</strong> žiada o prístup k vášmu Fiškusu.</p>
        <ul class="what">
          <li>Uvidí firmy a doklady, ku ktorým máte prístup vy.</li>
          <li>Každá jeho akcia sa zapíše do auditu pod vaším menom a názvom aplikácie.</li>
        </ul>

        <fieldset class="mode">
          <legend>Prístup</legend>
          <label>
            <RadioButton v-model="readOnly" :value="false" input-id="full" />
            <span><strong>Ako ja</strong><small>Môže vytvárať koncepty, evidovať doklady a párovať platby podľa mojich práv.</small></span>
          </label>
          <label>
            <RadioButton v-model="readOnly" :value="true" input-id="ro" />
            <span><strong>Iba na čítanie</strong><small>Nič nezmení.</small></span>
          </label>
        </fieldset>

        <p class="muted small">
          Po povolení sa vrátite na <strong>{{ info.redirectHost }}</strong>. Prístup kedykoľvek zrušíte v časti
          <em>Prístup pre AI</em>.
        </p>
        <div class="actions">
          <Button label="Zamietnuť" severity="secondary" text :loading="busy === 'deny'" :disabled="!!busy" @click="decide(false)" />
          <Button label="Povoliť" icon="pi pi-check" :loading="busy === 'allow'" :disabled="!!busy" @click="decide(true)" />
        </div>
      </template>
    </section>
  </div>
</template>

<style scoped>
.consent-page {
  min-height: 100%;
  display: grid;
  place-items: center;
  padding: 24px 16px;
  background: var(--paper);
}
.consent {
  width: 100%;
  max-width: 420px;
  background: var(--sheet);
  border: 1px solid var(--rule);
  border-top: 3px solid var(--blue-deep);
  border-radius: var(--radius);
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
}
.brand h1 {
  font-size: var(--fs-xl);
  margin: 0;
  color: var(--blue-deep);
  line-height: 1.15;
}
.brand p {
  margin: 0;
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.ask {
  margin: 4px 0 0;
  font-size: var(--fs-lg);
}
.what {
  margin: 0;
  padding-left: 1.2em;
  font-size: var(--fs-md);
  color: var(--ink-soft);
  display: grid;
  gap: 2px;
}
.mode {
  border: 1px solid var(--rule);
  border-radius: var(--radius);
  padding: 8px 12px 10px;
  margin: 0;
  display: grid;
  gap: 8px;
}
.mode legend {
  font-size: var(--fs-sm);
  color: var(--ink-soft);
  padding: 0 4px;
}
.mode label {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  cursor: pointer;
}
.mode span {
  display: flex;
  flex-direction: column;
  font-size: var(--fs-md);
}
.mode small {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
}
.muted {
  color: var(--ink-faint);
  margin: 0;
}
.small {
  font-size: var(--fs-sm);
}
.error {
  margin: 0;
  color: var(--tone-overdue);
  font-size: var(--fs-md);
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
