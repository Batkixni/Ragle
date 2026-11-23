# Raglex `AGENT.md` — 產品規格、架構基準與演進路線圖

本文件為 **Raglex** 的唯一真實規格（Source of Truth）與工程實作基準。
Raglex 是以 **Go + MyGo Native UI** 開發的開源、本地優先（Local-First）創作者數位資產管理、檔案工作流與靈感白板工具，核心資料層採用 **bbolt**。
視覺與互動規範請搭配閱讀 [DESIGN.md](./DESIGN.md)。

---

## 一、專案目的與核心原則

Raglex 要同時解決創作者的三個痛點：**檔案怎麼安全管理**、**靈感怎麼跨專案重複引用**、以及**如何無痛串接現代 AI 與跨裝置分享**。

### 核心原則
1. **本地優先，離線完整可用**：核心圖庫、檔案管理、標籤、專案、白板與純演算法搜圖（色系 / 感知雜湊）不依賴雲端服務。
2. **檔案主權歸使用者**：匯入時由使用者選擇存放策略（`index` / `symlink` / `move` / `copy`），軟體不強制鎖檔或改寫原檔名；批量改名等磁碟操作一律提供即時預覽與 Undo。
3. **引用與實體分離**：素材實體只存在母庫；專案與白板僅持有引用（Reference / Placement）。刪除專案或白板絕不刪除母庫檔案。
4. **原生輕量，能力靠擴充**：主程式維持單一高速原生執行檔；重型格式預覽、進階工作流與 AI 模型透過統一 Local API 與插件系統熱插拔。
5. **開源可自架**：分享、瀏覽器擴充、MCP 與伺服器協議全公開；App 內建一鍵開服，同時提供無 GUI 的獨立 CLI 版本給 VPS 長駐部署。

---

## 二、核心資料層級

Raglex 的概念層級為五個正交模組：

1. **母庫（Assets）**：所有素材的索引或實體檔案，採 ULID 識別碼與 SHA-256 去重；附帶縮圖、尺寸/時長、主色盤（Palette）、感知雜湊（pHash/dHash）、可選向量嵌入（Embedding）與來源中繼資料（URL / 頁面標題 / 備註）。
2. **階層標籤（Tags）**：支援多層母標籤 / 子標籤樹狀結構與自訂顏色（設定可切換彩色或純淨模式），同時用於分類素材與組織專案。
3. **專案（Projects）**：代表具體工作上下文，可隸屬於多個標籤；透過 `project_assets` 引用母庫素材，並可包含多個白板。
4. **白板（Boards）與放置（Placements）**：
   - **專案白板**：隸屬於特定專案（`project_id != ""`）。
   - **獨立白板（Standalone Boards，v2 新增）**：不屬於任何專案（`project_id == ""`），可直接在側邊欄建立與開啟，並支援隨時移入專案或升級為新專案。
   - 白板上的每個元素為一筆 Placement（圖片、文字、群組），記錄引用、位置、縮放、旋轉、圖層與註記。
5. **擴充與連接層（Server / Plugin / Browser / MCP / AI）**：共用同一套核心服務層，對外提供 Local Companion API、Web 分享檢視、插件系統、MCP Server 與 Laya / Ollaya + Embedding 離線開源智慧引擎。

---

## 三、已完成實作現況（v0.1 — 階段 1 至 10）

以下能力均已實作、通過 `mygo vet` 與自動化測試，為目前 `main` 分支基準：

### 1. 資料層與核心引擎（`internal/db`, `model`, `library`, `project`, `canvas`, `thumbnail`, `watcher`, `config`, `i18n`）
- **bbolt 單檔交易儲存**：Schema v1，全索引採 `parentID/childID` 前綴掃描（`assets`, `assets_by_hash`, `assets_by_path`, `tags`, `asset_tags`, `tag_assets`, `projects`, `project_tags`, `tag_projects`, `project_assets`, `asset_projects`, `boards`, `boards_by_project`, `placements`, `placements_by_board`, `placements_by_asset`）。
- **階層標籤**：`Tag` 支援 `ParentID`、`Color`，自動偵測並避免循環父子關係，支援樹狀展開/收合與繼承篩選。
- **四種存放策略**：`index`、`symlink`、`move`、`copy`；啟動與刪除前自動透過 `tx.WriteTo` 寫出 `raglex.db.bak`，損毀時可於歡迎頁一鍵還原。
- **檔案監聽與重連**：`fsnotify` 追蹤移動與遺失檔案（標記 `missing`），支援以 SHA-256 自動重新連結。
- **縮圖與影片播放**：非同步雙 worker 生成縮圖；支援 `ffmpeg` 影片抽幀與應用內影片 / GIF 預覽播放（`ui/video.go`）。

