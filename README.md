<p align="center">
  <img src="banner.png" alt="WaDeskLight" width="100%">
</p>

# WaDeskLight

Lightweight Windows desktop wrapper for [WhatsApp Web](https://web.whatsapp.com), built with Go and Microsoft Edge WebView2.

## Features

- Native Windows WebView2 window — no Electron, no Chromium bloat.
- Persistent WhatsApp session across restarts.
- System tray — close minimizes to tray, click restores, balloon notifications.
- Window size and position remembered between sessions.
- Dark Windows title bar and frame.
- Notification bridge via system tray balloon.
- Camera and microphone access for WhatsApp voice and video calls.
- Single-instance protection.
- High-DPI display support.
- Small native executable (~3 MB).

## Requirements

- Windows 10 or newer.
- Microsoft Edge WebView2 Runtime (the app shows a download prompt when missing).
- WhatsApp account paired with WhatsApp Web.

## Download

Download `WhatsApp.exe` from the latest [GitHub Release](https://github.com/Adytm404/whatsapp-web.view/releases).

Run the executable, scan the QR code, allow camera and microphone access when prompted by Windows, and keep using WhatsApp normally. The session is saved automatically.

## Session Data

Profile data is stored at:

```text
%APPDATA%\WaDeskLight\UserData
```

Do not delete this folder if the existing login session must remain available. Closing the app does not clear session data.

## Build From Source

Install Go and a Windows C compiler (MinGW or TDM-GCC), then run:

```powershell
go mod download
go build -ldflags="-H windowsgui -s -w" -o WhatsApp.exe .
```

The repository includes the Windows manifest (DPI aware), embedded icon, and version metadata (VERSIONINFO) used by the build.

## Project Structure

| File | Description |
|------|-------------|
| `main.go` | WebView2 window, persistent profile, dark frame, tray, and notifications |
| `app.manifest` | Windows DPI and application manifest |
| `resource.rc` | Windows icon, manifest, and version metadata |
| `icon.ico` | Application icon |
| `banner.png` | Project banner image |

## How It Works

1. Checks for an existing instance (mutex) and brings the existing window to focus if found.
2. Verifies WebView2 Runtime is installed (shows error dialog if not).
3. Creates a WebView2 window with dark frame and loads `https://web.whatsapp.com`.
4. Injects a User-Agent override and a Notification API polyfill that bridges to native system tray balloons.
5. Saves window position on close; restores it on next launch.
6. Hides to system tray on close instead of quitting.

## Privacy

This app loads WhatsApp Web directly. Chat data and authentication state are handled by WhatsApp Web and stored locally in the profile directory above. This project is not affiliated with WhatsApp or Meta.

## License

No license has been declared yet.
