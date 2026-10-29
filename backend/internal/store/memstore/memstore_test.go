package memstore

import (
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/store"
	"github.com/palo-krnac/fiscus/backend/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(*testing.T) store.Store { return New() })
}