### 2. 分享基礎（`internal/share`, `cmd/raglex-signal`）
- **區域網分享**：UDP 廣播（port `47321`，`RGL1`）自動發現 ＋ TCP 傳送專案副本與素材實體（接收確認 ＋ SHA-256 去重）。
- **跨網配對碼 E2E 分享**：基於 `pion/webrtc/v4` DTLS 加密 DataChannel 與輕量 HTTP 長輪詢配對中繼（6 碼 `XXX-XXX` 配對碼，32 KiB 分塊與背壓）。

### 3. 介面與體驗（`ui/`，目前 MyGo `v0.7.4`）
- **無邊框整合標題列**：自訂 `— □ ×`（支援 Windows 11 Snap Layouts）與可拖曳標題區，零系統原生白條。
- **視覺主題**：沉穩黑灰無邊框表層（深色 `#111111` / `#18181B`、淺色 `#FFFFFF` / `#F4F4F5`），預設品牌強調色 **`#FF5C33`**；支援設定頁自訂主題預設與四色覆寫。
- **品牌識別**：Windows 執行檔與歡迎頁使用深黑灰底＋`#FF5C33` 中央圖騰的圓角方形 Icon（[Logo.png](./Logo.png) / [ui/app_icon.png](./ui/app_icon.png)）；App 左上角使用原版滿版純色黑白標誌（[ui/logo.png](./ui/logo.png)）。
- **多排版與高效操作**：支援 **Bento / 瀑布流（Masonry）/ 網格（Grid）** 切換、滑鼠左鍵拖拉圈選（Marquee Selection）、`Ctrl`/`Shift` 多選與批次工具列、全域快捷鍵（`Ctrl+A` / `Ctrl+B` / `Ctrl+F` / `Ctrl+I` / `Ctrl+,` / `Space` / `Delete` / `0-5` / 方向鍵）。
- **側邊欄與專案頁**：可收合側邊欄（`Ctrl+B` 與防擠壓寬度下限 `204–340px`）、窄視窗自適應頂部工具列、明確的向量展開箭頭與連續白色樹狀導引線、彈窗式標籤管理器、雙層式專案 Hero、白板即時縮圖預覽卡片、底部自動消失懸浮 Toast、五步驟首次啟動互動教學。

---

## 四、v2 演進規劃與完整功能規格

> 所有新階段維持原則：**每一階段結束皆可離線獨立運作、通過 `mygo vet` 與單元/畫面測試、不破壞既有 `bbolt` 資料相容性（Schema 升級自動遷移）**。

### 階段 11：MyGo 框架升級（`v0.7.4` → `v0.10.0`）
- 檢查 `github.com/egoist/mygo` 自 `v0.8.0` 至 `v0.10.0` 的變更紀錄與新 API（特別評估：原生剪貼簿圖片讀寫、跨 App 檔案拖出、系統托盤、視窗置頂、Embedded WebView / 新元件）。
- 依 `.agents/skills/mygo-maintenance/SKILL.md` 完成升級、`mygo vet .` 驗證與全量回歸測試。

### 階段 12：檔案管理器能力（Batch File Manager）
將 Raglex 升級為創作者日常主力檔案管理工具：
1. **批量重新命名引擎（`internal/library/rename.go`）**
   - 規則管線可任意組合：
     - **模板命名**：`{name}`、`{ext}`、`{index:03}`、`{date:YYYYMMDD}`、`{w}x{h}`、`{tag}`、`{project}`、`{rating}`
     - **尋找與取代**：支援純文字、忽略大小寫、正規表達式（Regex）
     - **格式整理**：前綴 / 後綴、大寫 / 小寫 / Title Case、空白轉底線或連字號
   - **安全機制**：即時預覽對照表（偵測撞名、非法字元、路徑過長）；區分「庫內顯示名」與「磁碟檔名」；`index` / `symlink` 外部檔案預設提示並可切換是否連動原檔；操作寫入 `rename_history` 支援**一鍵批次復原（Undo）**。
