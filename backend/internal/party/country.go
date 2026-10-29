package party

import "strings"

// countryNames sú slovenské názvy krajín (rovnaký zoznam ako vo frontende).
var countryNames = map[string]string{
	"SK": "Slovensko", "CZ": "Česko", "HU": "Maďarsko", "PL": "Poľsko", "AT": "Rakúsko", "DE": "Nemecko",
	"UA": "Ukrajina", "BE": "Belgicko", "BG": "Bulharsko", "HR": "Chorvátsko", "CY": "Cyprus", "DK": "Dánsko",
	"EE": "Estónsko", "FI": "Fínsko", "FR": "Francúzsko", "GR": "Grécko", "IE": "Írsko", "IT": "Taliansko",
	"LV": "Lotyšsko", "LT": "Litva", "LU": "Luxembursko", "MT": "Malta", "NL": "Holandsko", "PT": "Portugalsko",
	"RO": "Rumunsko", "SI": "Slovinsko", "ES": "Španielsko", "SE": "Švédsko", "CH": "Švajčiarsko", "NO": "Nórsko",
	"GB": "Spojené kráľovstvo", "RS": "Srbsko", "US": "Spojené štáty", "CN": "Čína",
}

// CountryName vráti slovenský názov krajiny, pri neznámom kóde kód.
func CountryName(code string) string {
	c := strings.ToUpper(strings.TrimSpace(code))
	if n, ok := countryNames[c]; ok {
		return n
	}
	return c
}
