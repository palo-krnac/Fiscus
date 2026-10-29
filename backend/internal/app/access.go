package app

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

var (
	ErrBadLogin     = errors.New("Nesprávny e-mail alebo heslo")
	ErrNoSession    = errors.New("Prihláste sa")
	ErrForbidden    = errors.New("Na túto akciu nemáte oprávnenie")
	ErrLastAdmin    = errors.New("Firma musí mať aspoň jedného správcu")
	ErrSelfDemote   = errors.New("Vlastnú rolu správcu si nemôžete odobrať — požiadajte iného správcu")
	ErrReadOnly     = errors.New("kľúč je iba na čítanie — zmeny nie sú povolené")
	SessionDuration = 14 * 24 * time.Hour
)

// dummyHash: rovnaký čas odpovede aj pre neexistujúci e-mail (počíta sa raz, pri prvom použití).
var dummyHash = sync.OnceValue(func() string {
	h, _ := auth.HashPassword("fiskus-dummy-heslo")
	return h
})

// record zapíše záznam do auditu. Zlyhanie auditu sa zaloguje (nemá zhodiť
// už vykonanú zmenu), ale v produkcii by sa na to malo upozorniť.
// Audit zapíše záznam do auditu (pre iné balíky, napr. MCP server).
func (a *App) Audit(ctx context.Context, companyID int64, cat audit.Category, action, entityType string, entityID int64, summary string, before, after any) {
	a.record(ctx, companyID, cat, action, entityType, entityID, summary, before, after)
}

func (a *App) record(ctx context.Context, companyID int64, cat audit.Category, action, entityType string, entityID int64, summary string, before, after any) {
	e := audit.New(auth.ActorFrom(ctx), companyID, cat, action, entityType, entityID, summary, before, after)
	if err := a.Store.AppendAudit(ctx, &e); err != nil {
		log.Printf("AUDIT ZLYHAL (%s %s): %v", cat, summary, err)
	}
}

// ---- prihlásenie ----

// Login overí e-mail a heslo a vytvorí reláciu. Vráti token do cookie.
func (a *App) Login(ctx context.Context, email, password string) (string, auth.User, error) {
	u, err := a.Store.GetUserByEmail(ctx, email)
	if err != nil || !u.Active {
		auth.CheckPassword(dummyHash(), password) // vyrovnanie času
		a.record(ctx, 0, audit.CatAuth, "login_failed", "user", u.ID, "Neúspešné prihlásenie: "+strings.TrimSpace(email), nil, nil)
		return "", auth.User{}, ErrBadLogin
	}
	if !auth.CheckPassword(u.PasswordHash, password) {
		a.record(ctx, 0, audit.CatAuth, "login_failed", "user", u.ID, "Neúspešné prihlásenie: "+u.Email, nil, nil)
		return "", auth.User{}, ErrBadLogin
	}
	if u.MFA.Enabled {
		ticket, err := a.mfaTickets.issue(u.ID, a.now())
		if err != nil {
			return "", u, err
		}
		return "", u, &MFARequired{Ticket: ticket}
	}
	return a.startSession(ctx, u)
}

// startSession vytvorí reláciu po úspešnom overení.
func (a *App) startSession(ctx context.Context, u auth.User) (string, auth.User, error) {
	token, hash, err := auth.NewToken()
	if err != nil {
		return "", u, err
	}
	exp := time.Now().Add(SessionDuration).Unix()
	if err := a.Store.CreateSession(ctx, auth.Session{TokenHash: hash, UserID: u.ID, ExpiresAt: exp}); err != nil {
		return "", u, err
	}
	ctx = auth.WithActor(ctx, actorOf(u, auth.ActorFrom(ctx)))
	a.record(ctx, 0, audit.CatAuth, "login", "user", u.ID, "Prihlásenie "+u.Email, nil, nil)
	return token, u, nil
}

func actorOf(u auth.User, base auth.Actor) auth.Actor {
	return auth.Actor{UserID: u.ID, Name: u.Name, Channel: base.Channel, IP: base.IP, SuperAdmin: u.SuperAdmin}
}

