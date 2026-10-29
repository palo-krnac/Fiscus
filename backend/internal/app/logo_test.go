package app

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/auth"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
	"github.com/palo-krnac/fiscus/backend/internal/store"
)

func pngOf(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{31, 58, 92, 255})
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestLogo(t *testing.T) {
	a, c := newApp(t, edition.Desktop)
	if _, err := a.Logo(ctx, c.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bez loga: %v", err)
	}
	cases := []struct {
		name string
		data []byte
		ok   bool
	}{
		{"PNG", pngOf(120, 40), true},
		{"prázdny", nil, false},
		{"SVG", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), false},
		{"text", []byte("ahoj"), false},
		{"poškodený PNG", pngOf(10, 10)[:30], false},
		{"príliš veľký rozmer", pngOf(4100, 10), false},
		{"príliš veľký súbor", append(pngOf(10, 10), make([]byte, MaxLogoBytes)...), false},
	}
	for _, tc := range cases {
		_, err := a.SetLogo(ctx, c.ID, tc.data)
		if (err == nil) != tc.ok || (!tc.ok && !errors.Is(err, ErrBadLogo)) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	l, err := a.Logo(ctx, c.ID)
	if err != nil || l.ContentType != "image/png" || l.Updated.IsZero() {
		t.Fatalf("logo: %+v %v", l.ContentType, err)
	}
	if err := a.DeleteLogo(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteLogo(ctx, c.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("druhé zmazanie: %v", err)
	}
}

func TestLogoPermissions(t *testing.T) {
	o := newOffice(t)
	actx := as(o.admin)
	if _, err := o.a.SetLogo(actx, o.alfa.ID, pngOf(20, 20)); err != nil {
		t.Fatal(err)
	}
	mb, err := o.a.AddMember(actx, o.alfa.ID, NewMember{Email: "f@alfa.sk", Name: "F", Password: "heslo-1234567", Role: auth.RoleInvoicer})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := o.a.Store.GetUser(ctx, mb.User.ID)
	inv := as(actorOf(u, auth.Actor{Channel: auth.ChannelWeb}))
	if _, err := o.a.Logo(inv, o.alfa.ID); err != nil {
		t.Fatalf("fakturant číta logo: %v", err)
	}
	if _, err := o.a.SetLogo(inv, o.alfa.ID, pngOf(20, 20)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("fakturant mení logo: %v", err)
	}
	if _, err := o.a.Logo(inv, o.beta.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("logo cudzej firmy: %v", err)
	}
}
