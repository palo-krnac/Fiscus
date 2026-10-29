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
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/registry"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// Server obsluhuje API a statický frontend.
type Server struct {
	App *app.App
	Web fs.FS // zostavený frontend (index.html, assets/...); môže byť nil
	// Registry je vyhľadávanie firiem podľa IČO (RPO); nil = vypnuté.
	Registry registry.Lookup
}

// Handler vráti http.Handler s celým API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/capabilities", s.capabilities)
	mux.HandleFunc("GET /api/companies", s.listCompanies)
	mux.HandleFunc("POST /api/companies", s.createCompany)
	mux.HandleFunc("PUT /api/companies/{cid}", s.updateCompany)
	mux.HandleFunc("GET /api/registry/{ico}", s.lookupICO)
	mux.HandleFunc("POST /api/companies/{cid}/invoices/preview", s.preview)
	mux.HandleFunc("GET /api/companies/{cid}/contacts", s.listContacts)
	mux.HandleFunc("POST /api/companies/{cid}/contacts", s.saveContact)
	mux.HandleFunc("GET /api/companies/{cid}/contacts/{id}", s.getContact)
	mux.HandleFunc("PUT /api/companies/{cid}/contacts/{id}", s.saveContact)
	mux.HandleFunc("DELETE /api/companies/{cid}/contacts/{id}", s.deleteContact)
	mux.HandleFunc("GET /api/companies/{cid}/invoices", s.listInvoices)
	mux.HandleFunc("POST /api/companies/{cid}/invoices", s.saveInvoice)
	mux.HandleFunc("GET /api/companies/{cid}/invoices/{id}", s.getInvoice)
	mux.HandleFunc("PUT /api/companies/{cid}/invoices/{id}", s.saveInvoice)
	mux.HandleFunc("POST /api/companies/{cid}/invoices/{id}/issue", s.issue)
	mux.HandleFunc("POST /api/companies/{cid}/invoices/{id}/pay", s.pay)
	mux.HandleFunc("POST /api/companies/{cid}/invoices/{id}/cancel", s.cancel)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "neznáma adresa API", nil)
	})
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
	case errors.Is(err, invoice.ErrLocked), errors.Is(err, invoice.ErrBadTransition):
		writeErr(w, http.StatusConflict, err.Error(), nil)
	case errors.Is(err, app.ErrCompanyLimit):
		writeErr(w, http.StatusForbidden, err.Error(), nil)
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
	return cid, true
}

// ---- handlery ----

func (s *Server) capabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.App.Caps)
}

func (s *Server) listCompanies(w http.ResponseWriter, r *http.Request) {
	list, err := s.App.Store.ListCompanies(r.Context())
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
	writeJSON(w, http.StatusOK, inv)
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(inv); err != nil {
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
		Date invoice.Date `json:"date"`
	}
	if r.ContentLength > 0 && !decode(w, r, &body) {
		return
	}
	s.action(w, r, func(cid, id int64) (invoice.Invoice, error) {
		return s.App.MarkPaid(r.Context(), cid, id, body.Date)
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
