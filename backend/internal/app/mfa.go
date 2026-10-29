package app

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

// Dvojfázové overenie (2FA) v serverovom režime. Po správnom hesle dostane
// prehliadač iba lístok; reláciu vytvorí až správny kód z aplikácie alebo
// záložný kód. Nastavenie, vypnutie a nové záložné kódy vyžadujú heslo.

// MFAIssuer je názov, pod ktorým sa účet zobrazí v aplikácii.
const MFAIssuer = "Fiškus"

const (
	mfaTicketTTL   = 5 * time.Minute
	mfaTicketTries = 5
)

var (
	// ErrMFATicket: lístok vypršal, bol použitý alebo bolo priveľa pokusov.
	ErrMFATicket = errors.New("Prihlásenie vypršalo — zadajte znova e-mail a heslo")
	ErrMFACode   = errors.New("Kód nesedí")
)

// MFARequired vráti Login, keď heslo sedí a treba ešte kód.
type MFARequired struct{ Ticket string }

func (e *MFARequired) Error() string { return "Zadajte kód z aplikácie" }

type mfaTicket struct {
	userID int64
	exp    time.Time
	tries  int
}

// mfaTickets sú lístky v pamäti (iba odtlačky); po reštarte sa treba prihlásiť znova.
type mfaTickets struct {
	mu sync.Mutex
	m  map[string]*mfaTicket
}

func (t *mfaTickets) issue(userID int64, now time.Time) (string, error) {
	token, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]*mfaTicket{}
	}
	for k, v := range t.m { // upratanie starých
		if now.After(v.exp) {
			delete(t.m, k)
		}
	}
	t.m[hash] = &mfaTicket{userID: userID, exp: now.Add(mfaTicketTTL)}
	return token, nil
}

// try započíta pokus; vráti používateľa, ak lístok ešte platí.
func (t *mfaTickets) try(token string, now time.Time) (int64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	h := auth.TokenHash(token)
	v := t.m[h]
	if v == nil || now.After(v.exp) || v.tries >= mfaTicketTries {
		delete(t.m, h)
		return 0, false
	}
	v.tries++
	return v.userID, true
}

func (t *mfaTickets) drop(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, auth.TokenHash(token))
}

// LoginMFA dokončí prihlásenie kódom z aplikácie alebo záložným kódom.
func (a *App) LoginMFA(ctx context.Context, ticket, code string) (string, auth.User, error) {
	uid, ok := a.mfaTickets.try(ticket, a.now())
	if !ok {
		return "", auth.User{}, ErrMFATicket
	}
	u, err := a.Store.GetUser(ctx, uid)
	if err != nil || !u.Active || !u.MFA.Enabled {
		a.mfaTickets.drop(ticket)
		return "", auth.User{}, ErrMFATicket
	}
	uctx := auth.WithActor(ctx, actorOf(u, auth.ActorFrom(ctx)))
	how, err := a.checkMFACode(uctx, &u, code)
	if err != nil {
		a.record(uctx, 0, audit.CatAuth, "login_failed", "user", u.ID, "Nesprávny kód 2FA: "+u.Email, nil, nil)
		return "", auth.User{}, err
	}
	a.mfaTickets.drop(ticket)
	if how == "recovery" {
		a.record(uctx, 0, audit.CatAuth, "mfa_recovery_used", "user", u.ID,
			"Prihlásenie záložným kódom "+u.Email+" (zostáva "+strconv.Itoa(len(u.MFA.Recovery))+")", nil, nil)
	}
	return a.startSession(ctx, u)
}

// checkMFACode overí kód a uloží zmenu (posledný krok, spotrebovaný záložný kód).
func (a *App) checkMFACode(ctx context.Context, u *auth.User, code string) (string, error) {
	if auth.IsTOTPCode(code) {
		step, ok := auth.VerifyTOTP(u.MFA.Secret, code, a.now(), u.MFA.LastStep)
		if !ok {
			return "", ErrMFACode
		}
		u.MFA.LastStep = step
		return "totp", a.Store.UpdateUser(ctx, *u)
	}
	rest, ok := auth.UseRecoveryCode(u.MFA.Recovery, code)
	if !ok {
		return "", ErrMFACode
	}
	u.MFA.Recovery = rest
	return "recovery", a.Store.UpdateUser(ctx, *u)
}

// MFAStatus je stav 2FA prihláseného používateľa.
type MFAStatus struct {
	Enabled      bool   `json:"enabled"`
	RecoveryLeft int    `json:"recoveryLeft"`
	Account      string `json:"account"` // e-mail, pod ktorým je účet v aplikácii
}

// MyMFA vráti stav 2FA prihláseného používateľa.
func (a *App) MyMFA(ctx context.Context) (MFAStatus, error) {
	u, err := a.Store.GetUser(ctx, auth.ActorFrom(ctx).UserID)
	if err != nil {
		return MFAStatus{}, err
	}
	return MFAStatus{u.MFA.Enabled, len(u.MFA.Recovery), u.Email}, nil
}

// MFASetup je nový kľúč na naskenovanie do aplikácie.
type MFASetup struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

