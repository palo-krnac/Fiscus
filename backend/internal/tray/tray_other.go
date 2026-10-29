//go:build !windows

package tray

// Supported vráti true, ak systém má ikonu v oblasti oznámení.
func Supported() bool { return false }

// Run na tomto systéme nič nerobí (ikonu nahrádza tlačidlo Ukončiť v menu).
func Run(Menu) (stop func(), err error) { return func() {}, nil }
