\*\*Ragle Agent.md\*\*



本文件為 Ragle 的完整規格與實作指引。Ragle 是以 MyGo native UI 開發的開源本地優先素材工具，資料層使用 bbolt。母庫以標籤作為主要分類層級，專案隸屬於標籤之下，白版則隸屬於專案。



\---



\### 一、專案目的與意義



Ragle 要解決的是素材「怎麼存」與「怎麼在不同工作中重複使用」這兩件事同時發生的問題。



多數素材工具要么把檔案鎖進自己的庫裡用亂碼檔名，要么只提供一次性的白版而沒有可累積的母庫。創作者往往已經有自己的資料夾習慣，卻被迫在管理便利與檔案自由之間選擇。Ragle 讓使用者自己決定檔案進入系統的方式，同時提供一個以標籤分類、以專案組織、以白版排列的結構，讓同一批素材可以在不同專案中被引用而不產生多餘副本。



它的意義在於把控制權留在使用者手上：存放方式自由、分類方式清楚、引用關係明確，且核心功能不依賴任何外部服務。開源與可自架是為了確保即使官方不再維護，工具仍然可用。



\---



\### 二、核心邏輯與層級



Ragle 的結構分為四層：



1\. \*\*母庫（Assets）\*\*  

&#x20;  所有素材的實體或索引都在這裡。每個素材有唯一識別碼。匯入時決定存放策略（純索引、符號連結、移動、複製）。



2\. \*\*標籤（Tags）\*\*  

&#x20;  母庫的主要分類方式。標籤用來組織素材，也用來組織專案。一個素材可以有多個標籤；一個專案隸屬於一個或多個標籤。



3\. \*\*專案（Projects）\*\*  

&#x20;  專案放在標籤之下，代表一個具體的工作上下文。專案本身不持有素材檔案，只持有對母庫素材的引用。



4\. \*\*白版（Boards）與放置（Placements）\*\*  

&#x20;  每個專案可以有多個白版。白版上的每個物件是一筆 Placement，記錄「引用了哪個素材」以及「在畫布上的位置、縮放、旋轉、圖層」。



這個層級讓分類（標籤）與工作（專案）分開，同時保證素材實體只存在於母庫。刪除專案只會移除該專案下的白版與放置記錄，不會刪除母庫中的素材。



\---



\### 三、產品屬性與原則



\- 本地優先，核心功能離線完整可用。

\- 存放策略由使用者在匯入時選擇，軟體不強制複製或改名。

\- 標籤是母庫的第一層分類，專案隸屬於標籤。

\- 專案與白版只引用素材，不擁有素材。

\- 開源，同步與分享協議公開，可自架也可完全不使用。

\- 單機體驗必須先完整，分享與雲端為附加能力。



\---



\### 四、功能規格



\*\*母庫與標籤\*\*

\- 匯入檔案或資料夾時選擇存放策略。

\- 以標籤瀏覽與篩選素材。

\- 素材可屬於多個標籤。

\- 支援評分、格式篩選、縮圖與影片抽幀預覽。

\- 外部檔案移動時以雜湊嘗試重新連結。



\*\*專案\*\*

\- 在特定標籤下建立專案。

\- 專案可跨標籤歸屬（一個專案可屬於多個標籤）。

\- 從母庫將素材引用進專案。



\*\*白版\*\*

\- 專案下可建立多個白版。

\- 從母庫或專案已引用的素材拖曳至白版，建立 Placement。

\- 支援移動、縮放、旋轉、調整圖層順序、文字註記、群組。

\- 同一素材可出現在同一白版的多個位置，也可出現在不同專案的白版上。



\*\*分享\*\*

\- 同一區域網內自動發現，直接傳輸選定的專案（含其白版與引用）。

\- 非同一區域網時以配對碼建立端對端加密通道後傳輸。

\- 傳輸时可選擇是否包含素材實體檔案。



\---



\### 五、資料模型（bbolt）



