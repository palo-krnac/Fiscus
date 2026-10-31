// Package vcard číta kontakty vo formáte vCard (.vcf) — export z telefónu,
// Google Kontaktov, Outlooku či Apple Kontaktov. Podporuje verzie 2.1,
// 3.0 a 4.0: zalomené riadky, QUOTED-PRINTABLE, CHARSET, skupiny
// (item1.EMAIL) a typy (TYPE=WORK). Ostatné vlastnosti ignoruje.
package vcard

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/table"
)

// Address je poštová adresa (ADR).
type Address struct {
	Street, City, Zip, Country string
}

// Card je jeden kontakt.
type Card struct {
	FullName string // FN
	Given    string // N: krstné meno
	Family   string // N: priezvisko
	Org      string // ORG (prvá časť)
	Address  Address
	Email    string
	Phone    string
	Note     string
}

// Person vráti meno osoby (FN, inak krstné meno a priezvisko).
func (c Card) Person() string {
	if c.FullName != "" {
		return c.FullName
	}
	return strings.TrimSpace(c.Given + " " + c.Family)
}

// Name je názov kontaktu: firma, inak meno osoby.
func (c Card) Name() string {
	if c.Org != "" {
		return c.Org
	}
	return c.Person()
}

// ErrNotVCard — súbor neobsahuje BEGIN:VCARD.
var ErrNotVCard = errors.New("súbor nie je vizitka vCard (.vcf)")

type prop struct {
	name   string
	params map[string][]string
	value  string
}

func (p prop) hasType(t string) bool {
	for _, v := range p.params["TYPE"] {
		if strings.EqualFold(v, t) {
			return true
		}
	}
	return false
}

// rank: pracovná a preferovaná hodnota má prednosť.
func (p prop) rank() int {
	switch {
	case p.hasType("PREF") || p.params["PREF"] != nil:
		return 0
	case p.hasType("WORK"):
		return 1
	case p.hasType("CELL"):
		return 2
	}
	return 3
}

// Parse prečíta všetky vizitky zo súboru.
func Parse(data []byte) ([]Card, error) {
	text := table.Text(data)
	if !strings.Contains(strings.ToUpper(text), "BEGIN:VCARD") {
		return nil, ErrNotVCard
	}
	lines := unfold(text)
	var cards []Card
	var props []prop
	in := false
	for n, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		p, ok := parseLine(line)
		if !ok {
			continue
		}
		switch {
		case p.name == "BEGIN" && strings.EqualFold(p.value, "VCARD"):
			in, props = true, nil
		case p.name == "END" && strings.EqualFold(p.value, "VCARD"):
			if !in {
				return cards, fmt.Errorf("riadok %d: END:VCARD bez BEGIN:VCARD", n+1)
			}
			cards = append(cards, build(props))
			in = false
		case in:
			props = append(props, p)
		}
	}
	if in {
		return cards, errors.New("posledná vizitka nie je ukončená (chýba END:VCARD)")
	}
	return cards, nil
}

// unfold spojí zalomené riadky (medzera na začiatku, „=“ na konci pri QP).
func unfold(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var out []string
	qp := false
	for _, l := range raw {
		l = strings.TrimRight(l, "\r")
		switch {
		case len(out) > 0 && qp:
			out[len(out)-1] = strings.TrimSuffix(out[len(out)-1], "=") + l
		case len(out) > 0 && (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")):
			out[len(out)-1] += l[1:]
		default:
			out = append(out, l)
		}
		last := out[len(out)-1]
		head, _, _ := strings.Cut(last, ":")
		qp = strings.Contains(strings.ToUpper(head), "QUOTED-PRINTABLE") && strings.HasSuffix(last, "=")
	}
	return out
}

func parseLine(line string) (prop, bool) {
	head, value, ok := strings.Cut(line, ":")
	if !ok {
		return prop{}, false
	}
	parts := strings.Split(head, ";")
	name := strings.ToUpper(parts[0])
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:] // skupina item1.EMAIL
	}
	p := prop{name: name, params: map[string][]string{}}
	for _, pr := range parts[1:] {
		k, v, has := strings.Cut(pr, "=")
		k = strings.ToUpper(strings.TrimSpace(k))
		if !has { // vCard 2.1: ;WORK;QUOTED-PRINTABLE
			switch k {
			case "QUOTED-PRINTABLE", "BASE64", "8BIT":
				p.params["ENCODING"] = []string{k}
			default:
				p.params["TYPE"] = append(p.params["TYPE"], k)
			}
			continue
		}
		for _, x := range strings.Split(strings.Trim(v, `"`), ",") {
			p.params[k] = append(p.params[k], strings.ToUpper(strings.TrimSpace(x)))
		}
	}
	if enc := p.params["ENCODING"]; len(enc) > 0 && enc[0] == "QUOTED-PRINTABLE" {
		value = table.Text(decodeQP(value))
	}
	p.value = value
	return p, true
}

func decodeQP(s string) []byte {
	var b bytes.Buffer
	for i := 0; i < len(s); i++ {
		if s[i] == '=' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.Bytes()
}

// fields rozdelí hodnotu podľa „;“ a odstráni únikové znaky.
func fields(v string) []string {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\\' && i+1 < len(v) {
			i++
			switch v[i] {
			case 'n', 'N':
				cur.WriteByte('\n')
			default:
				cur.WriteByte(v[i])
			}
			continue
		}
		if c == ';' {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	return append(out, strings.TrimSpace(cur.String()))
}

func part(f []string, i int) string {
	if i < len(f) {
		return f[i]
	}
	return ""
}

func build(props []prop) Card {
	var c Card
	best := map[string]int{}
	pick := func(p prop) bool {
		r, seen := best[p.name]
		if seen && p.rank() >= r {
			return false
		}
		best[p.name] = p.rank()
		return true
	}
	for _, p := range props {
		f := fields(p.value)
		switch p.name {
		case "FN":
			c.FullName = part(f, 0)
		case "N":
			c.Family, c.Given = part(f, 0), part(f, 1)
		case "ORG":
			c.Org = part(f, 0)
		case "ADR":
			if pick(p) {
				street := strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(part(f, 2), "\n", ", ")), " "))
				c.Address = Address{Street: street, City: part(f, 3), Zip: part(f, 5), Country: part(f, 6)}
			}
		case "EMAIL":
			if pick(p) {
				c.Email = part(f, 0)
			}
		case "TEL":
			if pick(p) {
				c.Phone = strings.TrimPrefix(part(f, 0), "tel:")
			}
		case "NOTE":
			c.Note = strings.Join(f, ";")
		}
	}
	return c
}
