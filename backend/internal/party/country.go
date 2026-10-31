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

// countryAliases sú ďalšie názvy krajín z iných programov (česky, anglicky).
var countryAliases = map[string]string{
	"slovenska republika": "SK", "slovensko": "SK", "slovakia": "SK", "slovak republic": "SK",
	"ceska republika": "CZ", "cesko": "CZ", "czech republic": "CZ", "czechia": "CZ",
	"madarsko": "HU", "hungary": "HU", "polsko": "PL", "poland": "PL", "rakousko": "AT", "austria": "AT",
	"nemecko": "DE", "germany": "DE", "deutschland": "DE", "osterreich": "AT", "ukrajina": "UA", "ukraine": "UA",
	"united kingdom": "GB", "velka britanie": "GB", "united states": "US", "usa": "US", "spojene staty americke": "US",
	"italie": "IT", "italy": "IT", "francie": "FR", "france": "FR", "nizozemsko": "NL", "netherlands": "NL",
	"svycarsko": "CH", "switzerland": "CH", "chorvatsko": "HR", "croatia": "HR", "slovinsko": "SI", "slovenia": "SI",
	"spanelsko": "ES", "spain": "ES", "belgie": "BE", "belgium": "BE", "dansko": "DK", "denmark": "DK",
	"svedsko": "SE", "sweden": "SE", "rumunsko": "RO", "romania": "RO", "bulharsko": "BG", "bulgaria": "BG",
	"cina": "CN", "china": "CN",
}

// CountryCode nájde kód krajiny podľa kódu alebo názvu (slovensky, česky,
// anglicky; bez ohľadu na diakritiku). Neznámy názov vráti "".
func CountryCode(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 2 {
		return strings.ToUpper(s)
	}
	k := foldName(s)
	if k == "" {
		return ""
	}
	if c, ok := countryAliases[k]; ok {
		return c
	}
	for code, name := range countryNames {
		if foldName(name) == k {
			return code
		}
	}
	return ""
}

// foldName: malé písmená bez diakritiky.
func foldName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if f, ok := foldMap[r]; ok {
			r = f
		}
		b.WriteRune(r)
	}
	return b.String()
}

var foldMap = map[rune]rune{'á': 'a', 'ä': 'a', 'č': 'c', 'ď': 'd', 'é': 'e', 'ě': 'e', 'í': 'i', 'ĺ': 'l', 'ľ': 'l', 'ň': 'n',
	'ó': 'o', 'ô': 'o', 'ö': 'o', 'ŕ': 'r', 'ř': 'r', 'š': 's', 'ť': 't', 'ú': 'u', 'ů': 'u', 'ü': 'u', 'ý': 'y', 'ž': 'z'}