bbolt 為單檔鍵值儲存。所有資料以 bucket + 鍵值方式存放，關聯透過輔助索引維護。序列化建議使用 msgpack。



\*\*主要 Bucket 與鍵設計\*\*



\- `assets`  

&#x20; key: `assetID`  

&#x20; value: Asset 結構（path, storage\_mode, hash, mime, width, height, duration, thumbnail\_path, rating, created\_at）



\- `assets\_by\_hash`  

&#x20; key: `hash`  

&#x20; value: `assetID`



\- `assets\_by\_path`  

&#x20; key: `path`  

&#x20; value: `assetID`



\- `tags`  

&#x20; key: `tagID`  

&#x20; value: Tag 結構（name, created\_at）



\- `asset\_tags`  

&#x20; key: `assetID/tagID`  

&#x20; value: 空（表示該素材擁有此標籤）



\- `tag\_assets`  

&#x20; key: `tagID/assetID`  

&#x20; value: 空（反向，用於列出某標籤下的所有素材）



\- `projects`  

&#x20; key: `projectID`  

&#x20; value: Project 結構（name, description, created\_at）



\- `project\_tags`  

&#x20; key: `projectID/tagID`  

&#x20; value: 空



\- `tag\_projects`  

&#x20; key: `tagID/projectID`  

&#x20; value: 空（列出某標籤下的專案）



\- `boards`  

&#x20; key: `boardID`  

&#x20; value: Board 結構（project\_id, name, created\_at）



\- `boards\_by\_project`  

&#x20; key: `projectID/boardID`  

&#x20; value: 空



\- `placements`  

&#x20; key: `placementID`  

&#x20; value: Placement 結構（board\_id, asset\_id, x, y, scale, rotation, z\_index, notes）



\- `placements\_by\_board`  

&#x20; key: `boardID/placementID`  

&#x20; value: 空



\- `placements\_by\_asset`  

&#x20; key: `assetID/placementID`  

&#x20; value: 空（查詢某素材被哪些地方引用）



所有寫入必須在同一個 `db.Update` transaction 內同時更新主資料與相關索引。



\---



\### 六、技術架構與實作細節



\*\*技術選型\*\*

\- UI：MyGo native UI（`package ui`）

\- 資料：bbolt

\- 檔案監聽：fsnotify

\- 縮圖：imaging + 外部 ffmpeg

\- 區域網發現：mDNS 或 UDP 廣播

\- 跨網傳輸：pion/webrtc + 輕量信令



\*\*應用結構\*\*

```

ragle/

├── cmd/ragle/

├── internal/

│   ├── db/          # bbolt 開啟、bucket 初始化、transaction 輔助

│   ├── model/       # 結構定義與序列化

│   ├── library/     # 匯入、存放策略、標籤管理

│   ├── project/     # 專案、白版、Placement

│   ├── canvas/      # 白版渲染與互動狀態

│   ├── share/       # 區域網與 E2E

│   ├── thumbnail/

│   └── watcher/

└── ui/              # MyGo 視圖

```



\*\*匯入流程\*\*

1\. 使用者選擇路徑與 StorageMode。

2\. 依模式處理實體檔案（索引／連結／移動／複製）。

3\. 計算 hash、讀取媒體資訊、生成縮圖。

4\. 在 transaction 中寫入 `assets`、`assets\_by\_hash`、`assets\_by\_path`。

5\. 若使用者指定標籤，同時寫入 `asset\_tags` 與 `tag\_assets`。



\*\*白版渲染\*\*

在視圖的 Render 中：

\- 以目前 boardID 前綴掃描 `placements\_by\_board`。

\- 讀取對應 Placement 與 Asset 縮圖。

\- 只繪製可視範圍內的項目。

\- 互動時修改記憶體狀態，停止操作或定時以 transaction 寫回 `placements`。



\*\*標籤與專案瀏覽\*\*

\- 列出所有標籤：掃描 `tags` bucket。

\- 某標籤下的專案：前綴掃描 `tag\_projects`。

