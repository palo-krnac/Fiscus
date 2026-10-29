// Package web obsahuje zostavený frontend vložený priamo do binárky.
// `make build` skopíruje frontend/dist sem do dist/.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS vráti súbory frontendu, alebo nil, ak frontend nie je zostavený.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
