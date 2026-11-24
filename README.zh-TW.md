<p align="center">
  <img src="docs/screenshots/cover.png" alt="Raglex — Organize references. Build moodboards. Stay local." width="100%" />
</p>

<p align="center">
  <a href="README.md">English</a> &nbsp;·&nbsp;
  <a href="README.zh-TW.md"><strong>繁體中文</strong></a> &nbsp;·&nbsp;
  <a href="README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://shieldcn.dev/github/license/Batkixni/Raglex.svg?variant=secondary" alt="License: MPL 2.0" /></a>
  <a href="https://github.com/Batkixni/Raglex/stargazers"><img src="https://shieldcn.dev/github/stars/Batkixni/Raglex.svg?variant=secondary" alt="GitHub Stars" /></a>
  <img src="https://shieldcn.dev/badge/Version-v0.4.0-FF5C33.svg" alt="v0.4.0" />
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

**Raglex** 是一款原生桌面視覺素材管理與無限靈感白板工具——**原生極速啟動**、**完全離線可用**，所有索引、資料夾、標籤與白板都儲存在你本機的單一 `bbolt` 資料庫中，檔案主權 100% 歸你所有。

採用 **Go** 與 **[MyGo Native UI](https://github.com/egoist/mygo)**（`CGO_ENABLED=0`）打造，跨平台支援 **macOS（Apple Silicon & Intel）**、**Windows（x64 & ARM64）** 與 **Linux（x64 & ARM64）**，並原生內建 **繁體中文**、**English** 與 **日本語** 完整介面。

---

## 核心功能

### 平行階層資料夾、標籤與版本堆疊
- **資料夾與標籤完全平行**：左下角一鍵平滑切換「資料夾｜標籤」樹狀檢視，支援多層子資料夾、色彩繼承、拖曳排序與歸類、批次刪除資料夾（可選連同子資料夾移除），匯入本地資料夾時自動完整鏡像階層。
- **四種彈性匯入模式**：支援 **索引（原地不動）**、**軟連結**、**移動**、**複製**，內建檔案變更即時監控、自動備份還原（`raglex.db.bak`），並可於偏好設定自訂左側欄頂部快速篩選按鈕。
- **三種質感排版**：一鍵切換 **Bento 拼接**、**瀑布流 Masonry** 與 **等比格狀 Grid**，搭配滑順縮放、評分篩選與滑鼠拖拉框選多選。
- **版本堆疊（`Ctrl+G`）**：將 `v1 / v2 / final` 或角色多角度視圖折疊為單張卡片，支援 `Ctrl+T` 一鍵自動堆疊相似版本。

### Raglex Cloud、選擇性同步與好友協作空間（v0.4.0）
- **20 GB 雲端儲存空間與高速雙向同步**：支援登入 **Raglex Cloud**（亦可隨時切換至自架 **Self-Host** Docker 伺服器），內建 6-Worker HTTP/2 並發上傳、側邊欄即時進度條與 20 GB 空間用量顯示。
- **選擇性資料夾同步**：在 `偏好設定 → 同步與協作` 逐一勾選哪些資料夾要同步上雲，排除不需要同步的資料夾與子資料夾，輕鬆省下雲端配額。
- **好友協作空間（`CLB-XXXX-XXXX`）**：將參考資料夾或專案建立為好友協作空間，大家共同維護參考素材；**白板採個人擁有制**，每個人只能修改自己的白板、可瀏覽對方白板，零衝突免煩惱。
- **公開分享碼（`SHR-XXXX-XXXX`）與跨網直傳**：發佈資料夾或專案的公開分享碼，**任何用戶免註冊免登入**輸入代碼即可下載，亦支援同 Wi-Fi 與 6 碼端對端加密直傳。

### 設計師原生格式預覽與極簡無框 Markdown 寫作桌
免切換外部軟體，直接在 Raglex 檢視與編輯創作格式：
- **無框 Markdown 閱讀與寫作桌**：支援閱讀／分欄／編輯三模式、段落專注模式、YAML Frontmatter、表格、可點擊待辦勾選框、GitHub Alerts 與 `Ctrl+S` 直接存檔。
- **字體（`.ttf` / `.otf`）**：即時自訂預覽文字、`24pt / 36pt / 48pt / 72pt` 字級切換與 `Aa` 縮圖。
- **音訊（`.wav` / `.mp3` / `.flac` / `.ogg` / `.m4a` / `.aac`）**：96 條互動式波形條、直接播放與音量調整。
- **SVG 無損向量（`.svg`）與 PDF 多頁翻閱（`.pdf`）**：GPU 向量無損縮放（25%～800%）＋透明棋盤格背景，支援多頁 PDF 瀏覽。
- **圖片、RAW 與影片**：全舞台無彈窗預覽、GIF 與內建 `ffmpeg` 影片播放，右側即時顯示 **資訊 / 相似素材**。

### 無限白板與置頂參考浮窗
- **獨立白板與專案白板**：可在側邊欄或專案內建立無限畫布，支援圖片排板、分區框、向量箭頭、畫筆／螢光筆、便利貼與縮放旋轉。
- **置頂參考浮窗（Always on Top）**：一鍵將白板彈出成無邊框置頂浮窗，支援獨立調整背景／圖片透明度、點擊穿透鎖定、水平翻轉、黑白對照與按 `F` 置中符合畫面。

### 本地 AI、Eagle Import、MCP 與開放生態
- **內建離線視覺搜尋**：純 Go 128 維視覺向量 Embedding、pHash/dHash 感知相似比對與 5 色 OKLab 主色搜尋，100% 離線可用。
- **App 內一鍵部署 Laya / Ollaya**：在 `偏好設定 → AI 與模型` 免終端機部署本地 Laya 自動標籤模型。
- **官方 Eagle Import 插件**：從 `+ 匯入 → 從 Eagle 匯入` 一鍵匯入 `.library` 資料夾與 `.eaglepack` 封包，完整還原巢狀資料夾、標籤與網頁書籤縮圖。
- **內建伺服器（`raglex-server`）與 MCP Server（`raglex-mcp`）**：提供區網網頁藝廊、手機版 `/mobile` PWA 與 17 項 AI 助手 MCP 工具。
- **瀏覽器配套擴充套件**：支援 Chrome / Edge / Firefox / Zen，搭配離線收件資料夾，未開軟體也能存圖。

---

## 常用快捷鍵

所有快捷鍵皆可於 `偏好設定 → 快捷鍵` 自由修改（自動對應 Windows `Ctrl` 與 macOS `⌘`）。

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
| `Space` | Quick Look / 開啟完整內嵌預覽 |
| `Ctrl+Alt+C` | 切換專注模式 |
| `Ctrl+Alt+G` | 切換黑白預覽對照 |
| `0` – `5` | 設定星級評分 |
| `F` *（於置頂浮窗內）* | 畫面自動置中符合所有參考圖 |
| `Esc` | 關閉預覽 / 退回堆疊 / 關閉彈窗 |

---

## 支援平台與發行檔

| 作業系統 | 架構 | 發行格式 |
| :--- | :--- | :--- |
| **macOS** | Apple Silicon (`darwin/arm64`) | `Raglex-darwin-arm64.zip`（`.app` 應用程式包） |
| **macOS** | Intel x86_64 (`darwin/amd64`) | `Raglex-darwin-amd64.zip`（`.app` 應用程式包） |
| **Windows** | x64 (`windows/amd64`) | `raglex-setup-0.4.0-amd64.exe`（安裝版） / `Raglex-windows-amd64.zip` |
| **Windows** | ARM64 (`windows/arm64`) | `raglex-setup-0.4.0-arm64.exe`（安裝版） / `Raglex-windows-arm64.zip` |
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

## 參與貢獻與授權條款

- 歡迎任何形式的 Issue 回報與 Pull Request，提交前請先閱讀 [CONTRIBUTING.md](CONTRIBUTING.md) 與 [AGENT.md](AGENT.md)。
- 本專案採用 **[Mozilla Public License 2.0 (MPL-2.0)](LICENSE)** 開源授權。
