<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { canSee } from '../lib/session.js'
import { currentSetting, visibleSettings } from '../lib/settingsNav.js'

// Rám nastavení: zoznam sekcií vľavo, na mobile rozbaľovacie menu (☰).
const route = useRoute()
const groups = computed(() => visibleSettings(canSee))
const current = computed(() => currentSetting(route.path))
const open = ref(false)
watch(() => route.fullPath, () => (open.value = false))
const isActive = (item) => !item.external && (route.path === item.path || route.path.startsWith(item.path + '/'))
</script>

<template>
  <div class="settings-shell" :class="{ open }">
    <button type="button" class="toggle" :aria-expanded="open" aria-controls="settings-nav" @click="open = !open">
      <i class="pi" :class="open ? 'pi-times' : 'pi-bars'" aria-hidden="true" />
      <span class="crumb">Nastavenia</span>
      <template v-if="current">
        <i class="pi pi-angle-right sep" aria-hidden="true" />
        <strong>{{ current.label }}</strong>
      </template>
    </button>
    <nav id="settings-nav" class="snav" aria-label="Nastavenia">
      <h2 class="title">Nastavenia</h2>
      <section v-for="g in groups" :key="g.group">
        <h3>{{ g.group }}</h3>
        <RouterLink
          v-for="item in g.items"
          :key="item.path"
          :to="item.path"
          class="sitem"
          :class="{ active: isActive(item) }"
          :aria-current="isActive(item) ? 'page' : undefined"
        >
          <i class="pi" :class="item.icon" aria-hidden="true" />
          <span>{{ item.label }}</span>
          <i v-if="item.external" class="pi pi-external-link ext" aria-hidden="true" title="Otvorí sa v module E-maily" />
        </RouterLink>
      </section>
    </nav>
    <div class="spane">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.settings-shell {
  display: flex;
  height: 100%;
  min-height: 0;
}
.toggle {
  display: none;
}
.snav {
  flex: 0 0 208px;
  overflow-y: auto;
  padding: 12px 0 24px;
  background: var(--sheet);
  border-right: 1px solid var(--rule);
}
.title {
  padding: 0 14px 6px;
  font-size: var(--fs-lg);
  color: var(--blue-deep);
}
.snav section + section {
  margin-top: 6px;
}
h3 {
  padding: 8px 14px 3px;
  font-size: var(--fs-xs);
  font-weight: 600;
  color: var(--ink-faint);
}
.sitem {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 5px 14px 5px 11px;
  border-left: 3px solid transparent;
  color: var(--ink);
  text-decoration: none;
}
.sitem .pi {
  width: 15px;
  font-size: 0.8125rem;
  color: var(--ink-faint);
}
.sitem .ext {
  margin-left: auto;
  width: auto;
  font-size: 0.6875rem;
}
.sitem:hover {
  background: var(--rule-soft);
}
.sitem:focus-visible {
  outline: 2px solid var(--blue);
  outline-offset: -2px;
}
.sitem.active {
  background: var(--blue-wash);
  border-left-color: var(--blue);
  color: var(--blue-deep);
  font-weight: 600;
}
.sitem.active .pi {
  color: var(--blue);
}
.spane {
  flex: 1;
  min-width: 0;
  overflow: auto;
}

@media (max-width: 860px) {
  .settings-shell {
    flex-direction: column;
    position: relative;
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 0 0 auto;
    width: 100%;
    padding: 9px 16px;
    background: var(--sheet);
    border: 0;
    border-bottom: 1px solid var(--rule);
    color: var(--ink);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .toggle .pi {
    color: var(--blue);
  }
  .toggle .sep {
    color: var(--ink-faint);
    font-size: 0.75rem;
  }
  .crumb {
    color: var(--ink-soft);
  }
  .title {
    display: none;
  }
  .snav {
    display: none;
    position: absolute;
    z-index: 20;
    top: 40px;
    left: 0;
    right: 0;
    max-height: calc(100% - 40px);
    border-right: 0;
    border-bottom: 1px solid var(--rule);
    box-shadow: 0 8px 18px rgba(31, 58, 92, 0.16);
  }
  .open .snav {
    display: block;
  }
  .sitem {
    padding-top: 8px;
    padding-bottom: 8px;
  }
}
</style>