\- 某標籤下的素材：前綴掃描 `tag\_assets`。



\---



\### 七、分享實作



\*\*區域網\*\*

廣播自身存在。使用者選擇一個專案後，從 bbolt 讀出該專案所屬標籤、所有 boards、placements、相關 assets 元資料，以及使用者勾選的實體檔案，透過 TCP 傳送。



\*\*配對碼 E2E\*\*

產生短碼向信令服務註冊。對方輸入後建立 WebRTC DataChannel。先傳元資料幀，再傳檔案幀（含 hash 與位元組偏移）。接收方在 transaction 內寫入自己的 bbolt，並以 hash 做去重。



\---



\### 八、開發順序



1\. bbolt bucket 初始化、Asset 寫入與讀取、hash／path 索引。

2\. 標籤的建立與素材歸屬。

3\. 專案建立、專案與標籤的關聯、從母庫引用素材。

4\. Board 與 Placement 的寫入、白版基本渲染與拖曳。

5\. 縮圖與檔案監聽。

6\. 區域網分享。

7\. 配對碼 E2E 分享。



每完成一個階段，必須能在無網路狀態下正常使用已實作的功能。



\---



\### 九、邊界與注意事項



\- 標籤刪除時不自動刪除其下專案，只移除關聯；專案仍可透過其他標籤或直接列表找到。

\- 高頻白版更新必須緩衝，不可每次滑鼠移動都開 transaction。

\- 鍵設計必須支援前綴掃描，否則列表操作會變成全表讀取。

\- 單一 bbolt 檔案需有備份機制，損壞會影響整個庫。

\- 第一版分享以傳送副本為主，不做即時多人協作。



此文件為 Ragle 的完整實作基準。

---

### 十、第一版實作補充（階段 1–5）

這一版完成開發順序的第 1 到第 6 步：母庫、標籤、專案引用、白版、縮圖、檔案監聽，以及區域網分享。配對碼與即時協作尚未實作。

#### 識別碼與索引

- 識別碼是 ULID 字串，不含 `/`。索引鍵寫成 `父 ID/子 ID`，值是空的 byte slice，以便前綴掃描。
- `meta` bucket 的 schema 版號是 `"1"`。
- 相同 SHA-256 視為同一筆素材。再次匯入只補上標籤，不重複複製檔案。

#### 專案引用

- `project_assets` 與 `asset_projects` 記錄專案引用了哪些素材。引用本身不會在白版上建立放置。
- 取消引用時，同一筆交易刪掉該專案各白版上這個素材的圖片放置。母庫檔案與其他專案的放置留下。

#### 放置

Placement 在規格的欄位之外還有：`kind`（`image`、`text`、`group`）、`parent_id`、`width`、`height`、`text`、`font_size`。`notes` 是註記，文字物件的內文放在 `text`。

子物件的 `x`、`y`、`scale`、`rotation` 相對於父物件。原點在左上，Y 向下，旋轉以度為單位、順時針，與 MyGo Painter 一致。`x`、`y` 是物件中心。群組的縮放與旋轉會帶動子物件。

#### 刪除

- 刪除素材會移除標籤關聯、專案引用與所有放置。`index` 不刪使用者的原檔。`symlink` 只刪母庫裡的連結。`copy` 與 `move` 刪除母庫擁有的檔案。
- 刪除群組時，同一筆操作刪掉所有子孫放置。
- 刪除標籤只移除關聯。專案與素材都留著，仍可從列表找到。
- 刪除專案只移除該專案的白版與放置，母庫檔案留下。

#### 檔案不見與重新連結

Asset 有 `missing`。監聽發現檔案不見時把它設為 true，並拿掉 path 索引。`Path` 仍留在紀錄上，介面可以顯示舊路徑。以雜湊重新連結成功後清除 `missing`。

#### 備份

