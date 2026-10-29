// Package audit je nemenný záznam všetkých zmien: kto, kedy, cez aký kanál
// (web, API, MCP), v ktorej firme, čo urobil a ako vyzerali dáta pred a po.
//
// Záznamy sú zreťazené hashom (ako účtovná kniha s očíslovanými stranami):
// každý záznam obsahuje hash predchádzajúceho, takže dodatočná úprava
// alebo zmazanie záznamu v databáze sa odhalí funkciou Verify.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
)

// Category je oblasť, ktorej sa záznam týka (filter v audite).
type Category string

const (
	CatInvoices  Category = "invoices"  // vydané faktúry a dobropisy
	CatPurchases Category = "purchases" // prijaté faktúry
	CatContacts  Category = "contacts"  // adresár
	CatSettings  Category = "settings"  // nastavenia a číselníky firmy
	CatUsers     Category = "users"     // používatelia a roly
	CatAuth      Category = "auth"      // prihlásenia
	CatBackups   Category = "backups"   // zálohy a obnovy
)

// Categories je číselník kategórií pre filter.
var Categories = []struct {
	Code  Category `json:"code"`
	Label string   `json:"label"`
}{
	{CatInvoices, "Vydané faktúry"},
	{CatPurchases, "Prijaté faktúry"},
	{CatContacts, "Adresár"},
	{CatSettings, "Nastavenia"},
	{CatUsers, "Používatelia"},
	{CatAuth, "Prihlásenia"},
	{CatBackups, "Zálohy"},
}

// Entry je jeden záznam auditu.
type Entry struct {
	ID         int64           `json:"id"`
	Time       time.Time       `json:"time"`
	CompanyID  int64           `json:"companyId"` // 0 = mimo firmy (prihlásenie, systém)
	UserID     int64           `json:"userId"`
	UserName   string          `json:"userName"`
	Channel    auth.Channel    `json:"channel"`
	IP         string          `json:"ip,omitempty"`
	Category   Category        `json:"category"`
	Action     string          `json:"action"` // create, update, issue, pay, delete, login…
	EntityType string          `json:"entityType,omitempty"`
	EntityID   int64           `json:"entityId,omitempty"`
	Summary    string          `json:"summary"` // čitateľný popis: „Vystavená faktúra FA20260039“
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	PrevHash   string          `json:"prevHash"`
	Hash       string          `json:"hash"`
}

// New pripraví záznam s údajmi o actorovi. before/after sa uložia ako JSON.
func New(a auth.Actor, companyID int64, cat Category, action, entityType string, entityID int64, summary string, before, after any) Entry {
	return Entry{
		Time:       time.Now().UTC().Truncate(time.Millisecond),
		CompanyID:  companyID,
		UserID:     a.UserID,
		UserName:   a.Name,
		Channel:    a.Channel,
		IP:         a.IP,
		Category:   cat,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Summary:    summary,
		Before:     toJSON(before),
		After:      toJSON(after),
	}
}

func toJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return nil
	}
	return b
}

// Seal nastaví hash záznamu podľa predchádzajúceho záznamu v reťazci.
func Seal(e *Entry, prevHash string) {
	e.PrevHash = prevHash
	e.Hash = digest(*e)
}

func digest(e Entry) string {
	e.Hash = ""
	e.Time = e.Time.UTC()
	b, _ := json.Marshal(e) // poradie polí je pevné podľa štruktúry
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Verify skontroluje celý reťazec (záznamy zoradené podľa ID). Vráti chybu
// s ID prvého porušeného záznamu.
func Verify(entries []Entry) error {
	prev := ""
	for _, e := range entries {
		if e.PrevHash != prev {
			return fmt.Errorf("záznam %d: chýba alebo bol zmenený predchádzajúci záznam", e.ID)
		}
		if digest(e) != e.Hash {
			return fmt.Errorf("záznam %d bol dodatočne zmenený", e.ID)
		}
		prev = e.Hash
	}
	return nil
}

// Filter obmedzuje výpis auditu.
type Filter struct {
	CompanyIDs []int64 // nil = všetky (iba systémový správca)
	Category   Category
	UserID     int64
	From, To   time.Time // To je vrátane celého dňa (volajúci posiela začiatok nasledujúceho dňa)
	Search     string
	Limit      int
	Offset     int
}
