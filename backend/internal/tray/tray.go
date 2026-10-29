// Package tray zobrazí ikonu Fiškusa v oblasti oznámení (vpravo dole na
// paneli úloh Windows): je vidieť, že program beží, akú má verziu, a dá sa
// otvoriť alebo ukončiť bez Správcu úloh. Na iných systémoch nerobí nič.
package tray

// Menu sú akcie ikony.
type Menu struct {
	Title   string // napr. „Fiškus 0.1.0“
	Tooltip string // text pri ukázaní myšou
	Open    func() // klik na ikonu / „Otvoriť Fiškus“
	Quit    func() // „Ukončiť Fiškus“
}

// Items vráti položky ponuky v poradí (bez oddeľovačov) — spoločné pre
// všetky systémy a testy.
func (m Menu) Items() []string {
	return []string{m.Title, "Otvoriť Fiškus", "Ukončiť Fiškus"}
}
