<p align="center">
  <img src="docs/screenshots/cover.png" alt="Raglex — Native local-first visual reference library & infinite moodboard" width="100%" />
</p>

<p align="center">
  <a href="README.md"><strong>English</strong></a> &nbsp;·&nbsp;
  <a href="README.zh-TW.md">繁體中文</a> &nbsp;·&nbsp;
  <a href="README.ja.md">日本語</a> &nbsp;·&nbsp;
  <a href="https://raglex.sorai.tw">Website</a> &nbsp;·&nbsp;
  <a href="https://raglex.sorai.tw/docs">Docs</a>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://shieldcn.dev/github/license/Batkixni/Raglex.svg?variant=secondary" alt="License: MPL 2.0" /></a>
  <a href="https://github.com/Batkixni/Raglex/stargazers"><img src="https://shieldcn.dev/github/stars/Batkixni/Raglex.svg?variant=secondary" alt="GitHub Stars" /></a>
  <img src="https://shieldcn.dev/badge/Version-v0.5.1-FF5C33.svg" alt="v0.5.1" />
  <img src="https://shieldcn.dev/badge/Go-1.27+-00ADD8.svg" alt="Go 1.27+" />
  <img src="https://shieldcn.dev/badge/UI-MyGo_Native-FF5C33.svg" alt="MyGo Native UI" />
  <img src="https://shieldcn.dev/badge/Platform-macOS_%7C_Windows_%7C_Linux-3F3F46.svg" alt="Platforms" />
</p>

<div align="center">
  <a href="https://awesomemygo.dev/apps/raglex">
    <img src="https://awesomemygo.dev/badge.svg" alt="Listed on Awesome MyGo">
  </a>
</div>

<p align="center">
  <strong>Open-source, 100% native digital asset manager, offline visual AI search, and infinite reference board — built with Go + MyGo.</strong><br/>
  Zero Electron. Zero forced copying. Zero telemetry. Your files stay on your disk.
</p>

---

## Workspace Previews

| Native Library & Inspector | Project Workspace & Board Previews |
| :---: | :---: |
| <img src="docs/screenshots/inspector.png" alt="Native Library & Inspector" width="100%" /> | <img src="docs/screenshots/project.png" alt="Project Workspace & Board Previews" width="100%" /> |

| Infinite Moodboard Studio | Always-on-Top Floating Reference Window |
| :---: | :---: |
| <img src="docs/screenshots/board.png" alt="Infinite Moodboard Studio" width="100%" /> | <img src="docs/screenshots/refwin.png" alt="Always-on-Top Floating Reference Window" width="100%" /> |

---

## Core Highlights

