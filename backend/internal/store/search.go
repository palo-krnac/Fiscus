package store

import "strings"

var fold = strings.NewReplacer(
	"á", "a", "ä", "a", "č", "c", "ď", "d", "é", "e", "ě", "e", "í", "i",
	"ĺ", "l", "ľ", "l", "ň", "n", "ó", "o", "ô", "o", "ö", "o", "ŕ", "r",
	"ř", "r", "š", "s", "ť", "t", "ú", "u", "ů", "u", "ü", "u", "ý", "y", "ž", "z",
)

// SearchKey pripraví text na hľadanie: malé písmená, bez diakritiky,
// bez medzier navyše. "Kováč" aj "kovac" dajú rovnaký kľúč.
func SearchKey(s string) string {
	return fold.Replace(strings.ToLower(strings.Join(strings.Fields(s), " ")))
}

// Matches vráti true, ak hľadaný výraz (už prevedený cez SearchKey alebo nie)
// je obsiahnutý v niektorom z polí.
func Matches(query string, fields ...string) bool {
	q := SearchKey(query)
	if q == "" {
		return true
	}
	return strings.Contains(SearchKey(strings.Join(fields, " ")), q)
}
