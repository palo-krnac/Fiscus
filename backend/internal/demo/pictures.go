package demo

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// Ukážkové obrázky produktov (kreslené, bez cudzích fotografií).

var (
	bg      = color.RGBA{0xf6, 0xf7, 0xf9, 0xff}
	steel   = color.RGBA{0x9a, 0xa4, 0xb1, 0xff}
	dark    = color.RGBA{0x2b, 0x33, 0x3d, 0xff}
	ink     = color.RGBA{0x1f, 0x3a, 0x5c, 0xff}
	led     = color.RGBA{0x4c, 0xc3, 0x8a, 0xff}
	orange  = color.RGBA{0xe0, 0x7b, 0x2e, 0xff}
	copper  = color.RGBA{0xc2, 0x7a, 0x3e, 0xff}
	sheathe = color.RGBA{0x30, 0x34, 0x3a, 0xff}
)

func rect(img *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{c}, image.Point{}, draw.Src)
}

func disc(img *image.RGBA, cx, cy, r0, r1 int, c color.Color) {
	for y := cy - r1; y <= cy+r1; y++ {
		for x := cx - r1; x <= cx+r1; x++ {
			d := (x-cx)*(x-cx) + (y-cy)*(y-cy)
			if d <= r1*r1 && d >= r0*r0 {
				img.Set(x, y, c)
			}
		}
	}
}

func encode(img image.Image) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// drivePicture je frekvenčný menič spredu (side=false) alebo zboku.
func drivePicture(side bool) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 480, 480))
	rect(img, 0, 0, 480, 480, bg)
	if side {
		rect(img, 190, 60, 290, 420, steel)
		for y := 80; y < 400; y += 18 {
			rect(img, 200, y, 280, y+8, dark) // chladič
		}
		return encode(img)
	}
	rect(img, 130, 50, 350, 430, steel)
	rect(img, 145, 65, 335, 415, color.RGBA{0xe4, 0xe8, 0xed, 0xff})
	rect(img, 175, 100, 305, 170, dark) // displej
	rect(img, 190, 118, 290, 152, led)
	for i := 0; i < 3; i++ {
		disc(img, 200+i*40, 210, 0, 13, ink) // tlačidlá
	}
	disc(img, 240, 275, 0, 30, orange) // potenciometer
	disc(img, 240, 275, 0, 6, dark)
	for i := 0; i < 6; i++ {
		rect(img, 165+i*27, 370, 183+i*27, 395, dark) // svorky
	}
	return encode(img)
}

// cablePicture je kotúč kábla s odizolovaným koncom.
func cablePicture() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 480, 480))
	rect(img, 0, 0, 480, 480, bg)
	for r := 180; r > 70; r -= 22 {
		disc(img, 240, 240, r-16, r, sheathe)
	}
	disc(img, 240, 240, 0, 55, steel)
	rect(img, 300, 380, 440, 400, sheathe)
	for i, c := range []color.Color{color.RGBA{0x8b, 0x5a, 0x2b, 0xff}, dark, color.RGBA{0x9a, 0x9a, 0x9a, 0xff}, color.RGBA{0x3d, 0x6f, 0xb6, 0xff}, color.RGBA{0xd9, 0xc2, 0x2e, 0xff}} {
		rect(img, 440, 372+i*7, 462, 377+i*7, c)
		rect(img, 462, 373+i*7, 470, 376+i*7, copper)
	}
	return encode(img)
}
