package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

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

const (
	windowTitle = "WaDeskLight"
	appURL      = "https://web.whatsapp.com"
	mutexName   = "WaDeskLightSingleInstanceMutex"
	userAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"

	// DWM Window Attributes for Dark Theme
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

type rect struct {
	Left, Top, Right, Bottom int32
}

type point struct {
	X, Y int32
}

type windowState struct {
	X      int32 `json:"x"`
	Y      int32 `json:"y"`
	Width  int32 `json:"width"`
	Height int32 `json:"height"`
	Saved  bool  `json:"saved"`
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
	guidItem         windows.GUID
	hBalloonIcon     uintptr
}

var (
	gWinProc uintptr
	gOldProc uintptr
	gTray    notifyIconData
)

func setDarkWindowFrame(hwnd uintptr) {
	darkMode := int32(1)
	// Try standard DWMWA_USE_IMMERSIVE_DARK_MODE (Win10 20H1+ & Win11)
	procDwmSetAttr.Call(
		hwnd,
		uintptr(DWMWA_USE_IMMERSIVE_DARK_MODE),
		uintptr(unsafe.Pointer(&darkMode)),
		unsafe.Sizeof(darkMode),
	)
	// Try older Win10 build
	procDwmSetAttr.Call(
		hwnd,
		uintptr(DWMWA_USE_IMMERSIVE_DARK_MODE_BEFORE_20H1),
		uintptr(unsafe.Pointer(&darkMode)),
		unsafe.Sizeof(darkMode),
	)

	// Set dark caption color (COLORREF: 0x00111B21 WhatsApp Dark Header: RGB 17, 27, 33)
	captionColor := uint32(0x00211B11) // 0x00BBGGRR
	procDwmSetAttr.Call(
		hwnd,
		uintptr(DWMWA_CAPTION_COLOR),
		uintptr(unsafe.Pointer(&captionColor)),
		unsafe.Sizeof(captionColor),
	)

	// Set white caption text (RGB 255, 255, 255)
	textColor := uint32(0x00FFFFFF)
	procDwmSetAttr.Call(
		hwnd,
		uintptr(DWMWA_TEXT_COLOR),
		uintptr(unsafe.Pointer(&textColor)),
		unsafe.Sizeof(textColor),
	)
}

func checkSingleInstance() (uintptr, bool) {
	namePtr, _ := syscall.UTF16PtrFromString(mutexName)
	handle, _, err := procCreateMutex.Call(0, 1, uintptr(unsafe.Pointer(namePtr)))
	if err == windows.ERROR_ALREADY_EXISTS {
		titlePtr, _ := syscall.UTF16PtrFromString(windowTitle)
		hwnd, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))
		if hwnd != 0 {
			procShowWindow.Call(hwnd, swRestore)
			procSetFgWindow.Call(hwnd)
		}
		return handle, false
	}
	return handle, true
}

func getConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.Getenv("APPDATA")
		if configDir == "" {
			configDir = "."
		}
	}
	dir := filepath.Join(configDir, "WaDeskLight")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func getUserDataDir() string {
	dir := filepath.Join(getConfigDir(), "UserData")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func windowStatePath() string {
	return filepath.Join(getConfigDir(), "window.json")
}

func loadWindowState() windowState {
	var st windowState
	data, err := os.ReadFile(windowStatePath())
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

func saveWindowState(st *windowState) {
	data, _ := json.Marshal(st)
	_ = os.WriteFile(windowStatePath(), data, 0644)
}

func webView2RuntimeInstalled() bool {
	clsid := `Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	roots := []string{
		`SOFTWARE\WOW6432Node\` + clsid,
		`SOFTWARE\` + clsid,
	}
	for _, root := range roots {
		pathPtr, _ := windows.UTF16PtrFromString(root)
		var key windows.Handle
		if err := windows.RegOpenKeyEx(windows.HKEY_LOCAL_MACHINE, pathPtr, 0, windows.KEY_READ, &key); err == nil {
			windows.RegCloseKey(key)
			return true
		}
	}
	return false
}

func showErrorDialog(message string) {
	titlePtr, _ := windows.UTF16PtrFromString(windowTitle)
	msgPtr, _ := windows.UTF16PtrFromString(message)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(msgPtr)), uintptr(unsafe.Pointer(titlePtr)), 0x10 /*MB_ICONERROR*/)
}

func strPtr(s string) uintptr {
	p, _ := windows.UTF16PtrFromString(s)
	return uintptr(unsafe.Pointer(p))
}

func copyUTF16(dst []uint16, s string) {
	src, _ := windows.UTF16FromString(s)
	copy(dst, src)
}

func setTip(n *notifyIconData, s string)       { copyUTF16(n.szTip[:], s) }
func setInfoTitle(n *notifyIconData, s string) { copyUTF16(n.szInfoTitle[:], s) }
func setInfo(n *notifyIconData, s string)      { copyUTF16(n.szInfo[:], s) }

func loadTrayIcon(iconPath string) uintptr {
	pathPtr, _ := windows.UTF16PtrFromString(iconPath)
	hIcon, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(pathPtr)), imageIcon, 16, 16, lrLoadFromFile)
	if hIcon != 0 {
		return hIcon
	}
	hIcon, _, _ = procLoadIconW.Call(0, idiApplication)
	return hIcon
}

func trayAdd(hwnd uintptr, iconPath string) {
	if gTray.hWnd != 0 {
		return
	}
	var nid notifyIconData
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = 1
	nid.uFlags = nifMessage | nifIcon | nifTip
	nid.uCallbackMessage = wmTrayCallback
	nid.hIcon = loadTrayIcon(iconPath)
	setTip(&nid, "WaDeskLight")
	gTray = nid
	procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
}

func trayBalloon(title, message string) {
	if gTray.hWnd == 0 {
		return
	}
	gTray.uFlags = nifInfo
	setInfoTitle(&gTray, title)
	setInfo(&gTray, message)
	gTray.dwInfoFlags = niifInfo
	procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&gTray)))
}

func trayDelete() {
	if gTray.hWnd == 0 {
		return
	}
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&gTray)))
	gTray.hWnd = 0
}

func saveWindowBounds(hwnd uintptr) {
	var r rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	st := windowState{
		X:      r.Left,
		Y:      r.Top,
		Width:  r.Right - r.Left,
		Height: r.Bottom - r.Top,
		Saved:  true,
	}
	saveWindowState(&st)
}

func restoreWindow(hwnd uintptr) {
	procShowWindow.Call(hwnd, swRestore)
	procSetFgWindow.Call(hwnd)
}

func showTrayMenu(hwnd uintptr) {
	procSetFgWindow.Call(hwnd)
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	procAppendMenuW.Call(menu, 0, menuOpen, strPtr("Open WaDeskLight"))
	procAppendMenuW.Call(menu, 0, menuExit, strPtr("Exit"))
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightBtn|tpmBottom, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	procDestroyMenu.Call(menu)
	switch cmd {
	case menuOpen:
		restoreWindow(hwnd)
	case menuExit:
		saveWindowBounds(hwnd)
		trayDelete()
		procPostQuitMessage.Call(0)
	}
}

func windowProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmClose:
		// Close-to-tray: hide the window and keep running in the background.
		saveWindowBounds(hwnd)
		procShowWindow.Call(hwnd, swHide)
		go trayBalloon("WaDeskLight", "Masih berjalan di system tray. Klik ikon untuk membuka kembali.")
		return 0
	case wmTrayCallback:
		switch uint32(lp) & 0xFFFF {
		case wmLButtonUp, wmLButtonDblClk, ninBalloonUserClick:
			restoreWindow(hwnd)
		case wmRButtonUp:
			showTrayMenu(hwnd)
		}
		return 0
	default:
		r, _, _ := procCallWindowProcW.Call(gOldProc, hwnd, msg, wp, lp)
		return r
	}
}

func installWindowSubclass(hwnd uintptr) {
	gWinProc = windows.NewCallback(windowProc)
	gOldProc, _, _ = procSetWindowLongPtr.Call(hwnd, uintptr(gwlpWndProc), gWinProc)
}

func main() {
	_, isSingle := checkSingleInstance()
	if !isSingle {
		os.Exit(0)
	}

	if !webView2RuntimeInstalled() {
		showErrorDialog("WebView2 Runtime tidak ditemukan.\n\nSilakan install Microsoft Edge WebView2 Runtime dari:\nhttps://developer.microsoft.com/microsoft-edge/webview2/")
		os.Exit(1)
	}

	userDataDir := getUserDataDir()
	executablePath, _ := os.Executable()
	iconFullPath := filepath.Join(filepath.Dir(executablePath), "icon.ico")

	opts := webview2.WebViewOptions{
		Window:    nil,
		Debug:     false,
		DataPath:  userDataDir,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  windowTitle,
			Width:  1100,
			Height: 750,
			IconId: 2,
			Center: true,
		},
	}

	// Trim memory footprint: limit renderer count and disable unused Chromium
	// components (SmartScreen, in-app PDF viewer, background networking).
	// Read by WebView2 loader when the environment is created.
	_ = os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS",
		"--renderer-process-limit=1 --process-per-site --disable-site-isolation-trials --disable-gpu --disable-gpu-compositing --disable-features=SitePerProcess,IsolateOrigins,OutOfProcessNetworkService,msWebOOUI,msPdfOOUI,msSmartScreenProtection --disable-background-networking --disable-component-update --no-first-run --disable-sync")

	w := webview2.NewWithOptions(opts)
	if w == nil {
		showErrorDialog("Gagal menginisialisasi WebView2. Pastikan Microsoft Edge WebView2 Runtime terinstall.")
		os.Exit(1)
	}
	defer w.Destroy()
	startAudioSessionLabeler()

	hwnd := uintptr(w.Window())
	setDarkWindowFrame(hwnd)

	// Restore window size/position from the previous session, if any.
	if st := loadWindowState(); st.Saved {
		procSetWindowPos.Call(hwnd, 0, uintptr(uint32(st.X)), uintptr(uint32(st.Y)), uintptr(uint32(st.Width)), uintptr(uint32(st.Height)), swpNoZOrder|swpNoActivate)
	}

	installWindowSubclass(hwnd)
	trayAdd(hwnd, iconFullPath)
	defer trayDelete()

	w.SetTitle(windowTitle)

	// Bind native notification bridge (tray balloon; clicking it restores the window).
	_ = w.Bind("sendNativeNotification", func(title, body string) {
		go trayBalloon(title, body)
	})

	uaJSON, _ := json.Marshal(userAgent)

	// Inject JS: User-Agent spoofing + Notification API polyfill connecting to the native tray balloon.
	initScript := `
		// UserAgent override
		Object.defineProperty(navigator, 'userAgent', {
			get: () => ` + string(uaJSON) + `
		});
		Object.defineProperty(navigator, 'appVersion', {
			get: () => ` + string(uaJSON) + `
		});

		// Native Notification Polyfill for Windows Tray Balloon
		(function() {
			var hasBridge = typeof window.sendNativeNotification === 'function';
			window.Notification = function(title, options) {
				options = options || {};
				var body = options.body || '';
				if (hasBridge) {
					window.sendNativeNotification(String(title), String(body));
				}
				this.title = title;
				this.onclick = null;
				this.onclose = null;
				this.onerror = null;
				this.onshow = null;
			};
			Object.defineProperty(window.Notification, 'permission', {
				get: function() { return hasBridge ? 'granted' : 'default'; }
			});
			window.Notification.requestPermission = function(callback) {
				var perm = hasBridge ? 'granted' : 'default';
				if (typeof callback === 'function') {
					callback(perm);
				}
				return Promise.resolve(perm);
			};
		})();
	`

	w.Init(initScript)
	w.Navigate(appURL)
	w.Run()
}
