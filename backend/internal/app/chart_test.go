package app

import (
	"errors"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/chart"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func TestChart(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	v, err := a.Chart(ctx, c.ID)
	if err != nil || len(v.Accounts) < 200 || len(v.Framework) != len(v.Accounts) || len(v.Classes) != 8 {
		t.Fatalf("predvolený rozvrh: %d účtov %v", len(v.Accounts), err)
	}
	if _, err := a.SaveAccount(ctx, c.ID, "", chart.Account{Number: "221.001", Name: "Tatra banka", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveAccount(ctx, c.ID, "", chart.Account{Number: "221001", Name: "Iná"}); !fieldErr(err, "number") {
		t.Errorf("duplicita: %v", err)
	}
	if _, err := a.SaveAccount(ctx, c.ID, "221001", chart.Account{Number: "221001", Name: "Tatra banka – bežný účet", Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteAccount(ctx, c.ID, "221"); !errors.Is(err, chart.ErrInUse) {
		t.Errorf("syntetický s analytikou: %v", err)
	}
	if err := a.DeleteAccount(ctx, c.ID, "999"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("neexistujúci: %v", err)
	}
	if _, err := a.SaveCenter(ctx, c.ID, "", chart.Center{Code: "BB", Name: "Banská Bystrica", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveJob(ctx, c.ID, "", chart.Job{Code: "Z1", Name: "Linka Brezno", Active: true}); err != nil {
		t.Fatal(err)
	}
	v, _ = a.Chart(ctx, c.ID)
	acc, _ := chart.Chart{Accounts: v.Accounts}.Find("221001")
	if acc.Name != "Tatra banka – bežný účet" || len(v.Centers) != 1 || len(v.Jobs) != 1 {
		t.Fatalf("uložené: %+v %+v %+v", acc, v.Centers, v.Jobs)
	}
	if err := a.DeleteJob(ctx, c.ID, "Z1"); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteCenter(ctx, c.ID, "BB"); err != nil {
		t.Fatal(err)
	}

	// práva: klient číta, nemení
	o := newOffice(t)
	if _, err := o.a.Chart(as(o.klient), o.beta.ID); err != nil {
		t.Errorf("klient číta: %v", err)
	}
	if _, err := o.a.SaveCenter(as(o.klient), o.beta.ID, "", chart.Center{Code: "X", Name: "x"}); !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrReadOnly) {
		t.Errorf("klient mení: %v", err)
	}
	if _, err := o.a.Chart(as(o.klient), o.alfa.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("cudzia firma: %v", err)
	}
}
