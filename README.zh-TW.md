<p align="center">
  <img src="docs/screenshots/cover.png" alt="Raglex — Organize references. Build moodboards. Stay local." width="100%" />
</p>

<p align="center">
  <a href="README.md">English</a> &nbsp;·&nbsp;
  <a href="README.zh-TW.md"><strong>繁體中文</strong></a>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://shieldcn.dev/github/license/Batkixni/Raglex.svg?variant=secondary" alt="License: MPL 2.0" /></a>
  <a href="https://github.com/Batkixni/Raglex/stargazers"><img src="https://shieldcn.dev/github/stars/Batkixni/Raglex.svg?variant=secondary" alt="GitHub Stars" /></a>
  <img src="https://shieldcn.dev/badge/Go-1.24+-00ADD8.svg" alt="Go 1.24+" />
  <img src="https://shieldcn.dev/badge/UI-MyGo_Native-FF5C33.svg" alt="MyGo Native UI" />
  <img src="https://shieldcn.dev/badge/Platform-macOS_%7C_Windows_%7C_Linux-3F3F46.svg" alt="Platforms" />
</p>

## 介面預覽

| 毛玻璃視覺資料庫與資訊檢視器 | 專案空間與白板即時預覽 |
| :---: | :---: |
| <img src="docs/screenshots/inspector.png" alt="毛玻璃視覺資料庫與資訊檢視器" width="100%" /> | <img src="docs/screenshots/project.png" alt="專案空間" width="100%" /> |

| 無限靈感白板工作室 | 透明置頂參考浮窗（Always on Top） |
| :---: | :---: |
| <img src="docs/screenshots/board.png" alt="無限靈感白板" width="100%" /> | <img src="docs/screenshots/refwin.png" alt="透明置頂參考浮窗" width="100%" /> |

---

## 為什麼選擇 Raglex？

**Raglex** 是一款原生桌面視覺素材管理與無限靈感白板工具——**原生極速啟動**、**完全離線可用**，所有索引、標籤與白板都儲存在你本機的單一 `bbolt` 資料庫中，檔案主權 100% 歸你所有。