- 資料庫成功開啟後，用 bbolt 交易的 `tx.WriteTo` 寫出 `ragle.db.bak`。檔案正開著，所以不直接複製那個檔。
- 刪除專案、素材或標籤之前也先備份。
- 開啟失敗且備份存在時，歡迎畫面可以還原。
- 在 Windows 上取代檔案時，先刪掉目的地再改名。

#### 匯入與縮圖

- 存放方式是 `index`、`symlink`、`move`、`copy`。
- 複製、移動與連結放進母庫的 `files/`，沿用原檔名。撞名時使用 `name (2).ext`。
- 連結失敗時回傳錯誤，不改成複製，來源檔留在原地。
- 略過母庫自己的 `files/`、`thumbs/`、`ragle.db*`，以及隱藏名稱。
- 每個新檔案一筆寫入交易。縮圖在素材列寫入之後，由兩個 worker 非同步產生。
- 沒有 ffmpeg 時，影片卡片顯示副檔名，其餘功能照常可用。

#### 白版互動

- 拖曳先改記憶體。放開指標與結構性修改時立刻寫回；有未寫入的變更時，最遲每 400 毫秒寫一次。不在每次滑鼠移動時開交易，也不在繪製裡寫入。
- 滾輪以游標為中心縮放。中鍵，或按住空白鍵拖曳，用來平移。
- 從母庫或專案托盤把圖片拖進白版。新圖片的長邊約 320。文字工具可建立文字，雙擊進入編輯，側邊把手調整寬度。角點等比例縮放，上方把手旋轉。
- 可多選後群組或解散，也可調整圖層。從白版刪除只移除放置，不刪素材。
- 群組的點選範圍是變換後的外框，子物件優先於群組。只繪製與可視範圍相交的物件。
- 跨 goroutine 更新介面走 `Window.Update`。檔案對話框從 goroutine 呼叫，避免在視圖裡呼叫而卡住主執行緒。

#### 視窗、語系與外觀

- 單一視窗，畫面是歡迎、母庫、專案、白版。
- 第一次啟動時選擇母庫資料夾。資料庫、縮圖，以及複製、移動、連結的檔案都放在該資料夾。
- 偏好存在系統設定目錄的 `Ragle/config.json`（`library_path`、`language`），不寫進母庫。
- 介面可在繁體中文與英文之間切換。預設跟著作業系統：中文語系用繁體中文，其餘用英文。
- 刪除確認會說明母庫檔案會不會被刪除。
- 外觀跟著系統的淺色或深色，以及系統強調色。白版紙色在淺色是 `#f4f1ea`，深色是 `#1c1c1c`。應用圖示是 `Logo.png`。

#### 區域網分享

- 發現用 UDP 廣播，連接埠 47321，封包以 `RGL1` 開頭。同一台機器上的兩個程式用 `SO_REUSEADDR` 綁同一個連接埠。TCP 用系統分配的連接埠，並寫在廣播裡。
- 使用者在專案畫面選擇一台裝置，並決定要不要連素材檔案一起送。送出的是專案、標籤、白版、放置、引用與素材中繼資料的副本。
- 對方先看到確認。拒絕就不會寫進資料庫，也不會傳檔案。
- 接收時所有識別碼都換成新的。標籤以名稱對上，不分大小寫，沒有才建立。專案若沒有標籤，接收方會放進名為 Shared 的標籤。
- 素材以 SHA-256 去重。雜湊已存在就沿用那筆，多出來的檔案刪掉。新檔案寫進 `files/`，存放方式是 `copy`。沒有帶檔案的新素材標成 missing，存放方式是 `index`，路徑只留檔名，之後刪除不會去刪使用者的原檔。
- 關掉視窗時會取消還沒完成的分享。發現或傳輸失敗時，母庫仍可離線使用。

#### 程式配置

- 模組路徑是 `ragle`。進入點是 `cmd/ragle`。套件在 `internal/db`、`model`、`library`、`project`、`canvas`、`thumbnail`、`watcher`、`i18n`、`config`、`share`，視圖在 `ui/`。
- `mygo.json` 的 `main` 是 `./cmd/ragle`。



