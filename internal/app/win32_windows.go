//go:build windows

package app

import "golang.org/x/sys/windows"

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")

	procCreateMutex      = kernel32.NewProc("CreateMutexW")
	procFindWindow       = user32.NewProc("FindWindowW")
	procSetFgWindow      = user32.NewProc("SetForegroundWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
	procDwmSetAttr       = dwmapi.NewProc("DwmSetWindowAttribute")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW  = user32.NewProc("CallWindowProcW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procMessageBoxW      = user32.NewProc("MessageBoxW")
	procLoadImageW       = user32.NewProc("LoadImageW")
	procLoadIconW        = user32.NewProc("LoadIconW")
	procCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	procAppendMenuW      = user32.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	procDestroyMenu      = user32.NewProc("DestroyMenu")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
)

// Win32 constants used by the window, tray, and dark-frame code.
const (
	// DWM window attributes for dark theme
	DWMWA_USE_IMMERSIVE_DARK_MODE_BEFORE_20H1 = 19
	DWMWA_USE_IMMERSIVE_DARK_MODE             = 20
	DWMWA_CAPTION_COLOR                       = 35
	DWMWA_TEXT_COLOR                          = 36

	// Win32 messages
	wmClose             = 0x0010
	wmLButtonUp         = 0x0202
	wmRButtonUp         = 0x0205
	wmLButtonDblClk     = 0x0203
	wmApp               = 0x8000
	wmTrayCallback      = wmApp + 1
	ninBalloonUserClick = wmApp + 5

	// ShowWindow commands
	swHide    = 0
	swRestore = 9

	// SetWindowLongPtr / SetWindowPos
	gwlpWndProc   = ^uintptr(3) // -4
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	// Tray icon
	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010
	niifInfo   = 0x00000001

	// Icons
	imageIcon      = 1
	lrLoadFromFile = 0x0010
	idiApplication = 32512

	// Tray popup menu
	tpmReturnCmd = 0x0100
	tpmRightBtn  = 0x0002
	tpmBottom    = 0x0020
	menuOpen     = 1
	menuExit     = 2
)