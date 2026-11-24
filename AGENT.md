# Raglex `AGENT.md` — 工程規範、架構基準與不可違背守則

> **本文件為 AI Agent 與開發者操作 Raglex 程式碼庫的最高行為準則（Source of Truth）。**
> 每次進行 UI 變更、資料層擴充、跨平台調整、雲端同步或發佈前，必須先對照本文件與 [DESIGN.md](./DESIGN.md)，**嚴禁規範跑偏**。

---

## 一、不可違背的鐵律（Hard Rules）

### 1. 品牌、文案與競品隔離
- **產品名稱**：一律為 **Raglex**。
- **三語本地化標準用語（`zh-Hant` / `en` / `ja`）**：
  - 繁體中文資產庫一律稱為 **「資料庫」**（**嚴禁**使用「母庫」）；畫布一律稱為 **「白板」**（**嚴禁**使用「白版」）。
  - 所有使用者介面文案必須同步維護繁體中文（`zh-Hant`）、英文（`en`）與日文（`ja`）於 `internal/i18n/i18n.go`，並通過 `i18n.Parity()` 與 `TestFormatVerbParity`（嚴禁殘留未翻譯硬編碼字串）。
- **品牌隔離原則**：Raglex 核心介面、空白提示語、一般功能文案與文件**嚴禁**夾帶第三方競品軟體名稱；唯一經使用者明確授權的例外為官方 **`Eagle Import`** 插件（`raglex.library-migrator`）與其在 `+ 匯入` 選單中的單一整合入口 **`從 Eagle 匯入`**（子選單提供 `.library` 與 `.eaglepack`）。

### 2. 跨平台視窗框（Chrome）嚴格隔離
- **macOS 原生視窗框**必須嚴格封裝於 `useMacChrome = runtime.GOOS == "darwin"` 之下：
  - 左側邊欄頂部：左上原生紅黃綠交通燈留白、右上帳號／設定／說明按鈕。
  - 左側邊欄底部：左下 Raglex Logo、右下收合按鈕。
  - 主畫面右上角保持純淨空白。
- **Windows / Linux 視窗框絕對不可被 macOS 改動影響**：
  - 左側邊欄頂部：左上 Raglex Logo + 標題。
  - 左側邊欄底部：左下帳號／設定／說明按鈕、右下收合按鈕。
  - 主畫面右上角：自訂無邊框極簡單色 `— □ ×` 視窗控制鍵（支援 Windows 11 Snap Layouts）。

### 3. 視覺與資訊架構紀律（搭配 [DESIGN.md](./DESIGN.md)）
- **側邊欄固定結構（嚴禁任意改動既有排序或擅自發明多餘篩選項目）**：
  1. 頂部標題列（依平台規範）
  2. 統一搜尋欄 ＋ `Ctrl+T` 快速跳轉按鈕
  3. 可捲動導覽區（維持原版純淨結構）：
     - 原有快速篩選：`全部素材`（**固定顯示不可關閉**）、`未分類`、`近期新增`、`隨機探索`、（有遺失時顯示 `遺失檔案`）、`標籤管理`（除 `全部素材` 外皆可於「偏好設定 → 一般 → 側邊欄頂部按鈕」個別開關；**禁止擅自新增「未歸檔」等多餘項目**）
     - `白板` 區塊 ＋ `+` 按鈕
     - `專案` 區塊 ＋ `+` 按鈕
     - **平行分類區塊（資料夾與標籤完全同級）**：依目前模式顯示 `資料夾` 或 `標籤` 區段標頭＋右側批次刪除與 `+` 按鈕；切換時具滑動淡入動畫，空白時不塞多餘說明段落文字
  4. **即時雲端同步進度條（`sidebarSyncBar`）**：僅在 `syncBusy == true` 時浮現，平常零佔位
  5. **固定置底的 `資料夾｜標籤` 切換器**：固定在左下角底部列正上方（Windows/Linux 位於帳號/設定列正上方，macOS 位於左下 Logo 列正上方），採用 `attachGlidingPill` 平滑膠囊動畫
  6. 底部列（依平台規範）
