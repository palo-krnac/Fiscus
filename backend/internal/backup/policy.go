// Package backup robí zálohy databázy: vytvorenie, overenie, šifrovanie,
// rotáciu (dedo–otec–syn) a obnovu.
package backup

import (
	"fmt"
	"sort"
	"time"
)

// Policy určuje, koľko záloh sa ponechá.
// Ponechá sa najnovšia záloha z každého z posledných N dní, týždňov a mesiacov.
type Policy struct {
	Daily   int
	Weekly  int
	Monthly int
}

// DefaultPolicy: 7 denných, 4 týždenné, 12 mesačných.
var DefaultPolicy = Policy{Daily: 7, Weekly: 4, Monthly: 12}

// Keep vráti množinu časov záloh, ktoré sa majú ponechať.
// Najnovšia záloha sa ponechá vždy.
func (p Policy) Keep(times []time.Time) map[time.Time]bool {
	sorted := append([]time.Time(nil), times...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].After(sorted[j]) })

	keep := map[time.Time]bool{}
	if len(sorted) > 0 {
		keep[sorted[0]] = true
	}
	bucket := func(n int, key func(time.Time) string) {
		seen := map[string]bool{}
		for _, t := range sorted {
			if len(seen) >= n {
				return
			}
			k := key(t)
			if !seen[k] {
				seen[k] = true
				keep[t] = true // prvá v zoradení = najnovšia v danom období
			}
		}
	}
	bucket(p.Daily, func(t time.Time) string { return t.Format("2006-01-02") })
	bucket(p.Weekly, func(t time.Time) string {
		y, w := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", y, w)
	})
	bucket(p.Monthly, func(t time.Time) string { return t.Format("2006-01") })
	return keep
}
