package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/bank"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
	"github.com/palo-krnac/fiscus/backend/internal/purchase"
	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/vat"
)

// Tool je nástroj dostupný asistentovi.
type Tool struct {
	Name        string
	Title       string
	Description string
	Props       map[string]any // vlastnosti JSON Schema vstupu
	Required    []string
	Write       bool // mení dáta
	Run         func(c *callCtx, args json.RawMessage) (any, error)
}

func (t Tool) describe() map[string]any {
	props := map[string]any{}
	for k, v := range t.Props {
		props[k] = v
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(t.Required) > 0 {
		schema["required"] = t.Required
	}
	return map[string]any{
		"name": t.Name, "title": t.Title, "description": t.Description, "inputSchema": schema,
		"annotations": map[string]any{"title": t.Title, "readOnlyHint": !t.Write, "destructiveHint": false, "openWorldHint": false},
	}
}

type callCtx struct {
	ctx       context.Context
	s         *Server
	companyID int64
}

// company určí firmu: zadané ID, alebo jediná dostupná firma.
func (c *callCtx) company(id int64, perm auth.Perm) (int64, error) {
	a := c.s.App
	if id == 0 {
		list, err := a.VisibleCompanies(c.ctx)
		if err != nil {
			return 0, err
		}
		if len(list) != 1 {
			return 0, errors.New("zadajte company_id — zoznam firiem vráti nástroj list_companies")
		}
		id = list[0].ID
	}
	if err := a.Require(c.ctx, id, perm); err != nil {
		return 0, err
	}
	c.companyID = id
	return id, nil
}

func parse(args json.RawMessage, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(args)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("neplatné argumenty: %v", err)
	}
	return nil
}

// spoločné vlastnosti schémy
var (
	pCompany = map[string]any{"type": "integer", "description": "ID firmy (z list_companies); netreba, ak má používateľ jedinú firmu"}
	pID      = map[string]any{"type": "integer", "description": "ID dokladu"}
	pYear    = map[string]any{"type": "integer", "description": "Rok (podľa dátumu vystavenia / doručenia)"}
	pQuery   = map[string]any{"type": "string", "description": "Hľadaný text: číslo, partner, IČO"}
	pLimit   = map[string]any{"type": "integer", "description": "Najviac záznamov (predvolene 50, max 500)"}
	pDate    = map[string]any{"type": "string", "description": "Dátum RRRR-MM-DD"}
	pParty   = map[string]any{
		"type": "object", "description": "Partner (odberateľ / kontakt)",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"}, "street": map[string]any{"type": "string"},
			"city": map[string]any{"type": "string"}, "zip": map[string]any{"type": "string"},
			"country": map[string]any{"type": "string", "description": "ISO kód štátu, predvolene SK"},
			"ico":     map[string]any{"type": "string"}, "dic": map[string]any{"type": "string"},
			"icdph": map[string]any{"type": "string"}, "email": map[string]any{"type": "string"},
		},
		"required": []string{"name"},
	}
)

func limit(n int) int {
	switch {
	case n <= 0:
		return 50
	case n > 500:
		return 500
	}
	return n
}

// invoiceRow je faktúra v prehľade (bez položiek).
type invoiceRow struct {
	ID          int64          `json:"id"`
	Number      string         `json:"number"`
	Kind        invoice.Kind   `json:"kind"`
	Status      invoice.Status `json:"status"`
	Customer    string         `json:"customer"`
	ICO         string         `json:"ico,omitempty"`
	IssueDate   invoice.Date   `json:"issueDate"`
	DueDate     invoice.Date   `json:"dueDate"`
	PaidDate    invoice.Date   `json:"paidDate,omitempty"`
	Total       money.Dec      `json:"total"`
	VAT         money.Dec      `json:"vat"`
	Overdue     bool           `json:"overdue"`
	DaysOverdue int            `json:"daysOverdue,omitempty"`
	VS          string         `json:"variableSymbol"`
}

