import { createRouter, createWebHistory } from 'vue-router'
import InvoiceList from './views/InvoiceList.vue'
import InvoiceEdit from './views/InvoiceEdit.vue'
import OAuthConsent from './views/OAuthConsent.vue'
import JournalBook from './views/JournalBook.vue'
import OpenItems from './views/OpenItems.vue'
import Placeholder from './views/Placeholder.vue'
import Contacts from './views/Contacts.vue'
import Settings from './views/Settings.vue'
import InvoicePrint from './views/InvoicePrint.vue'
import PurchaseList from './views/PurchaseList.vue'
import PurchaseEdit from './views/PurchaseEdit.vue'
import Users from './views/Users.vue'
import AuditLog from './views/AuditLog.vue'
import Companies from './views/Companies.vue'
import VatReturn from './views/VatReturn.vue'
import Bank from './views/Bank.vue'
import AiAccess from './views/AiAccess.vue'
import Cash from './views/Cash.vue'
import Backups from './views/Backups.vue'
import CashPrint from './views/CashPrint.vue'
import QuoteList from './views/QuoteList.vue'
import RecurringList from './views/RecurringList.vue'
import Reminders from './views/Reminders.vue'
import Emails from './views/Emails.vue'
import Rates from './views/Rates.vue'
import ReminderPrint from './views/ReminderPrint.vue'
import Legal from './views/Legal.vue'
import Catalog from './views/Catalog.vue'

