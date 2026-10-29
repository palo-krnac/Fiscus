package payqr

// compressLZMA zakóduje dáta do prúdu LZMA (lc=3, lp=0, pb=2) bez hlavičky
// a bez koncovej značky — tak, ako ho ukladá PAY by square (dekodér pozná
// dĺžku z hlavičky QR). Kóduje iba literály: pri pár stovkách bajtov platby
// je výsledok len o málo väčší ako pri plnej kompresii a kód je jednoduchý.
func compressLZMA(data []byte) []byte {
	e := rangeEncoder{rng: 0xFFFFFFFF, cacheSize: 1}
	var isMatch [12 << 2]uint16
	var literal [0x300 << 3]uint16
	for i := range isMatch {
		isMatch[i] = 1024
	}
	for i := range literal {
		literal[i] = 1024
	}
	var prev byte
	for pos, b := range data {
		// stav ostáva 0 (po literáli nasleduje literál)
		e.bit(&isMatch[pos&3], 0)
		probs := literal[0x300*int(prev>>5):]
		sym := 1
		for i := 7; i >= 0; i-- {
			bit := int(b>>i) & 1
			e.bit(&probs[sym], bit)
			sym = sym<<1 | bit
		}
		prev = b
	}
	for i := 0; i < 5; i++ {
		e.shiftLow()
	}
	return e.out
}

type rangeEncoder struct {
	low       uint64
	rng       uint32
	cache     byte
	cacheSize int
	out       []byte
}

func (e *rangeEncoder) bit(prob *uint16, bit int) {
	bound := (e.rng >> 11) * uint32(*prob)
	if bit == 0 {
		e.rng = bound
		*prob += (2048 - *prob) >> 5
	} else {
		e.low += uint64(bound)
		e.rng -= bound
		*prob -= *prob >> 5
	}
	for e.rng < 1<<24 {
		e.rng <<= 8
		e.shiftLow()
	}
}

func (e *rangeEncoder) shiftLow() {
	if uint32(e.low) < 0xFF000000 || e.low>>32 != 0 {
		temp := e.cache
		for {
			e.out = append(e.out, temp+byte(e.low>>32))
			temp = 0xFF
			e.cacheSize--
			if e.cacheSize == 0 {
				break
			}
		}
		e.cache = byte(uint32(e.low) >> 24)
	}
	e.cacheSize++
	e.low = uint64(uint32(e.low) << 8)
}
