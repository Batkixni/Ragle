# Ragle

**Ragle** is an open-source, local-first digital asset manager, batch file workflow tool, and infinite moodboard built with **Go**, **MyGo Native UI**, and **bbolt**.

---

## Highlights

- **Local-First Database (`資料庫`)**: Import by **Index**, **Symlink**, **Move**, or **Copy** — your files stay on your disk, offline-ready, with live directory watching and one-click backup/restore.
- **Eagle-Grade Visual Workspace**: Switch between **Bento**, **Masonry**, and **Grid** layouts; inline full-stage **Image & Video Preview** with **Info / Similar** Inspector tabs; smooth collapsible sidebar & inspector; compact drag-to-tag/project/board previews.
- **Batch File Manager**: Pattern-based **Batch Rename** (`F2`) with live preview and full **Undo** (`Ctrl+Z`), **Batch Export**, native OS **Copy/Paste Files & Clipboard Images** (`Ctrl+C` / `Ctrl+V`), marquee drag-box selection, and `Ctrl+T` **Go to / Command Palette**.
- **Standalone & Project Boards (`白板`)**: Freeform moodboards inside projects or standalone in the sidebar — pan, zoom, drag assets, sticky notes, drawing strokes, and instant visual previews.
- **Visual Search & Built-in Local AI**: Pure-Go 128-dim visual embeddings, 64-bit pHash/dHash + 5-color OKLab palette reverse image & color search, plus one-click in-app **Ollaya / Laya** deployment in `Preferences → AI & Models`.
- **Built-in Web Server, CLI & MCP**: Share galleries & interactive boards over LAN from inside the app, self-host on a VPS with `ragle-server`, connect AI agents via `ragle-mcp`, and capture web references with the included Browser Companion Extension (`extensions/browser/`).
- **Plugin Ecosystem & GitHub Auto-Updater**: Extend Ragle with local JSON/script plugins (`examples/plugins/`) and stay up-to-date automatically via GitHub Releases (`Preferences → General → Software Updates`).

---

## Building from Source

Requires **Go 1.24+** (with `CGO_ENABLED=0` supported via MyGo).

```bash
# Run desktop app
go run ./cmd/ragle

# Build desktop app (Windows / macOS)
go tool mygo build -skip-installer

# Build standalone VPS web/share server CLI
go build -o ragle-server ./cmd/ragle-server

# Build MCP server (Stdio)
go build -o ragle-mcp ./cmd/ragle-mcp
```

## Browser Companion Extension

Load `extensions/browser/` as an unpacked extension in Chrome / Edge / Brave (`chrome://extensions` → Developer mode → Load unpacked), then enable the Built-in Server in Ragle (`Preferences → Sharing`).