// SessionActor vráti actora pre platný token relácie.
func (a *App) SessionActor(ctx context.Context, token string, channel auth.Channel, ip string) (auth.Actor, error) {
	if token == "" {
		return auth.Actor{}, ErrNoSession
	}
	ss, err := a.Store.GetSession(ctx, auth.TokenHash(token))
	if err != nil {
		return auth.Actor{}, ErrNoSession
	}
	if time.Now().Unix() > ss.ExpiresAt {
		_ = a.Store.DeleteSession(ctx, ss.TokenHash)
		return auth.Actor{}, ErrNoSession
	}
	u, err := a.Store.GetUser(ctx, ss.UserID)
	if err != nil || !u.Active {
		return auth.Actor{}, ErrNoSession
	}
	return actorOf(u, auth.Actor{Channel: channel, IP: ip}), nil
}

// Logout zruší reláciu.
func (a *App) Logout(ctx context.Context, token string) {
	_ = a.Store.DeleteSession(ctx, auth.TokenHash(token))
	act := auth.ActorFrom(ctx)
	a.record(ctx, 0, audit.CatAuth, "logout", "user", act.UserID, "Odhlásenie", nil, nil)
}

// EnsureAdmin vytvorí prvého správcu inštalácie, ak ešte nie je žiadny používateľ.
func (a *App) EnsureAdmin(ctx context.Context, email, name, password string) (bool, error) {
	users, err := a.Store.ListUsers(ctx)
	if err != nil || len(users) > 0 {
		return false, err
	}
	if err := auth.ValidatePassword(password); err != nil {
		return false, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, err
	}
	u, err := a.Store.CreateUser(ctx, auth.User{Email: email, Name: name, PasswordHash: hash, Active: true, SuperAdmin: true})
	if err != nil {
		return false, err
	}
	a.record(ctx, 0, audit.CatUsers, "create", "user", u.ID, "Vytvorený správca inštalácie "+email, nil, publicUser(u))
	return true, nil
}

// ---- roly ----

// RoleIn vráti rolu actora vo firme ("" = nie je členom).
func (a *App) RoleIn(ctx context.Context, companyID int64) (auth.Role, error) {
	act := auth.ActorFrom(ctx)
	if act.SuperAdmin || act.Channel == auth.ChannelSystem {
		return auth.RoleAdmin, nil
	}
	ms, err := a.Store.UserMemberships(ctx, act.UserID)
	if err != nil {
		return "", err
	}
	for _, m := range ms {
		if m.CompanyID == companyID {
			return m.Role, nil
		}
	}
	return "", nil
}

// Require overí, že actor má vo firme oprávnenie.
func (a *App) Require(ctx context.Context, companyID int64, p auth.Perm) error {
	role, err := a.RoleIn(ctx, companyID)
	if err != nil {
		return err
	}
	if role == "" {
		return store.ErrNotFound // cudzia firma sa tvári ako neexistujúca
	}
	if p != "" && !role.Can(p) {
		return ErrForbidden
	}
	if p.IsWrite() && auth.ActorFrom(ctx).ReadOnly {
		return ErrReadOnly
	}
	return nil
}

// VisibleCompanies vráti firmy, ku ktorým má actor prístup.
func (a *App) VisibleCompanies(ctx context.Context) ([]store.Company, error) {
	all, err := a.Store.ListCompanies(ctx)
	if err != nil {
		return nil, err
	}
	act := auth.ActorFrom(ctx)
	if act.SuperAdmin {
		return all, nil
	}
	ms, err := a.Store.UserMemberships(ctx, act.UserID)
	if err != nil {
		return nil, err
	}
	in := map[int64]bool{}
	for _, m := range ms {
		in[m.CompanyID] = true
	}
	out := []store.Company{}
	for _, c := range all {
		if in[c.ID] {
			out = append(out, c)
		}
	}
	return out, nil
}

// ---- správa používateľov firmy ----

// Member je používateľ s rolou vo firme.
type Member struct {
	User publicUserT `json:"user"`
	Role auth.Role   `json:"role"`
}