---

### 十一、介面架構與 MyGo Native UI 升級紀錄（v0.7.4）

1. **框架與相依版本升級**：
   - 升級至 `github.com/egoist/mygo v0.7.4` 與 Go 1.27.3。
   - 執行 `mygo vet .` 驗證，所有 Handle、生命週期與 Context 用法皆合規。

2. **原生佈局與組件全面重構**：
   - **雙層分割視圖（`ui.Split` / `ui.SplitPercent`）**：
     - 外層採用 `ui.Split` 分隔側邊欄與主要工作區，支援滑鼠拖曳與記憶寬度（預設 220 DIP）。
     - 母庫內部採用 `ui.SplitPercent`（預設 72% / 28%）分割素材網格與右側 Inspector 檢查面板，防止小視窗時元素擠壓變形。
     - 白版模式進入時全螢幕呈現畫布工作區，並以 `AlignItems(gui.Stretch)` 保證畫布與側邊檢查欄滿高展開。
   - **標準側邊導覽（`ui.Sidebar` / `ui.SidebarSection` / `ui.SidebarItem`）**：
     - 取代原有手繪按鈕與破碎的 ScrollView，整合鍵盤上/下/Home/End 選擇與自動折疊。
     - 底部新增標籤與新增專案收納於獨立管理列，避免因標籤項目少時將表單擠壓至螢幕底部的問題。
   - **素材網格（`ui.GridView`）渲染修正**：
     - 去除 Cell 內多餘嵌套的固定容器，改以 `Fill().Padding(4)` 配合 Cell 自動適配。
     - 圖片縮圖使用 `Image.Fit(gui.Contain).Grow(1).Radius(6)`，徹底解決高解析度（如 4x density）圖片尺寸溢出與黑色遮蔽塊問題。
     - 檔名使用 `SingleLine()` 配合適當字級，防止長檔名過度換行破壞卡片高度。
   - **原生評分組件（`ui.Rating`）**：
     - Inspector 內將原先手刻的 5 個星號按鈕替換為 MyGo 原生 `Rating` 組件，支援即時懸停預覽與無障礙鍵盤操作。
   - **頂部工具列（TopBar）視覺強化**：
     - 採用 `Background(t.Surface)` 獨立視覺層次，狀態文字設定 `SingleLine()` 與最大寬度，避免訊息過長破壞標題列佈局。

---

### 十二、階段 7 實作紀錄（配對碼 E2E 分享）

1. **輕量自架信令服務（`internal/share/signal.go`、`cmd/ragle-signal`）**：
   - 提供可自架的純 HTTP/JSON 長輪詢信令伺服器（`go run ./cmd/ragle-signal -addr :8787 -ttl 5m`），僅在記憶體暫存短生命週期 SDP Offer/Answer，不經手專案資料或檔案。
   - 配對碼採用 6 碼去歧義字元（格式 `XXX-XXX`，輸入時不分大小寫與連字號），支援過期清理、防重複領取與發送方取消清理。

2. **WebRTC DataChannel 端對端加密傳輸（`internal/share/pair.go`）**：
   - 使用 `github.com/pion/webrtc/v4` 建立 DTLS 加密、有序可靠的 DataChannel（`ragle-share`）。
   - 先分塊傳送 `Bundle` 元資料幀，經接收方確認後，再以 32 KiB 分塊傳送檔案幀（含 `Hash` 與位元組偏移 `Offset`）並具備背壓控制。
   - 接收方驗證檔案大小與 SHA-256 後，於單一 bbolt transaction 內寫入並依雜湊去重。

3. **設定與 UI 整合（`internal/config`、`internal/i18n`、`ui/share.go`、`ui/shell.go`）**：
   - `config.json` 新增選填 `signaling_url`；未設定時不影響單機離線與區域網分享。
   - 專案分享面板整合「產生配對碼 / 複製配對碼 / 取消」，頂部工具列新增「輸入配對碼」快速接收列。