2. **批量轉檔與壓縮匯出（`internal/library/export.go`）**
   - 支援圖片批次轉出為 `PNG` / `JPEG` / `WebP`（品質滑條、長邊限制、是否保留 EXIF），影片透過 `ffmpeg` 支援裁切/壓縮匯出。
3. **剪貼簿貼圖入庫 & 跨軟體拖出**
   - `Ctrl+V` 直接將系統剪貼簿圖片 / 截圖落地存檔入庫（或直接貼上當前白板）。
   - 支援把選中的一或多個素材複製為實體檔案清單至系統剪貼簿，並支援直接拖曳至各類外部創作與剪輯軟體。
4. **自動監控資料夾（Watch Folders）**
   - 每個資料庫可設定多個外部監控資料夾（指定預設存放策略與自動套用標籤），透過 `fsnotify` 即時同步新增、搬移與遺失重連。

### 階段 13：白板與置頂參考浮窗
1. **資料模型擴充（向後相容）**
   - `Board.ProjectID == ""` 視為獨立白板；新增索引 bucket `boards_standalone`（key: `boardID`）。
   - 側邊欄新增「**白板**」專屬區塊（支援 `+` 快速建立、右鍵改名 / 複製 / 移入專案 / 轉為新專案 / 刪除）。
2. **跨畫面無縫串聯**
   - 資料庫多選素材 → 右鍵「加入至白板（新建或現有）」。
   - 獨立白板頁眉提供一鍵「歸類到專案」或「升級成專案」。
3. **進階白板生產力**
   - 區域分組框（Section / Frame）、自動對齊與等距排版、高解析 PNG / PDF 匯出。
   - **置頂參考浮窗模式（選配）**：視窗置頂（Always-on-Top）與極簡可調透明浮動模式，方便繪圖與 3D 製作時對照參考。

### 階段 14：以圖搜圖、主色提取與智慧篩選
採 **三層漸進架構**，第 1 層 100% 內建免 AI 零延遲，第 2–3 層可選開啟：

#### 第 1 層：純 Go 內建即時視覺搜尋（零外部依賴、零顯卡需求）
- **自動主色盤提取（Palette Extraction）**：
  - 匯入 / 生成縮圖時同步以 k-means / median-cut 在 **OKLab / CIE Lab** 空間萃取 5 個主色（含權重比例），寫入 `Asset.Palette` 與色彩索引。
  - 右側 Inspector 顯示 5 顆色票（點擊即複製 HEX）；搜尋列支援**依顏色搜圖**（選擇色盤＋相似度門檻，以 Lab Delta-E 排序）。
- **感知雜湊以圖搜圖（Perceptual Reverse Image Search）**：
  - 匯入時同步計算 **pHash（64-bit DCT）** 與 **dHash（64-bit Gradient）** 存入 `Asset.PHash` / `Asset.DHash`。
  - 支援三種入口：「右鍵素材 → 尋找相似圖片」、「把外部圖片拖入搜尋列以圖搜圖」、「重複 / 近似圖清理工具（依漢明距離門檻分群）」。
- **智慧型篩選群組（Smart Folders）**：
  - 儲存複合條件（標籤、顏色、長寬比、尺寸、評分、格式、匯入日期、有無標記）至 `smart_folders` bucket，顯示於側邊欄並即時計算符合結果。
- **版本堆疊（Asset Stacks）**：
  - 透過 `Asset.StackID` 將多張改版/同系列素材摺疊為單一卡片，點開即展開全部版本。

#### 第 2 層：本地向量語意搜尋（Semantic Image & Text Search，選配）
- **Embedding 引擎（`internal/ai/embed`）**：
  - 透過標準介面串接輕量多模態向量模型（如 **SigLIP2**、**MobileCLIP**、**Jina-CLIP**，可經本機 ONNX Runtime、Ollama 或獨立 sidecar 執行）。
  - 向量存於 `asset_vectors` bucket（搭配輕量記憶體餘弦相似度索引 / HNSW），十萬張以內毫秒級檢索。
  - 支援：**自然語言搜圖**（例：「暗紅光影賽博龐克人物海報」）與 **語意以圖搜圖**（找到構圖/氛圍/風格接近、即使像素完全不同的圖）。