// Moduly z etapového plánu — zatiaľ prázdne stránky s popisom.
export const modules = [
  {
    group: 'Predaj',
    items: [
      { path: '/faktury', label: 'Vydané faktúry', icon: 'pi-file-export', ready: true, perm: 'invoices.read' },
      { path: '/ponuky', label: 'Cenové ponuky', icon: 'pi-file-edit', ready: true, perm: 'invoices.read' },
      { path: '/pravidelne', label: 'Pravidelná fakturácia', icon: 'pi-replay', ready: true, perm: 'invoices.read' },
      { path: '/upomienky', label: 'Upomienky', icon: 'pi-bell', ready: true, perm: 'invoices.read' },
      { path: '/emaily', label: 'E-maily', icon: 'pi-envelope', ready: true, perm: 'invoices.read' },
      { path: '/cennik', label: 'Cenník', icon: 'pi-tags', ready: true, perm: 'invoices.read' },
    ],
  },
  {
    group: 'Nákup',
    items: [{ path: '/prijate', label: 'Prijaté faktúry', icon: 'pi-file-import', ready: true, perm: 'purchases.read' }],
  },
  {
    group: 'Peniaze',
    items: [
      { path: '/banka', label: 'Banka', icon: 'pi-building-columns', ready: true, perm: 'bank.read' },
      { path: '/pokladna', label: 'Pokladňa', icon: 'pi-wallet', ready: true, perm: 'cash.read' },
    ],
  },
  {
    group: 'Dane',
    items: [
      { path: '/dph', label: 'Priznanie DPH', icon: 'pi-percentage', ready: true, perm: 'vat.read' },
      { path: '/kontrolny-vykaz', label: 'Kontrolný výkaz', icon: 'pi-list-check', ready: true, perm: 'vat.read' },
      { path: '/suhrnny-vykaz', label: 'Súhrnný výkaz', icon: 'pi-globe', ready: true, perm: 'vat.read' },
    ],
  },
  {
    group: 'Účtovníctvo',
    items: [
      { path: '/dennik', label: 'Peňažný denník', icon: 'pi-book', ready: true, perm: 'ledger.read' },
      { path: '/pohladavky-zavazky', label: 'Pohľadávky a záväzky', icon: 'pi-list', ready: true, perm: 'ledger.read' },
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
      { path: '/ai', label: 'Prístup pre AI', icon: 'pi-sparkles', ready: true },
      { path: '/zalohy', label: 'Zálohy', icon: 'pi-history', ready: true, superAdmin: true },
      { path: '/kurzy', label: 'Kurzy mien', icon: 'pi-euro', ready: true, perm: 'invoices.read' },
      { path: '/legislativa', label: 'Legislatíva', icon: 'pi-verified', ready: true, superAdmin: true },
      { path: '/nastavenia', label: 'Nastavenia', icon: 'pi-cog', ready: true, perm: 'settings.write' },
      { path: '/firmy', label: 'Firmy a licencia', icon: 'pi-building', ready: true, superAdmin: true, serverOnly: true },
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
    { path: '/oauth/povolit', component: OAuthConsent, meta: { title: 'Pripojenie aplikácie', consent: true } },
    { path: '/faktury', component: InvoiceList, meta: { title: 'Vydané faktúry', perm: 'invoices.read' } },
    { path: '/faktury/nova', component: InvoiceEdit, meta: { title: 'Nová faktúra', perm: 'invoices.write' } },
    { path: '/faktury/:id(\\d+)', component: InvoiceEdit, props: true, meta: { title: 'Faktúra', perm: 'invoices.read' } },
    { path: '/faktury/:id(\\d+)/tlac', component: InvoicePrint, props: true, meta: { title: 'Faktúra', bare: true, perm: 'invoices.read' } },
    { path: '/ponuky', component: QuoteList, meta: { title: 'Cenové ponuky', perm: 'invoices.read' } },
    { path: '/ponuky/nova', component: InvoiceEdit, props: { quote: true }, meta: { title: 'Nová ponuka', perm: 'invoices.write' } },
    { path: '/ponuky/:id(\\d+)', component: InvoiceEdit, props: (r) => ({ id: r.params.id, quote: true }), meta: { title: 'Cenová ponuka', perm: 'invoices.read' } },
    { path: '/ponuky/:id(\\d+)/tlac', component: InvoicePrint, props: (r) => ({ id: r.params.id, quote: true }), meta: { title: 'Cenová ponuka', bare: true, perm: 'invoices.read' } },
    { path: '/pravidelne', component: RecurringList, meta: { title: 'Pravidelná fakturácia', perm: 'invoices.read' } },
    { path: '/pravidelne/nova', component: InvoiceEdit, props: { recurring: true }, meta: { title: 'Nová pravidelná faktúra', perm: 'invoices.write' } },
    { path: '/pravidelne/:id(\\d+)', component: InvoiceEdit, props: (r) => ({ id: r.params.id, recurring: true }), meta: { title: 'Pravidelná faktúra', perm: 'invoices.read' } },
    { path: '/cennik', component: Catalog, meta: { title: 'Cenník', perm: 'invoices.read' } },
    { path: '/cennik/:id(\\d+|nova)', component: Catalog, props: true, meta: { title: 'Cenník', perm: 'invoices.read' } },
    { path: '/upomienky', component: Reminders, meta: { title: 'Upomienky', perm: 'invoices.read' } },
    { path: '/emaily', component: Emails, meta: { title: 'E-maily', perm: 'invoices.read' } },
    { path: '/upomienky/:id(\\d+)/tlac', component: ReminderPrint, props: true, meta: { title: 'Upomienka', bare: true, perm: 'invoices.read' } },
    { path: '/prijate', component: PurchaseList, meta: { title: 'Prijaté faktúry', perm: 'purchases.read' } },
    { path: '/pohladavky-zavazky', component: OpenItems, meta: { title: 'Pohľadávky a záväzky', perm: 'ledger.read' } },
    { path: '/dennik', component: JournalBook, meta: { title: 'Peňažný denník', perm: 'ledger.read' } },
    { path: '/prijate/nova', component: PurchaseEdit, meta: { title: 'Nová prijatá faktúra', perm: 'purchases.write' } },
    { path: '/prijate/:id(\\d+)', component: PurchaseEdit, props: true, meta: { title: 'Prijatá faktúra', perm: 'purchases.read' } },
    { path: '/adresar', component: Contacts, meta: { title: 'Adresár', perm: 'contacts.read' } },
    { path: '/adresar/:id(\\d+|novy)', component: Contacts, props: true, meta: { title: 'Adresár', perm: 'contacts.read' } },
    { path: '/nastavenia', component: Settings, meta: { title: 'Nastavenia', perm: 'settings.write' } },
    { path: '/pouzivatelia', component: Users, meta: { title: 'Používatelia', perm: 'users.manage' } },
    { path: '/ai', component: AiAccess, meta: { title: 'Prístup pre AI', noCompany: true } },
    { path: '/kurzy', component: Rates, meta: { title: 'Kurzy mien', perm: 'invoices.read' } },
    { path: '/legislativa', component: Legal, meta: { title: 'Legislatíva', superAdmin: true, noCompany: true } },
    { path: '/zalohy', component: Backups, meta: { title: 'Zálohy', superAdmin: true, noCompany: true } },
    { path: '/pokladna', component: Cash, meta: { title: 'Pokladňa', perm: 'cash.read' } },
    { path: '/pokladna/:id(\\d+)/tlac', component: CashPrint, props: true, meta: { title: 'Pokladničný doklad', bare: true, perm: 'cash.read' } },
    { path: '/banka', component: Bank, meta: { title: 'Banka', perm: 'bank.read' } },
    { path: '/dph', component: VatReturn, props: { tab: 'dph' }, meta: { title: 'Priznanie k DPH', perm: 'vat.read' } },
    { path: '/kontrolny-vykaz', component: VatReturn, props: { tab: 'kv' }, meta: { title: 'Kontrolný výkaz', perm: 'vat.read' } },
    { path: '/suhrnny-vykaz', component: VatReturn, props: { tab: 'sv' }, meta: { title: 'Súhrnný výkaz', perm: 'vat.read' } },
    { path: '/firmy', component: Companies, meta: { title: 'Firmy a licencia', superAdmin: true, noCompany: true } },
    { path: '/audit', component: AuditLog, meta: { title: 'Audit', perm: 'audit.read' } },
    ...placeholderRoutes,
    { path: '/:pathMatch(.*)*', redirect: '/faktury' },
  ],
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} – Fiškus` : 'Fiškus'
})
