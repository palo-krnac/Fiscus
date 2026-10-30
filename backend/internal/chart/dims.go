package chart

import (
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
)

// Center je stredisko (útvar, prevádzka) na sledovanie nákladov a výnosov.
type Center struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
	Note   string `json:"note,omitempty"`
}

// Job je zákazka (projekt) na sledovanie nákladov a výnosov.
type Job struct {
	Code      string       `json:"code"`
	Name      string       `json:"name"`
	Customer  string       `json:"customer,omitempty"`
	ContactID int64        `json:"contactId,omitempty"`
	From      invoice.Date `json:"from,omitzero"`
	To        invoice.Date `json:"to,omitzero"`
	Active    bool         `json:"active"`
	Note      string       `json:"note,omitempty"`
}

// Dims sú strediská a zákazky firmy.
type Dims struct {
	Centers []Center `json:"centers"`
	Jobs    []Job    `json:"jobs"`
}

// normCode upraví kód: bez medzier na okrajoch, veľké písmená.
func normCode(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func codeErr(code string) string {
	switch {
	case code == "" || len([]rune(code)) > 20:
		return "Zadajte kód (najviac 20 znakov)"
	case strings.ContainsAny(code, " \t"):
		return "Kód nesmie obsahovať medzery"
	}
	return ""
}

// SaveCenter pridá alebo upraví stredisko (old = pôvodný kód, "" = nové).
func (d *Dims) SaveCenter(c Center, old string) (Center, error) {
	c.Code, c.Name, c.Note = normCode(c.Code), strings.TrimSpace(c.Name), strings.TrimSpace(c.Note)
	var fe []invoice.FieldError
	if m := codeErr(c.Code); m != "" {
		fe = append(fe, invoice.FieldError{Field: "code", Message: m})
	}
	if c.Name == "" {
		fe = append(fe, invoice.FieldError{Field: "name", Message: "Zadajte názov strediska"})
	}
	idx := -1
	for i, x := range d.Centers {
		if x.Code == old && old != "" {
			idx = i
		} else if x.Code == c.Code {
			fe = append(fe, invoice.FieldError{Field: "code", Message: "Stredisko " + c.Code + " už existuje"})
		}
	}
	if old != "" && idx < 0 {
		return c, ErrNotFound
	}
	if len(fe) > 0 {
		return c, &invoice.ValidationError{Fields: fe}
	}
	if idx < 0 {
		d.Centers = append(d.Centers, c)
	} else {
		d.Centers[idx] = c
	}
	return c, nil
}

// DeleteCenter zmaže stredisko.
func (d *Dims) DeleteCenter(code string) error {
	for i, x := range d.Centers {
		if x.Code == code {
			d.Centers = append(d.Centers[:i], d.Centers[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// SaveJob pridá alebo upraví zákazku (old = pôvodný kód, "" = nová).
func (d *Dims) SaveJob(j Job, old string) (Job, error) {
	j.Code, j.Name, j.Customer, j.Note = normCode(j.Code), strings.TrimSpace(j.Name), strings.TrimSpace(j.Customer), strings.TrimSpace(j.Note)
	var fe []invoice.FieldError
	if m := codeErr(j.Code); m != "" {
		fe = append(fe, invoice.FieldError{Field: "code", Message: m})
	}
	if j.Name == "" {
		fe = append(fe, invoice.FieldError{Field: "name", Message: "Zadajte názov zákazky"})
	}
	if !j.From.IsZero() && !j.To.IsZero() && j.To.Before(j.From) {
		fe = append(fe, invoice.FieldError{Field: "to", Message: "Koniec je pred začiatkom"})
	}
	idx := -1
	for i, x := range d.Jobs {
		if x.Code == old && old != "" {
			idx = i
		} else if x.Code == j.Code {
			fe = append(fe, invoice.FieldError{Field: "code", Message: "Zákazka " + j.Code + " už existuje"})
		}
	}
	if old != "" && idx < 0 {
		return j, ErrNotFound
	}
	if len(fe) > 0 {
		return j, &invoice.ValidationError{Fields: fe}
	}
	if idx < 0 {
		d.Jobs = append(d.Jobs, j)
	} else {
		d.Jobs[idx] = j
	}
	return j, nil
}

// DeleteJob zmaže zákazku.
func (d *Dims) DeleteJob(code string) error {
	for i, x := range d.Jobs {
		if x.Code == code {
			d.Jobs = append(d.Jobs[:i], d.Jobs[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}
