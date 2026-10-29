import { createRouter, createWebHistory } from 'vue-router'
import InvoiceList from './views/InvoiceList.vue'
import InvoiceEdit from './views/InvoiceEdit.vue'
import Placeholder from './views/Placeholder.vue'
import Contacts from './views/Contacts.vue'
import Settings from './views/Settings.vue'
import InvoicePrint from './views/InvoicePrint.vue'
import PurchaseList from './views/PurchaseList.vue'
import PurchaseEdit from './views/PurchaseEdit.vue'
import Users from './views/Users.vue'
import AuditLog from './views/AuditLog.vue'

// Moduly z etapového plánu — zatiaľ prázdne stránky s popisom.
export const modules = [
  {
    group: 'Predaj',
    items: [
      { path: '/faktury', label: 'Vydané faktúry', icon: 'pi-file-export', ready: true, perm: 'invoices.read' },
      { path: '/ponuky', label: 'Cenové ponuky', icon: 'pi-file-edit', stage: 1, perm: 'invoices.read' },
      { path: '/pravidelne', label: 'Pravidelná fakturácia', icon: 'pi-replay', stage: 1, perm: 'invoices.read' },
    ],
  },
  {
    group: 'Nákup',
    items: [{ path: '/prijate', label: 'Prijaté faktúry', icon: 'pi-file-import', ready: true, perm: 'purchases.read' }],
  },
  {
    group: 'Peniaze',
    items: [
      { path: '/banka', label: 'Banka', icon: 'pi-building-columns', stage: 1, perm: 'purchases.read' },
      { path: '/pokladna', label: 'Pokladňa', icon: 'pi-wallet', stage: 1, perm: 'purchases.read' },
    ],
  },
  {
    group: 'Dane',
    items: [
      { path: '/dph', label: 'Priznanie DPH', icon: 'pi-percentage', stage: 1, perm: 'purchases.read' },
      { path: '/kontrolny-vykaz', label: 'Kontrolný výkaz', icon: 'pi-list-check', stage: 1, perm: 'purchases.read' },
    ],
  },
  {
    group: 'Účtovníctvo',
    items: [
      { path: '/dennik', label: 'Denník', icon: 'pi-book', stage: 2, perm: 'purchases.read' },
      { path: '/hlavna-kniha', label: 'Hlavná kniha', icon: 'pi-table', stage: 2, perm: 'purchases.read' },
      { path: '/majetok', label: 'Majetok', icon: 'pi-car', stage: 3, perm: 'purchases.read' },
      { path: '/sklad', label: 'Sklad', icon: 'pi-box', stage: 3, perm: 'purchases.read' },
    ],
  },
  {
    group: 'Ostatné',
    items: [
      { path: '/adresar', label: 'Adresár', icon: 'pi-address-book', ready: true, perm: 'contacts.read' },
      { path: '/pouzivatelia', label: 'Používatelia', icon: 'pi-users', ready: true, perm: 'users.manage', serverOnly: true },
      { path: '/audit', label: 'Audit', icon: 'pi-shield', ready: true, perm: 'audit.read' },
      { path: '/zalohy', label: 'Zálohy', icon: 'pi-history', stage: 1, perm: 'settings.write' },
      { path: '/nastavenia', label: 'Nastavenia', icon: 'pi-cog', ready: true, perm: 'settings.write' },
    ],
  },
]

const placeholderRoutes = modules
  .flatMap((g) => g.items)
  .filter((i) => !i.ready)
  .map((i) => ({
    path: i.path,
    component: Placeholder,
    props: { title: i.label, stage: i.stage },
    meta: { title: i.label, perm: i.perm },
  }))

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/faktury' },
    { path: '/faktury', component: InvoiceList, meta: { title: 'Vydané faktúry', perm: 'invoices.read' } },
    { path: '/faktury/nova', component: InvoiceEdit, meta: { title: 'Nová faktúra', perm: 'invoices.write' } },
    { path: '/faktury/:id(\\d+)', component: InvoiceEdit, props: true, meta: { title: 'Faktúra', perm: 'invoices.read' } },
    { path: '/faktury/:id(\\d+)/tlac', component: InvoicePrint, props: true, meta: { title: 'Faktúra', bare: true, perm: 'invoices.read' } },
    { path: '/prijate', component: PurchaseList, meta: { title: 'Prijaté faktúry', perm: 'purchases.read' } },
    { path: '/prijate/nova', component: PurchaseEdit, meta: { title: 'Nová prijatá faktúra', perm: 'purchases.write' } },
    { path: '/prijate/:id(\\d+)', component: PurchaseEdit, props: true, meta: { title: 'Prijatá faktúra', perm: 'purchases.read' } },
    { path: '/adresar', component: Contacts, meta: { title: 'Adresár', perm: 'contacts.read' } },
    { path: '/adresar/:id(\\d+|novy)', component: Contacts, props: true, meta: { title: 'Adresár', perm: 'contacts.read' } },
    { path: '/nastavenia', component: Settings, meta: { title: 'Nastavenia', perm: 'settings.write' } },
    { path: '/pouzivatelia', component: Users, meta: { title: 'Používatelia', perm: 'users.manage' } },
    { path: '/audit', component: AuditLog, meta: { title: 'Audit', perm: 'audit.read' } },
    ...placeholderRoutes,
    { path: '/:pathMatch(.*)*', redirect: '/faktury' },
  ],
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} – Fiskus` : 'Fiskus'
})
