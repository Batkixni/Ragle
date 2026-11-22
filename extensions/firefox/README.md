# Ragle Web Clipper — Firefox / Zen / Floorp / LibreWolf Edition

Manifest V3 companion extension tailored for **Firefox-based browsers** (Firefox, Zen Browser, Floorp, LibreWolf, Waterfox).

## 1. 先在 Ragle 開啟內建伺服器
1. 開啟 **Ragle → 偏好設定 → 分享（Sharing）**。
2. 開啟 **啟動內建伺服器（Built-in Server）**（預設 `http://127.0.0.1:41595`），並複製 **API Token**（如有設定）。

## 2. 安裝方式 A：快速測試載入（適用所有 Firefox 系瀏覽器）
1. 在網址列輸入 `about:debugging#/runtime/this-firefox` 並按 Enter。
2. 點選右上角 **「載入暫時附加元件…（Load Temporary Add-on…）」**。
3. 選擇本資料夾內的 `manifest.json`（`extensions/firefox/manifest.json`）。
4. 點選工具列的 **Ragle Web Clipper** 圖示，貼上 API Token，確認顯示 **Connected** 即可使用！
   > 注意：Firefox 的暫時附加元件在瀏覽器重新啟動後需要重新載入。

## 3. 安裝方式 B：永久安裝（適用 Zen Browser / Floorp / LibreWolf / Firefox Developer Edition / Nightly）
1. 在網址列輸入 `about:config`，搜尋 `xpinstall.signatures.required`，將它雙擊改為 **`false`**。
2. 將 `extensions/firefox/` 內的所有檔案壓縮成 `.zip`（確保 `manifest.json` 在壓縮檔最頂層，不是包在資料夾裡），再把副檔名改成 **`.xpi`**（例如 `ragle-web-clipper.xpi`）。
3. 在瀏覽器開啟 `about:addons` → 點右上角齒輪 → **「從檔案安裝附加元件…（Install Add-on From File…）」** → 選擇 `ragle-web-clipper.xpi` 即可永久生效！

## 權限提醒（Firefox MV3）
Firefox MV3 預設需要手動允許網站存取權限才能在網頁顯示拖拉膠囊與批次抓圖：
- 在 `about:addons` 點進 **Ragle Web Clipper → 權限（Permissions）**，開啟 **「存取您於所有網站的資料（Access your data for all websites）」**，即可在所有網站使用拖拉存圖與批次擷取！
