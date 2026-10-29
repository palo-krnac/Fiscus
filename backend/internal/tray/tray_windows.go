//go:build windows

package tray

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

//go:embed fiskus.ico
var icon []byte

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassEx  = user32.NewProc("RegisterClassExW")
	pCreateWindowEx   = user32.NewProc("CreateWindowExW")
	pDefWindowProc    = user32.NewProc("DefWindowProcW")
	pGetMessage       = user32.NewProc("GetMessageW")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessage  = user32.NewProc("DispatchMessageW")
	pPostMessage      = user32.NewProc("PostMessageW")
	pPostQuitMessage  = user32.NewProc("PostQuitMessage")
	pDestroyWindow    = user32.NewProc("DestroyWindow")
	pLoadImage        = user32.NewProc("LoadImageW")
	pLoadIcon         = user32.NewProc("LoadIconW")
	pCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	pAppendMenu       = user32.NewProc("AppendMenuW")
	pTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	pDestroyMenu      = user32.NewProc("DestroyMenu")
	pGetCursorPos     = user32.NewProc("GetCursorPos")
	pSetForeground    = user32.NewProc("SetForegroundWindow")
	pShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	pGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy      = 0x0002
	wmClose        = 0x0010
	wmCommand      = 0x0111
	wmLButtonUp    = 0x0202
	wmRButtonUp    = 0x0205
	wmApp          = 0x8000
	wmTray         = wmApp + 1
	nimAdd         = 0
	nimDelete      = 2
	nifMessage     = 0x1
	nifIcon        = 0x2
	nifTip         = 0x4
	imageIcon      = 1
	lrLoadFromFile = 0x10
	lrDefaultSize  = 0x40
	mfString       = 0x0
	mfGrayed       = 0x1
	mfSeparator    = 0x800
	tpmReturnCmd   = 0x100
	tpmRightButton = 0x2
	idOpen         = 1
	idQuit         = 2
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

type point struct{ x, y int32 }

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

// Supported vráti true — Windows má oblasť oznámení.
func Supported() bool { return true }

var current Menu // jedna ikona na proces

func utf16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmTray:
		switch lParam {
		case wmLButtonUp:
			if current.Open != nil {
				go current.Open()
			}
		case wmRButtonUp:
			showMenu(hwnd)
		}
		return 0
	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wParam, lParam)
	return r
}

func showMenu(hwnd uintptr) {
	m, _, _ := pCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	defer pDestroyMenu.Call(m)
	pAppendMenu.Call(m, mfString|mfGrayed, 0, uintptr(unsafe.Pointer(utf16(current.Title))))
	pAppendMenu.Call(m, mfSeparator, 0, 0)
	pAppendMenu.Call(m, mfString, idOpen, uintptr(unsafe.Pointer(utf16("Otvoriť Fiškus"))))
	pAppendMenu.Call(m, mfString, idQuit, uintptr(unsafe.Pointer(utf16("Ukončiť Fiškus"))))
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	// bez toho ponuka nezmizne po kliknutí mimo nej
	pSetForeground.Call(hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(m, tpmReturnCmd|tpmRightButton, uintptr(p.x), uintptr(p.y), 0, hwnd, 0)
	switch cmd {
	case idOpen:
		if current.Open != nil {
			go current.Open()
		}
	case idQuit:
		if current.Quit != nil {
			go current.Quit()
		}
	}
}

// loadIcon načíta ikonu Fiškusa (zo súboru v dočasnom priečinku), inak
// predvolenú ikonu aplikácie.
func loadIcon() uintptr {
	path := filepath.Join(os.TempDir(), "fiskus-tray.ico")
	if err := os.WriteFile(path, icon, 0o644); err == nil {
		h, _, _ := pLoadImage.Call(0, uintptr(unsafe.Pointer(utf16(path))), imageIcon, 0, 0, lrLoadFromFile|lrDefaultSize)
		if h != 0 {
			return h
		}
	}
	h, _, _ := pLoadIcon.Call(0, 32512) // IDI_APPLICATION
	return h
}

// Run pridá ikonu do oblasti oznámení a obsluhuje ju vo vlastnom vlákne.
// stop ikonu odstráni.
func Run(m Menu) (stop func(), err error) {
	current = m
	ready := make(chan error, 1)
	var hwnd uintptr
	go func() {
		// okno a jeho správy musia byť v jednom vlákne OS
		runtime.LockOSThread()
		inst, _, _ := pGetModuleHandle.Call(0)
		class := utf16("FiskusTray")
		wc := wndClassEx{lpfnWndProc: syscall.NewCallback(wndProc), hInstance: inst, lpszClassName: class}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		if r, _, e := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			ready <- e
			return
		}
		h, _, e := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(utf16("Fiškus"))), 0, 0, 0, 0, 0, 0, 0, inst, 0)
		if h == 0 {
			ready <- e
			return
		}
		hwnd = h
		nid := notifyIconData{hWnd: h, uID: 1, uFlags: nifMessage | nifIcon | nifTip, uCallbackMessage: wmTray, hIcon: loadIcon()}
		nid.cbSize = uint32(unsafe.Sizeof(nid))
		tip, _ := syscall.UTF16FromString(m.Tooltip)
		copy(nid.szTip[:len(nid.szTip)-1], tip)
		if r, _, _ := pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); r == 0 {
			pDestroyWindow.Call(h)
			ready <- errors.New("ikonu sa nepodarilo pridať do oblasti oznámení")
			return
		}
		ready <- nil
		var ms msg
		for {
			r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&ms)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&ms)))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(&ms)))
		}
		pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	}()
	if err := <-ready; err != nil {
		return func() {}, err
	}
	return func() { pPostMessage.Call(hwnd, wmClose, 0, 0) }, nil
}