type publicUserT struct {
	ID         int64  `json:"id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	Active     bool   `json:"active"`
	SuperAdmin bool   `json:"superAdmin"`
	TwoFactor  bool   `json:"twoFactor"`
}

func publicUser(u auth.User) publicUserT {
	return publicUserT{u.ID, u.Email, u.Name, u.Active, u.SuperAdmin, u.MFA.Enabled}
}

// CompanyUsers vráti používateľov firmy s rolami.
func (a *App) CompanyUsers(ctx context.Context, companyID int64) ([]Member, error) {
	ms, err := a.Store.CompanyMembers(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := []Member{}
	for _, m := range ms {
		u, err := a.Store.GetUser(ctx, m.UserID)
		if err != nil {
			continue
		}
		out = append(out, Member{publicUser(u), m.Role})
	}
	return out, nil
}

// NewMember sú údaje na pridanie používateľa do firmy.
type NewMember struct {
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Password string    `json:"password"` // iba pre nového používateľa
	Role     auth.Role `json:"role"`
}

// AddMember pridá do firmy existujúceho používateľa (podľa e-mailu) alebo vytvorí nového.
func (a *App) AddMember(ctx context.Context, companyID int64, nm NewMember) (Member, error) {
	nm.Email = strings.ToLower(strings.TrimSpace(nm.Email))
	var fe []invoice.FieldError
	if !strings.Contains(nm.Email, "@") || !validEmail(nm.Email) {
		fe = append(fe, invoice.FieldError{Field: "email", Message: "Zadajte platný e-mail"})
	}
	if !nm.Role.Valid() {
		fe = append(fe, invoice.FieldError{Field: "role", Message: "Vyberte rolu"})
	}
	u, err := a.Store.GetUserByEmail(ctx, nm.Email)
	isNew := errors.Is(err, store.ErrNotFound)
	if err != nil && !isNew {
		return Member{}, err
	}
	if isNew {
		if strings.TrimSpace(nm.Name) == "" {
			fe = append(fe, invoice.FieldError{Field: "name", Message: "Zadajte meno"})
		}
		if err := auth.ValidatePassword(nm.Password); err != nil {
			fe = append(fe, invoice.FieldError{Field: "password", Message: err.Error()})
		}
	}
	if len(fe) > 0 {
		return Member{}, &invoice.ValidationError{Fields: fe}
	}
	if isNew {
		hash, err := auth.HashPassword(nm.Password)
		if err != nil {
			return Member{}, err
		}
		u, err = a.Store.CreateUser(ctx, auth.User{Email: nm.Email, Name: strings.TrimSpace(nm.Name), PasswordHash: hash, Active: true})
		if err != nil {
			return Member{}, err
		}
		a.record(ctx, companyID, audit.CatUsers, "create", "user", u.ID, "Nový používateľ "+u.Email, nil, publicUser(u))
	}
	if err := a.Store.SetMembership(ctx, auth.Membership{UserID: u.ID, CompanyID: companyID, Role: nm.Role}); err != nil {
		return Member{}, err
	}
	a.record(ctx, companyID, audit.CatUsers, "grant", "user", u.ID,
		"Prístup do firmy pre "+u.Email+": "+roleLabel(nm.Role), nil, map[string]any{"email": u.Email, "role": nm.Role})
	return Member{publicUser(u), nm.Role}, nil
}

func roleLabel(r auth.Role) string {
	for _, x := range auth.Roles {
		if x.Code == r {
			return x.Label
		}
	}
	return string(r)
}

// SetRole zmení rolu používateľa vo firme.
func (a *App) SetRole(ctx context.Context, companyID, userID int64, role auth.Role) error {
	if !role.Valid() {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "role", Message: "Neznáma rola"}}}
	}
	old, err := a.memberRole(ctx, companyID, userID)
	if err != nil {
		return err
	}
	if old == auth.RoleAdmin && role != auth.RoleAdmin {
		if userID == auth.ActorFrom(ctx).UserID {
			return ErrSelfDemote
		}
		if err := a.ensureOtherAdmin(ctx, companyID, userID); err != nil {
			return err
		}
	}
	if err := a.Store.SetMembership(ctx, auth.Membership{UserID: userID, CompanyID: companyID, Role: role}); err != nil {
		return err
	}
	u, _ := a.Store.GetUser(ctx, userID)
	a.record(ctx, companyID, audit.CatUsers, "role", "user", userID,
		"Zmena roly "+u.Email+": "+roleLabel(old)+" → "+roleLabel(role), map[string]any{"role": old}, map[string]any{"role": role})
	return nil
}

// RemoveMember odoberie používateľovi prístup do firmy.
func (a *App) RemoveMember(ctx context.Context, companyID, userID int64) error {
	old, err := a.memberRole(ctx, companyID, userID)
	if err != nil {
		return err
	}
	if old == auth.RoleAdmin {
		if err := a.ensureOtherAdmin(ctx, companyID, userID); err != nil {
			return err
		}
	}
	if err := a.Store.RemoveMembership(ctx, userID, companyID); err != nil {
		return err
	}
	u, _ := a.Store.GetUser(ctx, userID)
	a.record(ctx, companyID, audit.CatUsers, "revoke", "user", userID, "Odobratý prístup do firmy: "+u.Email,
		map[string]any{"email": u.Email, "role": old}, nil)
	return nil
}

func (a *App) memberRole(ctx context.Context, companyID, userID int64) (auth.Role, error) {
	ms, err := a.Store.CompanyMembers(ctx, companyID)
	if err != nil {
		return "", err
	}
	for _, m := range ms {
		if m.UserID == userID {
			return m.Role, nil
		}
	}
	return "", store.ErrNotFound
}

func (a *App) ensureOtherAdmin(ctx context.Context, companyID, exceptUser int64) error {
	ms, err := a.Store.CompanyMembers(ctx, companyID)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m.UserID != exceptUser && m.Role == auth.RoleAdmin {
			return nil
		}
	}
	return ErrLastAdmin
}

// ChangePassword zmení heslo prihláseného používateľa a odhlási ho všade inde.
func (a *App) ChangePassword(ctx context.Context, current, next string) error {
	act := auth.ActorFrom(ctx)
	u, err := a.Store.GetUser(ctx, act.UserID)
	if err != nil {
		return err
	}
	if !auth.CheckPassword(u.PasswordHash, current) {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "current", Message: "Súčasné heslo nesedí"}}}
	}
	if err := auth.ValidatePassword(next); err != nil {
		return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: "password", Message: err.Error()}}}
	}
	if u.PasswordHash, err = auth.HashPassword(next); err != nil {
		return err
	}
	if err := a.Store.UpdateUser(ctx, u); err != nil {
		return err
	}
	_ = a.Store.DeleteUserSessions(ctx, u.ID)
	a.record(ctx, 0, audit.CatAuth, "password", "user", u.ID, "Zmena hesla "+u.Email, nil, nil)
	return nil
}

// AuditFor vráti audit firmy (alebo všetkých firiem pre správcu inštalácie, companyID 0).
func (a *App) AuditFor(ctx context.Context, companyID int64, f audit.Filter) ([]audit.Entry, int, error) {
	act := auth.ActorFrom(ctx)
	switch {
	case companyID != 0:
		if err := a.Require(ctx, companyID, auth.PermAuditRead); err != nil {
			return nil, 0, err
		}
		f.CompanyIDs = []int64{companyID}
	case act.SuperAdmin:
		// celá inštalácia; f.CompanyIDs môže zúžiť na vybrané firmy (0 = mimo firmy)
	default:
		return nil, 0, ErrForbidden
	}
	return a.Store.ListAudit(ctx, f)
}

// VerifyAudit overí celistvosť reťazca auditu.
func (a *App) VerifyAudit(ctx context.Context) (int, error) {
	if !auth.ActorFrom(ctx).SuperAdmin {
		return 0, ErrForbidden
	}
	chain, err := a.Store.AuditChain(ctx)
	if err != nil {
		return 0, err
	}
	return len(chain), audit.Verify(chain)
}
