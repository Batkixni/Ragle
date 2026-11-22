# Contributing to Ragle / 參與貢獻指南

Thank you for your interest in contributing to **Ragle**! Whether you are fixing a bug, improving UI polish, adding a format previewer, writing a plugin, or improving documentation, every contribution helps make Ragle better for creators.

感謝你對 **Ragle** 的支持！無論是修復錯誤、優化介面體驗、新增設計師格式預覽、開發插件或完善文件，我們都非常歡迎你的參與。

---

## Table of Contents

1. [Development Setup](#development-setup)
2. [Project Architecture](#project-architecture)
3. [Coding & MyGo UI Guidelines](#coding--mygo-ui-guidelines)
4. [Testing & Verification](#testing--verification)
5. [Pull Request Checklist](#pull-request-checklist)
6. [License (MPL-2.0)](#license-mpl-20)

---

## Development Setup

### Prerequisites
- **Go 1.24+** (Ragle builds with `CGO_ENABLED=0`, so no C compiler is required)
- **Git**
- Optional: `ffmpeg` on `PATH` for video frame extraction & OGG/FLAC audio waveform decoding

### Clone & Run
```bash
git clone https://github.com/Batkixni/Ragle.git
cd Ragle

# Run the native desktop app
go run ./cmd/ragle

# Verify static checks & all tests
go tool mygo vet .
CGO_ENABLED=0 go test -count=1 ./...
```

---

## Project Architecture

```text
Ragle/
├── cmd/
│   ├── ragle/          # Native desktop app entry point (MyGo)
│   ├── ragle-server/   # Standalone headless LAN/VPS gallery & share server CLI
│   ├── ragle-mcp/      # Model Context Protocol (MCP) server (Stdio)
│   └── ragle-signal/   # Optional lightweight WebRTC pairing relay
├── internal/
│   ├── model/          # Core data records (Asset, Stack, Tag, Project, Board, Placement)
│   ├── db/             # bbolt database buckets, indexes, atomic backups
│   ├── library/        # Importer, Stacks, Batch Rename/Undo, Export, Visual Search
│   ├── preview/        # Native Font (TTF/OTF), Audio waveform/player, PDF page renderer
│   ├── thumbnail/      # Background thumbnailing, pHash/dHash, OKLab palette extraction
│   ├── ai/             # Local 128-dim visual embeddings & Ollaya/Laya runtime client
│   ├── canvas/         # Infinite moodboard geometry, transforms & session state
│   ├── server/         # Embedded HTTP web gallery, board viewer, companion & MCP API
│   ├── mcp/            # JSON-RPC MCP tool & resource handlers
│   ├── plugin/         # Local plugin discovery & execution manager
│   ├── updater/        # GitHub Releases SemVer checker, SHA-256 verifier & self-updater
│   ├── config/         # User preference persistence (<UserConfigDir>/Ragle/config.json)
│   └── i18n/           # Bilingual Traditional Chinese (zh-Hant) & English catalog
├── ui/                 # MyGo native views, animations, PureRef floating window, UI tests
├── extensions/browser/ # Chrome / Edge MV3 Browser Companion Extension
└── examples/plugins/   # Reference plugin templates
```

---

## Coding & MyGo UI Guidelines

Ragle uses **[MyGo Native UI](https://github.com/egoist/mygo)** (`v0.10.0`). Please keep these key rules in mind when modifying `ui/`:

1. **Never store `ui.Element` or `*ui.Context`**: Elements expire after each build pass. Store persistent app state, widget state, or `ui.Handle` on `App` instead.
2. **Keep I/O off the UI thread**: Perform file reads, decoding, network requests, and database writes in goroutines and publish UI updates asynchronously through `win.Update(func() { ... })`. Always check `a.isClosed()` inside `win.Update`.
3. **Stable Keys**: Always assign unique `.Key(...)` identities before constructing stateful controls, animated containers, or dynamic lists.
4. **Register Window-Wide Shortcuts at Root**: Global shortcuts (`Ctrl+T`, `Ctrl+F`, `Ctrl+G`, `Esc`, etc.) belong in `registerWindowShortcuts(c)` at the root `View(c)` scope.
5. **Bilingual i18n Parity**: Every user-visible string MUST be added to `internal/i18n/i18n.go` in **both Traditional Chinese (`zh-Hant`) and English (`en`)**. `internal/i18n` tests enforce strict parity.
6. **Clean Terminology & Visual Grammar**:
   - In Traditional Chinese UI copy, refer to the asset database as **「資料庫」** and boards as **「白板」**.
   - Keep surfaces neutral obsidian/zinc monochrome (`#111111` / `#18181B` in dark mode, `#FFFFFF` / `#F4F4F5` in light mode) with crisp monochrome accent controls (`#EDEDED` dark / `#18181B` light) so the user's artwork remains the visual focus.

---

## Testing & Verification

Before opening a Pull Request, run:

```bash
# 1. MyGo UI static analyzer (catches stored elements & unsafe goroutine captures)
go tool mygo vet .

# 2. Full unit + headless UI test suite
CGO_ENABLED=0 go test -count=1 ./...
```

### Writing UI Regression Tests
Use `ui.NewTester(a.View, width, height)` in `ui/interaction_test.go` to simulate real clicks, keyboard shortcuts, drag interactions, and animation frames (`tt.Advance(...)`).

### Visual Snapshots (Optional)
You can capture rendered UI screenshots locally to inspect visual changes:
```bash
RAGLE_SNAPSHOT_DIR=/tmp/ragle-snaps CGO_ENABLED=0 go test -count=1 ./ui -run TestViewSnapshots
```

---

## Pull Request Checklist

- [ ] Code is cleanly formatted (`gofmt -s -w .`)
- [ ] `go tool mygo vet .` passes with zero warnings/errors
- [ ] `CGO_ENABLED=0 go test -count=1 ./...` passes completely
- [ ] New UI strings exist in both Traditional Chinese and English (`internal/i18n/i18n.go`)
- [ ] No build binaries (`build/`, `dist/`, `*.exe`, `*.zip`) or temporary test outputs are committed
- [ ] Commit messages and PR description clearly explain **what** changed and **why**

---

## License (MPL-2.0)

By contributing to Ragle, you agree that your contributions will be licensed under the **[Mozilla Public License 2.0 (MPL-2.0)](LICENSE)**.
