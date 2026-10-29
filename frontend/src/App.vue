<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Select from 'primevue/select'
import Toast from 'primevue/toast'
import ConfirmDialog from 'primevue/confirmdialog'
import { modules } from './router.js'
import { session, loadSession, selectCompany, can, isSuperAdmin } from './lib/session.js'
import { visibleModules } from './lib/access.js'
import Login from './views/Login.vue'
import UserMenu from './components/UserMenu.vue'

const route = useRoute()
const navOpen = ref(false)

onMounted(loadSession)
watch(() => route.path, () => (navOpen.value = false))

const menu = computed(() =>
  visibleModules(
    modules,
    (i) => (i.superAdmin ? isSuperAdmin() : can(i.perm)) && (!i.serverOnly || session.caps.mode === 'server'),
  ),
)
const allowed = computed(() => (route.meta.superAdmin ? isSuperAdmin() : can(route.meta.perm)))
// bez firmy sa dá otvoriť iba založenie firmy
const showView = computed(() => session.ready && allowed.value && (session.companyId || route.meta.noCompany))
const edition = computed(() => {
  if (session.caps.mode !== 'server') return 'Desktop · zadarmo'
  return session.caps.plan === 'multi' ? 'Server · viac firiem' : 'Server · jedna firma zadarmo'
})
const isActive = (path) => route.path === path || route.path.startsWith(path + '/')
</script>

<template>
  <template v-if="session.needLogin">
    <Login />
    <Toast position="bottom-right" />
  </template>
  <!-- tlačové zobrazenie bez menu a hlavičky -->
  <template v-else-if="route.meta.bare">
    <RouterView v-if="session.ready && session.companyId && allowed" />
    <Toast position="bottom-right" />
  </template>
  <div v-else class="shell" :class="{ 'nav-open': navOpen }">
    <aside class="sidebar" aria-label="Moduly">
      <RouterLink to="/faktury" class="brand" aria-label="Fiškus – úvod">
        <svg viewBox="0 0 32 32" width="26" height="26" aria-hidden="true">
          <rect width="32" height="32" rx="7" fill="#f6f7f9" />
          <path d="M10 8h13v3.4h-9v3.6h8v3.3h-8V24h-4z" fill="#1f3a5c" />
          <rect x="18" y="20.5" width="5" height="3.5" fill="#d9a441" />
        </svg>
        <span>Fiškus</span>
      </RouterLink>

      <nav>
        <section v-for="g in menu" :key="g.group" class="nav-group">
          <h2>{{ g.group }}</h2>
          <RouterLink
            v-for="item in g.items"
            :key="item.path"
            :to="item.path"
            class="nav-item"
            :class="{ active: isActive(item.path), pending: !item.ready }"
          >
            <i class="pi" :class="item.icon" aria-hidden="true" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </section>
      </nav>

      <footer class="edition">
        {{ edition }}
      </footer>
    </aside>

    <div class="scrim" @click="navOpen = false" />

    <div class="main">
      <header class="topbar">
        <button class="menu-btn" aria-label="Menu" @click="navOpen = !navOpen">
          <i class="pi pi-bars" />
        </button>

        <div class="company">
          <Select
            v-if="session.companies.length > 1"
            :model-value="session.companyId"
            :options="session.companies"
            option-label="name"
            option-value="id"
            size="small"
            aria-label="Firma"
            @update:model-value="selectCompany"
          />
          <template v-else-if="session.company">
            <strong>{{ session.company.name }}</strong>
            <span v-if="session.company.ico" class="muted">IČO {{ session.company.ico }}</span>
          </template>
        </div>
        <UserMenu />
      </header>

      <main class="content">
        <div v-if="session.error" class="fatal">
          <i class="pi pi-exclamation-triangle" />
          <p>{{ session.error }}</p>
        </div>
        <RouterView v-else-if="showView" :key="session.companyId ?? 0" />
        <div v-else-if="session.ready && !session.companyId" class="fatal muted-box">
          <i class="pi pi-building" />
          <template v-if="isSuperAdmin()">
            <p>Zatiaľ tu nie je žiadna firma.</p>
            <RouterLink to="/firmy">Založiť firmu</RouterLink>
          </template>
          <p v-else>Nemáte prístup do žiadnej firmy. Požiadajte správcu, aby vás pridal.</p>
        </div>
        <div v-else-if="session.ready && !allowed" class="fatal muted-box">
          <i class="pi pi-lock" />
          <p>Na túto časť nemáte oprávnenie.</p>
        </div>
      </main>
    </div>

    <Toast position="bottom-right" />
    <ConfirmDialog />
  </div>
</template>

<style scoped>
.shell {
  display: grid;
  grid-template-columns: var(--sidebar-w) 1fr;
  height: 100%;
}

.sidebar {
  background: var(--blue-deep);
  color: #c9d5e3;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  height: var(--topbar-h);
  padding: 0 16px;
  color: #fff;
  text-decoration: none;
  font-size: var(--fs-lg);
  font-weight: 700;
  letter-spacing: 0.01em;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

nav {
  padding: 6px 0 12px;
  flex: 1;
}

.nav-group h2 {
  font-size: var(--fs-xs);
  font-weight: 600;
  color: #8ea3bd;
  padding: 12px 16px 4px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 5px 16px;
  color: #dbe4ee;
  text-decoration: none;
  font-size: var(--fs-md);
  border-left: 3px solid transparent;
}
.nav-item .pi {
  font-size: 0.875rem;
  width: 16px;
  color: #8ea3bd;
}
.nav-item:hover {
  background: rgba(255, 255, 255, 0.06);
}
.nav-item.pending {
  color: #9fb0c5;
}
.nav-item.active {
  background: rgba(255, 255, 255, 0.1);
  border-left-color: var(--brass);
  color: #fff;
  font-weight: 600;
}
.nav-item.active .pi {
  color: var(--brass);
}

.edition {
  padding: 10px 16px;
  font-size: var(--fs-xs);
  color: #8ea3bd;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.main {
  display: flex;
  flex-direction: column;
  min-width: 0;
  height: 100%;
}

.topbar {
  height: var(--topbar-h);
  flex: none;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  background: var(--sheet);
  border-bottom: 1px solid var(--rule);
}

.company {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
}
.company strong {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.muted {
  color: var(--ink-faint);
  font-size: var(--fs-sm);
  white-space: nowrap;
}

.menu-btn {
  display: none;
  border: 0;
  background: none;
  padding: 6px;
  margin-left: -6px;
  color: var(--ink);
  cursor: pointer;
}

.content {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.fatal {
  margin: 48px auto;
  max-width: 440px;
  text-align: center;
  color: var(--tone-overdue);
}
.muted-box {
  color: var(--ink-faint);
}
.fatal .pi {
  font-size: 1.5rem;
}

.scrim {
  display: none;
}

@media (max-width: 860px) {
  .shell {
    grid-template-columns: 1fr;
  }
  .sidebar {
    position: fixed;
    inset: 0 auto 0 0;
    width: 260px;
    z-index: 30;
    transform: translateX(-100%);
    transition: transform 0.18s ease-out;
  }
  .nav-open .sidebar {
    transform: none;
  }
  .nav-open .scrim {
    display: block;
    position: fixed;
    inset: 0;
    z-index: 20;
    background: rgba(18, 24, 32, 0.4);
  }
  .menu-btn {
    display: block;
  }
  .topbar {
    padding: 0 12px;
  }
  .company .muted {
    display: none;
  }
}
</style>
