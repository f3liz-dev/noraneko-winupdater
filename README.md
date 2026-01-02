# Noraneko WinUpdater

An automatic update tool for [Noraneko Browser](https://github.com/AuroraLite/noraneko) on Windows.

This project is a **Go port** based on the [LibreWolf WinUpdater](https://codeberg.org/ltguillaume/librewolf-winupdater) by [ltguillaume](https://codeberg.org/ltguillaume).
Special thanks to ltguillaume for original AutoHotkey Implementation!

## Features

- Automatic update checking from GitHub releases
- Portable and installed version support
- Scheduled task support for automatic background updates
- SHA256 checksum verification
- Silent and interactive installation modes
- Self-update capability

## Getting Started

### For Portable Version

1. Download and extract the portable version of Noraneko Browser
2. Place `Noraneko-WinUpdater.exe` in the same directory as `Noraneko-Portable.exe`
3. Run `Noraneko-WinUpdater.exe` to check for and install updates

### For Installed Version

1. Download `Noraneko-WinUpdater.exe`
2. Place it in a location like `%AppData%\Noraneko\WinUpdater`
3. Run `Noraneko-WinUpdater.exe` to check for and install updates

## Command Line Options

```
  -scheduled      Run as scheduled task (silent mode)
  -portable       Force portable mode
  -check-only     Only check for updates, do not install
  -create-task    Create a Windows scheduled task for automatic updates
  -remove-task    Remove the Windows scheduled task
  -version        Print version and exit
```

## Scheduled Updates

To set up automatic update checks:

1. Run `Noraneko-WinUpdater.exe -create-task`
2. This will create a Windows scheduled task that checks for updates:
   - At system startup
   - Every 4 hours while the user is logged in

To remove automatic updates:

```
Noraneko-WinUpdater.exe -remove-task
```

## Configuration

Configuration is stored in `Noraneko-WinUpdater.ini` in the same directory as the executable:

```ini
[Settings]
; Path to noraneko.exe (auto-detected if empty)
Path=0
; Working directory for downloads (empty = system temp folder)
WorkDir=
; Enable/disable self-updates (1 = enabled)
UpdateSelf=1
; Ignore certificate revocation errors (0 = disabled)
IgnoreCrlErrors=0
; Release branch to track (nightly, beta, stable)
Branch=nightly
```

## Building from Source

Requirements:
- Go 1.21 or later

```bash
# Clone the repository
git clone https://github.com/f3liz-dev/noraneko-winupdater
cd noraneko-winupdater

# Build for Windows
GOOS=windows GOARCH=amd64 go build -o Noraneko-WinUpdater.exe .
```

## Credits

- **Original LibreWolf WinUpdater** by [ltguillaume](https://codeberg.org/ltguillaume): [Codeberg](https://codeberg.org/ltguillaume) | [GitHub](https://github.com/ltguillaume) | [Buy me a beer](https://coff.ee/ltguillaume) 🍺

## License

This project is licensed under the GNU General Public License v3.0 - see the [LICENSE](LICENSE) file for details.

The original LibreWolf WinUpdater by ltguillaume is also licensed under GPL-3.0.
