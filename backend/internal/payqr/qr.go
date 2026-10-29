package payqr

import (
	"errors"
	"strings"
)

// QR kód (ISO/IEC 18004) v alfanumerickom režime s úrovňou opravy chýb M —
// reťazec PAY by square obsahuje iba znaky 0–9 a A–V. Postup podľa
// referenčnej implementácie (Project Nayuki, licencia MIT).

const alnumChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

// Úroveň M: počet bajtov opravy chýb v bloku a počet blokov pre verzie 1–40.
var (
	eccPerBlock = [41]int{-1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28}
	eccBlocks   = [41]int{-1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49}
)

// QR je hotový QR kód: Modules[y][x] = tmavý modul.
type QR struct {
	Version int
	Size    int
	Mask    int
	Modules [][]bool
}

// ErrTooLong — text sa nezmestí ani do verzie 40.
var ErrTooLong = errors.New("payqr: text je pre QR kód príliš dlhý")

// NewQR zakóduje text zo znakov alfanumerickej sady QR.
func NewQR(text string) (*QR, error) {
	for _, r := range text {
		if !strings.ContainsRune(alnumChars, r) {
			return nil, errors.New("payqr: znak mimo alfanumerickej sady QR")
		}
	}
	ver := 0
	for v := 1; v <= 40; v++ {
		if dataBits(len(text), v) <= numDataCodewords(v)*8 {
			ver = v
			break
		}
	}
	if ver == 0 {
		return nil, ErrTooLong
	}

	// bity: režim, počet znakov, dáta, ukončenie, zarovnanie a výplň
	var bb bitBuf
	bb.append(0x2, 4)
	bb.append(len(text), countBits(ver))
	for i := 0; i+1 < len(text); i += 2 {
		bb.append(strings.IndexByte(alnumChars, text[i])*45+strings.IndexByte(alnumChars, text[i+1]), 11)
	}
	if len(text)%2 == 1 {
		bb.append(strings.IndexByte(alnumChars, text[len(text)-1]), 6)
	}
	capacity := numDataCodewords(ver) * 8
	bb.append(0, min(4, capacity-len(bb)))
	bb.append(0, (8-len(bb)%8)%8)
	for pad := 0xEC; len(bb) < capacity; pad ^= 0xEC ^ 0x11 {
		bb.append(pad, 8)
	}
	data := make([]byte, len(bb)/8)
	for i, b := range bb {
		if b {
			data[i>>3] |= 1 << (7 - i&7)
		}
	}

	q := &QR{Version: ver, Size: ver*4 + 17}
	q.Modules = make([][]bool, q.Size)
	fn := make([][]bool, q.Size)
	for i := range q.Modules {
		q.Modules[i] = make([]bool, q.Size)
		fn[i] = make([]bool, q.Size)
	}
	q.drawFunctionPatterns(fn)
	q.drawCodewords(addECC(data, ver), fn)

	best, bestPenalty := 0, -1
	for m := 0; m < 8; m++ {
		q.applyMask(m, fn)
		q.drawFormatBits(m, fn)
		if p := q.penalty(); bestPenalty < 0 || p < bestPenalty {
			best, bestPenalty = m, p
		}
		q.applyMask(m, fn) // XOR vráti späť
	}
	q.Mask = best
	q.applyMask(best, fn)
	q.drawFormatBits(best, fn)
	return q, nil
}

func countBits(ver int) int {
	switch {
	case ver <= 9:
		return 9
	case ver <= 26:
		return 11
	}
	return 13
}

func dataBits(n, ver int) int { return 4 + countBits(ver) + n/2*11 + n%2*6 }

func rawDataModules(ver int) int {
	r := (16*ver+128)*ver + 64
	if ver >= 2 {
		align := ver/7 + 2
		r -= (25*align-10)*align - 55
		if ver >= 7 {
			r -= 36
		}
	}
	return r
}

func numDataCodewords(ver int) int {
	return rawDataModules(ver)/8 - eccPerBlock[ver]*eccBlocks[ver]
}

type bitBuf []bool

func (b *bitBuf) append(v, n int) {
	for i := n - 1; i >= 0; i-- {
		*b = append(*b, (v>>i)&1 == 1)
	}
}

// addECC rozdelí dáta do blokov, doplní Reed-Solomon a bloky prekladá.
func addECC(data []byte, ver int) []byte {
	numBlocks, eccLen := eccBlocks[ver], eccPerBlock[ver]
	raw := rawDataModules(ver) / 8
	numShort := numBlocks - raw%numBlocks
	shortLen := raw / numBlocks
	div := rsDivisor(eccLen)
	blocks := make([][]byte, numBlocks)
	k := 0
	for i := range blocks {
		n := shortLen - eccLen
		if i >= numShort {
			n++
		}
		dat := data[k : k+n]
		k += n
		b := append([]byte(nil), dat...)
		if i < numShort {
			b = append(b, 0) // miesto, ktoré krátky blok nemá
		}
		blocks[i] = append(b, rsRemainder(dat, div)...)
	}
	out := make([]byte, 0, raw)
	for i := 0; i < len(blocks[0]); i++ {
		for j, b := range blocks {
			if i != shortLen-eccLen || j >= numShort {
				out = append(out, b[i])
			}
		}
	}
	return out
}

func gfMul(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11D)
		z ^= int((y>>i)&1) * int(x)
	}
	return byte(z)
}

func rsDivisor(degree int) []byte {
	res := make([]byte, degree)
	res[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := range res {
			res[j] = gfMul(res[j], root)
			if j+1 < len(res) {
				res[j] ^= res[j+1]
			}
		}
		root = gfMul(root, 0x02)
	}
	return res
}

