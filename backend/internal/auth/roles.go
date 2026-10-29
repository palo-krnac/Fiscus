// Package auth: používatelia, roly, práva a heslá.
//
// Rola sa prideľuje pre každú firmu zvlášť (účtovníčka kancelárie môže byť
// účtovník v 30 firmách a správca vo vlastnej). Práva kontroluje backend
// pri každej požiadavke — rozhranie iba skrýva, čo používateľ nesmie.
package auth

import "strings"

// Role je rola používateľa vo firme.
type Role string

const (
	RoleAdmin      Role = "admin"      // správca: všetko vrátane používateľov a nastavení
	RoleAccountant Role = "accountant" // účtovník: všetky doklady, DPH, výkazy, audit
	RoleInvoicer   Role = "invoicer"   // fakturant: vydané faktúry, dobropisy, adresár
	RoleClient     Role = "client"     // zákazník (klient kancelárie): čítanie dokladov
	RoleViewer     Role = "viewer"     // len čítanie (audítor, konateľ)
)

// Roles je číselník rolí v poradí pre ponuku.
var Roles = []struct {
	Code  Role   `json:"code"`
	Label string `json:"label"`
	Info  string `json:"info"`
}{
	{RoleAdmin, "Správca", "Všetko vrátane používateľov, nastavení firmy a záloh"},
	{RoleAccountant, "Účtovník", "Všetky doklady, nákup, DPH, výkazy a audit"},
	{RoleInvoicer, "Fakturant", "Vydané faktúry, dobropisy a adresár"},
	{RoleClient, "Zákazník", "Klient kancelárie: prezerá svoje doklady a výkazy"},
	{RoleViewer, "Len čítanie", "Prezerá doklady bez možnosti zmien"},
}

// Valid vráti true pre rolu z číselníka.
func (r Role) Valid() bool {
	for _, x := range Roles {
		if x.Code == r {
			return true
		}
	}
	return false
}

// Perm je jedno oprávnenie.
type Perm string

const (
	PermInvoicesRead   Perm = "invoices.read"
	PermInvoicesWrite  Perm = "invoices.write"
	PermPurchasesRead  Perm = "purchases.read"
	PermPurchasesWrite Perm = "purchases.write"
	PermContactsRead   Perm = "contacts.read"
	PermContactsWrite  Perm = "contacts.write"
	PermSettingsWrite  Perm = "settings.write"
	PermUsersManage    Perm = "users.manage"
	PermAuditRead      Perm = "audit.read"
	PermVATRead        Perm = "vat.read" // priznanie k DPH, kontrolný výkaz
	PermBankRead       Perm = "bank.read"
	PermBankWrite      Perm = "bank.write" // import výpisov a párovanie úhrad
	PermCashRead       Perm = "cash.read"
	PermCashWrite      Perm = "cash.write" // pokladničné doklady
)

var rolePerms = map[Role][]Perm{
	RoleAdmin: {PermInvoicesRead, PermInvoicesWrite, PermPurchasesRead, PermPurchasesWrite,
		PermContactsRead, PermContactsWrite, PermSettingsWrite, PermUsersManage, PermAuditRead, PermVATRead, PermBankRead, PermBankWrite, PermCashRead, PermCashWrite},
	RoleAccountant: {PermInvoicesRead, PermInvoicesWrite, PermPurchasesRead, PermPurchasesWrite,
		PermContactsRead, PermContactsWrite, PermAuditRead, PermVATRead, PermBankRead, PermBankWrite, PermCashRead, PermCashWrite},
	RoleInvoicer: {PermInvoicesRead, PermInvoicesWrite, PermContactsRead, PermContactsWrite, PermCashRead, PermCashWrite},
	RoleClient:   {PermInvoicesRead, PermPurchasesRead, PermContactsRead, PermVATRead, PermBankRead, PermCashRead},
	RoleViewer:   {PermInvoicesRead, PermPurchasesRead, PermContactsRead, PermVATRead, PermBankRead, PermCashRead},
}

// Can vráti true, ak rola má oprávnenie.
func (r Role) Can(p Perm) bool {
	for _, x := range rolePerms[r] {
		if x == p {
			return true
		}
	}
	return false
}

// Perms vráti zoznam oprávnení roly (pre frontend).
func (r Role) Perms() []Perm {
	return append([]Perm(nil), rolePerms[r]...)
}

// User je používateľ (v serverovom režime).
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Active       bool   `json:"active"`
	// SuperAdmin je správca celej inštalácie (napr. majiteľ účtovnej kancelárie):
	// zakladá firmy a používateľov a vidí všetky firmy.
	SuperAdmin bool `json:"superAdmin"`
}

// Membership je rola používateľa v jednej firme.
type Membership struct {
	UserID    int64 `json:"userId"`
	CompanyID int64 `json:"companyId"`
	Role      Role  `json:"role"`
}

// Session je prihlásenie (token je v cookie, v databáze iba jeho odtlačok).
type Session struct {
	TokenHash string
	UserID    int64
	ExpiresAt int64 // Unix sekundy
}

// IsWrite vráti true pre oprávnenie, ktoré mení dáta.
func (p Perm) IsWrite() bool {
	return strings.HasSuffix(string(p), ".write") || p == PermUsersManage
}

// APIKey je kľúč pre AI asistenta (MCP) alebo iný program.
// Uložený je iba hash; samotný kľúč sa zobrazí raz pri vytvorení.
type APIKey struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"userId"` // 0 = lokálny používateľ desktopovej verzie
	Name       string `json:"name"`
	Prefix     string `json:"prefix"` // začiatok kľúča na rozpoznanie (fk_ab12…)
	Hash       string `json:"-"`
	ReadOnly   bool   `json:"readOnly"`
	CreatedAt  int64  `json:"createdAt"`  // Unix sekundy
	LastUsedAt int64  `json:"lastUsedAt"` // 0 = ešte nepoužitý
	Revoked    bool   `json:"revoked"`
}
