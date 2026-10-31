package memstore

import (
	"context"
	"reflect"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/store"
)

// ofCompany: patrí hodnota (štruktúra s poľom CompanyID) firme?
func ofCompany(v reflect.Value, id int64) bool {
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	f := v.FieldByName("CompanyID")
	return f.IsValid() && f.Kind() == reflect.Int64 && f.Int() == id
}

// dropCompany zmaže z mapy alebo zoznamu (cez ukazovateľ) záznamy firmy.
func dropCompany(ptr any, id int64) {
	v := reflect.ValueOf(ptr).Elem()
	switch v.Kind() {
	case reflect.Map:
		for _, k := range v.MapKeys() {
			if ofCompany(v.MapIndex(k), id) {
				v.SetMapIndex(k, reflect.Value{})
			}
		}
	case reflect.Slice:
		out := reflect.MakeSlice(v.Type(), 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			if !ofCompany(v.Index(i), id) {
				out = reflect.Append(out, v.Index(i))
			}
		}
		v.Set(out)
	}
}

func (s *Store) DeleteCompany(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	companies := s.companies[:0]
	for _, c := range s.companies {
		if c.ID == id {
			found = true
			continue
		}
		companies = append(companies, c)
	}
	if !found {
		return store.ErrNotFound
	}
	s.companies = companies
	for _, m := range []any{&s.invoices, &s.contacts, &s.purchases, &s.bankTx, &s.apiKeys, &s.cashDocs, &s.quotes, &s.assets,
		&s.vehicles, &s.trips, &s.fuel, &s.recurring, &s.reminders, &s.items, &s.stockDocs, &s.emails, &s.einvoices, &s.auditLog, &s.files} {
		dropCompany(m, id)
	}
	delete(s.journal, id)
	delete(s.logos, id)
	for k := range s.members {
		if k.company == id {
			delete(s.members, k)
		}
	}
	for k := range s.seq {
		if k.company == id {
			delete(s.seq, k)
		}
	}
	suffix := "." + strconv.FormatInt(id, 10)
	for k := range s.settings {
		if strings.HasSuffix(k, suffix) {
			delete(s.settings, k)
		}
	}
	return nil
}