#### 第 3 層：Laya + Ollaya 開源本地決策分類引擎（Open-Source Local System-1 AI，選配）
- **為何選用 Laya（搭配 Ollaya 本地執行環境）而非閉源雲端 API？**
  - Raglex 是純本地優先的開源專案，AI 分類不應綁定任何閉源付費 API。
  - **Laya**（`convaiinnovations/laya`，~421M 參數開源權重，ModernBERT-large 架構）是非自回歸決策模型，在一般 CPU / GPU 上只需約 **33ms** 即可於**單次前向推論**內對「輸入上下文＋候選選項清單 / 規則」輸出**校準後的機率分布、分數或布林判斷**，不逐字生成廢話、不亂造垃圾標籤。
  - 透過開源本地伺服器 **Ollaya**（`ollaya-dev/ollaya`，支援 `/v1/decisions` 與 `/v1/systemone` 標準端點），使用者可一鍵在本機離線執行 `laya`（文字/檔名/來源 URL/視覺描述與中繼資料決策）或相容的開源視覺決策模型（如基於 Qwen-VL 的 Vev）。
- **在 Raglex 中的四大落地場景（`internal/ai/laya`）**：
  1. **匯入自動歸類（Closed-Set Auto-Tagging）**：結合檔名、來源標題/URL、主色特徵與選配視覺摘要，將使用者母庫現有的階層標籤樹作為候選選項送入 Laya，依信心度門檻分流——高於 `0.80` 自動套用、`0.45–0.80` 列為待確認建議標籤（絕不亂建雜亂標籤）。
  2. **條件式智慧檢索與重排（Rerank & Policy Filter）**：先用第 1/2 層 pHash / 色彩 / Embeddings 粗篩出 Top-50 候選，再由 Laya 執行精準條件篩選與重排（例：「海報排版且偏暗色，排除人像攝影」）。
  3. **專案歸屬建議**：匯入新素材時，依既有專案名稱與說明評估該素材最可能屬於哪個專案。
  4. **批次整理與分類分流**：自動識別「截圖 / 攝影 / 插畫 / 3D 渲染 / UI 設計稿 / 字體排印」等維度。
- **統一開源介面抽象（Provider Agnostic）**：
  - 預設對接本機 **Ollaya**（`http://127.0.0.1:11435` 或使用者自訂位址，完全離線私密），同時相容任何 `/v1/decisions` / `/v1/systemone` 決策端點；使用者可在偏好設定頁一鍵測試連線、設定模型名稱與信心門檻。

### 階段 15：統一內建伺服器＋獨立 CLI（`internal/server` + `cmd/raglex-server`）
將原本分散的分享、配對中繼、瀏覽器串接與插件通訊收斂到同一套伺服器核心：

1. **雙形態部署（100% 共用 `internal/server`）**
   - **App 內建伺服器**：在 Raglex 視窗內一鍵啟動/關閉、自訂監聽位址（預設 `127.0.0.1:41595` 供本機插件/瀏覽器使用；可額外開啟 `0.0.0.0:<port>` 供區網/外網分享），內建 QR Code 與存取狀態監控。
   - **獨立 Headless CLI（`cmd/raglex-server`）**：無 GUI 單一執行檔（Linux / Windows / macOS / Docker），可直接部署至 VPS（配合 Caddy / Nginx 自動 HTTPS）。
2. **伺服器四大能力**
   - **Web 分享檢視器（免安裝瀏覽器直開）**：將單一專案、標籤集或白板產生唯讀 / 密碼保護 / 限時連結（`/s/<token>`），網頁端提供響應式圖庫、大圖檢視、互動縮放白板與原檔下載（可選開啟「外部上傳收集箱」與「白板審圖批註」）。
   - **內嵌配對與盲轉發中繼服務**：吸收原有 `cmd/raglex-signal`，App 內建即具備跨網配對與 E2E 盲轉發能力，VPS 版同時作為跨網穩定中繼節點。
   - **Push-to-VPS 同步**：桌面版可將指定專案或白板一鍵發佈到自己架設的 `raglex-server` VPS，即時更新公開作品集或客戶審圖頁。
   - **Local Companion REST / WebSocket API（`/api/v1/...`）**：提供給瀏覽器擴充、外部插件與自動化腳本使用（具一次性配對 Token 與權限控管）。

