package api

import (
	"context"
	"io"
	"net/http"

	"github.com/palo-krnac/fiscus/backend/internal/app"
	"github.com/palo-krnac/fiscus/backend/internal/catalog"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	list, err := s.App.Items(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// saveItem: POST (nová) alebo PUT /{id} (úprava) položky cenníka.
func (s *Server) saveItem(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	var it catalog.Item
	if !decodeLoose(w, r, &it) {
		return
	}
	it.ID = 0
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		it.ID = id
	}
	isNew := it.ID == 0
	out, err := s.App.SaveItem(r.Context(), cid, it)
	if err != nil {
		fail(w, err)
		return
	}
	code := http.StatusOK
	if isNew {
		code = http.StatusCreated
	}
	writeJSON(w, code, out)
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.App.DeleteItem(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// importItems: POST …/items/import — telo je súbor CSV alebo XLSX;
// ?apply=1 import vykoná, inak vráti náhľad.
func (s *Server) importItems(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, app.MaxImportBytes))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Súbor je príliš veľký (najviac 5 MB)", nil)
		return
	}
	res, err := s.App.ImportItems(r.Context(), cid, data, r.URL.Query().Get("apply") == "1")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// importMigrate: POST …/import/{source} — prenos dát z iného programu
// (pohoda, money: XML export; omega: textový export KROS Omega; idoklad: ZIP s XML pre POHODU; mrp: XML tabuľky MRP-K/S; table: Excel/CSV s ?kind=contacts|items|invoices|purchases). ?apply=1 prenos vykoná, inak vráti
// náhľad; ?paidUntil=RRRR-MM-DD označí faktúry so splatnosťou do toho dňa
// ako uhradené.
func (s *Server) importMigrate(w http.ResponseWriter, r *http.Request) {
	cid, _ := pathID(w, r, "cid")
	report := func(f func(context.Context, int64, []byte, app.MigrateOptions, bool) (app.ImportReport, error)) func(context.Context, int64, []byte, app.MigrateOptions, bool) (any, error) {
		return func(ctx context.Context, cid int64, data []byte, opt app.MigrateOptions, apply bool) (any, error) {
			return f(ctx, cid, data, opt, apply)
		}
	}
	run := map[string]func(context.Context, int64, []byte, app.MigrateOptions, bool) (any, error){
		"pohoda":  report(s.App.ImportPohoda),
		"money":   report(s.App.ImportMoney),
		"omega":   report(s.App.ImportOmega),
		"idoklad": report(s.App.ImportIDoklad),
		"mrp":     report(s.App.ImportMRP),
		"table": func(ctx context.Context, cid int64, data []byte, opt app.MigrateOptions, apply bool) (any, error) {
			return s.App.ImportTable(ctx, cid, data, opt, apply)
		},
	}[r.PathValue("source")]
	if run == nil {
		writeErr(w, http.StatusNotFound, "Neznámy zdroj prenosu", nil)
		return
	}
	opt := app.MigrateOptions{Kind: r.URL.Query().Get("kind")}
	if v := r.URL.Query().Get("paidUntil"); v != "" {
		d, err := invoice.ParseDate(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Chybný dátum", []invoice.FieldError{{Field: "paidUntil", Message: "Zadajte dátum v tvare RRRR-MM-DD"}})
			return
		}
		opt.PaidUntil = d
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, app.MaxMigrateBytes))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "Súbor je príliš veľký (najviac 50 MB)", nil)
		return
	}
	res, err := run(r.Context(), cid, data, opt, r.URL.Query().Get("apply") == "1")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