func (s *Server) allTools() []Tool {
	a := s.App
	return []Tool{
		{
			Name: "list_companies", Title: "Firmy",
			Description: "Zoznam firiem, ku ktorým má používateľ prístup, s jeho rolou.",
			Run: func(c *callCtx, _ json.RawMessage) (any, error) {
				list, err := a.VisibleCompanies(c.ctx)
				if err != nil {
					return nil, err
				}
				type row struct {
					ID       int64     `json:"id"`
					Name     string    `json:"name"`
					ICO      string    `json:"ico"`
					ICDPH    string    `json:"icdph"`
					VATPayer bool      `json:"vatPayer"`
					Role     auth.Role `json:"role"`
				}
				out := []row{}
				for _, co := range list {
					role, _ := a.RoleIn(c.ctx, co.ID)
					out = append(out, row{co.ID, co.Name, co.ICO, co.ICDPH, co.VATPayer, role})
				}
				return map[string]any{"companies": out}, nil
			},
		},
		{
			Name: "company_overview", Title: "Prehľad firmy",
			Description: "Pohľadávky, záväzky a bankové pohyby na spárovanie: neuhradené a po splatnosti, fakturované tento mesiac.",
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
				today := a.Today()
				invs, err := a.Store.ListInvoices(c.ctx, cid, store.InvoiceFilter{})
				if err != nil {
					return nil, err
				}
				var unpaid, overdue, month money.Dec
				var nUnpaid, nOverdue int
				for _, inv := range invs {
					if inv.Status == invoice.StatusIssued && inv.Kind != invoice.KindCreditNote {
						unpaid, nUnpaid = unpaid.Add(inv.Summary.Total), nUnpaid+1
						if inv.Overdue(today) {
							overdue, nOverdue = overdue.Add(inv.Summary.Total), nOverdue+1
						}
					}
					if (inv.Status == invoice.StatusIssued || inv.Status == invoice.StatusPaid) &&
						inv.IssueDate.Year() == today.Year() && inv.IssueDate.Month() == today.Month() {
						month = month.Add(inv.Summary.Total)
					}
				}
				out := map[string]any{
					"today":             today,
					"receivables":       map[string]any{"unpaid": unpaid, "unpaidCount": nUnpaid, "overdue": overdue, "overdueCount": nOverdue},
					"invoicedThisMonth": month,
				}
				if a.Require(c.ctx, cid, auth.PermPurchasesRead) == nil {
					purs, err := a.Store.ListPurchases(c.ctx, cid, store.PurchaseFilter{})
					if err != nil {
						return nil, err
					}
					var toPay, late money.Dec
					var nPay, nLate int
					for _, p := range purs {
						if p.Status == purchase.StatusBooked {
							toPay, nPay = toPay.Add(p.Summary.Total), nPay+1
							if p.Overdue(today) {
								late, nLate = late.Add(p.Summary.Total), nLate+1
							}
						}
					}
					out["payables"] = map[string]any{"toPay": toPay, "toPayCount": nPay, "overdue": late, "overdueCount": nLate}
				}
				if a.Require(c.ctx, cid, auth.PermBankRead) == nil {
					txs, err := a.Store.ListBankTransactions(c.ctx, cid, store.BankFilter{Status: bank.StatusOpen})
					if err != nil {
						return nil, err
					}
					out["bankUnmatched"] = len(txs)
				}
				return out, nil
			},
		},
		{
			Name: "list_invoices", Title: "Vydané faktúry",
			Description: "Vydané faktúry a dobropisy s filtrom podľa roka, stavu a textu. Stav: draft (koncept), issued (vystavená, neuhradená), paid, cancelled, overdue (po splatnosti).",
			Props: map[string]any{"company_id": pCompany, "year": pYear, "query": pQuery, "limit": pLimit,
				"status": map[string]any{"type": "string", "enum": []string{"draft", "issued", "paid", "cancelled", "overdue"}}},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64  `json:"company_id"`
					Year      int    `json:"year"`
					Status    string `json:"status"`
					Query     string `json:"query"`
					Limit     int    `json:"limit"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermInvoicesRead)
				if err != nil {
					return nil, err
				}
				f := store.InvoiceFilter{Year: p.Year, Search: p.Query}
				if p.Status != "overdue" {
					f.Status = invoice.Status(p.Status)
				}
				invs, err := a.Store.ListInvoices(c.ctx, cid, f)
				if err != nil {
					return nil, err
				}
				today := a.Today()
				out := []invoiceRow{}
				for _, inv := range invs {
					od := inv.Overdue(today)
					if p.Status == "overdue" && !od {
						continue
					}
					r := invoiceRow{ID: inv.ID, Number: inv.Number, Kind: inv.Kind, Status: inv.Status, Customer: inv.Customer.Name,
						ICO: inv.Customer.ICO, IssueDate: inv.IssueDate, DueDate: inv.DueDate, PaidDate: inv.PaidDate,
						Total: inv.Summary.Total, VAT: inv.Summary.VAT, Overdue: od, VS: inv.VariableSymbol}
					if od {
						r.DaysOverdue = inv.DueDate.DaysUntil(today)
					}
					out = append(out, r)
					if len(out) >= limit(p.Limit) {
						break
					}
				}
				return map[string]any{"invoices": out, "count": len(out)}, nil
			},
		},
		{
			Name: "get_invoice", Title: "Detail faktúry",
			Description: "Celá vydaná faktúra vrátane položiek a rekapitulácie DPH.",
			Props:       map[string]any{"company_id": pCompany, "id": pID}, Required: []string{"id"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64 `json:"company_id"`
					ID        int64 `json:"id"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermInvoicesRead)
				if err != nil {
					return nil, err
				}
				return a.Store.GetInvoice(c.ctx, cid, p.ID)
			},
		},
		{
			Name: "list_purchases", Title: "Prijaté faktúry",
			Description: "Prijaté faktúry (nákup). Stav: booked (neuhradená), paid, overdue.",
			Props: map[string]any{"company_id": pCompany, "year": pYear, "query": pQuery, "limit": pLimit,
				"status": map[string]any{"type": "string", "enum": []string{"booked", "paid", "overdue"}}},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64  `json:"company_id"`
					Year      int    `json:"year"`
					Status    string `json:"status"`
					Query     string `json:"query"`
					Limit     int    `json:"limit"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermPurchasesRead)
				if err != nil {
					return nil, err
				}
				purs, err := a.Store.ListPurchases(c.ctx, cid, store.PurchaseFilter{Year: p.Year, Search: p.Query})
				if err != nil {
					return nil, err
				}
				today := a.Today()
				type row struct {
					ID             int64           `json:"id"`
					Number         string          `json:"number"`
					SupplierNumber string          `json:"supplierNumber"`
					Supplier       string          `json:"supplier"`
					Description    string          `json:"description"`
					ReceivedDate   invoice.Date    `json:"receivedDate"`
					DueDate        invoice.Date    `json:"dueDate"`
					Status         purchase.Status `json:"status"`
					Total          money.Dec       `json:"total"`
					VAT            money.Dec       `json:"vat"`
					Overdue        bool            `json:"overdue"`
				}
				out := []row{}
				for _, pu := range purs {
					od := pu.Overdue(today)
					if (p.Status == "overdue" && !od) || (p.Status != "" && p.Status != "overdue" && string(pu.Status) != p.Status) {
						continue
					}
					out = append(out, row{pu.ID, pu.Number, pu.SupplierNumber, pu.Supplier.Name, pu.Description, pu.ReceivedDate, pu.DueDate, pu.Status, pu.Summary.Total, pu.Summary.VAT, od})
					if len(out) >= limit(p.Limit) {
						break
					}
				}
				return map[string]any{"purchases": out, "count": len(out)}, nil
			},
		},
		{
			Name: "list_contacts", Title: "Adresár",
			Description: "Odberatelia a dodávatelia v adresári firmy.",
			Props:       map[string]any{"company_id": pCompany, "query": pQuery, "limit": pLimit},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64  `json:"company_id"`
					Query     string `json:"query"`
					Limit     int    `json:"limit"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermContactsRead)
				if err != nil {
					return nil, err
				}
				list, err := a.Store.ListContacts(c.ctx, cid, p.Query)
				if err != nil {
					return nil, err
				}
				if n := limit(p.Limit); len(list) > n {
					list = list[:n]
				}
				return map[string]any{"contacts": list, "count": len(list)}, nil
			},
		},
		{
			Name: "list_bank_transactions", Title: "Bankové pohyby",
			Description: "Pohyby z importovaných výpisov. Stav: open (nespárované), matched, ignored.",
			Props: map[string]any{"company_id": pCompany, "year": pYear, "limit": pLimit,
				"status": map[string]any{"type": "string", "enum": []string{"open", "matched", "ignored"}}},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64  `json:"company_id"`
					Year      int    `json:"year"`
					Status    string `json:"status"`
					Limit     int    `json:"limit"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermBankRead)
				if err != nil {
					return nil, err
				}
				list, err := a.ListBank(c.ctx, cid, store.BankFilter{Year: p.Year, Status: bank.Status(p.Status)})
				if err != nil {
					return nil, err
				}
				if n := limit(p.Limit); len(list) > n {
					list = list[:n]
				}
				return map[string]any{"transactions": list, "count": len(list)}, nil
			},
		},
		{
			Name: "vat_return", Title: "Priznanie k DPH",
			Description: "Priznanie k DPH, kontrolný a súhrnný výkaz za obdobie (mesiac 2026-10 alebo štvrťrok 2026-Q3) vrátane dokladov na kontrolu.",
			Props:       map[string]any{"company_id": pCompany, "period": map[string]any{"type": "string", "description": "2026-10 alebo 2026-Q3"}},
			Required:    []string{"period"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64  `json:"company_id"`
					Period    string `json:"period"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermVATRead)
				if err != nil {
					return nil, err
				}
				r, _, err := a.VATReturn(c.ctx, cid, p.Period, money.Zero)
				return r, err
			},
		},
		// ---- zmeny ----
		{
			Name: "create_invoice_draft", Title: "Nový koncept faktúry", Write: true,
			Description: "Vytvorí koncept vydanej faktúry. DPH a súčty vypočíta Fiškus. Koncept sa dá skontrolovať v programe; vystaví sa až nástrojom issue_invoice.",
			Props: map[string]any{
				"company_id": pCompany,
				"contact_id": map[string]any{"type": "integer", "description": "Odberateľ z adresára (namiesto customer)"},
				"customer":   pParty,
				"issue_date": pDate, "delivery_date": pDate,
				"due_days": map[string]any{"type": "integer", "description": "Splatnosť v dňoch (predvolene podľa nastavení)"},
				"lines": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"description": map[string]any{"type": "string"},
						"quantity":    map[string]any{"type": "string", "description": "Množstvo, napr. \"10\""},
						"unit":        map[string]any{"type": "string", "description": "ks, hod, …"},
						"unit_price":  map[string]any{"type": "string", "description": "Cena za jednotku bez DPH, napr. \"45.50\""},
						"vat_rate":    map[string]any{"type": "integer", "description": "Sadzba DPH v % (23, 19, 5, 0)"},
					},
					"required": []string{"description", "quantity", "unit_price", "vat_rate"},
				}},
				"note": map[string]any{"type": "string"},
			},
			Required: []string{"lines"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID    int64          `json:"company_id"`
					ContactID    int64          `json:"contact_id"`
					Customer     *invoice.Party `json:"customer"`
					IssueDate    invoice.Date   `json:"issue_date"`
					DeliveryDate invoice.Date   `json:"delivery_date"`
					DueDays      int            `json:"due_days"`
					Note         string         `json:"note"`
					Lines        []struct {
						Description string    `json:"description"`
						Quantity    money.Dec `json:"quantity"`
						Unit        string    `json:"unit"`
						UnitPrice   money.Dec `json:"unit_price"`
						Rate        vat.Rate  `json:"vat_rate"`
					} `json:"lines"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermInvoicesWrite)
				if err != nil {
					return nil, err
				}
				inv := invoice.Invoice{IssueDate: p.IssueDate, DeliveryDate: p.DeliveryDate, Note: p.Note}
				switch {
				case p.ContactID != 0:
					ct, err := a.Store.GetContact(c.ctx, cid, p.ContactID)
					if err != nil {
						return nil, err
					}
					inv.Customer, inv.ContactID = app.PartyFromContact(ct), ct.ID
					if p.DueDays == 0 {
						p.DueDays = ct.DueDays
					}
				case p.Customer != nil:
					inv.Customer = *p.Customer
				default:
					return nil, errors.New("zadajte customer alebo contact_id")
				}
				if inv.Customer.Country == "" {
					inv.Customer.Country = "SK"
				}
				if inv.IssueDate.IsZero() {
					inv.IssueDate = a.Today()
				}
				if inv.DeliveryDate.IsZero() {
					inv.DeliveryDate = inv.IssueDate
				}
				if p.DueDays > 0 {
					inv.DueDate = inv.IssueDate.AddDays(p.DueDays)
				}
				for _, l := range p.Lines {
					unit := l.Unit
					if unit == "" {
						unit = "ks"
					}
					inv.Lines = append(inv.Lines, invoice.Line{Description: l.Description, Quantity: l.Quantity, Unit: unit, UnitPrice: l.UnitPrice, Rate: l.Rate})
				}
				saved, err := a.SaveDraft(c.ctx, cid, inv)
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": saved.ID, "status": saved.Status, "customer": saved.Customer.Name,
					"dueDate": saved.DueDate, "summary": saved.Summary,
					"message": "Koncept je uložený. Vystaví sa až nástrojom issue_invoice."}, nil
			},
		},
		{
			Name: "issue_invoice", Title: "Vystaviť faktúru", Write: true,
			Description: "Vystaví koncept faktúry: pridelí číslo a variabilný symbol. Vystavená faktúra sa už nedá meniť (opravuje sa dobropisom). Použi iba na výslovný pokyn používateľa.",
			Props:       map[string]any{"company_id": pCompany, "id": pID}, Required: []string{"id"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64 `json:"company_id"`
					ID        int64 `json:"id"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermInvoicesWrite)
				if err != nil {
					return nil, err
				}
				inv, err := a.Issue(c.ctx, cid, p.ID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": inv.ID, "number": inv.Number, "variableSymbol": inv.VariableSymbol, "total": inv.Summary.Total, "dueDate": inv.DueDate}, nil
			},
		},
		{
			Name: "mark_invoice_paid", Title: "Označiť faktúru ako uhradenú", Write: true,
			Description: "Označí vystavenú faktúru ako uhradenú k dátumu (predvolene dnes).",
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
				cid, err := c.company(p.CompanyID, auth.PermInvoicesWrite)
				if err != nil {
					return nil, err
				}
				inv, err := a.MarkPaid(c.ctx, cid, p.ID, p.Date)
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": inv.ID, "number": inv.Number, "status": inv.Status, "paidDate": inv.PaidDate}, nil
			},
		},
		{
			Name: "create_contact", Title: "Nový kontakt", Write: true,
			Description: "Pridá odberateľa alebo dodávateľa do adresára.",
			Props: map[string]any{"company_id": pCompany, "contact": pParty,
				"due_days": map[string]any{"type": "integer", "description": "Splatnosť faktúr pre tohto odberateľa v dňoch"}},
			Required: []string{"contact"},
			Run: func(c *callCtx, args json.RawMessage) (any, error) {
				var p struct {
					CompanyID int64         `json:"company_id"`
					Contact   invoice.Party `json:"contact"`
					DueDays   int           `json:"due_days"`
				}
				if err := parse(args, &p); err != nil {
					return nil, err
				}
				cid, err := c.company(p.CompanyID, auth.PermContactsWrite)
				if err != nil {
					return nil, err
				}
				x := p.Contact
				if x.Country == "" {
					x.Country = "SK"
				}
				return a.SaveContact(c.ctx, cid, store.Contact{Name: x.Name, Street: x.Street, City: x.City, Zip: x.Zip, Country: x.Country,
					ICO: x.ICO, DIC: x.DIC, ICDPH: x.ICDPH, Email: x.Email, DueDays: p.DueDays})
			},
		},
	}
}
