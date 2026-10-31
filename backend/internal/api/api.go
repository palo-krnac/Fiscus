// Package api je HTTP/JSON rozhranie. Rovnaké API používa desktopová aj
// webová verzia — frontend je jeden.
package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/mcp"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/registry"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Server obsluhuje API a statický frontend.
type Server struct {
	App *app.App
	Web fs.FS // zostavený frontend (index.html, assets/...); môže byť nil
	// Registry je vyhľadávanie firiem podľa IČO (RPO); nil = vypnuté.
	Registry registry.Lookup
	// VAT overuje IČ DPH vo VIES; nil = vypnuté.
	VAT registry.VATChecker

	// MCP je server pre AI asistentov (POST /mcp); nil = vypnuté.
	MCP *mcp.Server

	// PublicURL je verejná adresa servera (https://…) pre OAuth; prázdna =
	// podľa požiadavky (Host a X-Forwarded-Proto).
	PublicURL string

	// Version je verzia programu (zobrazí sa v rozhraní).
	Version string

	// Desktop je spúšťač desktopovej verzie z inštalátora (okno sa ozýva,
	// tlačidlo Ukončiť); nil pri spustení z príkazového riadka a na serveri.
	Desktop Launcher

	limit limiter // pokusy o prihlásenie
	// shareLimit: neplatné odkazy na ponuky z jednej IP (hádanie tokenov)
	shareLimit limiter
}

// Launcher ukončí program po zatvorení okna alebo na pokyn používateľa.
type Launcher interface {
	Ping()
	Quit()
}