### 階段 16：Raglex MCP Server（`internal/mcp` + `cmd/raglex-mcp`）
讓任何支援 MCP 協定的 AI 助手直接操作你的本地 Raglex 資料庫：
- **雙傳輸模式**：
  - **Stdio 模式（`cmd/raglex-mcp --library <path>`）**：零網路開銷，直接給桌面 AI 用戶端掛載；若 Raglex 主程式正開啟同一母庫，自動轉發至 Local API 避免 `bbolt` 鎖檔衝突。
  - **Streamable HTTP / SSE 模式**：直接掛載於 `internal/server` 的 `/mcp` 端點（需 Bearer Token 驗證）。
- **核心 MCP Tools**：
  - 檢索：`search_assets`（關鍵字 / 標籤 / 顏色 HEX / 評分 / 尺寸 / 語意）、`find_similar_assets`（以圖搜圖）、`get_asset_detail`、`list_tags`、`list_projects`、`list_boards`。
  - 組織：`import_assets`、`tag_assets`、`batch_rename_assets`（支援 `dry_run` 預覽）、`create_project`、`add_assets_to_project`。
  - 白板：`create_board`、`place_assets_on_board`（支援自動網格 / 瀑布排版座標計算）、`add_board_note`。
  - 智慧：`classify_assets_with_laya`（呼叫本機 Ollaya / Laya 模型對指定素材回傳分類機率與建議標籤）。
- **核心 MCP Resources**：
  - `raglex://library/summary`（母庫統計、標籤樹、專案列表）
  - `raglex://asset/{id}`（素材完整中繼資料與本機路徑）
  - `raglex://board/{id}`（白板佈局 JSON 描述）

### 階段 17：瀏覽器配套擴充（`extensions/browser/`，Manifest V3）
支援 Chrome / Edge / Brave / Firefox，透過配對 Token 連線至 Raglex Local Companion API：
- **拖曳存圖（Drag-to-Save）**：拖曳網頁圖片即浮出快速目標選擇器（直接丟進指定標籤 / 專案 / 白板）。
- **右鍵收集與截圖**：收集單圖、影片/GIF、區域截圖、可視範圍截圖、整頁長截圖。
- **批次抓圖器（Batch Page Grabber）**：自動解析當前頁面所有高解析原圖（含尺寸過濾與勾選），一次匯入母庫。
- **自動來源綁定**：自動寫入 `source_url`、`page_title`、`domain` 至素材中繼資料。

### 階段 18：插件生態系統（`internal/plugin` + Plugin Manager UI）
不依賴 Go `.so`，採跨平台安全架構：
- **插件清單（`manifest.json`）**：宣告 ID、版本、權限（讀圖庫 / 寫標籤 / 存取磁碟 / 網路）、觸發掛載點（`context_menu`、`batch_action`、`importer`、`exporter`、`format_preview`、`ai_provider`、`toolbar`）。
- **兩種執行環境**：
  1. **WASM 沙盒插件（`wazero`）**：純 Go 零外部依賴、毫秒啟動、跨平台一致，適合命名規則、濾鏡、格式解析器。
  2. **外部行程插件（JSON-RPC over stdio / Local HTTP）**：不限開發語言（Python / Node / Go / Rust），適合串接重型工具（FFmpeg 特效、AI 去背、超解析放大、格式轉圖、本地推論服務）。
- **宣告式 UI 與管理介面**：插件透過 JSON Schema 宣告設定欄位與操作表單，由 MyGo 原生元件渲染；偏好設定內建插件管理頁（安裝 / 啟用停用 / 權限檢視 / 日誌除錯）。

---

## 五、v2 資料庫（bbolt）新增欄位與 Bucket 規劃

所有擴充保持向後相容（msgpack 欄位新增採 `omitempty`，Schema 版號推進時自動補建新 Bucket）：