- **偏好設定與彈窗風格**：全面採用**扁平細分隔線（Flat Divider-Based）**設計，避免層層疊疊的厚重巢狀外框卡片。
- **克制使用強調色**：品牌橘紅 `#FF5C33` 僅用於主行動按鈕、焦點進度、開啟開關、星等與關鍵狀態點，絕不混入冷藍高光或花俏漸層。
- **右側 Inspector 保持精簡**：無資料夾時不顯示空的「所屬資料夾」佔位區塊；所有文字按鈕皆加 `SingleLine()` 防止窄寬折行變形。
- **動態物理**：所有彈簧與轉場採用臨界阻尼（`Bounce: 0`），果斷不彈跳。

### 4. MyGo (`v0.10.0`) 生命週期與執行緒紀律
詳見 `.agents/skills/mygo-maintenance/SKILL.md`：
- **絕不跨 frame 或跨 goroutine 儲存 `ui.Element` 或 `*ui.Context`**；需長期持有的系統服務改取 `c.Services()`。
- **UI 執行緒零阻塞**：所有磁碟 I/O、縮圖解碼、匯入、雲端同步與網路請求一律放進背景 goroutine，完成或進度更新時透過 `win.Update(...)`（並檢查 `!a.isClosed()`）寫回狀態。
- **單一流程安全開檔**：`sync.db` 必須統一走 `internal/sync/store.go` 的行程內參考計數池（`openSyncDBs`），避免內建伺服器與雲端同步同時開啟造成 bbolt `flock` timeout。
- **全域快捷鍵**：一律在 `View(c)` 根作用域透過 `registerWindowShortcuts` 註冊，支援使用者於「偏好設定 → 快捷鍵」自訂並自動映射 macOS `⌘/⌥/⌃/⇧`。

### 5. 資料庫向後相容與安全性
- **零破壞資料庫升級**：新增資料表優先採**純增補 bbolt Bucket**（`CreateBucketIfNotExists`）並維持 `SchemaVersion = "1"`，確保舊版 `raglex.db` 無痛升級、零資料遺失。
- **單一交易一致性**：主紀錄與所有反向索引（如 `assets` + `asset_folders` + `folder_assets`）必須在同一個 `db.Update` 交易內原子寫入；刪除前自動觸發備份。
- **機密絕對不上版**：`.env`、OpenMail API Key、`RAGLEX_CLOUD_ADMIN_TOKEN`、私鑰等僅存放於伺服器本機（`chmod 600`），**嚴禁**寫入程式碼或提交至 Git。

---

## 二、目前系統架構全貌（v0.4.0 基準）

Raglex 是以 **Go 1.27 + MyGo Native UI (`v0.10.0`) + bbolt** 打造的本地優先創作者數位資產管理、檔案工作流與靈感白板工具。

### 1. 六大正交資料層
1. **資料庫素材（Assets）**：ULID + SHA-256 去重，支援 `index` / `symlink` / `move` / `copy` 四種存放策略；支援專業相機 RAW（`NAF`/`RAF`/`CR3`/`ARW`/`NEF` 等）、SVG、字體、音訊波形、PDF、影片（內建靜態 `ffmpeg`）與版本堆疊（Stacks）。
2. **平行階層「資料夾（Folders）」**：獨立 bucket（`folders` / `asset_folders` / `folder_assets`），與標籤完全平行同級，支援多層巢狀子資料夾、8 色繼承、拖曳排序、拖曳歸類、遞迴篩選；匯入本地資料夾（含拖放）與外部資料庫／素材封包檔時自動完整鏡像還原階層。
3. **平行階層「標籤（Tags）」**：獨立 bucket（`tags` / `asset_tags` / `tag_assets`），支援母/子標籤、8 色繼承與獨立標籤管理器，與資料夾完全正交並存。
4. **專案（Projects）與白板（Boards）**：專案透過引用關聯素材（刪專案不刪原檔）；白板支援「專案白板」與「獨立白板（Standalone Boards）」，並附置頂參考浮窗（Always-on-Top）。
5. **視覺搜尋與本機 AI**：內建 OKLab 主色盤、pHash/dHash 以圖搜圖、可選本地 Embedding 與 Laya / Ollaya 離線分類器。
6. **分享、雲端與擴充生態**：
   - **區域網 & 跨網直傳（`internal/share`）**：同時支援「專案」與「資料夾」透過同 Wi-Fi 或 6 碼 E2E WebRTC/Relay 傳送。
   - **Raglex Cloud（`internal/cloud` + `cmd/raglex-cloud`）**：部署於 `https://cloud.raglex.sorai.tw`（Hetzner + 1TB Storage Box + OpenMail 驗證信/重設密碼 + 邀請碼註冊制 + 每帳號 **20 GB** 雲端配額顯示）；提供 **6-Worker 並發 HTTP/2 雙向同步**、**選擇性資料夾同步**（`偏好設定 → 同步與協作`，未登入或未連 Self-Host 時鎖定）、**好友協作空間（`CLB-XXXX-XXXX`）**（共享參考資料夾與專案，**白板嚴格採個人擁有制，不可即時協作、不可覆蓋他人白板，僅能編輯自己的白板與瀏覽他人白板**）與 **公開分享碼（`SHR-XXXX-XXXX`）**（任何用戶免註冊免登入輸入代碼即可下載，嚴格限定 `AllowedHashes` 白名單存取）。
   - **無框 Markdown 寫作桌**：透過官方 `raglex.markdown-notes` 插件提供無外框卡片的閱讀／分欄／編輯三模式、段落專注、表格、可點擊任務勾選框與 `Ctrl+S` 原地存檔。
   - **內建伺服器 / MCP / 瀏覽器擴充 / 插件系統**：`internal/server`、`internal/mcp`、`extensions/browser`、`internal/plugin`（含官方 `Eagle Import` 與重複素材掃描）。