// Handler vráti http.Handler s celým API.
//
// Každá adresa okrem prihlásenia a číselníkov prechádza cez s.authed
// (kto to je) a s.need (či na to má vo firme právo). Zmeny (POST/PUT/DELETE)
// musia mať hlavičku X-Fiskus — obrana proti CSRF, aj pre lokálny desktop.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	public := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, s.csrf(h)) }
	open := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, s.csrf(s.authed(h))) }
	need := func(pattern string, p auth.Perm, h http.HandlerFunc) {
		mux.HandleFunc(pattern, s.csrf(s.authed(s.need(p, h))))
	}

	public("GET /api/capabilities", s.capabilities)
	public("GET /api/codes", s.codes)
	if s.Desktop != nil {
		public("POST /api/desktop/ping", func(w http.ResponseWriter, _ *http.Request) {
			s.Desktop.Ping()
			w.WriteHeader(http.StatusNoContent)
		})
		public("POST /api/desktop/quit", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
			s.Desktop.Quit()
		})
	}
	if s.MCP != nil {
		mux.HandleFunc("/mcp", s.mcpHandler)
	}
	open("GET /api/backups", s.backupStatus)
	open("POST /api/backups", s.createBackup)
	open("DELETE /api/backups/restore", s.cancelRestore)
	open("PUT /api/backups/settings", s.setBackupSettings)
	open("PUT /api/backups/remote", s.setBackupRemote)
	open("GET /api/backups/remote", s.remoteBackups)
	open("POST /api/backups/remote/{name}/fetch", s.fetchRemoteBackup)
	open("GET /api/backups/{name}", s.downloadBackup)
	open("POST /api/backups/{name}/restore", s.restoreBackup)
	open("GET /api/me/keys", s.listKeys)
	open("POST /api/me/keys", s.createKey)
	open("DELETE /api/me/keys/{id}", s.revokeKey)
	open("GET /api/ai/settings", s.getAISettings)
	open("PUT /api/ai/settings", s.putAISettings)
	public("POST /api/auth/login", s.login)
	public("POST /api/auth/mfa", s.loginMFA)
	open("GET /api/me/mfa", s.myMFA)
	open("POST /api/me/mfa/setup", s.setupMFA)
	open("POST /api/me/mfa/enable", s.enableMFA)
	open("POST /api/me/mfa/disable", s.disableMFA)
	open("POST /api/me/mfa/recovery", s.recoveryMFA)
	open("POST /api/users/{uid}/mfa-reset", s.resetMFA)
	open("POST /api/auth/logout", s.logout)
	public("POST /api/auth/pin", s.unlockPIN)
	public("POST /api/auth/lock", s.lockPIN)
	open("GET /api/security", s.getSecurity)
	open("PUT /api/security/pin", s.setPIN)
	open("PUT /api/security/require-mfa", s.setRequireMFA)
	open("POST /api/auth/password", s.changePassword)
	open("GET /api/me", s.me)
	open("GET /api/license", s.getLicense)
	open("POST /api/license", s.setLicense)
	open("GET /api/registry/{ico}", s.lookupICO)
	open("GET /api/vies/{vatid}", s.checkVAT)
	open("GET /api/legal", s.getLegal)
	open("PUT /api/legal", s.setLegal)
	open("POST /api/legal/reset", s.resetLegal)
	open("POST /api/legal/ecb", s.updateECBRates)
	open("GET /api/rates", s.listRates)
	open("PUT /api/rates", s.setRate)
	open("POST /api/rates/update", s.updateRates)
	open("DELETE /api/rates/{currency}/{date}", s.deleteRate)

	open("GET /api/companies", s.listCompanies)
	open("POST /api/companies", s.createCompany)
	need("PUT /api/companies/{cid}", auth.PermSettingsWrite, s.updateCompany)
	open("GET /api/companies/{cid}/logo", s.getLogo)
	open("PUT /api/companies/{cid}/logo", s.putLogo)
	open("DELETE /api/companies/{cid}/logo", s.deleteLogo)

	need("GET /api/companies/{cid}/invoices", auth.PermInvoicesRead, s.listInvoices)
	need("GET /api/companies/{cid}/invoices/{id}", auth.PermInvoicesRead, s.getInvoice)
	need("GET /api/companies/{cid}/invoices/{id}/email", auth.PermInvoicesRead, s.mailDoc(app.MailInvoice))
	need("POST /api/companies/{cid}/invoices/{id}/email", auth.PermInvoicesWrite, s.mailDoc(app.MailInvoice))
	need("GET /api/companies/{cid}/invoices/{id}/pdf", auth.PermInvoicesRead, s.pdfHandler((*app.App).InvoicePDF, fail))
	need("GET /api/companies/{cid}/invoices/{id}/ubl", auth.PermInvoicesRead, s.pdfHandler((*app.App).InvoiceUBL, failUBL))
	need("POST /api/companies/{cid}/invoices", auth.PermInvoicesWrite, s.saveInvoice)
	need("POST /api/companies/{cid}/invoices/preview", auth.PermInvoicesWrite, s.preview)
	need("PUT /api/companies/{cid}/invoices/{id}", auth.PermInvoicesWrite, s.saveInvoice)
	need("POST /api/companies/{cid}/invoices/{id}/issue", auth.PermInvoicesWrite, s.issue)
	need("POST /api/companies/{cid}/invoices/{id}/pay", auth.PermInvoicesWrite, s.pay)
	need("POST /api/companies/{cid}/invoices/{id}/cancel", auth.PermInvoicesWrite, s.cancel)
	need("POST /api/companies/{cid}/invoices/{id}/credit-note", auth.PermInvoicesWrite, s.creditNote)
	need("POST /api/companies/{cid}/invoices/{id}/settle", auth.PermInvoicesWrite, s.settleAdvance)
	need("POST /api/companies/{cid}/invoices/{id}/receipts", auth.PermInvoicesWrite, s.issueMissingReceipts)
	need("GET /api/companies/{cid}/invoices/{id}/advances", auth.PermInvoicesRead, s.advanceCandidates)
	need("POST /api/companies/{cid}/invoices/{id}/advances/{pid}", auth.PermInvoicesWrite, s.addAdvance)
	need("DELETE /api/companies/{cid}/invoices/{id}/advances/{pid}", auth.PermInvoicesWrite, s.removeAdvance)

	need("GET /api/companies/{cid}/quotes", auth.PermInvoicesRead, s.listQuotes)
	need("GET /api/companies/{cid}/quotes/{id}", auth.PermInvoicesRead, s.getQuote)
	need("GET /api/companies/{cid}/quotes/{id}/email", auth.PermInvoicesRead, s.mailDoc(app.MailQuote))
	need("POST /api/companies/{cid}/quotes/{id}/email", auth.PermInvoicesWrite, s.mailDoc(app.MailQuote))
	need("GET /api/companies/{cid}/quotes/{id}/pdf", auth.PermInvoicesRead, s.pdfHandler((*app.App).QuotePDF, failQuote))
	need("POST /api/companies/{cid}/quotes", auth.PermInvoicesWrite, s.saveQuote)
	need("POST /api/companies/{cid}/quotes/preview", auth.PermInvoicesWrite, s.previewQuote)
	need("PUT /api/companies/{cid}/quotes/{id}", auth.PermInvoicesWrite, s.saveQuote)
	need("POST /api/companies/{cid}/quotes/{id}/issue", auth.PermInvoicesWrite, s.quoteAction((*Server).issueQuote))
	need("POST /api/companies/{cid}/quotes/{id}/accept", auth.PermInvoicesWrite, s.quoteAction((*Server).acceptQuote))
	need("POST /api/companies/{cid}/quotes/{id}/reject", auth.PermInvoicesWrite, s.quoteAction((*Server).rejectQuote))
	need("POST /api/companies/{cid}/quotes/{id}/copy", auth.PermInvoicesWrite, s.quoteAction((*Server).copyQuote))
	need("POST /api/companies/{cid}/quotes/{id}/invoice", auth.PermInvoicesWrite, s.invoiceQuote)
	need("POST /api/companies/{cid}/quotes/{id}/share", auth.PermInvoicesWrite, s.quoteAction((*Server).shareQuote))
	need("DELETE /api/companies/{cid}/quotes/{id}/share", auth.PermInvoicesWrite, s.quoteAction((*Server).unshareQuote))
	// ponuka pre zákazníka cez tajný odkaz (bez prihlásenia)
	public("GET /api/public/quotes/{token}", s.publicQuote)
	public("GET /api/public/quotes/{token}/pdf", s.publicQuotePDF)
	public("POST /api/public/quotes/{token}/respond", s.publicQuoteRespond)

	need("GET /api/companies/{cid}/recurring", auth.PermInvoicesRead, s.listRecurring)
	need("GET /api/companies/{cid}/recurring/drafts", auth.PermInvoicesRead, s.recurringDrafts)
	need("GET /api/companies/{cid}/recurring/{id}", auth.PermInvoicesRead, s.getRecurring)
	need("POST /api/companies/{cid}/recurring", auth.PermInvoicesWrite, s.saveRecurring)
	need("POST /api/companies/{cid}/recurring/preview", auth.PermInvoicesWrite, s.previewRecurring)
	need("PUT /api/companies/{cid}/recurring/{id}", auth.PermInvoicesWrite, s.saveRecurring)
	need("DELETE /api/companies/{cid}/recurring/{id}", auth.PermInvoicesWrite, s.deleteRecurring)
	need("POST /api/companies/{cid}/recurring/{id}/pause", auth.PermInvoicesWrite, s.setRecurringActive(false))
	need("POST /api/companies/{cid}/recurring/{id}/resume", auth.PermInvoicesWrite, s.setRecurringActive(true))
	need("POST /api/companies/{cid}/recurring/{id}/generate", auth.PermInvoicesWrite, s.generateRecurring)

	need("GET /api/companies/{cid}/overdue", auth.PermInvoicesRead, s.overdue)
	need("GET /api/companies/{cid}/reminders", auth.PermInvoicesRead, s.listReminders)
	need("GET /api/companies/{cid}/reminders/{id}", auth.PermInvoicesRead, s.getReminder)
	need("GET /api/companies/{cid}/reminders/{id}/email", auth.PermInvoicesRead, s.mailDoc(app.MailReminder))
	need("POST /api/companies/{cid}/reminders/{id}/email", auth.PermInvoicesWrite, s.mailDoc(app.MailReminder))
	need("GET /api/companies/{cid}/emails", auth.PermInvoicesRead, s.listEmails)
	need("GET /api/companies/{cid}/einvoice-settings", auth.PermInvoicesRead, s.getEInvoiceSettings)
	need("PUT /api/companies/{cid}/einvoice-settings", auth.PermSettingsWrite, s.putEInvoiceSettings)
	need("POST /api/companies/{cid}/invoices/{id}/einvoice", auth.PermInvoicesWrite, s.sendEInvoice)
	need("GET /api/companies/{cid}/einvoices", auth.PermInvoicesRead, s.listEInvoices)
	need("POST /api/companies/{cid}/einvoices/{id}/reopen", auth.PermPurchasesWrite, s.reopenEInvoice)
	need("GET /api/companies/{cid}/einvoice-inbox", auth.PermPurchasesWrite, s.einvoiceInbox)
	need("POST /api/companies/{cid}/einvoice-inbox/{msg}/take", auth.PermPurchasesWrite, s.takeEInvoice)
	need("GET /api/companies/{cid}/mail-settings", auth.PermSettingsWrite, s.getMailSettings)
	need("PUT /api/companies/{cid}/mail-settings", auth.PermSettingsWrite, s.putMailSettings)
	need("POST /api/companies/{cid}/mail-settings/test", auth.PermSettingsWrite, s.testMail)
	need("GET /api/companies/{cid}/reminders/{id}/pdf", auth.PermInvoicesRead, s.pdfHandler((*app.App).ReminderPDF, failReminder))
	need("POST /api/companies/{cid}/reminders/preview", auth.PermInvoicesWrite, s.reminderRequest(false))
	need("POST /api/companies/{cid}/reminders", auth.PermInvoicesWrite, s.reminderRequest(true))
	need("DELETE /api/companies/{cid}/reminders/{id}", auth.PermInvoicesWrite, s.deleteReminder)

	need("GET /api/companies/{cid}/items", auth.PermInvoicesRead, s.listItems)
	need("POST /api/companies/{cid}/items", auth.PermInvoicesWrite, s.saveItem)
	need("POST /api/companies/{cid}/items/import", auth.PermInvoicesWrite, s.importItems)
	need("POST /api/companies/{cid}/import/{source}", auth.PermInvoicesWrite, s.importMigrate)
	need("POST /api/companies/{cid}/contacts/import", auth.PermContactsWrite, s.importContacts)
	need("POST /api/companies/{cid}/purchases/extract", auth.PermPurchasesWrite, s.extractPurchase)
	need("POST /api/companies/{cid}/purchases/import-ubl", auth.PermPurchasesWrite, s.importUBL)
	need("PUT /api/companies/{cid}/items/{id}", auth.PermInvoicesWrite, s.saveItem)
	need("DELETE /api/companies/{cid}/items/{id}", auth.PermInvoicesWrite, s.deleteItem)

	need("GET /api/companies/{cid}/purchases", auth.PermPurchasesRead, s.listPurchases)
	need("GET /api/companies/{cid}/purchases/payment-order", auth.PermPurchasesWrite, s.paymentCandidates)
	need("POST /api/companies/{cid}/purchases/payment-order", auth.PermPurchasesWrite, s.paymentOrder)
	need("GET /api/companies/{cid}/purchases/{id}", auth.PermPurchasesRead, s.getPurchase)
	need("POST /api/companies/{cid}/purchases", auth.PermPurchasesWrite, s.savePurchase)
	need("POST /api/companies/{cid}/purchases/preview", auth.PermPurchasesWrite, s.previewPurchase)
	need("PUT /api/companies/{cid}/purchases/{id}", auth.PermPurchasesWrite, s.savePurchase)
	need("DELETE /api/companies/{cid}/purchases/{id}", auth.PermPurchasesWrite, s.deletePurchase)
	need("POST /api/companies/{cid}/purchases/{id}/pay", auth.PermPurchasesWrite, s.payPurchase)
	need("POST /api/companies/{cid}/purchases/{id}/unpay", auth.PermPurchasesWrite, s.unpayPurchase)

	need("GET /api/companies/{cid}/contacts", auth.PermContactsRead, s.listContacts)
	need("GET /api/companies/{cid}/contacts/{id}", auth.PermContactsRead, s.getContact)
	need("POST /api/companies/{cid}/contacts", auth.PermContactsWrite, s.saveContact)
	need("PUT /api/companies/{cid}/contacts/{id}", auth.PermContactsWrite, s.saveContact)
	need("DELETE /api/companies/{cid}/contacts/{id}", auth.PermContactsWrite, s.deleteContact)

	need("GET /api/companies/{cid}/journal", auth.PermLedgerRead, s.getJournal)
	need("PUT /api/companies/{cid}/journal/opening", auth.PermLedgerWrite, s.setJournalOpening)
	need("POST /api/companies/{cid}/journal/closing", auth.PermLedgerWrite, s.addClosing)
	need("GET /api/companies/{cid}/open-items", auth.PermLedgerRead, s.openItems)
	need("GET /api/companies/{cid}/statements", auth.PermLedgerRead, s.getStatements)
	need("PUT /api/companies/{cid}/statements/{year}", auth.PermLedgerWrite, s.setStatementInputs)
	need("GET /api/companies/{cid}/vehicles", auth.PermLedgerRead, s.listVehicles)
	need("POST /api/companies/{cid}/vehicles", auth.PermLedgerWrite, s.saveVehicle)
	need("PUT /api/companies/{cid}/vehicles/{id}", auth.PermLedgerWrite, s.saveVehicle)
	need("DELETE /api/companies/{cid}/vehicles/{id}", auth.PermLedgerWrite, s.deleteVehicle)
	need("GET /api/companies/{cid}/vehicles/{id}/trips", auth.PermLedgerRead, s.listTrips)
	need("GET /api/companies/{cid}/vehicles/{id}/fuel", auth.PermLedgerRead, s.listFuel)
	need("GET /api/companies/{cid}/vehicles/{id}/report", auth.PermLedgerRead, s.vehicleReport)
	need("POST /api/companies/{cid}/vehicles/{id}/report/{year}", auth.PermLedgerWrite, s.postLogbook)
	need("POST /api/companies/{cid}/trips", auth.PermLedgerWrite, s.saveTrip)
	need("PUT /api/companies/{cid}/trips/{id}", auth.PermLedgerWrite, s.saveTrip)
	need("DELETE /api/companies/{cid}/trips/{id}", auth.PermLedgerWrite, s.deleteTrip)
	need("POST /api/companies/{cid}/fuel", auth.PermLedgerWrite, s.saveFuel)
	need("PUT /api/companies/{cid}/fuel/{id}", auth.PermLedgerWrite, s.saveFuel)
	need("DELETE /api/companies/{cid}/fuel/{id}", auth.PermLedgerWrite, s.deleteFuel)
	need("GET /api/companies/{cid}/fuel-docs", auth.PermLedgerRead, s.fuelDocs)
	open("GET /api/companies/{cid}/calendar", s.calendarView)
	open("GET /api/companies/{cid}/calendar.ics", s.calendarICS)
	open("GET /api/companies/{cid}/calendar/own", s.ownEvents)
	need("POST /api/companies/{cid}/calendar/own", auth.PermLedgerWrite, s.saveOwnEvent)
	need("PUT /api/companies/{cid}/calendar/own/{id}", auth.PermLedgerWrite, s.saveOwnEvent)
	need("DELETE /api/companies/{cid}/calendar/own/{id}", auth.PermLedgerWrite, s.deleteOwnEvent)
	need("POST /api/companies/{cid}/calendar/own/{id}/done", auth.PermLedgerWrite, s.markOwnEvent)
	open("GET /api/companies/{cid}/calendar/sync", s.calSyncStatus)
	need("POST /api/companies/{cid}/calendar/sync/connect", auth.PermSettingsWrite, s.calSyncConnect)
	open("POST /api/companies/{cid}/calendar/sync/run", s.calSyncRun)
	need("DELETE /api/companies/{cid}/calendar/sync", auth.PermSettingsWrite, s.calSyncDisconnect)
	open("PUT /api/calendar-sync/config", s.calSyncConfig)
	public("GET "+app.CalSyncCallbackPath, s.calSyncCallback)
	open("GET /api/me/calendar-feed", s.calendarFeed)
	open("POST /api/me/calendar-feed", s.createCalendarFeed)
	open("DELETE /api/me/calendar-feed", s.revokeCalendarFeed)
	public("GET /api/public/calendar/{cid}/{token}/fiskus.ics", s.publicCalendar)
	need("GET /api/companies/{cid}/chart", auth.PermLedgerRead, s.chartView)
	need("POST /api/companies/{cid}/chart/accounts", auth.PermLedgerWrite, s.saveAccount)
	need("PUT /api/companies/{cid}/chart/accounts/{number}", auth.PermLedgerWrite, s.saveAccount)
	need("DELETE /api/companies/{cid}/chart/accounts/{number}", auth.PermLedgerWrite, s.deleteAccount)
	need("POST /api/companies/{cid}/chart/centers", auth.PermLedgerWrite, s.saveCenter)
	need("PUT /api/companies/{cid}/chart/centers/{code}", auth.PermLedgerWrite, s.saveCenter)
	need("DELETE /api/companies/{cid}/chart/centers/{code}", auth.PermLedgerWrite, s.deleteCenter)
	need("POST /api/companies/{cid}/chart/jobs", auth.PermLedgerWrite, s.saveJob)
	need("PUT /api/companies/{cid}/chart/jobs/{code}", auth.PermLedgerWrite, s.saveJob)
	need("DELETE /api/companies/{cid}/chart/jobs/{code}", auth.PermLedgerWrite, s.deleteJob)
	need("GET /api/companies/{cid}/stock", auth.PermPurchasesRead, s.getStock)
	need("PUT /api/companies/{cid}/stock/config", auth.PermPurchasesWrite, s.saveStockConfig)
	need("POST /api/companies/{cid}/stock/docs", auth.PermPurchasesWrite, s.saveStockDoc)
	need("PUT /api/companies/{cid}/stock/docs/{id}", auth.PermPurchasesWrite, s.saveStockDoc)
	need("DELETE /api/companies/{cid}/stock/docs/{id}", auth.PermPurchasesWrite, s.deleteStockDoc)
	need("GET /api/companies/{cid}/stock/cards/{warehouse}/{item}", auth.PermPurchasesRead, s.stockCard)
	need("POST /api/companies/{cid}/stock/feed", auth.PermPurchasesWrite, s.createStockFeed)
	need("DELETE /api/companies/{cid}/stock/feed", auth.PermPurchasesWrite, s.revokeStockFeed)
	public("GET /api/public/stock/{cid}/{token}/{format}", s.publicStockFeed)
	need("GET /api/companies/{cid}/eshops", auth.PermInvoicesRead, s.getEshops)
	need("PUT /api/companies/{cid}/eshops", auth.PermSettingsWrite, s.saveEshops)
	need("POST /api/companies/{cid}/eshops/{id}/test", auth.PermSettingsWrite, s.testEshop)
	need("POST /api/companies/{cid}/eshops/{id}/orders", auth.PermInvoicesWrite, s.syncEshopOrders)
	need("POST /api/companies/{cid}/eshops/{id}/stock", auth.PermPurchasesWrite, s.pushEshopStock)
	public("POST /api/public/stock/{cid}/{token}/objednavky", s.publicShopOrder)
	need("GET /api/companies/{cid}/ledger", auth.PermLedgerRead, s.getLedger)
	need("PUT /api/companies/{cid}/chart/money", auth.PermLedgerWrite, s.setMoneyAccounts)
	need("PUT /api/companies/{cid}/ledger/{year}/postings/{key}", auth.PermLedgerWrite, s.setPosting)
	need("PUT /api/companies/{cid}/ledger/{year}/opening", auth.PermLedgerWrite, s.setLedgerOpening)
	need("GET /api/companies/{cid}/ledger/{year}/opening-proposal", auth.PermLedgerRead, s.openingProposal)
	need("POST /api/companies/{cid}/ledger/opening/import", auth.PermLedgerWrite, s.importOpening)
	need("GET /api/companies/{cid}/ledger/{year}/statements", auth.PermLedgerRead, s.ledgerStatements)
	need("GET /api/companies/{cid}/ledger/{year}/fx", auth.PermLedgerRead, s.fxRevaluation)
	need("POST /api/companies/{cid}/ledger/{year}/fx", auth.PermLedgerWrite, s.postFXRevaluation)
	need("POST /api/companies/{cid}/ledger/{year}/internal", auth.PermLedgerWrite, s.saveInternal)
	need("PUT /api/companies/{cid}/ledger/{year}/internal/{id}", auth.PermLedgerWrite, s.saveInternal)
	need("DELETE /api/companies/{cid}/ledger/{year}/internal/{id}", auth.PermLedgerWrite, s.deleteInternal)
	need("POST /api/companies/{cid}/chart/templates", auth.PermLedgerWrite, s.saveTemplate)
	need("PUT /api/companies/{cid}/chart/templates/{code}", auth.PermLedgerWrite, s.saveTemplate)
	need("DELETE /api/companies/{cid}/chart/templates/{code}", auth.PermLedgerWrite, s.deleteTemplate)
	need("GET /api/companies/{cid}/assets", auth.PermLedgerRead, s.listAssets)
	need("POST /api/companies/{cid}/assets", auth.PermLedgerWrite, s.saveAsset)
	need("PUT /api/companies/{cid}/assets/{id}", auth.PermLedgerWrite, s.saveAsset)
	need("DELETE /api/companies/{cid}/assets/{id}", auth.PermLedgerWrite, s.deleteAsset)
	need("POST /api/companies/{cid}/assets/depreciation/{year}", auth.PermLedgerWrite, s.postDepreciation)
	need("PUT /api/companies/{cid}/journal/{key}", auth.PermLedgerWrite, s.classifyJournal)
	need("DELETE /api/companies/{cid}/journal/{key}", auth.PermLedgerWrite, s.resetJournal)
	need("GET /api/companies/{cid}/users", auth.PermUsersManage, s.listMembers)
	need("POST /api/companies/{cid}/users", auth.PermUsersManage, s.addMember)
	need("PUT /api/companies/{cid}/users/{uid}", auth.PermUsersManage, s.setRole)
	need("DELETE /api/companies/{cid}/users/{uid}", auth.PermUsersManage, s.removeMember)

	need("GET /api/companies/{cid}/audit", auth.PermAuditRead, s.companyAudit)

	need("GET /api/companies/{cid}/cash", auth.PermCashRead, s.cashBook)
	need("GET /api/companies/{cid}/cash-registers", auth.PermCashRead, s.cashRegisters)
	need("PUT /api/companies/{cid}/cash-registers", auth.PermCashWrite, s.saveCashRegisters)
	need("GET /api/companies/{cid}/cash/candidates", auth.PermCashRead, s.cashCandidates)
	need("GET /api/companies/{cid}/cash/{id}", auth.PermCashRead, s.getCash)
	need("POST /api/companies/{cid}/cash", auth.PermCashWrite, s.createCash)
	need("POST /api/companies/{cid}/cash/preview", auth.PermCashWrite, s.previewCash)
	need("PUT /api/companies/{cid}/cash/{id}", auth.PermCashWrite, s.updateCash)
	need("POST /api/companies/{cid}/cash/{id}/cancel", auth.PermCashWrite, s.cancelCash)

	need("GET /api/companies/{cid}/bank", auth.PermBankRead, s.listBank)
	need("POST /api/companies/{cid}/bank/import", auth.PermBankWrite, s.importBank)
	need("GET /api/companies/{cid}/bank/{id}/suggestions", auth.PermBankRead, s.bankSuggestions)
	need("POST /api/companies/{cid}/bank/{id}/match", auth.PermBankWrite, s.matchBank)
	need("POST /api/companies/{cid}/bank/{id}/unmatch", auth.PermBankWrite, s.unmatchBank)
	need("POST /api/companies/{cid}/bank/{id}/ignore", auth.PermBankWrite, s.ignoreBank)

	need("GET /api/companies/{cid}/vat", auth.PermVATRead, s.vatReturn)
	need("GET /api/companies/{cid}/vat/filings", auth.PermVATRead, s.vatFilings)
	need("POST /api/companies/{cid}/vat/filings", auth.PermLedgerWrite, s.recordVATFiling)
	need("GET /api/companies/{cid}/vat/coefficient/{year}", auth.PermVATRead, s.vatCoefficient)
	need("PUT /api/companies/{cid}/vat/coefficient/{year}", auth.PermLedgerWrite, s.setVATCoefficient)
	need("GET /api/companies/{cid}/vat/{kind}", auth.PermVATRead, s.vatExport)
	open("GET /api/audit", s.allAudit)
	open("GET /api/audit/verify", s.verifyAudit)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "neznáma adresa API", nil)
	})
	s.routeOAuth(mux, open)
	if s.Web != nil {
		mux.Handle("/", spa(s.Web))
	}
	return mux
}