- **Asset 擴充欄位**：
  - `display_name`（獨立於磁碟檔名的顯示名稱）
  - `source_url`, `source_title`, `notes`（瀏覽器擴充與手動備註）
  - `palette`（`[]PaletteColor{Hex, L, A, B, Ratio}`）
  - `phash`, `dhash`（`uint64`，毫秒級以圖搜圖）
  - `stack_id`（版本堆疊群組 ID）
- **新增 Buckets**：
  - `boards_standalone`：key `boardID`（獨立白板索引）
  - `smart_folders`：key `folderID` → 篩選條件結構
  - `watch_folders`：key `watchID` → 外部監控目錄設定
  - `rename_history`：key `batchID` → 批次改名還原紀錄
  - `asset_vectors`：key `modelID/assetID` → float32 向量位元組
  - `share_links`：key `token` → 內建 / VPS 網頁分享設定（權限、密碼雜湊、過期時間）
  - `plugins`：key `pluginID` → 啟用狀態與插件設定

---

## 六、程式碼目錄藍圖（現有＋v2 規劃）

```text
raglex/
├── AGENT.md                 # 本規格書（架構與功能路線圖）
├── DESIGN.md                # 視覺、色彩、動態與 UX 設計規範
├── Logo.png                 # 512x512 深黑灰底＋#FF5C33 圖騰 App Icon
├── cmd/
│   ├── raglex/               # 桌面原生主程式（MyGo GUI）
│   ├── raglex-server/        # [v2] 獨立無頭伺服器 CLI（VPS Host / Web Viewer / Signal）
│   ├── raglex-mcp/           # [v2] 獨立 Stdio MCP Server 入口
│   └── raglex-signal/        # [現有] 輕量配對中繼（後續併入 internal/server 並保留相容）
├── internal/
│   ├── db/                  # bbolt 開啟、自動遷移、備份與交易輔助
│   ├── model/               # 核心資料結構與 msgpack 序列化
│   ├── library/             # 母庫、匯入、階層標籤、批量改名、轉檔匯出、智慧資料夾
│   ├── project/             # 專案、專案白板與獨立白板、Placement
│   ├── canvas/              # 白板幾何、命中測試、群組與渲染狀態
│   ├── thumbnail/           # 縮圖、影片抽幀、主色盤提取、pHash/dHash 計算
│   ├── watcher/             # fsnotify 檔案遺失追蹤與監控資料夾同步
│   ├── share/               # 區域網發現與 WebRTC 配對碼 E2E
│   ├── server/              # [v2] 內建伺服器、Local API、Web Viewer、配對中繼與 Push 同步
│   ├── mcp/                 # [v2] MCP Server（Tools / Resources，共用 Stdio 與 HTTP）
│   ├── plugin/              # [v2] 插件管理器（manifest / wazero WASM / JSON-RPC）
│   ├── ai/                  # [v2] Embedding 檢索引擎與 Laya / Ollaya 開源決策分類器
│   ├── config/              # 系統設定目錄 config.json 讀寫
│   └── i18n/                # 繁體中文 / 英文語系字典
├── extensions/
│   └── browser/             # [v2] Chrome / Edge / Firefox Manifest V3 瀏覽器擴充
└── ui/                      # MyGo Native UI 視圖與主題系統
```

---

## 七、工程紀律與驗證要求

1. **MyGo 生命週期紀律**：遵循 `.agents/skills/mygo-maintenance/SKILL.md`；絕不跨 frame 或跨 goroutine 持有 `ui.Element` 或 `*ui.Context`；所有非同步回呼透過 `win.Update(...)` 回到 UI 執行緒；全域快捷鍵必須在 `View(c)` 根層級註冊。
2. **單一交易一致性**：所有涉及主資料與輔助索引的寫入，必須在同一個 `db.Update` 交易中完成；破壞性操作前一律觸發資料庫備份。
3. **驗證門檻**：每次里程碑提交前必須通過 `go tool mygo vet .`、`CGO_ENABLED=0 go test -count=1 ./...`，並針對重大 UI 改動以 `RAGLEX_SNAPSHOT_DIR` 產出截圖做視覺審查。
