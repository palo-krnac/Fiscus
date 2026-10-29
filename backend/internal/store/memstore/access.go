package memstore

import (
	"context"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/audit"
	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

type memKey struct{ user, company int64 }

func (s *Store) initAccess() {
	if s.users == nil {
		s.users = map[int64]auth.User{}
		s.members = map[memKey]auth.Role{}
		s.sessions = map[string]auth.Session{}
	}
}

func (s *Store) CreateUser(_ context.Context, u auth.User) (auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	for _, x := range s.users {
		if strings.EqualFold(x.Email, u.Email) {
			return u, store.ErrDuplicate
		}
	}
	u.ID = s.id()
	s.users[u.ID] = u
	return u, nil
}

func (s *Store) GetUser(_ context.Context, id int64) (auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	u, ok := s.users[id]
	if !ok {
		return u, store.ErrNotFound
	}
	return u, nil
}

func (s *Store) GetUserByEmail(_ context.Context, email string) (auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	for _, u := range s.users {
		if strings.EqualFold(u.Email, strings.TrimSpace(email)) {
			return u, nil
		}
	}
	return auth.User{}, store.ErrNotFound
}

func (s *Store) UpdateUser(_ context.Context, u auth.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	if _, ok := s.users[u.ID]; !ok {
		return store.ErrNotFound
	}
	for _, x := range s.users {
		if x.ID != u.ID && strings.EqualFold(x.Email, u.Email) {
			return store.ErrDuplicate
		}
	}
	s.users[u.ID] = u
	return nil
}

func (s *Store) ListUsers(context.Context) ([]auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	out := []auth.User{}
	for _, u := range s.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) SetMembership(_ context.Context, m auth.Membership) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	if _, ok := s.users[m.UserID]; !ok {
		return store.ErrNotFound
	}
	s.members[memKey{m.UserID, m.CompanyID}] = m.Role
	return nil
}

func (s *Store) RemoveMembership(_ context.Context, userID, companyID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	k := memKey{userID, companyID}
	if _, ok := s.members[k]; !ok {
		return store.ErrNotFound
	}
	delete(s.members, k)
	return nil
}

func (s *Store) memberships(match func(memKey) bool) []auth.Membership {
	out := []auth.Membership{}
	for k, r := range s.members {
		if match(k) {
			out = append(out, auth.Membership{UserID: k.user, CompanyID: k.company, Role: r})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CompanyID != out[j].CompanyID {
			return out[i].CompanyID < out[j].CompanyID
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}

func (s *Store) UserMemberships(_ context.Context, userID int64) ([]auth.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	return s.memberships(func(k memKey) bool { return k.user == userID }), nil
}

func (s *Store) CompanyMembers(_ context.Context, companyID int64) ([]auth.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	return s.memberships(func(k memKey) bool { return k.company == companyID }), nil
}

func (s *Store) CreateSession(_ context.Context, ss auth.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	s.sessions[ss.TokenHash] = ss
	return nil
}

func (s *Store) GetSession(_ context.Context, hash string) (auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	ss, ok := s.sessions[hash]
	if !ok {
		return ss, store.ErrNotFound
	}
	return ss, nil
}

func (s *Store) DeleteSession(_ context.Context, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	delete(s.sessions, hash)
	return nil
}

func (s *Store) DeleteUserSessions(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAccess()
	for h, ss := range s.sessions {
		if ss.UserID == userID {
			delete(s.sessions, h)
		}
	}
	return nil
}

func (s *Store) AppendAudit(_ context.Context, e *audit.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := ""
	if n := len(s.auditLog); n > 0 {
		prev = s.auditLog[n-1].Hash
	}
	s.auditSeq++
	e.ID = s.auditSeq
	audit.Seal(e, prev)
	s.auditLog = append(s.auditLog, *e)
	return nil
}

func (s *Store) ListAudit(_ context.Context, f audit.Filter) ([]audit.Entry, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []audit.Entry
	for i := len(s.auditLog) - 1; i >= 0; i-- {
		if f.Match(s.auditLog[i]) {
			all = append(all, s.auditLog[i])
		}
	}
	return append([]audit.Entry{}, f.Page(all)...), len(all), nil
}

func (s *Store) AuditChain(context.Context) ([]audit.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]audit.Entry(nil), s.auditLog...), nil
}