### 1. 100% Native Performance (Zero Electron)
- Compiled into a single **~21 MB native binary** in **Go + [MyGo](https://github.com/egoist/mygo)** (`CGO_ENABLED=0` cross-compilable) backed by embedded **bbolt**.
- Instant cold start, fluid 120Hz scrolling across tens of thousands of assets with viewport virtualization, and ultra-low memory footprint.
- Native platform craft: Windows 11 Acrylic translucency & Snap Layouts, macOS Vibrancy & native window controls, and clean Linux support.

### 2. Non-Destructive Indexing — Your Files Stay Yours
- **Default `Index` mode never copies or moves your original files.** Raglex reads your folders in place with zero duplication and zero proprietary lock-in.
- Need managed storage? Switch anytime per import between **Index**, **Symlink**, **Copy**, and **Move**.
- Safe by default: deleting an indexed asset only removes the library record — and if you actually want to remove the file from disk, an explicit opt-in checkbox (**Also permanently delete from disk**) is right inside the delete modal.

### 3. Infinite Reference Boards & Always-on-Top Floating Window
- GPU-accelerated freeform moodboards with smooth pan/zoom, multi-select alignment, groups, frames, freehand markup, arrows, and multiline text notes.
- Pop any board out into an **always-on-top translucent floating reference window** (with opacity, zoom, and grayscale toggle) while painting, modeling, or designing in Blender, Photoshop, Figma, or Clip Studio.

### 4. Local Visual AI Embedding, Color Palette & Offline Auto-Tagging
- **Built-in on-device Visual AI Embedding**: computes visual feature vectors + perceptual hashes locally for instant **Search by Image** and **Find Similar Assets** — 100% offline.
- **OKLab Dominant Color Palette**: automatically extracts 5-color palettes per asset so you can click any swatch to copy HEX or filter your entire library by color harmony.
- **Optional Local AI Auto-Tagging (Ollama / Laya)**: connect your own local vision model to get non-intrusive tag suggestions without sending a single pixel to third-party APIs.

### 5. Parallel Folders, Hierarchical Tags, Stacks & Full Creative Media
- **Parallel Folders & Hierarchical Tags** with 8-color inheritance, drag-and-drop sorting, version **Stacks**, **Watch Folders** for automatic ingestion, and a rule-based **Batch Rename** engine with live conflict preview & undo.
- **Bento**, **Masonry**, and **Grid** views, Quick Look (`Space`), and native playback/preview for images, videos (**bundled static `ffmpeg`**), Camera RAW (`CR3`, `ARW`, `NEF`, `RAF`, `NAF`, `DNG`), SVG, fonts, audio, and PDFs.

### 6. Browser Companion Extension (Chrome / Edge / Brave / Arc / Firefox / Zen)
- Right-click any image to clip with source URL & title, batch-collect images from a webpage, or save clips into an **Offline Clipper Inbox** even when Raglex is closed.
- Full guide: [`docs/BROWSER_EXTENSION.md`](docs/BROWSER_EXTENSION.md) · Online Docs: [raglex.sorai.tw/docs](https://raglex.sorai.tw/docs#extension)

### 7. Self-Hosted Zero-Knowledge E2EE Sync & 6-Char P2P Transfer
- Host your own private sync server in one command via Docker (`deploy/docker-compose.yml` + `raglex-server`) with optional **Cloudflare R2 / S3 / MinIO** storage, Mobile `/mobile` PWA, and MCP Server.
- **Zero-Knowledge AES-256-GCM E2EE enabled by default**: Raglex automatically generates a local 256-bit random AES key (`rglx-aes256-...`) that encrypts personal sync files, filenames, tags, folders, and notes on-device before upload. Export (`Save Key File`) or load (`Load Key File`) to sync across your own devices.
- **Selective Folder Sync** + **Smart Delta Auto-Sync** (~150 B idle polling) + **6-character E2E encrypted P2P transfers**.
- Full guide: [`docs/SELF_HOST.md`](docs/SELF_HOST.md) · Online Docs: [raglex.sorai.tw/docs](https://raglex.sorai.tw/docs#self-host)

### 8. Open Plugin Marketplace & Built-in Local API / MCP
- Write plugins in **any language** (Go, Python, Node.js, Rust, PowerShell, Bash) using Raglex's stdin/stdout JSON SDK ([`docs/PLUGIN_SDK.md`](docs/PLUGIN_SDK.md)) and publish via PR to [`Batkixni/raglex-plugins`](https://github.com/Batkixni/raglex-plugins).
- Official optional plugins available in 1 click: **Eagle Import** (`.library` / `.eaglepack`), **Smart Duplicate Finder**, **Color Palette Exporter (CSS/JSON)**, **Auto Aspect Ratio & Resolution Tagger**, and **Markdown Notes & Moodboard Brief**.
- Token-protected built-in local server (`X-Raglex-Token` + strict localhost/extension CORS + `HttpOnly` Web Gallery / Mobile PWA) and **MCP Server** for AI assistants.

---

## Download & Installation

Grab the latest release from **[GitHub Releases](https://github.com/Batkixni/Raglex/releases/latest)** or **[raglex.sorai.tw](https://raglex.sorai.tw)**:

| Platform | Architecture | Package |
| :--- | :--- | :--- |
| **Windows** | x64 (`windows/amd64`) | `raglex-setup-0.5.1-amd64.exe` (Installer) / `Raglex-windows-amd64.zip` |
| **Windows** | ARM64 (`windows/arm64`) | `raglex-setup-0.5.1-arm64.exe` (Installer) / `Raglex-windows-arm64.zip` |
| **macOS** | Apple Silicon (`darwin/arm64`) | `Raglex-darwin-arm64.zip` |
| **macOS** | Intel (`darwin/amd64`) | `Raglex-darwin-amd64.zip` |
| **Linux** | x64 / ARM64 | `.deb`, `.rpm`, `.tar.gz` |
| **Browser Extension** | Chromium / Firefox & Zen | `raglex-browser-extension.zip` / `raglex-firefox-extension.xpi` |
| **Self-Host CLI** | Windows / macOS / Linux | `raglex-cli-<os>-<arch>.zip` (`raglex-server` & `raglex-mcp`) |

> [!NOTE]
> **About macOS Gatekeeper / Unsigned Builds (Full Transparency)**
> Raglex is 100% open source under MPL-2.0, contains **zero telemetry and zero malicious code**, and every binary is built directly by GitHub Actions with verifiable `SHA256SUMS.txt`.
> However, because this is an independent open-source project without a paid Apple Developer ID certificate, macOS may show an "unidentified developer" or "damaged" Gatekeeper warning on first launch. To open it after moving `Raglex.app` to `/Applications`:
> ```bash
> xattr -cr /Applications/Raglex.app
> ```
> If you have any concern about running unsigned prebuilt binaries, you are completely welcome to audit the source and compile it yourself — or simply choose not to use it, no hard feelings at all.

---

## Quick Install: Browser Companion Extension

1. Open **Raglex → Preferences → Sharing / Advanced**, turn on **Built-in Local Server** (port `47595`), and click **Copy Access Key**.
2. Download `raglex-browser-extension.zip` (Chrome / Edge / Brave / Arc) or `raglex-firefox-extension.xpi` (Firefox / Zen) from [GitHub Releases](https://github.com/Batkixni/Raglex/releases/latest).
3. **Chromium**: Unzip the archive → open `chrome://extensions` (or `edge://extensions`) → enable **Developer mode** → click **Load unpacked** → pick the unzipped folder.
4. **Firefox / Zen**: Drag `raglex-firefox-extension.xpi` into the browser (or load `manifest.json` via `about:debugging#/runtime/this-firefox`).
5. Click the extension icon, paste your **Access Key**, and start clipping. See [`docs/BROWSER_EXTENSION.md`](docs/BROWSER_EXTENSION.md) for Offline Inbox setup.

---

## Quick Start: Self-Hosted Sync Server

```bash
git clone https://github.com/Batkixni/Raglex.git
cd Raglex/deploy
export RAGLEX_TOKEN="$(openssl rand -hex 32)"
docker compose up -d
```

Then connect in **Raglex → Preferences → Account / Sync → Self-Host** using `http://<your-server>:47595` (or your HTTPS reverse proxy URL) and your `RAGLEX_TOKEN`. See [`docs/SELF_HOST.md`](docs/SELF_HOST.md) for Caddy/Nginx HTTPS, R2/S3 storage, and E2EE key backup.

---

## Build from Source

Requires **Go 1.27+**. All desktop binaries compile cleanly with `CGO_ENABLED=0`:

```bash
git clone https://github.com/Batkixni/Raglex.git
cd Raglex
CGO_ENABLED=0 go tool mygo build -o build/dev
```

---

## Documentation & Privacy

- **Website & Trilingual Docs**: [raglex.sorai.tw](https://raglex.sorai.tw) · [raglex.sorai.tw/docs](https://raglex.sorai.tw/docs)
- **Self-Host Guide**: [`docs/SELF_HOST.md`](docs/SELF_HOST.md)
- **Browser Extension Guide**: [`docs/BROWSER_EXTENSION.md`](docs/BROWSER_EXTENSION.md)
- **Plugin SDK**: [`docs/PLUGIN_SDK.md`](docs/PLUGIN_SDK.md) · **Plugin Registry**: [`Batkixni/raglex-plugins`](https://github.com/Batkixni/raglex-plugins)
- **Design System**: [`DESIGN.md`](DESIGN.md) · **Changelog**: [`CHANGELOG.md`](CHANGELOG.md) · **Privacy Policy**: [`PRIVACY.md`](PRIVACY.md)

## License

Licensed under the [Mozilla Public License 2.0 (MPL-2.0)](LICENSE).
