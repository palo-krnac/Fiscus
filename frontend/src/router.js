import { createRouter, createWebHistory } from 'vue-router'
import InvoiceList from './views/InvoiceList.vue'
import InvoiceEdit from './views/InvoiceEdit.vue'
import Placeholder from './views/Placeholder.vue'
import Contacts from './views/Contacts.vue'
import Settings from './views/Settings.vue'

// Moduly z etapového plánu — zatiaľ prázdne stránky s popisom.
export const modules = [
  {
    group: 'Predaj',
    items: [
      { path: '/faktury', label: 'Vydané faktúry', icon: 'pi-file-export', ready: true },
      { path: '/ponuky', label: 'Cenové ponuky', icon: 'pi-file-edit', stage: 1 },
      { path: '/pravidelne', label: 'Pravidelná fakturácia', icon: 'pi-replay', stage: 1 },
    ],
  },
  {
    group: 'Nákup',
    items: [{ path: '/prijate', label: 'Prijaté faktúry', icon: 'pi-file-import', stage: 1 }],
  },
  {
    group: 'Peniaze',
    items: [
      { path: '/banka', label: 'Banka', icon: 'pi-building-columns', stage: 1 },
      { path: '/pokladna', label: 'Pokladňa', icon: 'pi-wallet', stage: 1 },
    ],
  },
  {
    group: 'Dane',
    items: [
      { path: '/dph', label: 'Priznanie DPH', icon: 'pi-percentage', stage: 1 },
      { path: '/kontrolny-vykaz', label: 'Kontrolný výkaz', icon: 'pi-list-check', stage: 1 },
    ],
  },
  {
    group: 'Účtovníctvo',
    items: [
      { path: '/dennik', label: 'Denník', icon: 'pi-book', stage: 2 },
      { path: '/hlavna-kniha', label: 'Hlavná kniha', icon: 'pi-table', stage: 2 },
      { path: '/majetok', label: 'Majetok', icon: 'pi-car', stage: 3 },
      { path: '/sklad', label: 'Sklad', icon: 'pi-box', stage: 3 },
    ],
  },
  {
    group: 'Ostatné',
    items: [
      { path: '/adresar', label: 'Adresár', icon: 'pi-address-book', ready: true },
      { path: '/zalohy', label: 'Zálohy', icon: 'pi-history', stage: 1 },
      { path: '/nastavenia', label: 'Nastavenia', icon: 'pi-cog', ready: true },
    ],
  },
]

const placeholderRoutes = modules
  .flatMap((g) => g.items)
  .filter((i) => !i.ready)
  .map((i) => ({ path: i.path, component: Placeholder, props: { title: i.label, stage: i.stage }, meta: { title: i.label } }))

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/faktury' },
    { path: '/faktury', component: InvoiceList, meta: { title: 'Vydané faktúry' } },
    { path: '/faktury/nova', component: InvoiceEdit, meta: { title: 'Nová faktúra' } },
    { path: '/faktury/:id(\\d+)', component: InvoiceEdit, props: true, meta: { title: 'Faktúra' } },
    { path: '/adresar', component: Contacts, meta: { title: 'Adresár' } },
    { path: '/adresar/:id(\\d+|novy)', component: Contacts, props: true, meta: { title: 'Adresár' } },
    { path: '/nastavenia', component: Settings, meta: { title: 'Nastavenia' } },
    ...placeholderRoutes,
    { path: '/:pathMatch(.*)*', redirect: '/faktury' },
  ],
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} – Fiskus` : 'Fiskus'
})