採用 **Go** 與 **[MyGo Native UI](https://github.com/egoist/mygo)**（`CGO_ENABLED=0`）打造，跨平台支援 **macOS（Apple Silicon & Intel）**、**Windows（x64 & ARM64）** 與 **Linux（x64 & ARM64）**。

---

## 核心功能

### 視覺資料庫與版本堆疊（Asset Stacks）
- **四種彈性匯入模式**：支援 **索引（原地不動）**、**軟連結**、**移動**、**複製**，內建檔案變更即時監控與自動備份還原（`raglex.db.bak`）。
- **三種質感排版**：一鍵切換 **Bento 拼接**、**瀑布流 Masonry** 與 **等比格狀 Grid**，搭配滑順縮放、多層彩色標籤樹、評分篩選與滑鼠拖拉框選多選。
- **版本堆疊（`Ctrl+G`）**：將同一張海報的 `v1 / v2 / v3 / final` 或角色多角度視圖折疊為單張卡片，保持資料庫乾淨清爽；也支援透過 `Ctrl+T` 一鍵 **自動堆疊相似版本**，點開即可並排檢視或在預覽頁即時 A/B 切換版本。

### 設計師原生格式預覽
免切換外部軟體，直接在 Raglex 內嵌大圖視窗檢視創作原生格式：
- **字體（`.ttf` / `.otf`）**：支援即時 **自訂預覽文字**、`24pt / 36pt / 48pt / 72pt` 字級切換、完整大小寫與數字字樣瀑布流，並於列表顯示該字體實際渲染的 `Aa` 縮圖。
- **音訊（`.wav` / `.mp3` / `.flac` / `.ogg` / `.m4a` / `.aac`）**：96 條 **互動式波形條** 點擊跳轉進度、原生直接試聽播放與音量調整，列表自動產生波形縮圖。
- **SVG 向量無損檢視（`.svg`）**：GPU 原生向量繪製，支援 **25%～800% 無損放大**，可一鍵切換 **透明棋盤格背景**。
- **PDF 多頁翻閱（`.pdf`）**：支援多頁翻閱、頁碼顯示、縮放與首頁縮圖生成。
- **圖片與影片**：無彈窗內嵌全舞台檢視、動圖 GIF 與影片循環播放，右側同步提供 **資訊 / 相似素材** 檢視面板。

### 無限白板與置頂參考浮窗
- **獨立白板與專案白板**：除了專案內白板，也可在側邊欄直接建立白板，自由拖放素材、分組框、向量箭頭、便利貼、縮放與旋轉。
- **置頂參考浮窗（Always on Top）**：一鍵將目前白板彈出成無邊框置頂浮窗，懸浮於各類繪圖、3D 與影音創作軟體上方；支援自動隱藏控制列、**獨立調整背景與圖片透明度**、獨立拖曳平移縮放與按 `F` 置中符合畫面。

### 批次檔案管理與高效率操作
- **批次重新命名（`F2`）與復原（`Ctrl+Z`）**：支援範本變數（`{name}`、`{n}`、`{date}`、`{ext}`、尋找取代）、衝突檢查、即時預覽與完整批次還原紀錄。
- **批次匯出與系統剪貼簿**：支援一次匯出多檔、直接 `Ctrl+C` 複製系統實體檔案、`Ctrl+V` 貼上系統檔案或剪貼簿截圖進資料庫。
- **快速跳轉與指令面板（`Ctrl+T`）**：一鍵模糊搜尋標籤、專案、白板與常用動作。

### 本地 AI、以圖搜圖與開放生態
- **內建離線視覺搜尋**：純 Go 128 維視覺向量 Embedding、64-bit pHash/dHash 感知相似比對與 5 色 OKLab 色彩搜尋，無需連網即可使用。
- **App 內一鍵部署 Laya / Ollaya**：在 **偏好設定 → AI 與模型** 即可免終端機一鍵下載、背景啟動與管理本地開源 Laya 決策模型（`laya:multilingual`、`laya`、`winnow:e4b`）。
- **內建分享伺服器與 VPS CLI（`raglex-server`）**：在 App 內一鍵啟動區域網路網頁藝廊與互動白板分享，亦提供獨立命令列版本方便部署至 VPS。
- **MCP Server（`raglex-mcp`）**：透過 Stdio 或 HTTP（`/mcp`）讓 AI 助手直接查詢、標記與搜尋你的 Raglex 素材庫。
- **瀏覽器配套擴充套件（`extensions/browser/`）**：支援拖拉存圖浮動膠囊、右鍵抓圖、網頁截圖與整頁批次擷取。
- **插件系統與 GitHub 自動更新**：支援任意語言撰寫本機插件（[`examples/plugins/`](examples/plugins/)），內建 GitHub Releases 自動更新與 SHA-256 完整性驗證。

---

## 常用快捷鍵

| 快捷鍵 | 功能說明 |
| :--- | :--- |
| `Ctrl+T` / `Cmd+T` | 開啟快速跳轉與操作面板 |
| `Ctrl+F` / `Cmd+F` | 聚焦搜尋列 |
| `Ctrl+I` / `Cmd+I` | 匯入檔案 |
| `Ctrl+A` / `Cmd+A` | 全選目前檢視素材 |
| `Ctrl+G` / `Cmd+G` | 將選取素材合併為版本堆疊 |
| `F2` | 批次重新命名選取項目 |
| `Ctrl+Z` / `Cmd+Z` | 復原上一次批次重新命名 |
| `Ctrl+C` / `Ctrl+V` | 複製實體檔案 / 貼上檔案或剪貼簿圖片 |
| `Space` | 開啟 / 關閉內嵌大圖預覽 |
| `0` – `5` | 設定星級評分 |
| `F` *（於置頂浮窗內）* | 畫面自動置中符合所有參考圖 |
| `Esc` | 關閉預覽 / 退回堆疊 / 關閉彈窗 |

---

## 支援平台與發行檔

| 作業系統 | 架構 | 發行格式 |
| :--- | :--- | :--- |
| **macOS** | Apple Silicon (`darwin/arm64`) | `Raglex-darwin-arm64.zip`（`.app` 應用程式包） |
| **macOS** | Intel x86_64 (`darwin/amd64`) | `Raglex-darwin-amd64.zip`（`.app` 應用程式包） |
| **Windows** | x64 (`windows/amd64`) | `raglex-setup-0.3.2-amd64.exe`（安裝版） / `Raglex-windows-amd64.zip` |
| **Windows** | ARM64 (`windows/arm64`) | `raglex-setup-0.3.2-arm64.exe`（安裝版） / `Raglex-windows-arm64.zip` |
| **Linux** | x64 (`linux/amd64`) | `.tar.gz`、`.deb`、`.rpm`、`PKGBUILD` |
| **Linux** | ARM64 (`linux/arm64`) | `.tar.gz`、`.deb`、`.rpm`、`PKGBUILD` |

請前往 [GitHub Releases](../../releases) 下載對應平台版本。

---

## 從原始碼建置

需安裝 **Go 1.24+**（全程純 Go、`CGO_ENABLED=0`，可在任一作業系統直接跨平台編譯全部桌面版本）。

```bash
# 直接啟動桌面開發版
go run ./cmd/raglex

# 編譯目前平台桌面版
CGO_ENABLED=0 go tool mygo build -skip-installer

# 一次跨平台編譯 macOS (Silicon & Intel)、Windows (x64 & ARM64) 與 Linux (x64 & ARM64)
CGO_ENABLED=0 go tool mygo build -skip-installer -skip-dmg -skip-notarize \
  -platform "darwin/arm64,darwin/amd64,windows/amd64,windows/arm64,linux/amd64,linux/arm64"

# 編譯獨立伺服器 CLI 與 MCP Server
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o raglex-server ./cmd/raglex-server
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o raglex-mcp ./cmd/raglex-mcp

# 執行靜態檢查與完整測試
go tool mygo vet .
CGO_ENABLED=0 go test -count=1 ./...
```

---

## 瀏覽器配套擴充套件

- **Chrome / Edge / Brave**：開啟 `chrome://extensions` → 開啟「開發人員模式」→ 點選「載入未封裝項目」→ 選擇 [`extensions/browser/`](extensions/browser/)。
- **Firefox / Zen Browser / Floorp / LibreWolf**：開啟 `about:debugging#/runtime/this-firefox` → 點選「載入暫時附加元件…」→ 選擇 [`extensions/firefox/manifest.json`](extensions/firefox/manifest.json)（永久 `.xpi` 安裝與網站權限設定請見完整教學：[extensions/firefox/README.md](extensions/firefox/README.md)）。

---

## 參與貢獻與授權條款

- 歡迎任何形式的 Issue 回報與 Pull Request，提交前請先閱讀 [CONTRIBUTING.md](CONTRIBUTING.md)。
- 本專案採用 **[Mozilla Public License 2.0 (MPL-2.0)](LICENSE)** 開源授權。你可以自由使用、修改與散佈本軟體；若修改既有受 MPL-2.0 保護之原始碼檔案，該檔案的修改內容須維持以 MPL-2.0 條款公開。
