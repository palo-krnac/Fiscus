package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/reminder"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Nástroje pre prijaté faktúry, párovanie platieb, dobropisy a upomienky.
// Výpočty (DPH, úroky, limity dobropisu) robí Fiškus; asistent iba zadáva údaje.

func (s *Server) moreTools() []Tool {
	a := s.App
	return []Tool{
		{
			Name: "create_purchase", Title: "Zaevidovať prijatú faktúru", Write: true,
			Description: "Zaeviduje prijatú faktúru (náklad) z rekapitulácie DPH dodávateľa: pre každú sadzbu základ a DPH z dokladu " +
				"(DPH možno vynechať, dopočíta sa). Fiškus pridelí interné číslo, skontroluje duplicitu a upozorní na rozdiel v DPH. " +
				"V cudzej mene doplní kurz ECB k dátumu dodania.",
			Props: map[string]any{
				"company_id":      pCompany,
				"contact_id":      map[string]any{"type": "integer", "description": "Dodávateľ z adresára (namiesto supplier)"},
				"supplier":        pParty,
				"supplier_number": map[string]any{"type": "string", "description": "Číslo faktúry dodávateľa"},
				"variable_symbol": map[string]any{"type": "string", "description": "Predvolene číslice z čísla faktúry"},
				"supplier_iban":   map[string]any{"type": "string"},
				"issue_date":      pDate, "delivery_date": pDate, "due_date": pDate,
				"received_date": map[string]any{"type": "string", "description": "Dátum doručenia RRRR-MM-DD — určuje obdobie odpočtu DPH (predvolene dnes)"},
				"rates": map[string]any{"type": "array", "minItems": 1, "description": "Rekapitulácia DPH z dokladu", "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"vat_rate": map[string]any{"type": "integer", "description": "Sadzba DPH v % (23, 19, 5, 0)"},
						"base":     map[string]any{"type": "string", "description": "Základ dane, napr. \"100.00\""},
						"vat":      map[string]any{"type": "string", "description": "DPH z dokladu; prázdne = dopočíta sa"},
					},
					"required": []string{"vat_rate", "base"},
				}},
				"description":    map[string]any{"type": "string", "description": "Predmet plnenia (krátko)"},
				"currency":       map[string]any{"type": "string", "description": "Mena dokladu (predvolene EUR)"},
				"payment_method": map[string]any{"type": "string", "enum": []string{"transfer", "cash", "card"}},
				"reverse_charge": map[string]any{"type": "boolean", "description": "Prenesenie daňovej povinnosti (dodávateľ fakturuje bez DPH)"},
				"supply_kind":    map[string]any{"type": "string", "enum": []string{"goods", "services"}, "description": "Pri zahraničnom dodávateľovi: tovar alebo služba"},
				"no_deduction":   map[string]any{"type": "boolean", "description": "DPH sa neodpočítava (reprezentácia, súkromná spotreba)"},
				"note":           map[string]any{"type": "string"},
			},
			Required: []string{"supplier_number", "rates"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID      int64          `json:"company_id"`
					ContactID      int64          `json:"contact_id"`
					Supplier       *invoice.Party `json:"supplier"`
					SupplierNumber string         `json:"supplier_number"`
					VariableSymbol string         `json:"variable_symbol"`
					SupplierIBAN   string         `json:"supplier_iban"`
					IssueDate      invoice.Date   `json:"issue_date"`
					DeliveryDate   invoice.Date   `json:"delivery_date"`
					DueDate        invoice.Date   `json:"due_date"`
					ReceivedDate   invoice.Date   `json:"received_date"`
					Rates          []struct {
						Rate vat.Rate  `json:"vat_rate"`
						Base money.Dec `json:"base"`
						VAT  money.Dec `json:"vat"`
					} `json:"rates"`
					Description   string                `json:"description"`
					Currency      string                `json:"currency"`
					PaymentMethod invoice.PaymentMethod `json:"payment_method"`
					ReverseCharge bool                  `json:"reverse_charge"`
					SupplyKind    invoice.SupplyKind    `json:"supply_kind"`
					NoDeduction   bool                  `json:"no_deduction"`
					Note          string                `json:"note"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermPurchasesWrite)
				if err != nil {
					return nil, err
				}
				pu := purchase.Purchase{SupplierNumber: p.SupplierNumber, VariableSymbol: p.VariableSymbol, SupplierIBAN: p.SupplierIBAN,
					IssueDate: p.IssueDate, DeliveryDate: p.DeliveryDate, DueDate: p.DueDate, ReceivedDate: p.ReceivedDate,
					Description: p.Description, Currency: p.Currency, PaymentMethod: p.PaymentMethod, ReverseCharge: p.ReverseCharge,
					SupplyKind: p.SupplyKind, NoDeduction: p.NoDeduction, Note: p.Note}
				switch {
				case p.ContactID != 0:
					ct, err := a.Store.GetContact(c.ctx, cid, p.ContactID)
					if err != nil {
						return nil, err
					}
					pu.Supplier, pu.ContactID = app.PartyFromContact(ct), ct.ID
				case p.Supplier != nil:
					pu.Supplier = *p.Supplier
				default:
					return nil, errors.New("zadajte supplier alebo contact_id")
				}
				if pu.Supplier.Country == "" {
					pu.Supplier.Country = "SK"
				}
				if pu.ReceivedDate.IsZero() {
					pu.ReceivedDate = a.Today()
				}
				if pu.IssueDate.IsZero() {
					pu.IssueDate = pu.ReceivedDate
				}
				for _, r := range p.Rates {
					pu.Rates = append(pu.Rates, purchase.RateRow{Rate: r.Rate, Base: r.Base, VAT: r.VAT})
				}
				saved, warns, err := a.SavePurchase(c.ctx, cid, pu)
				if err != nil {
					return nil, err
				}
				out := map[string]any{"id": saved.ID, "number": saved.Number, "supplier": saved.Supplier.Name,
					"receivedDate": saved.ReceivedDate, "dueDate": saved.DueDate, "summary": saved.Summary, "warnings": warnsOrEmpty(warns)}
				if saved.Foreign() {
					out["currency"], out["fxRate"], out["totalEur"] = saved.Currency, saved.FXRate, saved.TotalEUR()
				}
				return out, nil
			},
		},
		{
			Name: "mark_purchase_paid", Title: "Označiť prijatú faktúru ako uhradenú", Write: true,
			Description: "Označí prijatú faktúru ako uhradenú k dátumu (predvolene dnes). Platbu z banky radšej spárujte nástrojom match_bank_transaction.",
			Props:       map[string]any{"company_id": pCompany, "id": pID, "date": pDate}, Required: []string{"id"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64        `json:"company_id"`
					ID        int64        `json:"id"`
					Date      invoice.Date `json:"date"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermPurchasesWrite)
				if err != nil {
					return nil, err
				}
				if p.Date.IsZero() {
					p.Date = a.Today()
				}
				pu, err := a.MarkPurchasePaid(c.ctx, cid, p.ID, p.Date)
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": pu.ID, "number": pu.Number, "status": pu.Status, "paidDate": pu.PaidDate}, nil
			},
		},
		{
			Name: "bank_match_suggestions", Title: "Návrhy na spárovanie platby",
			Description: "Pre bankový pohyb vráti doklady, ktoré mu pravdepodobne zodpovedajú (skóre 100 = sedí VS aj suma), " +
				"a ak treba, aj všetky neuhradené doklady v rovnakom smere platby.",
			Props: map[string]any{"company_id": pCompany, "id": map[string]any{"type": "integer", "description": "ID bankového pohybu (z list_bank_transactions)"},
				"all_open": map[string]any{"type": "boolean", "description": "Pridať aj všetky neuhradené doklady na ručný výber"}},
			Required: []string{"id"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64 `json:"company_id"`
					ID        int64 `json:"id"`
					AllOpen   bool  `json:"all_open"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermBankRead)
				if err != nil {
					return nil, err
				}
				sug, err := a.Suggestions(c.ctx, cid, p.ID)
				if err != nil {
					return nil, err
				}
				out := map[string]any{"suggestions": sug}
				if p.AllOpen {
					open, err := a.OpenDocuments(c.ctx, cid, p.ID)
					if err != nil {
						return nil, err
					}
					out["open"] = open
				}
				return out, nil
			},
		},
		{
			Name: "match_bank_transaction", Title: "Spárovať platbu s dokladom", Write: true,
			Description: "Spáruje bankový pohyb s vydanou (kind=invoice) alebo prijatou (kind=purchase) faktúrou a zapíše úhradu. " +
				"Pri vydanej faktúre sa započíta aj čiastočná platba. Doklad vyber z bank_match_suggestions.",
			Props: map[string]any{"company_id": pCompany,
				"id":     map[string]any{"type": "integer", "description": "ID bankového pohybu"},
				"kind":   map[string]any{"type": "string", "enum": []string{"invoice", "purchase"}},
				"doc_id": map[string]any{"type": "integer", "description": "ID faktúry"}},
			Required: []string{"id", "kind", "doc_id"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64  `json:"company_id"`
					ID        int64  `json:"id"`
					Kind      string `json:"kind"`
					DocID     int64  `json:"doc_id"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				if p.Kind != "invoice" && p.Kind != "purchase" {
					return nil, errors.New("kind musí byť invoice alebo purchase")
				}
				cid, err := c.company(p.CompanyID, auth.PermBankWrite)
				if err != nil {
					return nil, err
				}
				t, err := a.MatchTransaction(c.ctx, cid, p.ID, p.Kind, p.DocID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": t.ID, "status": t.Status, "match": t.Match}, nil
			},
		},
		{
			Name: "create_credit_note", Title: "Koncept dobropisu", Write: true,
			Description: "Vytvorí koncept dobropisu (opravnej faktúry) k vystavenej faktúre — celý, alebo iba vybrané položky a množstvá. " +
				"Sadzby DPH sa prevezmú z pôvodnej faktúry; limit (nedobropisovať viac, ako bolo fakturované) kontroluje Fiškus pri vystavení. " +
				"Dobropis sa vystaví nástrojom issue_invoice, iba na výslovný pokyn.",
			Props: map[string]any{"company_id": pCompany,
				"invoice_id": map[string]any{"type": "integer", "description": "ID pôvodnej vystavenej faktúry"},
				"reason":     map[string]any{"type": "string", "description": "Dôvod opravy (povinný na dobropise), napr. „Vrátenie tovaru“"},
				"lines": map[string]any{"type": "array", "description": "Iba časť faktúry: ktoré položky a koľko dobropisovať; prázdne = celá faktúra", "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"line":     map[string]any{"type": "integer", "description": "Poradie položky na pôvodnej faktúre (od 1)"},
						"quantity": map[string]any{"type": "string", "description": "Dobropisované množstvo (kladné), predvolene celé"},
					},
					"required": []string{"line"},
				}}},
			Required: []string{"invoice_id", "reason"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64  `json:"company_id"`
					InvoiceID int64  `json:"invoice_id"`
					Reason    string `json:"reason"`
					Lines     []struct {
						Line     int       `json:"line"`
						Quantity money.Dec `json:"quantity"`
					} `json:"lines"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				if strings.TrimSpace(p.Reason) == "" {
					return nil, errors.New("zadajte dôvod opravy (reason) — je povinný na dobropise")
				}
				cid, err := c.company(p.CompanyID, auth.PermInvoicesWrite)
				if err != nil {
					return nil, err
				}
				if len(p.Lines) > 0 { // výber položiek overiť skôr, ako vznikne koncept
					orig, err := a.Store.GetInvoice(c.ctx, cid, p.InvoiceID)
					if err != nil {
						return nil, err
					}
					if _, err := pickCreditLines(negate(orig.Lines), p.Lines); err != nil {
						return nil, err
					}
				}
				cn, err := a.CreateCreditNote(c.ctx, cid, p.InvoiceID)
				if err != nil {
					return nil, err
				}
				if cn.Correction != nil {
					cn.Correction.Reason = p.Reason
				}
				if len(p.Lines) > 0 {
					if cn.Lines, err = pickCreditLines(cn.Lines, p.Lines); err != nil {
						return nil, err
					}
				}
				saved, err := a.SaveDraft(c.ctx, cid, cn)
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": saved.ID, "status": saved.Status, "original": saved.Correction.OriginalNumber,
					"reason": saved.Correction.Reason, "summary": saved.Summary,
					"message": "Koncept dobropisu je uložený. Vystaví sa až nástrojom issue_invoice."}, nil
			},
		},
		{
			Name: "money_journal", Title: "Peňažný denník",
			Description: "Peňažný denník jednoduchého účtovníctva za rok: pohyby v pokladnici a na účtoch rozpísané do stĺpcov " +
				"(d – príjmy a e – výdavky do základu dane, f/g mimo základu dane, c – priebežné položky), súčty a rozdiel príjmov a výdavkov. " +
				"Nerozpísané pohyby (unclassified_only) treba rozpísať v programe.",
			Props: map[string]any{"company_id": pCompany, "year": pYear,
				"unclassified_only": map[string]any{"type": "boolean", "description": "Iba pohyby bez rozpisu"}},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID        int64 `json:"company_id"`
					Year             int   `json:"year"`
					UnclassifiedOnly bool  `json:"unclassified_only"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermLedgerRead)
				if err != nil {
					return nil, err
				}
				book, err := a.Journal(c.ctx, cid, p.Year)
				if err != nil {
					return nil, err
				}
				if p.UnclassifiedOnly {
					open := book.Entries[:0:0]
					for _, e := range book.Entries {
						if !e.Classified() {
							open = append(open, e)
						}
					}
					book.Entries = open
				}
				return map[string]any{"year": book.Year, "totals": book.Totals, "columns": book.Columns, "entries": book.Entries}, nil
			},
		},
		{
			Name: "list_overdue", Title: "Faktúry po splatnosti",
			Description: "Neuhradené faktúry po splatnosti podľa odberateľov: dni omeškania, doterajšie upomienky a stupeň ďalšej upomienky.",
			Props:       map[string]any{"company_id": pCompany},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64 `json:"company_id"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermInvoicesRead)
				if err != nil {
					return nil, err
				}
				groups, err := a.Overdue(c.ctx, cid)
				if err != nil {
					return nil, err
				}
				return map[string]any{"customers": groups, "count": len(groups)}, nil
			},
		},
		{
			Name: "create_reminder", Title: "Upomienka", Write: true,
			Description: "Vytvorí očíslovanú upomienku k faktúram po splatnosti jedného odberateľa. Stupeň (1., 2., posledná) určí Fiškus " +
				"podľa doterajších upomienok; úrok z omeškania a paušálnu náhradu 40 € vypočíta podľa zákona. Použi iba na výslovný pokyn.",
			Props: map[string]any{"company_id": pCompany,
				"invoice_ids":   map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "integer"}, "description": "Faktúry jedného odberateľa (z list_overdue)"},
				"date":          pDate,
				"pay_by":        map[string]any{"type": "string", "description": "Dodatočná lehota RRRR-MM-DD (predvolene o 7 dní)"},
				"interest_mode": map[string]any{"type": "string", "enum": []string{"statutory", "custom", "none"}, "description": "Úrok: zákonný (predvolene), dohodnutý alebo žiadny"},
				"custom_rate":   map[string]any{"type": "string", "description": "Dohodnutá ročná sadzba v % pri interest_mode=custom"},
				"charge_fee":    map[string]any{"type": "boolean", "description": "Paušálna náhrada nákladov 40 € za faktúru (iba podnikateľ)"},
				"note":          map[string]any{"type": "string"}},
			Required: []string{"invoice_ids"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID  int64                 `json:"company_id"`
					InvoiceIDs []int64               `json:"invoice_ids"`
					Date       invoice.Date          `json:"date"`
					PayBy      invoice.Date          `json:"pay_by"`
					Interest   reminder.InterestMode `json:"interest_mode"`
					CustomRate money.Dec             `json:"custom_rate"`
					ChargeFee  bool                  `json:"charge_fee"`
					Note       string                `json:"note"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermInvoicesWrite)
				if err != nil {
					return nil, err
				}
				r, err := a.CreateReminder(c.ctx, cid, app.ReminderRequest{InvoiceIDs: p.InvoiceIDs, Options: reminder.Options{
					Date: p.Date, PayBy: p.PayBy, Interest: p.Interest, CustomRate: p.CustomRate, ChargeFee: p.ChargeFee, Note: p.Note}})
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": r.ID, "number": r.Number, "level": r.Level, "customer": r.Customer.Name,
					"payBy": r.PayBy, "principal": r.Principal, "total": r.Total, "items": r.Items}, nil
			},
		},
	}
}