// ---- pomocné ----

type errorBody struct {
	Error  string               `json:"error"`
	Fields []invoice.FieldError `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: zápis odpovede: %v", err)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string, fields []invoice.FieldError) {
	writeJSON(w, status, errorBody{Error: msg, Fields: fields})
}

// fail preloží doménovú chybu na HTTP stav.
func fail(w http.ResponseWriter, err error) {
	var ve *invoice.ValidationError
	switch {
	case errors.As(err, &ve):
		writeErr(w, http.StatusUnprocessableEntity, "Doklad obsahuje chyby", ve.Fields)
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "Záznam neexistuje", nil)
	case errors.Is(err, invoice.ErrLocked), errors.Is(err, invoice.ErrBadTransition), errors.Is(err, app.ErrReceiptIssued), errors.Is(err, app.ErrAmend):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	case errors.Is(err, invoice.ErrBadPayment):
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
	case errors.Is(err, app.ErrCompanyLimit), errors.Is(err, app.ErrForbidden), errors.Is(err, app.ErrReadOnly):
		writeErr(w, http.StatusForbidden, err.Error(), nil)
	case errors.Is(err, app.ErrNotVATPayer), errors.Is(err, app.ErrPeriodTooOld), errors.Is(err, app.ErrPeriodInFuture),
		errors.Is(err, app.ErrMissingVATData):
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
	case errors.Is(err, app.ErrNoSession):
		writeErr(w, http.StatusUnauthorized, err.Error(), nil)
	case errors.Is(err, app.ErrLastAdmin), errors.Is(err, app.ErrSelfDemote), errors.Is(err, store.ErrDuplicate):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	default:
		log.Printf("api: %v", err)
		writeErr(w, http.StatusInternalServerError, "Interná chyba servera", nil)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatné JSON dáta: "+err.Error(), nil)
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "Neplatné ID", nil)
		return 0, false
	}
	return id, true
}

// company overí, že firma existuje, a vráti jej ID.
func (s *Server) company(w http.ResponseWriter, r *http.Request) (int64, bool) {
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return 0, false
	}
	if _, err := s.App.Store.GetCompany(r.Context(), cid); err != nil {
		fail(w, err)
		return 0, false
	}
	// člen firmy (akákoľvek rola); konkrétne právo overuje s.need
	if err := s.App.Require(r.Context(), cid, ""); err != nil {
		fail(w, err)
		return 0, false
	}
	return cid, true
}

// ---- handlery ----

// capabilities je verejné (aj pred prihlásením), preto bez údajov o držiteľovi licencie.
func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	c := s.App.Capabilities()
	c.License = nil
	writeJSON(w, http.StatusOK, struct {
		edition.Capabilities
		Launcher bool   `json:"launcher"` // desktop z inštalátora: okno sa ozýva, dá sa ukončiť
		Version  string `json:"version"`
		PIN      bool   `json:"pin"` // desktop je chránený PIN-om
	}{c, s.Desktop != nil, s.Version, s.App.PINEnabled(r.Context())})
}

func (s *Server) listCompanies(w http.ResponseWriter, r *http.Request) {
	list, err := s.App.VisibleCompanies(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if list == nil {
		list = []store.Company{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createCompany(w http.ResponseWriter, r *http.Request) {
	if s.App.Caps.Mode == edition.Server && !auth.ActorFrom(r.Context()).SuperAdmin {
		fail(w, app.ErrForbidden)
		return
	}
	var c store.Company
	if !decode(w, r, &c) {
		return
	}
	c, err := s.App.CreateCompany(r.Context(), c)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// invoiceRow je faktúra v zozname s vypočítanými príznakmi.
type invoiceRow struct {
	invoice.Invoice
	Overdue     bool `json:"overdue"`
	DaysOverdue int  `json:"daysOverdue"`
}

func (s *Server) listInvoices(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := store.InvoiceFilter{Status: invoice.Status(q.Get("status")), Search: q.Get("q")}
	if y := q.Get("year"); y != "" {
		var err error
		if f.Year, err = strconv.Atoi(y); err != nil {
			writeErr(w, http.StatusBadRequest, "Neplatný rok", nil)
			return
		}
	}
	list, err := s.App.Store.ListInvoices(r.Context(), cid, f)
	if err != nil {
		fail(w, err)
		return
	}
	today := s.App.Today()
	rows := make([]invoiceRow, len(list))
	for i, inv := range list {
		rows[i] = invoiceRow{Invoice: inv, Overdue: inv.Overdue(today)}
		if rows[i].Overdue {
			rows[i].DaysOverdue = inv.DueDate.DaysUntil(today)
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) getInvoice(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	inv, err := s.App.Store.GetInvoice(r.Context(), cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	// vystavená faktúra sa dá upravovať, kým nie je uhradená (a priznaná)
	lock := ""
	if inv.Status != invoice.StatusDraft {
		lock = s.App.AmendBlock(r.Context(), cid, inv)
	}
	writeJSON(w, http.StatusOK, struct {
		invoice.Invoice
		Amendable  bool   `json:"amendable"`
		LockReason string `json:"lockReason,omitempty"`
	}{inv, inv.Status != invoice.StatusDraft && lock == "", lock})
}

func (s *Server) saveInvoice(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var inv invoice.Invoice
	if !decodeInvoice(w, r, &inv) {
		return
	}
	status := http.StatusCreated
	if r.Method == http.MethodPut {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		inv.ID, status = id, http.StatusOK
	} else {
		inv.ID = 0
	}
	saved, err := s.App.SaveDraft(r.Context(), cid, inv)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, status, saved)
}

// decodeInvoice je tolerantnejší ako decode: frontend posiela celý objekt
// faktúry vrátane vypočítaných polí (summary, overdue...), ktoré sa ignorujú.
func decodeInvoice(w http.ResponseWriter, r *http.Request, inv *invoice.Invoice) bool {
	return decodeLoose(w, r, inv)
}

// decodeLoose načíta JSON a ignoruje neznáme (vypočítané) polia.
func decodeLoose(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatné JSON dáta: "+err.Error(), nil)
		return false
	}
	return true
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var inv invoice.Invoice
	if !decodeInvoice(w, r, &inv) {
		return
	}
	out, err := s.App.Preview(r.Context(), cid, inv)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	resp := struct {
		invoice.Invoice
		Errors []invoice.FieldError `json:"errors"`
	}{Invoice: out, Errors: []invoice.FieldError{}}
	var ve *invoice.ValidationError
	if errors.As(out.Validate(), &ve) {
		resp.Errors = ve.Fields
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) action(w http.ResponseWriter, r *http.Request, fn func(cid, id int64) (invoice.Invoice, error)) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	inv, err := fn(cid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (s *Server) issue(w http.ResponseWriter, r *http.Request) {
	s.action(w, r, func(cid, id int64) (invoice.Invoice, error) { return s.App.Issue(r.Context(), cid, id) })
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	s.action(w, r, func(cid, id int64) (invoice.Invoice, error) { return s.App.Cancel(r.Context(), cid, id) })
}

func (s *Server) pay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Date   invoice.Date `json:"date"`
		Amount *money.Dec   `json:"amount"` // nepovinné: čiastočná úhrada, inak celý zostatok
	}
	if r.ContentLength > 0 && !decode(w, r, &body) {
		return
	}
	s.action(w, r, func(cid, id int64) (invoice.Invoice, error) {
		if body.Amount == nil {
			return s.App.MarkPaid(r.Context(), cid, id, body.Date)
		}
		return s.App.AddPayment(r.Context(), cid, id, invoice.Payment{Date: body.Date, Amount: *body.Amount, Source: invoice.PaymentManual}, false)
	})
}

// spa servíruje frontend; neznáme cesty vracajú index.html (routing vo Vue).
func spa(web fs.FS) http.Handler {
	files := http.FileServer(http.FS(web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(web, p); err == nil {
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, web, "index.html")
	})
}