func rsRemainder(data, div []byte) []byte {
	res := make([]byte, len(div))
	for _, b := range data {
		factor := b ^ res[0]
		copy(res, res[1:])
		res[len(res)-1] = 0
		for i := range res {
			res[i] ^= gfMul(div[i], factor)
		}
	}
	return res
}

func (q *QR) set(fn [][]bool, x, y int, dark bool) {
	q.Modules[y][x] = dark
	fn[y][x] = true
}

func (q *QR) drawFunctionPatterns(fn [][]bool) {
	n := q.Size
	for i := 0; i < n; i++ {
		q.set(fn, 6, i, i%2 == 0)
		q.set(fn, i, 6, i%2 == 0)
	}
	finder := func(cx, cy int) {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := cx+dx, cy+dy
				if x >= 0 && x < n && y >= 0 && y < n {
					d := max(abs(dx), abs(dy))
					q.set(fn, x, y, d != 2 && d != 4)
				}
			}
		}
	}
	finder(3, 3)
	finder(n-4, 3)
	finder(3, n-4)
	pos := alignmentPositions(q.Version)
	for i := range pos {
		for j := range pos {
			if i == 0 && j == 0 || i == 0 && j == len(pos)-1 || i == len(pos)-1 && j == 0 {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					q.set(fn, pos[i]+dx, pos[j]+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	q.drawFormatBits(0, fn) // rezervácia, prepíše sa po výbere masky
	if q.Version >= 7 {
		rem := q.Version
		for i := 0; i < 12; i++ {
			rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
		}
		bits := q.Version<<12 | rem
		for i := 0; i < 18; i++ {
			bit := (bits>>i)&1 == 1
			a, b := n-11+i%3, i/3
			q.set(fn, a, b, bit)
			q.set(fn, b, a, bit)
		}
	}
}

func alignmentPositions(ver int) []int {
	if ver == 1 {
		return nil
	}
	num := ver/7 + 2
	step := (ver*4 + num*2 + 1) / (num*2 - 2) * 2
	if ver == 32 {
		step = 26
	}
	res := make([]int, num)
	res[0] = 6
	for i, pos := num-1, ver*4+17-7; i >= 1; i, pos = i-1, pos-step {
		res[i] = pos
	}
	return res
}

func (q *QR) drawFormatBits(mask int, fn [][]bool) {
	data := 0<<3 | mask // úroveň M má kód 0
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return (bits>>i)&1 == 1 }
	n := q.Size
	for i := 0; i <= 5; i++ {
		q.set(fn, 8, i, bit(i))
	}
	q.set(fn, 8, 7, bit(6))
	q.set(fn, 8, 8, bit(7))
	q.set(fn, 7, 8, bit(8))
	for i := 9; i < 15; i++ {
		q.set(fn, 14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		q.set(fn, n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		q.set(fn, 8, n-15+i, bit(i))
	}
	q.set(fn, 8, n-8, true) // tmavý modul
}

func (q *QR) drawCodewords(data []byte, fn [][]bool) {
	n, i := q.Size, 0
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < n; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = n - 1 - vert
				}
				if !fn[y][x] && i < len(data)*8 {
					q.Modules[y][x] = (data[i>>3]>>(7-i&7))&1 == 1
					i++
				}
			}
		}
	}
}

func (q *QR) applyMask(m int, fn [][]bool) {
	for y := 0; y < q.Size; y++ {
		for x := 0; x < q.Size; x++ {
			var inv bool
			switch m {
			case 0:
				inv = (x+y)%2 == 0
			case 1:
				inv = y%2 == 0
			case 2:
				inv = x%3 == 0
			case 3:
				inv = (x+y)%3 == 0
			case 4:
				inv = (x/3+y/2)%2 == 0
			case 5:
				inv = x*y%2+x*y%3 == 0
			case 6:
				inv = (x*y%2+x*y%3)%2 == 0
			case 7:
				inv = ((x+y)%2+x*y%3)%2 == 0
			}
			if inv && !fn[y][x] {
				q.Modules[y][x] = !q.Modules[y][x]
			}
		}
	}
}

// penalty je skóre masky podľa normy (nižšie = lepšie čitateľné).
func (q *QR) penalty() int {
	n := q.Size
	at := func(x, y int, col bool) bool {
		if col {
			return q.Modules[x][y]
		}
		return q.Modules[y][x]
	}
	p := 0
	for _, col := range []bool{false, true} {
		for y := 0; y < n; y++ {
			run := 1
			for x := 1; x <= n; x++ {
				if x < n && at(x, y, col) == at(x-1, y, col) {
					run++
					continue
				}
				if run >= 5 {
					p += 3 + run - 5
				}
				run = 1
			}
			// vzor 1:1:3:1:1 so 4 svetlými modulmi z jednej strany
			for x := 0; x+11 <= n; x++ {
				a := [11]bool{}
				for k := range a {
					a[k] = at(x+k, y, col)
				}
				if a == [11]bool{true, false, true, true, true, false, true, false, false, false, false} ||
					a == [11]bool{false, false, false, false, true, false, true, true, true, false, true} {
					p += 40
				}
			}
		}
	}
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if q.Modules[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				c := q.Modules[y][x]
				if c == q.Modules[y][x+1] && c == q.Modules[y+1][x] && c == q.Modules[y+1][x+1] {
					p += 3
				}
			}
		}
	}
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	return p + max(k, 0)*10
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
