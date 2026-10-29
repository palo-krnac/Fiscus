package party

import "strings"

// euCountries sú členské štáty EÚ (ISO 3166-1 alpha-2; Grécko má v DPH kód EL).
var euCountries = map[string]bool{
	"AT": true, "BE": true, "BG": true, "HR": true, "CY": true, "CZ": true, "DK": true, "EE": true, "FI": true,
	"FR": true, "DE": true, "GR": true, "EL": true, "HU": true, "IE": true, "IT": true, "LV": true, "LT": true,
	"LU": true, "MT": true, "NL": true, "PL": true, "PT": true, "RO": true, "SK": true, "SI": true, "ES": true, "SE": true,
}

// IsEU vráti true pre členský štát EÚ (vrátane Slovenska).
func IsEU(country string) bool { return euCountries[strings.ToUpper(strings.TrimSpace(country))] }

// Foreign vráti true pre partnera mimo Slovenska (prázdna krajina = Slovensko).
func Foreign(country string) bool {
	c := strings.ToUpper(strings.TrimSpace(country))
	return c != "" && c != "SK"
}

// VATCountry vráti kód štátu z IČ DPH (DE123… → DE), inak krajinu partnera.
// Grécko používa v IČ DPH kód EL.
func VATCountry(icdph, country string) string {
	id := NormalizeID(icdph)
	if len(id) > 2 && id[0] >= 'A' && id[0] <= 'Z' && id[1] >= 'A' && id[1] <= 'Z' {
		return id[:2]
	}
	c := strings.ToUpper(strings.TrimSpace(country))
	if c == "GR" {
		return "EL"
	}
	return c
}
