package audit

import (
	"strings"
)

var fold = strings.NewReplacer(
	"á", "a", "ä", "a", "č", "c", "ď", "d", "é", "e", "í", "i", "ĺ", "l", "ľ", "l", "ň", "n",
	"ó", "o", "ô", "o", "ŕ", "r", "š", "s", "ť", "t", "ú", "u", "ý", "y", "ž", "z",
)

// Match vráti true, ak záznam vyhovuje filtru (spoločné pre všetky úložiská).
func (f Filter) Match(e Entry) bool {
	if f.CompanyIDs != nil {
		ok := false
		for _, id := range f.CompanyIDs {
			if id == e.CompanyID {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if f.Category != "" && e.Category != f.Category {
		return false
	}
	if f.UserID != 0 && e.UserID != f.UserID {
		return false
	}
	if !f.From.IsZero() && e.Time.Before(f.From) {
		return false
	}
	if !f.To.IsZero() && !e.Time.Before(f.To) {
		return false
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		key := func(s string) string { return fold.Replace(strings.ToLower(s)) }
		if !strings.Contains(key(e.Summary+" "+e.UserName+" "+e.Action), key(q)) {
			return false
		}
	}
	return true
}

// Page vráti stránku záznamov podľa Limit/Offset (Limit 0 = 100, max 500).
func (f Filter) Page(all []Entry) []Entry {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if f.Offset >= len(all) {
		return []Entry{}
	}
	end := f.Offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[f.Offset:end]
}