func warnsOrEmpty(w []purchase.Warning) []purchase.Warning {
	if w == nil {
		return []purchase.Warning{}
	}
	return w
}

// negate vráti položky so záporným množstvom (ako na dobropise).
func negate(ls []invoice.Line) []invoice.Line {
	out := make([]invoice.Line, len(ls))
	for i, l := range ls {
		l.Quantity = l.Quantity.Neg()
		out[i] = l
	}
	return out
}

// pickCreditLines vyberie položky dobropisu (záporné množstvá z pôvodnej faktúry).
func pickCreditLines(all []invoice.Line, pick []struct {
	Line     int       `json:"line"`
	Quantity money.Dec `json:"quantity"`
}) ([]invoice.Line, error) {
	out := make([]invoice.Line, 0, len(pick))
	seen := map[int]bool{}
	for _, x := range pick {
		if x.Line < 1 || x.Line > len(all) {
			return nil, fmt.Errorf("faktúra nemá položku %d (má %d)", x.Line, len(all))
		}
		if seen[x.Line] {
			return nil, fmt.Errorf("položka %d je zadaná dvakrát", x.Line)
		}
		seen[x.Line] = true
		l := all[x.Line-1]
		full := l.Quantity.Neg() // na dobropise je záporné
		if !x.Quantity.IsZero() {
			if x.Quantity.Sign() < 0 || x.Quantity.Cmp(full) > 0 {
				return nil, fmt.Errorf("položka %d: množstvo musí byť od 0 do %s", x.Line, full)
			}
			l.Quantity = x.Quantity.Neg()
		}
		out = append(out, l)
	}
	return out, nil
}
