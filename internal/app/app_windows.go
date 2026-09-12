//go:build windows

package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"wadesklight/internal/audio"
)

const (
	windowTitle = "WaDeskLight"
	appURL      = "https://web.whatsapp.com"
	mutexName   = "WaDeskLightSingleInstanceMutex"
	userAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
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

	gTrayMu sync.Mutex
	// Large app icon used as the balloon icon when a message carries no avatar.
	gAppBalloonIcon uintptr
	// Avatar icon of the most recent notification, owned by us.
	gBalloonIcon uintptr
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

func loadTrayIcon(iconPath string, size uintptr) uintptr {
	pathPtr, _ := windows.UTF16PtrFromString(iconPath)
	hIcon, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(pathPtr)), imageIcon, size, size, lrLoadFromFile)
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
	nid.hIcon = loadTrayIcon(iconPath, 16)
	setTip(&nid, "WaDeskLight")
	gAppBalloonIcon = loadTrayIcon(iconPath, 32)
	gTray = nid
	procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
}

// trayBalloon shows a notification. iconData, when it decodes to an image, is
// used as the balloon icon so the sender's avatar appears; otherwise the app
// icon is used. Passing nil is fine for app-generated messages.
func trayBalloon(title, message string, iconData []byte) {
	gTrayMu.Lock()
	defer gTrayMu.Unlock()
	if gTray.hWnd == 0 {
		return
	}

	// NIF_INFO alone would drop the icon, tip, and callback flags.
	gTray.uFlags = nifMessage | nifIcon | nifTip | nifInfo
	setInfoTitle(&gTray, title)
	setInfo(&gTray, message)

	avatar := createIconFromImage(decodeIconImage(iconData))
	icon := avatar
	if icon == 0 {
		icon = gAppBalloonIcon
	}
	if icon != 0 {
		gTray.hBalloonIcon = icon
		gTray.dwInfoFlags = niifUser | niifLargeIcon
	} else {
		gTray.dwInfoFlags = niifInfo
	}
	procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&gTray)))

	// Release the previous avatar, not the one just handed to the shell.
	if gBalloonIcon != 0 {
		procDestroyIcon.Call(gBalloonIcon)
	}
	gBalloonIcon = avatar
}

// decodeDataURL unwraps a base64 "data:image/...;base64,..." URL.
func decodeDataURL(s string) []byte {
	const marker = ";base64,"
	if !strings.HasPrefix(s, "data:image/") {
		return nil
	}
	i := strings.Index(s, marker)
	if i < 0 {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(s[i+len(marker):])
	if err != nil {
		return nil
	}
	return data
}

func trayDelete() {
	if gTray.hWnd == 0 {
		return
	}
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&gTray)))
	gTray.hWnd = 0
	if gBalloonIcon != 0 {
		procDestroyIcon.Call(gBalloonIcon)
		gBalloonIcon = 0
	}
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
	setMemoryUsageTargetLevel(memoryUsageNormal)
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
		setMemoryUsageTargetLevel(memoryUsageLow)
		go trayBalloon("WaDeskLight", "Masih berjalan di system tray. Klik ikon untuk membuka kembali.", nil)
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

// Run starts the WaDeskLight window and blocks until the app exits.
// It returns the process exit code.
func Run() int {
	_, isSingle := checkSingleInstance()
	if !isSingle {
		return 0
	}

	if !webView2RuntimeInstalled() {
		showErrorDialog("WebView2 Runtime tidak ditemukan.\n\nSilakan install Microsoft Edge WebView2 Runtime dari:\nhttps://developer.microsoft.com/microsoft-edge/webview2/")
		return 1
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
	//
	// max-old-space-size caps V8's old space. It does not free memory by
	// itself; it makes GC run sooner. Too low a value crashes the renderer on
	// large chat histories, so 512 MB is deliberately conservative.
	_ = os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS",
		"--js-flags=--max-old-space-size=512 --renderer-process-limit=1 --process-per-site --disable-site-isolation-trials --disable-gpu --disable-gpu-compositing --disable-features=SitePerProcess,IsolateOrigins,OutOfProcessNetworkService,msWebOOUI,msPdfOOUI,msSmartScreenProtection --disable-background-networking --disable-component-update --no-first-run --disable-sync")

	w := webview2.NewWithOptions(opts)
	if w == nil {
		showErrorDialog("Gagal menginisialisasi WebView2. Pastikan Microsoft Edge WebView2 Runtime terinstall.")
		return 1
	}
	defer w.Destroy()
	initMemoryControl(w)
	audio.StartLabeler()

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
	_ = w.Bind("sendNativeNotification", func(title, body, iconDataURL string) {
		icon := decodeDataURL(iconDataURL)
		go trayBalloon(title, body, icon)
	})

	uaJSON, _ := json.Marshal(userAgent)

	// Inject JS: User-Agent spoofing + Notification API polyfill connecting to the native tray balloon.
	initScript := fmt.Sprintf(`
		// UserAgent override
		Object.defineProperty(navigator, 'userAgent', {
			get: () => %[1]s
		});
		Object.defineProperty(navigator, 'appVersion', {
			get: () => %[1]s
		});

		// Native Notification Polyfill for Windows Tray Balloon
		(function() {
			var hasBridge = typeof window.sendNativeNotification === 'function';

			// Re-encode the sender avatar as a PNG data URL the native side can
			// turn into an icon. WhatsApp hands us a blob: URL, which is
			// same-origin and therefore does not taint the canvas.
			function avatarDataURL(url) {
				if (!url) { return Promise.resolve(''); }
				return fetch(url)
					.then(function(r) { return r.blob(); })
					.then(function(blob) { return createImageBitmap(blob); })
					.then(function(bmp) {
						var size = 64;
						var canvas = document.createElement('canvas');
						canvas.width = size;
						canvas.height = size;
						canvas.getContext('2d').drawImage(bmp, 0, 0, size, size);
						bmp.close();
						return canvas.toDataURL('image/png');
					})
					.catch(function() { return ''; });
			}

			window.Notification = function(title, options) {
				options = options || {};
				var body = options.body || '';
				if (hasBridge) {
					// Never let a slow avatar fetch hold up the notification.
					var timeout = new Promise(function(resolve) {
						setTimeout(function() { resolve(''); }, 1500);
					});
					Promise.race([avatarDataURL(options.icon), timeout])
						.then(function(icon) {
							window.sendNativeNotification(String(title), String(body), icon || '');
						});
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
	`, string(uaJSON))

	w.Init(initScript)
	w.Navigate(appURL)
	w.Run()

	return 0
}
