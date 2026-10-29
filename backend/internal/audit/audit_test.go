package audit

import (
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
)

func chain(n int) []Entry {
	var out []Entry
	prev := ""
	for i := 1; i <= n; i++ {
		e := New(auth.Actor{UserID: 1, Name: "Jana", Channel: auth.ChannelWeb}, 1, CatInvoices, "issue", "invoice", int64(i),
			"Vystavená faktúra", nil, map[string]string{"number": "FA2026000" + string(rune('0'+i))})
		e.ID = int64(i)
		Seal(&e, prev)
		prev = e.Hash
		out = append(out, e)
	}
	return out
}

func TestVerifyChain(t *testing.T) {
	c := chain(5)
	if err := Verify(c); err != nil {
		t.Fatal(err)
	}
	// zmena obsahu
	bad := append([]Entry(nil), c...)
	bad[2].Summary = "Nič sa nestalo"
	if err := Verify(bad); err == nil || !strings.Contains(err.Error(), "záznam 3") {
		t.Fatalf("zmena záznamu 3: %v", err)
	}
	// zmazanie záznamu
	gap := append(append([]Entry(nil), c[:2]...), c[3:]...)
	if err := Verify(gap); err == nil || !strings.Contains(err.Error(), "záznam 4") {
		t.Fatalf("zmazaný záznam: %v", err)
	}
	// prepočítaný hash bez väzby na predchádzajúci tiež nepomôže
	forged := append([]Entry(nil), c...)
	forged[2].Summary = "Podvrh"
	forged[2].Hash = digest(forged[2])
	if Verify(forged) == nil {
		t.Fatal("podvrhnutý záznam s novým hashom")
	}
}

func TestNewStoresJSON(t *testing.T) {
	e := New(auth.Local, 2, CatSettings, "update", "company", 2, "Zmena nastavení", map[string]int{"a": 1}, nil)
	if string(e.Before) != `{"a":1}` || e.After != nil || e.UserName != "Lokálny používateľ" {
		t.Fatalf("%+v", e)
	}
}
