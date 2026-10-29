package reminder

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/palo-krnac/fiscus/backend/internal/invoice"
	"github.com/palo-krnac/fiscus/backend/internal/money"
)

// ECBDataURL je séria ECB Data Portal: sadzba hlavných refinančných operácií
// (MRO, „základná úroková sadzba ECB“) v pracovných dňoch, CSV bez metadát.
const ECBDataURL = "https://data-api.ecb.europa.eu/service/data/FM/B.U2.EUR.4F.KR.MRR_FR.LEV?format=csvdata&detail=dataonly"

// ParseECBData prečíta CSV zo série ECB (stĺpce TIME_PERIOD a OBS_VALUE)
// a vráti iba dni, keď sa sadzba zmenila (od kedy platí nová sadzba).
// Prvý riadok súboru sa berie ako začiatok platnosti.
func ParseECBData(r io.Reader) ([]Rate, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("sadzby ECB: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}
	ti, ok1 := col["TIME_PERIOD"]
	vi, ok2 := col["OBS_VALUE"]
	if !ok1 || !ok2 {
		return nil, errors.New("sadzby ECB: v súbore chýbajú stĺpce TIME_PERIOD a OBS_VALUE")
	}
	type obs struct {
		day  invoice.Date
		rate money.Dec
	}
	var all []obs
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("sadzby ECB: %w", err)
		}
		if ti >= len(rec) || vi >= len(rec) || strings.TrimSpace(rec[vi]) == "" {
			continue
		}
		day, err := invoice.ParseDate(strings.TrimSpace(rec[ti]))
		if err != nil {
			return nil, fmt.Errorf("sadzby ECB: dátum %q", rec[ti])
		}
		v, err := money.Parse(strings.TrimSpace(rec[vi]))
		if err != nil || v.Sign() < 0 {
			return nil, fmt.Errorf("sadzby ECB: sadzba %q", rec[vi])
		}
		all = append(all, obs{day, v.RoundCents()})
	}
	if len(all) == 0 {
		return nil, errors.New("sadzby ECB: súbor neobsahuje žiadnu sadzbu")
	}
	sort.Slice(all, func(i, j int) bool { return all[i].day.Before(all[j].day) })
	var out []Rate
	for i, o := range all {
		if i == 0 || !o.rate.Equal(all[i-1].rate) {
			out = append(out, Rate{From: o.day, Rate: o.rate})
		}
	}
	return out, nil
}

// MergeECB doplní do tabuľky sadzieb zmeny zo súboru ECB. Deň, ktorý
// v tabuľke už je, sa neprepisuje (ručná oprava má prednosť) a riadok, ktorý
// sadzbu v skutočnosti nemení (napr. začiatok stiahnutého obdobia), sa
// nepridá. Vráti novú tabuľku a pridané riadky.
func MergeECB(table []Rate, fetched []Rate) (ECBTable, []Rate) {
	out := Sorted(table)
	have := map[string]bool{}
	for _, r := range out {
		have[r.From.String()] = true
	}
	var added []Rate
	for _, r := range fetched {
		if have[r.From.String()] || (len(out) > 0 && out.On(r.From).Equal(r.Rate)) {
			continue
		}
		out = Sorted(append(out, r))
		have[r.From.String()] = true
		added = append(added, r)
	}
	return out, added
}
