//go:build windows

package tray

import (
	"testing"
	"unsafe"
)

// Veľkosti štruktúr musia sedieť s Windows SDK (x64), inak Shell_NotifyIcon zlyhá.
func TestStructSizes(t *testing.T) {
	if s := unsafe.Sizeof(notifyIconData{}); s != 976 {
		t.Errorf("NOTIFYICONDATAW %d, chcem 976", s)
	}
	if s := unsafe.Sizeof(wndClassEx{}); s != 80 {
		t.Errorf("WNDCLASSEXW %d, chcem 80", s)
	}
	if s := unsafe.Sizeof(msg{}); s != 48 {
		t.Errorf("MSG %d, chcem 48", s)
	}
}
