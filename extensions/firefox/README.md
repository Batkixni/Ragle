# Raglex Web Clipper — Firefox / Zen / Floorp / LibreWolf Edition

Manifest V3 配套擴充套件，專為 **Firefox 系瀏覽器**（Firefox、Zen Browser、Floorp、LibreWolf、Waterfox）設計。

## 1. 在 Raglex 開啟連線支援
1. 開啟 **Raglex → 偏好設定 → 分享**。
2. 開啟 **網頁與手機瀏覽**（若需要存取金鑰，可展開「進階連線設定」複製）。

## 2. 安裝方式 A：快速載入
1. 在網址列輸入 `about:debugging#/runtime/this-firefox` 並按 Enter。
2. 點選右上角 **「載入暫時附加元件…（Load Temporary Add-on…）」**。
3. 選擇 `extensions/firefox/manifest.json`。
4. 點選工具列的 **Raglex** 圖示即可自動連線；若需填寫存取金鑰或切換繁中／英文，點右上角 **⚙ 設定** 即可。

## 3. 安裝方式 B：永久安裝（Zen Browser / Floorp / LibreWolf / Firefox Developer Edition）
1. 在網址列輸入 `about:config`，搜尋 `xpinstall.signatures.required` 改為 **`false`**。
2. 將 `extensions/firefox/` 內所有檔案壓縮為 `.zip`（確保 `manifest.json` 位於最頂層），將副檔名改為 **`.xpi`**。
3. 開啟 `about:addons` → 齒輪圖示 → **「從檔案安裝附加元件…」** → 選擇 `.xpi` 檔案。

## 功能特色
- **儲存此頁（書籤 / 來源）**：一鍵擷取 YouTube 高畫質封面、oEmbed / JSON-LD 多圖頁面（自動堆疊）、RSS/Atom 或可雙擊開啟的視覺書籤。
- **直接歸類**：在主畫面直接指定 **標籤**、**專案**、**白板** 與 **備註**。
- **可視截圖 & 整頁批次抓圖**：掃描頁面所有圖片（含解析度標示）並可自動合併為版本堆疊。
- **毛玻璃拖拉存圖膠囊**：拖曳網頁圖片即可丟進底部懸浮膠囊存檔（可在設定中開關）。
