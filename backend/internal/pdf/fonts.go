package pdf

import (
	_ "embed"
	"sync"
)

// Písmo Source Sans 3 (Adobe, SIL Open Font License, pozri fonts/LICENSE.md).
var (
	//go:embed fonts/SourceSans3-Regular.ttf
	regularTTF []byte
	//go:embed fonts/SourceSans3-Semibold.ttf
	semiboldTTF []byte
	//go:embed fonts/SourceSans3-Bold.ttf
	boldTTF []byte
)

// Family sú rezy písma pre jeden dokument.
type Family struct {
	Regular, Semibold, Bold *Font
}

var (
	familyOnce sync.Once
	family     Family
	familyErr  error
)

// SourceSans vráti rezy vloženého písma Source Sans 3 pre nový dokument.
func SourceSans() (Family, error) {
	familyOnce.Do(func() {
		var err error
		if family.Regular, err = ParseTTF("SourceSans3-Regular", regularTTF); err != nil {
			familyErr = err
			return
		}
		if family.Semibold, err = ParseTTF("SourceSans3-Semibold", semiboldTTF); err != nil {
			familyErr = err
			return
		}
		family.Bold, familyErr = ParseTTF("SourceSans3-Bold", boldTTF)
	})
	if familyErr != nil {
		return Family{}, familyErr
	}
	return Family{family.Regular.Fresh(), family.Semibold.Fresh(), family.Bold.Fresh()}, nil
}