// mfaUser načíta prihláseného používateľa a overí jeho heslo.
func (a *App) mfaUser(ctx context.Context, password string) (auth.User, error) {
	u, err := a.Store.GetUser(ctx, auth.ActorFrom(ctx).UserID)
	if err != nil {
		return u, ErrNoSession
	}
	if !auth.CheckPassword(u.PasswordHash, password) {
		return u, mfaFieldErr("password", "Heslo nesedí")
	}
	return u, nil
}

func mfaFieldErr(field, msg string) error {
	return &invoice.ValidationError{Fields: []invoice.FieldError{{Field: field, Message: msg}}}
}

// SetupMFA pripraví nový kľúč. Zapne sa až po overení kódu (EnableMFA),
// aby si používateľ nezamkol účet zle naskenovaným kľúčom.
func (a *App) SetupMFA(ctx context.Context, password string) (MFASetup, error) {
	u, err := a.mfaUser(ctx, password)
	if err != nil {
		return MFASetup{}, err
	}
	if u.MFA.Enabled {
		return MFASetup{}, mfaFieldErr("password", "Dvojfázové overenie už je zapnuté")
	}
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		return MFASetup{}, err
	}
	u.MFA = auth.MFA{Secret: secret}
	if err := a.Store.UpdateUser(ctx, u); err != nil {
		return MFASetup{}, err
	}
	return MFASetup{secret, auth.TOTPURI(MFAIssuer, u.Email, secret)}, nil
}

// EnableMFA zapne 2FA po overení prvého kódu a vráti záložné kódy
// (zobrazia sa iba raz).
func (a *App) EnableMFA(ctx context.Context, code string) ([]string, error) {
	u, err := a.Store.GetUser(ctx, auth.ActorFrom(ctx).UserID)
	if err != nil {
		return nil, ErrNoSession
	}
	if u.MFA.Enabled {
		return nil, mfaFieldErr("code", "Dvojfázové overenie už je zapnuté")
	}
	if u.MFA.Secret == "" {
		return nil, mfaFieldErr("code", "Najprv si pripravte kľúč")
	}
	step, ok := auth.VerifyTOTP(u.MFA.Secret, code, a.now(), 0)
	if !ok {
		return nil, mfaFieldErr("code", "Kód nesedí — skontrolujte čas v telefóne a skúste aktuálny kód")
	}
	codes, hashes, err := auth.NewRecoveryCodes(auth.RecoveryCount)
	if err != nil {
		return nil, err
	}
	u.MFA = auth.MFA{Enabled: true, Secret: u.MFA.Secret, LastStep: step, Recovery: hashes}
	if err := a.Store.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	a.record(ctx, 0, audit.CatAuth, "mfa_on", "user", u.ID, "Zapnuté dvojfázové overenie "+u.Email, nil, nil)
	return codes, nil
}

// DisableMFA vypne 2FA; treba heslo aj kód (alebo záložný kód).
func (a *App) DisableMFA(ctx context.Context, password, code string) error {
	u, err := a.mfaUser(ctx, password)
	if err != nil {
		return err
	}
	if !u.MFA.Enabled {
		return nil
	}
	if _, err := a.checkMFACode(ctx, &u, code); err != nil {
		return mfaFieldErr("code", err.Error())
	}
	u.MFA = auth.MFA{}
	if err := a.Store.UpdateUser(ctx, u); err != nil {
		return err
	}
	a.record(ctx, 0, audit.CatAuth, "mfa_off", "user", u.ID, "Vypnuté dvojfázové overenie "+u.Email, nil, nil)
	return nil
}

// NewRecoveryCodes vydá nové záložné kódy (staré prestanú platiť).
func (a *App) NewRecoveryCodes(ctx context.Context, password string) ([]string, error) {
	u, err := a.mfaUser(ctx, password)
	if err != nil {
		return nil, err
	}
	if !u.MFA.Enabled {
		return nil, mfaFieldErr("password", "Dvojfázové overenie nie je zapnuté")
	}
	codes, hashes, err := auth.NewRecoveryCodes(auth.RecoveryCount)
	if err != nil {
		return nil, err
	}
	u.MFA.Recovery = hashes
	if err := a.Store.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	a.record(ctx, 0, audit.CatAuth, "mfa_recovery_new", "user", u.ID, "Nové záložné kódy "+u.Email, nil, nil)
	return codes, nil
}

// ResetMFA vypne 2FA inému používateľovi (stratený telefón aj záložné kódy).
// Smie iba správca inštalácie; používateľ sa potom prihlási iba heslom.
func (a *App) ResetMFA(ctx context.Context, userID int64) error {
	act := auth.ActorFrom(ctx)
	if !act.SuperAdmin {
		return ErrForbidden
	}
	u, err := a.Store.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if !u.MFA.Enabled && u.MFA.Secret == "" {
		return nil
	}
	u.MFA = auth.MFA{}
	if err := a.Store.UpdateUser(ctx, u); err != nil {
		return err
	}
	_ = a.Store.DeleteUserSessions(ctx, u.ID)
	a.record(ctx, 0, audit.CatAuth, "mfa_reset", "user", u.ID, "Zrušené dvojfázové overenie používateľa "+u.Email, nil, nil)
	return nil
}
