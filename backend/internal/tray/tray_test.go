package tray

import "testing"

func TestMenuItems(t *testing.T) {
	m := Menu{Title: "Fiškus 0.1.0"}
	got := m.Items()
	if len(got) != 3 || got[0] != "Fiškus 0.1.0" || got[2] != "Ukončiť Fiškus" {
		t.Fatal(got)
	}
}
