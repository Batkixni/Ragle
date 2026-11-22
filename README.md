<p align="center">
  <img src="Logo.png" alt="Ragle Logo" width="112" height="112" />
</p>

<h1 align="center">Ragle</h1>

<p align="center">
  <strong>Open-source, local-first digital asset manager, batch file workflow tool, and infinite moodboard for creators.</strong>
</p>

<p align="center">
  <a href="README.md"><strong>English</strong></a> &nbsp;·&nbsp;
  <a href="README.zh-TW.md">繁體中文</a>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MPL_2.0-ff5c33.svg?style=flat-square" alt="License: MPL 2.0" /></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.24+" />
  <img src="https://img.shields.io/badge/UI-MyGo_Native-18181B?style=flat-square" alt="MyGo Native UI" />
  <img src="https://img.shields.io/badge/Platform-macOS_|_Windows_|_Linux-52525B?style=flat-square" alt="Platforms" />
</p>

---

## Why Ragle?

**Ragle** brings together the speed of a native desktop app, the visual clarity of Eagle, and the floating reference ergonomics of PureRef — with **zero Electron bloat**, **zero cloud lock-in**, and a **single local database** (`bbolt`) that stays 100% yours on disk.

Built with **Go** and **[MyGo Native UI](https://github.com/egoist/mygo)** (`CGO_ENABLED=0`), Ragle starts instantaneously, runs smoothly across **macOS (Apple Silicon & Intel)**, **Windows (x64 & ARM64)**, and **Linux (x64 & ARM64)**, and ships as a compact portable binary.

---

## Key Features

### Visual Database & Asset Stacks
- **Flexible Storage Modes**: Import by **Index** (leave files in place), **Symlink**, **Move**, or **Copy** — backed by live filesystem watching and automatic database snapshots (`ragle.db.bak`).
- **Editorial Layouts**: Switch effortlessly between **Bento**, **Masonry**, and **Grid** views with real-time zoom, nested colored tags, star ratings, and marquee rubber-band multi-selection.
- **Asset Stacks (`Ctrl+G`)**: Fold `poster_v1` / `poster_v2` / `poster_final` or multi-angle character sheets into a single tidy stacked card, or auto-group matching filenames in one click via `Ctrl+T`. Expand anytime to compare versions side-by-side or switch versions right inside the inline viewer.

### Native Designer Format Previews
Inspect creative assets inline without leaving Ragle:
- **Fonts (`.ttf` / `.otf`)**: Live custom preview text input, `24pt / 36pt / 48pt / 72pt` size switcher, full glyph waterfall, and `Aa` specimen thumbnails.
- **Audio (`.wav` / `.mp3` / `.flac` / `.ogg` / `.m4a` / `.aac`)**: Interactive 96-bar clickable waveform scrubber, direct playback, volume control, and waveform grid thumbnails.
- **SVG Vectors (`.svg`)**: Lossless GPU vector zoom (25% – 800%) with toggleable transparency checkerboard backdrop.
- **Multi-Page PDF (`.pdf`)**: Crisp page-by-page navigation (`Prev / Next`, zoom) with automatic cover thumbnails.
- **Images & Video**: Full-stage inline viewer with zoom, animated GIF & video playback, and an **Info / Similar** Inspector sidebar.

### Moodboards & PureRef Floating Reference Window
- **Standalone & Project Boards**: Create freeform infinite moodboards right from the sidebar or inside any project — drag assets, arrange groups, add sticky notes, pan, zoom, and rotate.
- **PureRef Always-on-Top Floating Window**: Pop out any board into a frameless, always-on-top reference window with an auto-hiding HUD, adjustable **opacity (35%–100%)**, and independent pan/zoom/fit (`F`) — ideal while illustrating, modeling in 3D, or editing video.

### Batch File Manager & Productivity
- **Batch Rename (`F2`) & Undo (`Ctrl+Z`)**: Pattern-based renaming (`{name}`, `{n}`, `{date}`, `{ext}`, find/replace) with live collision-safe preview and full reversible history.
- **Batch Export & Native Clipboard**: Export selected assets in bulk, copy real OS files (`Ctrl+C`), and paste files or clipboard screenshots (`Ctrl+V`) directly into Ragle.
- **Command Palette (`Ctrl+T`)**: Instant fuzzy jump across tags, projects, boards, and power actions.

### Local AI, Visual Search & Ecosystem
- **Built-in Visual Search**: Pure-Go 128-dim visual embeddings, 64-bit pHash/dHash perceptual similarity, and 5-color OKLab palette search — works 100% offline.
- **One-Click In-App Laya / Ollaya Deployment**: Deploy and manage local open-source **Laya** decision models (`laya:multilingual`, `laya`, `winnow:e4b`) directly inside `Preferences → AI & Models` with zero terminal setup.
- **Built-in Web Server & VPS CLI (`ragle-server`)**: Share dark-mode web galleries & interactive boards over LAN from inside the app, or self-host headless on a VPS.
- **MCP Server (`ragle-mcp`)**: Connect AI coding/design agents directly to your Ragle library over Stdio or HTTP (`/mcp`).
- **Browser Companion Extension (`extensions/browser/`)**: Chrome / Edge MV3 extension with drag-to-save capsule, right-click image capture, screenshot clipping, and batch page grabber.
- **Plugin Ecosystem & GitHub Auto-Updater**: Write local plugins in any language (`examples/plugins/`) and stay up to date automatically via GitHub Releases with SHA-256 verification.

---

## Keyboard Shortcuts

| Shortcut | Action |
| :--- | :--- |
| `Ctrl+T` / `Cmd+T` | Open Go to / Command Palette |
| `Ctrl+F` / `Cmd+F` | Focus Library Search |
| `Ctrl+I` / `Cmd+I` | Import Files |
| `Ctrl+A` / `Cmd+A` | Select All Visible Assets |
| `Ctrl+G` / `Cmd+G` | Group Selected Assets into a Version Stack |
| `F2` | Batch Rename Selection |
| `Ctrl+Z` / `Cmd+Z` | Undo Last Batch Rename |
| `Ctrl+C` / `Ctrl+V` | Copy OS Files / Paste Files or Clipboard Images |
| `Space` | Open / Close Inline Asset Preview |
| `0` – `5` | Set Star Rating |
| `F` *(in Floating Ref)* | Fit All References to Window |
| `Esc` | Close Preview / Stack / Modal |

---

## Supported Platforms

| Platform | Architecture | Package |
| :--- | :--- | :--- |
| **macOS** | Apple Silicon (`darwin/arm64`) | `Ragle-darwin-arm64.zip` (`.app` bundle) |
| **macOS** | Intel x86_64 (`darwin/amd64`) | `Ragle-darwin-amd64.zip` (`.app` bundle) |
| **Windows** | x64 (`windows/amd64`) | `Ragle-windows-amd64.zip` / `.exe` |
| **Windows** | ARM64 (`windows/arm64`) | `Ragle-windows-arm64.zip` / `.exe` |
| **Linux** | x64 (`linux/amd64`) | `.tar.gz`, `.deb`, `.rpm`, `PKGBUILD` |
| **Linux** | ARM64 (`linux/arm64`) | `.tar.gz`, `.deb`, `.rpm`, `PKGBUILD` |

Download prebuilt releases from the [GitHub Releases](../../releases) page.

---

## Building from Source

Requires **Go 1.24+** (pure Go, `CGO_ENABLED=0` — cross-compile any desktop target from any OS!).

```bash
# Run the desktop app
go run ./cmd/ragle

# Build for your current platform
CGO_ENABLED=0 go tool mygo build -skip-installer

# Cross-compile for macOS (Apple Silicon & Intel), Windows (x64 & ARM64), and Linux (x64 & ARM64)
CGO_ENABLED=0 go tool mygo build -skip-installer -skip-dmg -skip-notarize \
  -platform "darwin/arm64,darwin/amd64,windows/amd64,windows/arm64,linux/amd64,linux/arm64"

# Build standalone VPS server CLI & MCP server
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ragle-server ./cmd/ragle-server
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ragle-mcp ./cmd/ragle-mcp

# Run static checks & test suite
go tool mygo vet .
CGO_ENABLED=0 go test -count=1 ./...
```

---

## Browser Companion Extension & Plugins

- **Browser Extension**: Open `chrome://extensions` (or `edge://extensions`), enable **Developer mode**, click **Load unpacked**, and select [`extensions/browser/`](extensions/browser/). Enable the Built-in Server under `Preferences → Sharing`.
- **Plugins**: Drop plugin folders containing `manifest.json` into `<UserConfigDir>/Ragle/plugins/` (or open the folder from `Preferences → Plugins`). See [`examples/plugins/asset-stats/`](examples/plugins/asset-stats/) for a working template.

---

## Contributing

Contributions are warmly welcomed! Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.

## License

Ragle is licensed under the **[Mozilla Public License 2.0 (MPL-2.0)](LICENSE)**.
You are free to use, modify, and distribute Ragle under the terms of MPL-2.0; modifications to MPL-covered source files must remain available under MPL-2.0.