---

## 三、核心目錄對照

```text
raglex/
├── AGENT.md                  # 本規範書（最高工程守則與架構基準）
├── DESIGN.md                 # 視覺系統、色彩、圓角、動態與介面標準
├── Logo.png                  # 512x512 深黑灰底＋#FF5C33 圖騰 App Icon
├── cmd/
│   ├── raglex/               # 桌面原生主程式（MyGo GUI）
│   ├── raglex-cloud/         # 多租戶 Raglex Cloud 伺服器（Auth / Sync / Public Share / Signal）
│   ├── raglex-server/        # 獨立自架無頭伺服器 CLI
│   ├── raglex-mcp/           # 獨立 Stdio MCP Server
│   └── raglex-signal/        # 輕量配對中繼
├── internal/
│   ├── db/                   # bbolt 儲存層（asset / tag / folder / project / board）
│   ├── model/                # 核心模型與 msgpack/JSON 結構
│   ├── library/              # 匯入、資料夾樹、標籤樹、外部資料庫/封包遷移、批次改名/轉檔
│   ├── project/              # 專案、專案白板與獨立白板
│   ├── canvas/               # 白板幾何與渲染狀態
│   ├── share/                # 專案與資料夾打包（Bundle）、LAN 與 WebRTC/Relay 傳輸
│   ├── sync/                 # 6-Worker HTTP/2 並發雙向同步、公開分享發佈/免登入下載、sync.db 鎖池
│   ├── cloud/                # Raglex Cloud 多租戶 API、邀請碼、OpenMail 寄信、Public Shares
│   ├── preview/ & thumbnail/ # RAW / 創意格式 / 音訊 / 字體 / PDF / 影片縮圖與預覽
│   ├── server/ & mcp/ & plugin/ & ai/ & watcher/ & config/ & i18n/ & updater/
└── ui/                       # MyGo Native UI（shell / library_view / folders / tags_view / share / settings / theme）
```

---

## 四、每次提交與發佈前的必要驗證清單

1. **靜態與生命週期檢查**：`CGO_ENABLED=0 go tool mygo vet .`（必須 0 警告）。
2. **全量單元與 UI 互動測試**：`CGO_ENABLED=0 go test -count=1 ./...`（包含 `i18n.Parity()`、bbolt/library/sync/cloud 與 `ui.NewTester` 互動測試）。
3. **跨平台編譯確認**：
   - Windows 測試版 GUI：編譯至**新的版本化目錄**（例如 `go tool mygo build -skip-installer -o build/v040-preview3 -platform windows/amd64`），**嚴禁覆蓋使用者既有正在使用的編譯目錄或強制終止運行中的 Raglex**。
   - macOS ARM64：`GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o NUL ./cmd/raglex`
   - Linux Cloud 伺服器：`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o NUL ./cmd/raglex-cloud`
4. **伺服器熱更新原則**：若更動 `internal/cloud`，必須同步編譯靜態 Linux 執行檔並以原子替換方式部署至 Hetzner `/opt/raglex-cloud/bin/raglex-cloud`，驗證 `/healthz` 與新端點正常回應。
5. **交付前自檢**：確認無殘留暫存檔、無洩漏密鑰、無競品字樣、無誤改他平台視窗框，再回報使用者。
