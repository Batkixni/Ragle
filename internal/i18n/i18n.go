// Package i18n is the app's Traditional Chinese and English string table.
// The choice is an app preference, not a library record.
package i18n

import (
	"fmt"
	"os"
	"strings"
)

const (
	// ZhHant is Traditional Chinese.
	ZhHant = "zh-Hant"
	// En is English.
	En = "en"
)

type pair struct {
	key string
	zh  string
	en  string
}

// pairs is the whole catalog. Both languages are filled from this list.
var pairs = []pair{
	{"welcome.choose", "選擇資料夾", "Choose a folder"},
	{"welcome.defaultDir", "預設位置（安裝位置）", "Default location (Install folder)"},
	{"welcome.restore", "從備份還原", "Restore backup"},
	{"welcome.lead", "選擇一個資料夾當作資料庫。索引、縮圖，以及複製、移動或連結進來的檔案都會放在這裡。", "Pick a folder for the library. The database, thumbnails, and files you copy, move, or link live there."},
	{"welcome.openFailed", "這個資料夾的資料庫打不開。", "This library database could not be opened."},
	{"welcome.busy", "資料庫正在被別的程式使用。", "The database is in use by another process."},

	{"shell.library", "資料庫", "Library"},
	{"shell.tags", "標籤", "Tags"},
	{"shell.projects", "專案", "Projects"},
	{"shell.all", "全部", "All"},

	{"library.files", "匯入檔案", "Import files"},
	{"library.folder", "匯入資料夾", "Import folder"},
	{"library.mode", "存放方式", "Storage"},
	{"library.mode.index", "索引", "Index"},
	{"library.mode.symlink", "連結", "Link"},
	{"library.mode.move", "移動", "Move"},
	{"library.mode.copy", "複製", "Copy"},
	{"library.kind", "格式", "Kind"},
	{"library.kind.all", "全部", "All"},
	{"library.kind.image", "圖片", "Images"},
	{"library.kind.video", "影片", "Video"},
	{"library.kind.other", "其他", "Other"},
	{"library.rating", "評分", "Rating"},
	{"library.empty", "這個範圍還沒有素材。", "Nothing in this view yet."},
	{"library.missing", "找不到檔案", "Missing"},
	{"library.delete", "刪除素材", "Delete asset"},
	{"library.deleteCopy", "僅會刪除 Raglex 資料庫內的複本、縮圖與白板引用。原本資料夾中的檔案不會受影響。", "Deletes only the copy inside the Raglex library, thumbnail, and board references. Your original source file is untouched."},
	{"library.deleteMove", "注意：此素材以「移動」方式存放，刪除將會永久移除資料庫中的實體檔案與白板引用。", "Warning: this asset was imported with Move mode. Deleting it permanently removes the file from the library and boards."},
	{"library.deleteIndex", "不會刪除硬碟上的原始檔案！僅從資料庫移除索引、縮圖與白板引用。", "Your original file on disk will NOT be deleted. This removes the library index, thumbnail, and board references only."},
	{"library.deleteLink", "不會刪除硬碟上的原始檔案！僅移除資料庫內的捷徑連結、縮圖與白板引用。", "Your original file on disk will NOT be deleted. This removes the library symlink, thumbnail, and board references only."},
	{"library.imported", "匯入完成：新增 %d，已存在 %d，失敗 %d", "Import finished: %d added, %d already there, %d failed"},
	{"library.importing", "匯入中 %d / %d", "Importing %d / %d"},
	{"library.symlinkFailed", "無法建立連結，原檔仍在原地。", "The link could not be created. The original file is still in place."},
	{"library.noFfmpeg", "找不到 ffmpeg，影片會以副檔名顯示。", "ffmpeg was not found. Videos show their extension."},
	{"library.reference", "引用到專案", "Reference in project"},
	{"library.referencing", "點選素材，引用到「%s」。", "Select an asset to reference it in “%s”."},
	{"library.path", "路徑", "Path"},
	{"library.mime", "類型", "Type"},
	{"library.storage", "存放", "Storage"},
	{"library.tags", "標籤", "Tags"},

	{"tag.new", "新標籤", "New tag"},
	{"tag.create", "建立標籤", "Create tag"},
	{"tag.delete", "刪除標籤", "Delete tag"},
	{"tag.deleteNote", "只會移除關聯。專案與素材都還在，仍可從列表找到。", "This removes the association only. Projects and assets stay, and remain listed."},
	{"tag.needName", "請輸入標籤名稱。", "Enter a tag name."},
	{"tag.exists", "已有這個標籤。", "That tag already exists."},

	{"project.new", "新專案", "New project"},
	{"project.create", "建立專案", "Create project"},
	{"project.delete", "刪除專案", "Delete project"},
	{"project.deleteNote", "會刪除這個專案的白板與放置。資料庫裡的素材檔案會留下。", "This deletes the project's boards and placements. Library files stay."},
	{"project.needTag", "先選一個標籤，專案會建在它下面。", "Select a tag first. The project is created under it."},
	{"project.needName", "請輸入專案名稱。", "Enter a project name."},
	{"project.assets", "引用的素材", "Referenced assets"},
	{"project.boards", "白板", "Boards"},
	{"project.newBoard", "新白板", "New board"},
	{"project.emptyAssets", "還沒有引用素材。", "No assets referenced yet."},
	{"project.emptyBoards", "還沒有白板。", "No boards yet."},
	{"project.unreference", "移除引用", "Remove reference"},
	{"project.unreferenceNote", "會拿掉這個專案白板上該素材的放置。資料庫檔案與其他專案會留下。", "This removes that asset's placements on this project's boards. The library file and other projects stay."},
	{"project.description", "說明", "Description"},
	{"project.fromLibrary", "從資料庫引用", "Reference from library"},
	{"project.openBoard", "開啟", "Open"},
	{"project.deleteBoard", "刪除白板", "Delete board"},
	{"project.deleteBoardNote", "會刪除這面白板上的放置。資料庫裡的素材會留下。", "This deletes the placements on this board. Library assets stay."},

	{"board.select", "選取", "Select"},
	{"board.pan", "平移", "Pan"},
	{"board.text", "文字", "Text"},
	{"board.group", "群組", "Group"},
	{"board.ungroup", "解散", "Ungroup"},
	{"board.front", "最前", "Front"},
	{"board.back", "最後", "Back"},
	{"board.notes", "註記", "Notes"},
	{"board.body", "內文", "Text"},
	{"board.placeholder", "文字", "Text"},
	{"board.needTwo", "請選兩個以上的物件再群組。", "Select two or more objects to group them."},
	{"board.delete", "從白版移除", "Remove from board"},
	{"board.deleteNote", "只會從白板移除這些物件。資料庫裡的素材會留下。", "This removes the objects from the board. Library assets stay."},
	{"board.leave", "返回專案", "Back to project"},
	{"board.trayProject", "專案", "Project"},
	{"board.trayLibrary", "資料庫", "Library"},
	{"board.empty", "把素材拖到白版上，或用文字工具寫一句話。", "Drag an asset onto the board, or use the text tool."},

	{"share.send", "分享", "Share"},
	{"share.sendTo", "傳送", "Send"},
	{"share.title", "區域網", "On this network"},
	{"share.files", "包含素材檔案", "Include asset files"},
	{"share.none", "還沒有發現同一網路上的 Raglex。", "No other Raglex on this network yet."},
	{"share.sending", "正在傳送…", "Sending…"},
	{"share.receiving", "正在接收…", "Receiving…"},
	{"share.sent", "已送出。", "Sent."},
	{"share.incoming", "接收專案", "Incoming project"},
	{"share.incomingFiles", "%s 想傳送專案「%s」，包含 %d 個檔案。", "%s wants to send project %s, including %d files."},
	{"share.incomingMeta", "%s 想傳送專案「%s」的結構，不含檔案。", "%s wants to send the structure of project %s, without files."},
	{"share.accept", "接收", "Receive"},
	{"share.received", "已接收專案。", "Project received."},
	{"share.unavailable", "區域網分享現在不可用。資料庫仍可離線使用。", "Sharing on this network is unavailable. The library still works offline."},
	{"share.declined", "對方沒有接收。", "The other side declined."},
	{"share.interrupted", "傳送中斷。", "The transfer stopped."},
	{"share.pairTitle", "配對碼跨網分享（端對端加密）", "Pairing code (E2E encrypted)"},
	{"share.signalURL", "信令伺服器網址", "Signaling server URL"},
	{"share.createCode", "產生配對碼", "Create pairing code"},
	{"share.codeWaiting", "配對碼：%s（等待對方連線…）", "Code: %s (waiting for peer…)"},
	{"share.copyCode", "複製配對碼", "Copy code"},
	{"share.codeCopied", "已複製配對碼。", "Pairing code copied."},
	{"share.receiveByCode", "輸入配對碼", "Receive by code"},
	{"share.enterCode", "輸入配對碼（例如 ABC-DEF）", "Enter code (e.g. ABC-DEF)"},
	{"share.connect", "連線接收", "Connect"},
	{"share.connecting", "正在透過配對碼連線…", "Connecting via pairing code…"},
	{"share.needSignal", "請先輸入信令伺服器網址。", "Enter a signaling server URL first."},
	{"share.badCode", "配對碼無效或已過期。", "Pairing code is invalid or expired."},
	{"share.codeClaimed", "此配對碼已被使用。", "This pairing code was already used."},

	{"common.cancel", "取消", "Cancel"},
	{"common.delete", "刪除", "Delete"},
	{"common.remove", "移除", "Remove"},
	{"common.close", "關閉", "Close"},
	{"common.done", "完成", "Done"},
	{"common.clear", "清除篩選", "Clear filters"},
	{"common.copyPath", "複製路徑", "Copy path"},
	{"common.copied", "已複製到剪貼簿。", "Copied to clipboard."},
	{"common.openFile", "開啟檔案", "Open file"},
	{"common.revealFile", "在資料夾顯示", "Show in folder"},
	{"common.save", "儲存", "Save"},
	{"common.itemsCount", "%d 個素材", "%d items"},
	{"inspector.none", "點選任一素材查看詳細資訊與標籤。", "Select an asset to inspect metadata and tags."},
	{"inspector.multi", "已選多個物件。", "Several objects are selected."},
	{"inspector.details", "檔案資訊", "Details"},
	{"inspector.dimensions", "尺寸", "Dimensions"},
	{"inspector.aspect", "比例", "Aspect"},
	{"inspector.size", "大小", "Size"},
	{"inspector.hash", "雜湊", "SHA-256"},
	{"inspector.added", "匯入時間", "Added"},
	{"inspector.usedIn", "引用於專案", "Used in projects"},
	{"inspector.usedNone", "尚未被任何專案引用", "Not referenced by any project"},
	{"inspector.placements", "白板放置 %d 次", "%d board placements"},
	{"inspector.actions", "操作", "Actions"},
	{"inspector.fileActions", "開啟 / 複製", "Open & Copy"},
	{"inspector.toggle", "切換資訊欄", "Toggle inspector"},
	{"sidebar.toggle", "切換側邊欄 (Ctrl+B)", "Toggle sidebar (Ctrl+B)"},
	{"inspector.quickTag", "新增並套用標籤…", "Add & apply tag…"},
	{"inspector.relink", "掃描資料夾重連…", "Relink from folder…"},
	{"inspector.relinkFile", "選取檔案重連…", "Relink file…"},
	{"inspector.relinkResult", "已重新連結 %d 個遺失檔案。", "Relinked %d missing files."},
	{"inspector.relinkNone", "沒有找到符合雜湊的遺失檔案。", "No missing files matched by hash."},

	{"shell.allAssets", "全部素材", "All Assets"},
	{"shell.untagged", "未分類", "Untagged"},
	{"shell.recent", "近期新增", "Recent"},
	{"shell.random", "隨機探索", "Random"},
	{"shell.missing", "遺失檔案", "Missing"},
	{"shell.switchLib", "切換資料庫資料夾…", "Switch library folder…"},
	{"shell.theme", "外觀主題", "Appearance"},
	{"shell.themeAuto", "跟隨系統", "System"},
	{"shell.themeLight", "淺色", "Light"},
	{"shell.themeDark", "深色", "Dark"},
	{"shell.language", "語言", "Language"},
	{"shell.localFirst", "本地資料庫 · 離線可用", "Local-first · Offline ready"},

	{"library.search", "搜尋…", "Search…"},
	{"library.import", "匯入", "Import"},
	{"library.filter", "篩選", "Filter"},
	{"library.sort", "排序", "Sort"},
	{"library.layout", "排版", "Layout"},
	{"library.layout.bento", "Bento 拼貼", "Bento"},
	{"library.layout.masonry", "瀑布流", "Waterfall"},
	{"library.layout.grid", "標準網格", "Grid"},
	{"library.sortNewest", "最新匯入", "Newest first"},
	{"library.sortOldest", "最舊匯入", "Oldest first"},
	{"library.sortName", "檔名 A–Z", "Name A–Z"},
	{"library.sortRating", "評分高到低", "Highest rated"},
	{"library.dropHint", "放開滑鼠以匯入檔案（模式：%s）", "Drop files or folders to import (mode: %s)"},
	{"library.emptyHint", "拖曳圖片或資料夾到這裡，或點擊上方「匯入」開始建立你的參考資料庫。", "Drag images or folders here, or click Import above to build your reference library."},
	{"library.noMatch", "找不到符合篩選條件的素材。", "No assets match the current search or filters."},
	{"library.preview", "快速預覽", "Quick Preview"},
	{"library.addToProject", "加入專案", "Add to project"},
	{"library.referencedIn", "已引用到「%s」。", "Referenced in “%s”."},

	{"preview.prev", "上一個", "Previous"},
	{"preview.next", "下一個", "Next"},
	{"preview.loading", "載入原圖中…", "Loading full image…"},

	{"project.createTitle", "建立新專案", "Create new project"},
	{"project.edit", "編輯專案", "Edit project"},
	{"project.namePlaceholder", "例如：品牌視覺提案、角色設定…", "e.g. Brand Identity, Character Sheet…"},
	{"project.descPlaceholder", "選填說明或工作備註…", "Optional description or notes…"},
	{"project.pickTags", "歸屬標籤（可多選）", "Assign tags (multi-select)"},
	{"project.boardItems", "%d 個物件", "%d items"},
	{"project.emptyBoardsHint", "建立一個白版來自由排列與組合你的參考素材。", "Create a board to arrange and compose your references."},
	{"project.emptyAssetsHint", "點擊上方「從資料庫引用」將資料庫素材加入此專案。", "Click “Reference from library” above to bring assets into this project."},

	{"board.zoomReset", "重設縮放", "Reset zoom"},
	{"board.drawer", "素材托盤", "Asset tray"},
	{"board.inspector", "屬性欄", "Properties"},
	{"board.clickToPlace", "點擊或拖曳至白版", "Click or drag onto board"},
	{"board.transform", "變換資訊", "Transform"},
	{"board.fontSize", "字級", "Font size"},
	{"board.placementCount", "%d 個放置", "%d items"},

	{"win.minimize", "最小化", "Minimize"},
	{"win.maximize", "最大化", "Maximize"},
	{"win.restore", "還原視窗", "Restore"},
	{"win.close", "關閉視窗", "Close"},

	{"project.summary", "%d 白版 · %d 素材", "%d boards · %d assets"},

	{"settings.title", "偏好設定", "Preferences"},
	{"settings.open", "偏好設定 (Ctrl+,)", "Preferences (Ctrl+,)"},
	{"settings.tab.general", "一般", "General"},
	{"settings.tab.database", "資料庫", "Database"},
	{"settings.tab.import", "匯入", "Import"},
	{"settings.tab.sharing", "分享", "Sharing"},

	{"settings.appearance", "外觀", "Appearance"},
	{"settings.workspace", "介面顯示", "Workspace"},
	{"settings.sidebarCounts", "在側邊欄顯示項目數量", "Show item counts in sidebar"},
	{"settings.tagColors", "顯示標籤色彩標記", "Show tag colors"},
	{"settings.cardCaption", "卡片底部顯示檔名與評分", "Show caption strip on asset cards"},
	{"settings.cardSize", "預設卡片尺寸", "Default card size"},
	{"settings.defaultSort", "預設排序方式", "Default sort order"},
	{"settings.defaultLayout", "預設排版方式", "Default layout"},

	{"settings.dbLocation", "資料庫位置", "Library location"},
	{"settings.openFolder", "開啟資料夾", "Open folder"},
	{"settings.dbStorage", "空間與統計", "Storage & statistics"},
	{"settings.dbFile", "資料庫檔案", "Database file"},
	{"settings.dbFilesDir", "受管檔案 (files/)", "Managed files (files/)"},
	{"settings.dbThumbsDir", "縮圖快取 (thumbs/)", "Thumbnail cache (thumbs/)"},
	{"settings.dbCounts", "庫內資料", "Catalog totals"},
	{"settings.dbCountsVal", "%d 素材 · %d 標籤 · %d 專案 · %d 白版", "%d assets · %d tags · %d projects · %d boards"},
	{"settings.recalc", "重新計算", "Recalculate"},
	{"settings.dbBackup", "備份與安全", "Backup & safety"},
	{"settings.backupStatus", "自動備份快照", "Snapshot backup"},
	{"settings.backupNone", "尚未建立備份檔", "No backup snapshot yet"},
	{"settings.backupBeforeDel", "刪除素材或專案前自動備份", "Auto-backup before deleting assets or projects"},
	{"settings.backupNow", "立即備份資料庫", "Back up now"},
	{"settings.backupDone", "資料庫已備份至 raglex.db.bak。", "Database backed up to raglex.db.bak."},
	{"settings.exportBackup", "匯出備份至…", "Export backup…"},
	{"settings.exportBackupDone", "已匯出資料庫備份至 %s。", "Database backup exported to %s."},
	{"settings.dbMaint", "維護與修復", "Maintenance"},
	{"settings.relinkMissing", "掃描遺失檔案重連", "Relink missing files"},
	{"settings.rebuildThumbs", "重建全部縮圖", "Rebuild thumbnails"},
	{"settings.rebuildThumbsDone", "已排入 %d 個素材重新生成縮圖。", "Queued %d assets for thumbnail regeneration."},
	{"settings.cleanThumbs", "清理孤立縮圖快取", "Clean orphaned cache"},
	{"settings.cleanThumbsDone", "已清理 %d 個無用縮圖（釋放 %s）。", "Removed %d orphaned thumbnails (freed %s)."},
	{"settings.ffmpegStatus", "影片縮圖引擎 (ffmpeg)", "Video thumbnail engine (ffmpeg)"},
	{"settings.ffmpegOK", "可用", "Available"},
	{"settings.ffmpegMissing", "未偵測到（影片將顯示副檔名佔位圖）", "Not detected (videos show extension placeholders)"},

	{"settings.importMode", "預設存放方式", "Default storage mode"},
	{"settings.modeDesc.index", "僅記錄原檔路徑與雜湊，不複製檔案（最省空間）。", "Record original path & hash only without duplicating files."},
	{"settings.modeDesc.symlink", "於資料庫 files/ 建立符號連結，原檔保留原地。", "Create a symlink in library files/; original stays in place."},
	{"settings.modeDesc.move", "將檔案搬移至資料庫 files/ 統一管理。", "Move files into library files/ for unified management."},
	{"settings.modeDesc.copy", "複製一份檔案進資料庫 files/，保留原檔。", "Copy files into library files/ while keeping originals."},
	{"settings.importBehavior", "匯入行為", "Import behavior"},
	{"settings.autoSelect", "匯入後自動選取新項目", "Auto-select newly imported item"},
	{"settings.autoTag", "於標籤檢視匯入時自動套用該標籤", "Auto-apply active tag when importing"},
	{"settings.ignoreHidden", "略過隱藏檔案與系統暫存檔（.DS_Store、Thumbs.db）", "Ignore hidden and system files (.DS_Store, Thumbs.db)"},
	{"settings.importRecursive", "匯入資料夾時遞迴掃描子資料夾", "Scan subfolders recursively when importing folders"},
	{"settings.notifyDup", "匯入重複檔案時顯示摘要通知", "Show duplicate count notice on import"},
	{"settings.importFormats", "支援格式與去重", "Formats & deduplication"},
	{"settings.importFormatsDesc", "所有匯入皆以 SHA-256 雜湊自動去重，不會重複寫入相同檔案。支援 PNG、JPG、WEBP、GIF、BMP、SVG、MP4、MOV、MKV、WEBM 及任意設計原檔。", "All imports deduplicate automatically by SHA-256 hash. Supports PNG, JPG, WEBP, GIF, BMP, SVG, MP4, MOV, MKV, WEBM, and any design source file."},

	{"settings.shareDevice", "裝置與預設傳輸", "Device & transfer defaults"},
	{"settings.deviceName", "裝置顯示名稱", "Device display name"},
	{"settings.deviceNameHint", "留空則使用系統主機名稱（重新啟動後生效）", "Leave blank for system hostname (applies on restart)"},
	{"settings.shareFilesDefault", "分享專案時預設包含原始素材檔案", "Include original asset files by default when sharing"},
	{"settings.shareSignal", "跨網配對信令伺服器", "Remote pairing signaling server"},
	{"settings.shareSignalDesc", "同一區域網內會自動透過 mDNS 發現裝置，無需任何伺服器。跨網配對碼分享則使用您自架的 raglex-signal 服務。", "Devices on the same LAN discover each other via mDNS with no server. Pairing codes use your self-hosted raglex-signal server."},
	{"settings.lanStatus", "區域網探索狀態", "LAN discovery"},
	{"settings.lanOK", "運作中（mDNS）", "Active (mDNS)"},
	{"settings.lanOff", "離線模式", "Offline mode"},

	{"settings.customTheme", "自訂主題色彩", "Custom theme"},
	{"settings.themePreset", "主題預設", "Theme preset"},
	{"settings.preset.default", "預設純粹", "Default"},
	{"settings.preset.graphite", "石墨灰", "Graphite"},
	{"settings.preset.warm", "暖紙米", "Warm Paper"},
	{"settings.preset.midnight", "深夜墨黑", "Midnight"},
	{"settings.preset.custom", "自訂", "Custom"},
	{"settings.colorCanvas", "主背景色", "Canvas background"},
	{"settings.colorSurface", "面板表面色", "Surface color"},
	{"settings.colorAccent", "主按鈕強調色", "Primary accent"},
	{"settings.colorText", "文字色", "Text color"},
	{"settings.resetTheme", "重設預設色彩", "Reset defaults"},

	{"tags.manage", "標籤管理", "Tag Manager"},
	{"tags.search", "搜尋標籤…", "Search tags…"},
	{"tags.addRoot", "新增母標籤", "New root tag"},
	{"tags.addChild", "新增子標籤", "Add child tag"},
	{"tags.edit", "編輯標籤", "Edit tag"},
	{"tags.parent", "上層母標籤", "Parent tag"},
	{"tags.noParent", "無（最上層母標籤）", "None (root tag)"},
	{"tags.empty", "尚未建立任何標籤。", "No tags created yet."},
	{"tags.emptyHint", "建立母標籤與子標籤，像資料夾一樣層級整理你的參考素材。", "Create parent and child tags to organize your library hierarchically."},
	{"tags.viewAssets", "檢視素材", "View assets"},
	{"tags.cycleErr", "無法將標籤移動到自己或子標籤下。", "A tag cannot be moved under itself or its child."},
	{"tags.childCount", "%d 個子標籤", "%d child tags"},

	{"batch.selected", "已選取 %d 個素材", "%d selected"},
	{"batch.selectAll", "全選", "Select all"},
	{"batch.clear", "取消選取", "Clear"},
	{"batch.multiMode", "多選模式", "Multi-select"},
	{"batch.addToProject", "加入專案", "Add to project"},
	{"batch.applyTag", "套用標籤", "Apply tag"},
	{"batch.delete", "批次刪除", "Delete selected"},
	{"batch.deleteConfirm", "確定要移除已選取的 %d 個素材嗎？此動作會移除資料庫紀錄與白板引用。", "Remove %d selected assets from the library and boards?"},
	{"batch.deleteSafeIndex", "確定要從資料庫移除已選取的 %d 個素材嗎？\n\n不會刪除你硬碟上的原始檔案（僅移除索引 / 連結紀錄、縮圖與白板引用）。", "Remove %d selected assets from the library?\n\nYour original files on disk will NOT be deleted (only index/link records, thumbnails, and board references are removed)."},
	{"batch.deleteSafeCopy", "確定要從資料庫移除已選取的 %d 個素材嗎？\n\n不會刪除原本資料夾的檔案（僅移除索引紀錄、資料庫內 %d 份複本、縮圖與白板引用）。", "Remove %d selected assets from the library?\n\nYour original source files will NOT be deleted (only index records, %d library copies, thumbnails, and board references are removed)."},
	{"batch.deleteWithMove", "確定要刪除已選取的 %d 個素材嗎？\n\n⚠️ 注意：其中 %d 個為「移動」存放模式，其檔案將從資料庫中永久刪除；其餘 %d 個（索引 / 連結 / 複製）不會刪除你原本的檔案。", "Delete %d selected assets?\n\n⚠️ Warning: %d asset(s) use Move mode and their files will be permanently deleted from the library. The remaining %d asset(s) (Index / Symlink / Copy) will NOT delete your original files."},
	{"batch.deleting", "正在批次刪除 %d 個素材…", "Deleting %d assets…"},
	{"batch.addedProject", "已將 %d 個素材引用至「%s」。", "Referenced %d assets in “%s”."},
	{"batch.appliedTag", "已將 %d 個素材套用標籤「%s」。", "Applied tag “%s” to %d assets."},
	{"batch.deleted", "已刪除 %d 個素材。", "Deleted %d assets."},
	{"batch.dragHint", "拖曳素材至左側專案或標籤即可批次歸類", "Drag selected cards onto any sidebar project or tag"},

	{"preview.play", "播放", "Play"},
	{"preview.pause", "暫停", "Pause"},
	{"preview.loop", "循環", "Loop"},
	{"preview.openSystem", "使用系統播放器開啟", "Open in system player"},
	{"preview.videoNoFfmpeg", "未偵測到 ffmpeg，無法於應用內解碼此影片。可安裝 ffmpeg 或使用系統播放器開啟。", "ffmpeg was not found for in-app video decoding. Install ffmpeg or open in your system player."},
	{"preview.videoFailed", "無法於應用內解碼此影片格式，請改用系統播放器開啟。", "Could not decode this video in-app. Use your system player to play it."},

	{"onboarding.open", "新手教學", "Quick-start guide"},
	{"onboarding.reopen", "重新檢視新手教學", "Replay onboarding guide"},
	{"onboarding.skip", "略過教學", "Skip"},
	{"onboarding.prev", "上一步", "Back"},
	{"onboarding.next", "下一步", "Next"},
	{"onboarding.start", "開始使用 Raglex", "Start using Raglex"},

	{"onboarding.s1.badge", "01 · 核心理念", "01 · CORE CONCEPT"},
	{"onboarding.s1.title", "歡迎使用 Raglex", "Welcome to Raglex"},
	{"onboarding.s1.lead", "為創作者與設計師打造的本地優先參考資料庫與靈感白板。所有檔案與資料都留在你的電腦裡，完全離線運作。", "A local-first reference library and moodboard studio. Everything stays on your machine and works 100% offline."},
	{"onboarding.s1.c1.title", "本地資料庫 · 四種匯入模式", "Local Library · 4 Storage Modes"},
	{"onboarding.s1.c1.desc", "支援「索引、連結、移動、複製」四種存放方式，匯入時自動以 SHA-256 去重。", "Choose Index, Symlink, Move, or Copy with automatic SHA-256 deduplication."},
	{"onboarding.s1.c2.title", "零重複檔案的專案引用", "Zero-Copy Project References"},
	{"onboarding.s1.c2.desc", "同一張素材可被多個專案與白板自由引用，不會在硬碟裡複製出一堆重複檔。", "Reference the same asset across multiple projects and boards without duplicating files."},
	{"onboarding.s1.c3.title", "跨裝置區域網與配對碼分享", "LAN & Pairing-Code Sharing"},
	{"onboarding.s1.c3.desc", "同網域自動 mDNS 發現，跨網透過端對端加密配對碼直接傳輸專案與素材。", "Discover teammates automatically on LAN or share E2E encrypted via pairing codes."},

	{"onboarding.s2.badge", "02 · 瀏覽與排版", "02 · IMPORT & LAYOUTS"},
	{"onboarding.s2.title", "拖曳即匯入，三種排版隨心切", "Drop to Import, 3 Adaptive Layouts"},
	{"onboarding.s2.lead", "把圖片、影片或整個資料夾直接拖進視窗即可匯入；上方工具列隨時切換最順眼的檢視方式。", "Drag images, videos, or folders straight into the window, and switch layouts anytime from the top bar."},
	{"onboarding.s2.c1.title", "Bento 拼貼", "Bento Grid"},
	{"onboarding.s2.c1.desc", "2×2 主圖塊搭配寬版重點列，像作品集一樣呈現重點靈感。", "Editorial 2×2 hero tiles paired with balanced wide accent rows."},
	{"onboarding.s2.c2.title", "瀑布流", "Waterfall Masonry"},
	{"onboarding.s2.c2.desc", "完整保留每張圖片的原始長寬比，直圖橫圖自然交錯排列。", "Preserves every image's natural aspect ratio in balanced columns."},
	{"onboarding.s2.c3.title", "標準網格", "Uniform Grid"},
	{"onboarding.s2.c3.desc", "等寬等高高速捲動，適合快速檢視上千張大型素材庫。", "Virtualized uniform grid built for huge reference catalogs."},

	{"onboarding.s3.badge", "03 · 多選與階層標籤", "03 · MULTI-SELECT & TAGS"},
	{"onboarding.s3.title", "左鍵圈選多選，拖曳直達專案", "Marquee Multi-Select & Hierarchical Tags"},
	{"onboarding.s3.lead", "像操作系統資料夾一樣順手：一次框選大量素材，直接拖進左側專案或標籤完成歸類。", "Select dozens of assets in one motion and drop them onto any project or tag in the sidebar."},
	{"onboarding.s3.c1.title", "滑鼠左鍵拖拉圈選", "Rubber-Band Box Select"},
	{"onboarding.s3.c1.desc", "在卡片間距或空白處按住左鍵拖拉即可框選；按住 Alt 也能直接從卡片上起手圈選。", "Hold left-click on empty space/gaps and drag to select; hold Alt to start anywhere."},
	{"onboarding.s3.c2.title", "拖曳批次歸類與批次刪除", "Drag to Organize & Batch Actions"},
	{"onboarding.s3.c2.desc", "選取多張後直接拖曳至左側專案或標籤，或利用上方批次列一鍵加入專案、貼標、刪除。", "Drag selected cards onto sidebar projects/tags, or use the batch bar to organize or delete."},
	{"onboarding.s3.c3.title", "母子標籤與標籤管理頁", "Parent/Child Tags & Tag Manager"},
	{"onboarding.s3.c3.desc", "像資料夾一樣建立母標籤與子標籤，點擊母標籤會自動包含全部子標籤素材。", "Nest child tags under parent tags and organize the entire tree in Tag Manager."},

	{"onboarding.s4.badge", "04 · 白板與媒體預覽", "04 · MOODBOARD & PREVIEW"},
	{"onboarding.s4.title", "自由靈感白板與影片即時預覽", "Freeform Moodboards & In-App Playback"},
	{"onboarding.s4.lead", "在專案內建立多個無限畫布白板，並支援圖片高解析燈箱與影片／GIF 應用內直接播放。", "Compose infinite-canvas moodboards inside each project, and inspect images, videos, and GIFs in-app."},
	{"onboarding.s4.c1.title", "無限白板畫布", "Infinite Moodboard Canvas"},
	{"onboarding.s4.c1.desc", "自由縮放、平移、旋轉、群組、調整圖層前後順序並加上文字與註記。", "Pan, zoom, scale, rotate, group, reorder layers, and annotate with text notes."},
	{"onboarding.s4.c2.title", "影片與動態 GIF 播放", "Video & Animated GIF Playback"},
	{"onboarding.s4.c2.desc", "雙擊或按空白鍵開啟快速預覽，支援播放／暫停、循環播放與系統播放器開啟。", "Double-click or press Space to preview with play/pause, looping, and system player launch."},
	{"onboarding.s4.c3.title", "右側即時檢視與自訂主題", "Live Inspector & Custom Themes"},
	{"onboarding.s4.c3.desc", "點選卡片立即檢視尺寸、比例、專案引用與備份狀態，並可在偏好設定自訂主題色。", "Inspect dimensions, project usage, and customize colors or DB backups in Preferences."},

	{"onboarding.s5.badge", "05 · 快捷鍵速查", "05 · KEYBOARD SHORTCUTS"},
	{"onboarding.s5.title", "常用快捷鍵一覽", "Essential Keyboard Shortcuts"},
	{"onboarding.s5.lead", "熟記幾個按鍵，瀏覽與整理上千張素材會快上許多。你隨時可以從左下角「？」或偏好設定再次打開本教學。", "A few shortcuts make organizing thousands of assets effortless. Reopen this guide anytime from “?” or Preferences."},
	{"onboarding.k.selectAll", "全選目前檢視所有素材", "Select all visible assets"},
	{"onboarding.k.search", "聚焦頂部搜尋欄", "Focus search bar"},
	{"onboarding.k.import", "匯入檔案至資料庫", "Import files"},
	{"onboarding.k.settings", "開啟偏好設定（資料庫／匯入／自訂主題）", "Open Preferences (DB / Import / Theme)"},
	{"onboarding.k.preview", "快速預覽 / 影片播放暫停", "Quick Preview / Play-Pause video"},
	{"onboarding.k.delete", "刪除或批次刪除選取素材", "Delete selected asset(s)"},
	{"onboarding.k.nav", "切換選取素材（搭 Shift 連續多選）", "Navigate assets (Shift to extend)"},
	{"onboarding.k.rating", "快速設定 0–5 星評分", "Quickly set 0–5 star rating"},
	{"onboarding.k.esc", "取消多選或關閉預覽／彈窗", "Clear selection or close modal"},

	{"sidebar.filter", "篩選標籤與專案…", "Filter tags & projects…"},
	{"tags.color", "標籤色彩", "Tag color"},
	{"tags.colorDefault", "預設（無色）", "Default (neutral)"},
	{"tags.color.red", "緋紅", "Crimson"},
	{"tags.color.orange", "暖橘", "Orange"},
	{"tags.color.amber", "琥珀黃", "Amber"},
	{"tags.color.green", "翠綠", "Emerald"},
	{"tags.color.cyan", "青藍", "Cyan"},
	{"tags.color.blue", "鈷藍", "Cobalt"},
	{"tags.color.purple", "紫羅蘭", "Violet"},
	{"tags.color.pink", "玫瑰粉", "Rose"},
	{"tags.moveTo", "移動至母標籤", "Move to parent"},
	{"tags.expand", "展開子標籤", "Expand children"},
	{"tags.collapse", "收合子標籤", "Collapse children"},
	{"tags.expandAll", "展開全部", "Expand all"},
	{"tags.collapseAll", "收合全部", "Collapse all"},
	{"tags.copyName", "複製名稱", "Copy name"},
	{"tags.summary", "%d 個標籤 · %d 個母分類", "%d tags · %d root groups"},
	{"project.heroBadge", "專案空間", "PROJECT"},
	{"project.boardBlank", "空白畫布", "Empty canvas"},
	{"settings.receiveByCodeBtn", "輸入配對碼接收專案…", "Receive project by pairing code…"},

	// Stage 12: Batch File Manager (Rename, Export, Clipboard, Notes).
	{"rename.title", "批量重新命名", "Batch Rename"},
	{"rename.singleTitle", "重新命名素材", "Rename Asset"},
	{"rename.action", "重新命名", "Rename"},
	{"rename.batchAction", "批量改名", "Batch Rename"},
	{"rename.template", "命名模板", "Naming template"},
	{"rename.templatePlaceholder", "例如：{tag}_{index:03} 或留空沿用原檔名", "e.g. {tag}_{index:03} or leave blank for original"},
	{"rename.tokens", "快速插入變數", "Insert token"},
	{"rename.find", "尋找文字", "Find"},
	{"rename.replace", "取代為", "Replace with"},
	{"rename.regex", "正規表達式 (Regex)", "Use regex"},
	{"rename.ignoreCase", "忽略大小寫", "Ignore case"},
	{"rename.prefix", "前綴", "Prefix"},
	{"rename.suffix", "後綴", "Suffix"},
	{"rename.case", "大小寫", "Letter case"},
	{"rename.caseKeep", "維持", "Keep"},
	{"rename.caseLower", "小寫", "lower"},
	{"rename.caseUpper", "大寫", "UPPER"},
	{"rename.caseTitle", "字首大寫", "Title"},
	{"rename.startIndex", "起始編號", "Start #"},
	{"rename.diskSync", "同步變更磁碟檔名（複製/移動檔）", "Also rename files on disk (copy/move)"},
	{"rename.includeExt", "包含外部索引 / 連結的原檔", "Include external index/symlink files"},
	{"rename.preview", "即時預覽", "Live Preview"},
	{"rename.summary", "共 %d 個素材 · 將變更 %d 個", "%d items · %d will change"},
	{"rename.conflicts", "發現 %d 個檔名衝突，請先調整規則", "%d naming conflicts — adjust rules first"},
	{"rename.apply", "套用重新命名", "Apply Rename"},
	{"rename.undo", "復原上次改名", "Undo Last Rename"},
	{"rename.done", "已重新命名 %d 個素材（可按 Ctrl+Z 復原）", "Renamed %d assets (Ctrl+Z to undo)"},
	{"rename.undone", "已復原上次批量改名（%d 個素材）", "Reverted last rename (%d assets)"},
	{"rename.noChange", "目前規則沒有產生任何檔名變更", "No filename changes with current rules"},

	{"export.title", "批量匯出與轉檔", "Batch Export & Convert"},
	{"export.action", "批量匯出", "Export"},
	{"export.format", "輸出格式", "Output format"},
	{"export.fmtOriginal", "保留原檔", "Original"},
	{"export.maxEdge", "長邊尺寸限制", "Max edge size"},
	{"export.edgeOriginal", "原尺寸", "Original"},
	{"export.quality", "JPEG 品質", "JPEG quality"},
	{"export.chooseAndRun", "選擇目標資料夾並匯出", "Choose Folder & Export"},
	{"export.done", "匯出完成：成功 %d，失敗 %d", "Exported %d files (%d failed)"},

	{"clipboard.copyFiles", "複製實體檔案", "Copy files"},
	{"clipboard.copiedFiles", "已複製 %d 個實體檔案至剪貼簿", "Copied %d files to clipboard"},
	{"clipboard.pastedImg", "已從剪貼簿匯入圖片：%s", "Imported image from clipboard: %s"},
	{"clipboard.pastedFiles", "已從剪貼簿匯入 %d 個檔案", "Imported %d files from clipboard"},
	{"clipboard.empty", "剪貼簿中沒有可匯入的圖片或檔案", "No importable image or files in clipboard"},

	{"inspector.notes", "備註", "Notes"},
	{"inspector.notesPlaceholder", "記錄靈感、用法或來源備註…", "Add notes or reference context…"},
	{"inspector.source", "來源網址", "Source URL"},
	{"inspector.saveMeta", "儲存備註", "Save notes"},
	{"inspector.metaSaved", "已儲存素材備註", "Asset metadata saved"},

	// Stage 13: Boards.
	{"shell.boards", "白板", "Boards"},
	{"board.newStandalone", "新白板", "New board"},
	{"board.leaveToLib", "返回資料庫", "Back to library"},
	{"board.sendTo", "送到白板", "Send to board"},
	{"board.sendToNew", "+ 建立新白板並放置", "+ New board with selection"},
	{"board.addedTo", "已將 %d 個素材加入白板「%s」", "Added %d assets to board “%s”"},
	{"board.moveToProject", "歸類到專案", "Move to project"},
	{"board.movedToProject", "已將白板移入專案「%s」", "Moved board to project “%s”"},
	{"board.deleteBoard", "刪除白板", "Delete board"},
	{"board.deleteBoardConfirm", "確定要刪除白板「%s」嗎？資料庫中的原始素材不會被刪除。", "Delete board “%s”? Original assets in your library will not be deleted."},

	// Stage 14: Visual Search (Reverse Image, Dominant Palette) & Laya/Ollaya AI.
	{"visual.findSimilar", "尋找相似圖片", "Find similar images"},
	{"visual.searchByImage", "以圖搜圖", "Search by image"},
	{"visual.similarBanner", "以圖搜圖：%s（找到 %d 筆相近素材）", "Similar to “%s” (%d matches)"},
	{"visual.noSimilar", "目前資料庫中沒有高度相似的圖片", "No visually similar images found"},
	{"visual.colorFilter", "主色篩選", "Color"},
	{"visual.colorBanner", "主色調篩選：%s（%d 筆）", "Color filter: %s (%d items)"},
	{"visual.palette", "主色盤", "Palette"},
	{"visual.copyHex", "點擊複製色碼 %s", "Click to copy %s"},
	{"visual.searchColor", "依此色調搜尋", "Search similar color"},
	{"laya.classify", "Laya 智慧分類", "Classify with Laya"},
	{"laya.suggestions", "Laya 建議標籤（點擊套用）", "Laya suggestions (click to apply)"},
	{"laya.done", "Laya 分類完成：自動套用 %d 個標籤，提供 %d 個建議", "Laya classified: %d auto-tagged, %d suggestions"},
	{"laya.section", "本地 AI 分類（Laya / Ollaya）", "Local AI Classification (Laya / Ollaya)"},
	{"laya.url", "Ollaya 端點位址", "Ollaya endpoint URL"},
	{"laya.model", "模型名稱", "Model name"},
	{"laya.test", "測試連線", "Test connection"},
	{"laya.connected", "已連線至本地 Ollaya（%s）", "Connected to local Ollaya (%s)"},
	{"laya.needTags", "請先建立至少一個標籤，再使用 Laya 自動分類", "Create at least one tag before running Laya classification"},

	// Stage 15 & 16: Built-in Server, Web Viewer, Browser Companion & MCP.
	{"server.section", "內建伺服器（網頁分享 / 瀏覽器擴充 / MCP / 信令）", "Built-in Server (Web Gallery / Extension / MCP / Signaling)"},
	{"server.enable", "啟動內建伺服器", "Run built-in server"},
	{"server.port", "監聽連接埠 (Port)", "Listen port"},
	{"server.bindAll", "允許區網裝置存取 (0.0.0.0)", "Allow LAN access (0.0.0.0)"},
	{"server.autoStart", "開啟資料庫時自動啟動伺服器", "Auto-start server when opening library"},
	{"server.token", "API 配對權杖 (Token)", "API Token"},
	{"server.copyToken", "複製權杖", "Copy token"},
	{"server.openWeb", "在瀏覽器開啟網頁圖庫", "Open Web Gallery"},
	{"server.copyLAN", "複製區網分享網址", "Copy LAN URL"},
	{"server.copyMCP", "複製 MCP 設定 JSON", "Copy MCP Config JSON"},
	{"server.started", "內建伺服器已啟動：%s", "Built-in server running at %s"},
	{"server.stopped", "內建伺服器已停止", "Built-in server stopped"},
	{"server.mcpCopied", "已複製 MCP JSON 設定至剪貼簿", "Copied MCP JSON config to clipboard"},

	// Stage 18: Plugin Ecosystem.
	{"settings.tab.plugins", "插件", "Plugins"},
	{"plugins.section", "已安裝插件", "Installed Plugins"},
	{"plugins.empty", "尚未安裝任何插件。將包含 manifest.json 的插件資料夾放入資料庫的 plugins/ 目錄，點擊「重新掃描」即可啟用。", "No plugins installed yet. Place plugin folders with manifest.json inside your library's plugins/ directory and click Reload."},
	{"plugins.openDir", "開啟插件資料夾", "Open plugins folder"},
	{"plugins.reload", "重新掃描", "Reload"},
	{"plugins.reloaded", "已重新掃描插件：共 %d 個", "Reloaded plugins: %d found"},
	{"plugins.menu", "執行插件", "Run plugin"},
	{"plugins.ran", "插件「%s」執行完成", "Plugin “%s” finished"},

	// UX Redesign: Built-in AI Tab, Inline Viewer, Inspector Info/Similar Tabs, Ctrl+T Go To.
	{"settings.tab.ai", "AI 與模型", "AI & Models"},
	{"ai.embedSection", "內建視覺向量模型（Embedding）", "Built-in Visual Embedding Engine"},
	{"ai.embedEnable", "啟用內建視覺向量索引（以圖搜圖與相似推薦）", "Enable built-in visual embeddings (similar image search)"},
	{"ai.embedDesc", "100% 內建於 Raglex 執行檔，無需額外安裝環境或顯卡，自動為每張圖片計算 128 維色彩、構圖與紋理向量。", "100% built into Raglex with zero external setup or GPU required. Computes 128-dim color, composition, and texture vectors locally."},
	{"ai.embedStatus", "已索引 %d / %d 個素材", "Indexed %d / %d assets"},
	{"ai.reindex", "重建視覺索引", "Rebuild Index"},
	{"ai.reindexDone", "視覺向量索引完成：已更新 %d 個素材", "Visual index rebuilt: %d assets indexed"},
	{"ai.layaEnable", "啟用 Laya 本地決策分類引擎", "Enable Laya local decision engine"},
	{"ai.layaBuiltInDesc", "可直接在 Raglex 內一鍵下載部署官方開源 Ollaya 執行環境與 Laya 模型（全程背景運行，免開終端機）；尚未部署時亦可使用內建離線向量分類。", "One-click deploy the open-source Ollaya runtime & Laya decision model directly inside Raglex (zero terminal commands); falls back to built-in offline embeddings when stopped."},
	{"ai.layaDisabled", "請先在「偏好設定 → AI 與模型」開啟 Laya 智慧分類", "Enable Laya classification in Preferences → AI & Models first"},
	{"ai.layaDeploy", "一鍵部署並啟動 Laya", "Deploy & Start Laya"},
	{"ai.layaStart", "啟動 Laya 服務", "Start Laya"},
	{"ai.layaStop", "停止服務", "Stop"},
	{"ai.layaStatus", "運行狀態", "Runtime status"},
	{"ai.layaStateRunning", "運行中（本地神經網路 Laya 就緒）", "Running (Local neural Laya ready)"},
	{"ai.layaStateInstalled", "已安裝執行檔（未啟動）", "Installed (stopped)"},
	{"ai.layaStateNotInstalled", "尚未部署（點擊下方一鍵安裝）", "Not deployed yet (click below to install)"},
	{"ai.layaDeploying", "部署進度：%s", "Deploying: %s"},
	{"ai.layaStageDownload", "下載 Ollaya 執行環境（%.0f%%）…", "Downloading Ollaya runtime (%.0f%%)…"},
	{"ai.layaStageStart", "正在啟動本地 Ollaya 背景服務…", "Starting local Ollaya service…"},
	{"ai.layaStagePull", "拉取模型中（%s %.0f%%）…", "Pulling model (%s %.0f%%)…"},
	{"ai.layaDeployDone", "Laya 模型（%s）已在 App 內部署並啟動完成！", "Laya (%s) deployed and running inside Raglex!"},
	{"ai.layaStopped", "已停止內建 Laya 服務", "Stopped managed Laya service"},
	{"ai.layaModelsInstalled", "已拉取模型：%s", "Pulled models: %s"},
	{"ai.layaModelPreset", "選擇部署模型", "Select model"},

	{"inspector.tabInfo", "資訊", "Info"},
	{"inspector.tabSimilar", "相似", "Similar"},
	{"inspector.similarLike", "與「%s」相似的素材", "Items like “%s”"},
	{"inspector.similarNone", "尚無高度相似的素材", "No similar items found"},

	{"viewer.back", "返回", "Back"},
	{"viewer.fit", "重設", "Fit"},

	{"goto.title", "快速跳轉與操作", "Go to"},
	{"goto.placeholder", "搜尋標籤、專案、白板、素材或快捷操作…", "Search tags, projects, boards, assets, or actions…"},
	{"goto.kindTag", "標籤", "Tag"},
	{"goto.kindProject", "專案", "Project"},
	{"goto.kindBoard", "白板", "Board"},
	{"goto.kindAsset", "素材", "Asset"},
	{"goto.kindAction", "操作", "Action"},
	{"goto.empty", "找不到相符的項目", "No matching items"},
	{"goto.actImport", "匯入檔案至資料庫", "Import files"},
	{"goto.actNewBoard", "建立新白板", "New board"},
	{"goto.actNewProject", "建立新專案", "New project"},
	{"goto.actBatchRename", "批量重新命名選取素材", "Batch rename selection"},
	{"goto.actSearchImage", "以圖搜圖（選擇外部圖片）", "Search by image"},
	{"goto.actToggleTheme", "切換深色 / 淺色主題", "Toggle dark / light theme"},
	{"goto.actToggleSidebar", "展開 / 收合左側邊欄", "Toggle left sidebar"},
	{"goto.actToggleInspector", "展開 / 收合右側資訊欄", "Toggle right inspector"},
	{"goto.actToggleServer", "啟動 / 停止內建伺服器", "Start / stop built-in server"},
	{"goto.actSettings", "開啟偏好設定", "Open preferences"},

	// Auto-updater (GitHub Releases).
	{"update.section", "軟體更新", "Software Updates"},
	{"update.current", "目前版本", "Current version"},
	{"update.autoCheck", "啟動時自動檢查 GitHub 更新", "Check GitHub for updates on launch"},
	{"update.betaChannel", "接收 Beta 測試版更新", "Include Beta pre-releases"},
	{"update.checkBtn", "檢查更新", "Check for Updates"},
	{"update.checking", "正在檢查 GitHub 最新版本…", "Checking GitHub for updates…"},
	{"update.upToDate", "目前已是最新版本（%s）", "You are up to date (%s)"},
	{"update.available", "發現新版本：%s", "New version available: %s"},
	{"update.toastAvailable", "發現新版本 %s，可於偏好設定一鍵更新", "Update %s available — install in Preferences"},
	{"update.installBtn", "下載並更新", "Download & Update"},
	{"update.downloading", "正在下載更新（%d%%）…", "Downloading update (%d%%)…"},
	{"update.verifying", "正在驗證 SHA-256 校驗碼…", "Verifying SHA-256 checksum…"},
	{"update.extracting", "正在解壓縮更新檔…", "Extracting update archive…"},
	{"update.installing", "正在套用更新…", "Installing update…"},
	{"update.ready", "更新已安裝完成，重新啟動即可套用新版本。", "Update installed. Restart Raglex to launch the new version."},
	{"update.restartBtn", "立即重新啟動", "Restart Now"},
	{"update.openRelease", "查看發行說明", "Release Notes"},
	{"update.noAsset", "此版本尚未提供目前平台的執行檔，請前往 GitHub 查看。", "No binary for this platform in release; open GitHub to view."},
	{"update.failed", "檢查或更新失敗：%s", "Update failed: %s"},

	// Asset Stacks (Version Stacking).
	{"stack.create", "合併為版本堆疊", "Stack Versions"},
	{"stack.created", "已合併 %d 個素材為版本堆疊", "Stacked %d assets together"},
	{"stack.expand", "展開堆疊（%d 個版本）", "Expand Stack (%d versions)"},
	{"stack.ungroup", "解散堆疊", "Ungroup Stack"},
	{"stack.ungrouped", "已解散版本堆疊", "Stack ungrouped"},
	{"stack.setCover", "設為堆疊封面", "Set as Stack Cover"},
	{"stack.coverUpdated", "已更新堆疊封面", "Stack cover updated"},
	{"stack.removeAsset", "移出此堆疊", "Remove from Stack"},
	{"stack.autoGroup", "自動堆疊相似版本（v1/v2/final）", "Auto-stack similar versions (v1/v2/final)"},
	{"stack.autoGrouped", "已自動建立 %d 組版本堆疊", "Auto-grouped %d version stacks"},
	{"stack.autoNone", "找不到可自動合併的相似版本檔名", "No matching version filenames found to stack"},
	{"stack.banner", "版本堆疊：%s（%d 個版本）", "Stack: %s (%d versions)"},
	{"stack.back", "返回列表", "Back to Library"},
	{"stack.versions", "版本", "Versions"},

	// Native Designer Format Previews (Font / Audio / SVG / PDF).
	{"preview.fontSample", "輸入自訂預覽文字…", "Type custom preview text…"},
	{"preview.fontGlyphs", "%d 個字形", "%d glyphs"},
	{"preview.audioPlay", "播放", "Play"},
	{"preview.audioPause", "暫停", "Pause"},
	{"preview.audioVolume", "音量", "Volume"},
	{"preview.svgVector", "SVG 無損向量", "SVG Lossless Vector"},
	{"preview.svgChecker", "透明棋盤格背景", "Transparency checkerboard"},
	{"preview.pdfPage", "第 %d / %d 頁", "Page %d / %d"},
	{"preview.pdfPrev", "上一頁", "Previous Page"},
	{"preview.pdfNext", "下一頁", "Next Page"},
	{"preview.pdfLoading", "正在渲染 PDF 頁面…", "Rendering PDF page…"},

	// PureRef-style Always-on-Top Floating Reference Window.
	{"pureref.open", "置頂參考浮窗", "Floating Ref Window"},
	{"pureref.pinWindow", "視窗置頂 (Always on Top)", "Pin Always on Top"},
	{"pureref.opacity", "透明度", "Opacity"},
	{"pureref.fit", "符合畫面 (F)", "Fit All (F)"},
	{"pureref.hint", "左鍵拖曳平移 · 滾輪縮放 · F 置中", "Drag to pan · Scroll to zoom · F to fit"},
	{"preview.quickLookHint", "再按一次 Space 進入完整檢視 · Esc 關閉", "Press Space again for full view · Esc to close"},
	{"preview.fullView", "完整檢視 (Space)", "Full View (Space)"},
	{"inspector.addTag", "加入標籤", "Add tag"},
	{"import.scanning", "正在掃描資料夾…", "Scanning folder…"},
	{"import.progressTitle", "正在匯入素材", "Importing Assets"},
	{"import.canceling", "正在取消…", "Canceling…"},
	{"import.canceled", "已取消匯入：已新增 %d，已存在 %d", "Import canceled: %d added, %d existing"},
	{"import.liveStats", "新增 %d · 已存在 %d", "%d added · %d existing"},

	// Global View Display Options, Thumbnail Backgrounds & Grayscale.
	{"view.layoutMode", "排版方式", "Layout Mode"},
	{"view.thumbBg", "縮圖背景", "Thumbnail Background"},
	{"view.thumbBgNone", "無", "None"},
	{"view.thumbBgBlack", "黑", "Black"},
	{"view.thumbBgGray", "灰", "Gray"},
	{"view.thumbBgWhite", "白", "White"},
	{"view.thumbBgChecker", "網格", "Checker"},
	{"view.showTitle", "顯示標題", "Show Titles"},
	{"view.showExt", "顯示副檔名", "Show Extension"},
	{"view.showRating", "顯示評分", "Show Rating"},
	{"view.grayscale", "黑白預覽", "Grayscale Preview"},
	{"view.showSidebar", "顯示側邊欄", "Show Sidebar"},
	{"view.showInspector", "顯示檢查器", "Show Inspector"},

	// Web URL Import (og:image auto-extraction).
	{"webimport.menu", "匯入網頁連結…", "Import Web URL…"},
	{"webimport.title", "從網頁連結匯入圖片", "Import Image from Web URL"},
	{"webimport.hint", "貼上任一網頁網址（自動擷取 og:image / 主圖）或圖片直連網址，並自動保留來源標題與連結。", "Paste any webpage URL (auto-fetches og:image / hero image) or direct image URL. Source title and link are saved automatically."},
	{"webimport.placeholder", "https://example.com/article 或圖片網址…", "https://example.com/page or image URL…"},
	{"webimport.action", "擷取並匯入", "Fetch & Import"},
	{"webimport.fetching", "正在擷取網頁與圖片…", "Fetching page & image…"},
	{"webimport.done", "已從網頁匯入：%s", "Imported from web: %s"},
	{"webimport.failed", "網頁匯入失敗：%s", "Web import failed: %s"},
	{"webimport.openPage", "在瀏覽器開啟網頁", "Open Webpage in Browser"},
	{"webimport.copyURL", "複製網頁連結", "Copy Webpage URL"},
	{"webimport.badge", "網頁", "Web"},

	// Video Player Timeline & Audio Controls.
	{"video.mute", "靜音", "Mute"},
	{"video.unmute", "取消靜音", "Unmute"},
	{"video.speed", "播放速度", "Playback Speed"},

	// Upgraded Board Studio Tools & Transparent Ref Window.
	{"board.toolPen", "畫筆 (P)", "Pen (P)"},
	{"board.toolHighlight", "螢光筆 (M)", "Highlighter (M)"},
	{"board.toolEraser", "橡皮擦 (E)", "Eraser (E)"},
	{"board.toolRect", "矩形 (R)", "Rectangle (R)"},
	{"board.toolArrow", "箭頭 (A)", "Arrow (A)"},
	{"board.toolFrame", "分區框 (F)", "Section Frame (F)"},
	{"board.strokeWidth", "筆畫粗細", "Stroke Width"},
	{"board.strokeColor", "標註顏色", "Annotation Color"},
	{"board.minimap", "導覽小地圖", "Minimap"},
	{"board.undo", "還原 (Ctrl+Z)", "Undo (Ctrl+Z)"},
	{"board.redo", "重做 (Ctrl+Shift+Z)", "Redo (Ctrl+Shift+Z)"},
	{"pureref.bgOpacity", "背景透明度", "Background Opacity"},
	{"pureref.imgOpacity", "圖片透明度", "Image Opacity"},
	{"pureref.transparentBg", "透明背景模式", "Transparent Background"},
	{"pureref.flipH", "水平翻轉 (H)", "Flip Horizontal (H)"},
	{"pureref.grayscale", "黑白對照 (G)", "Grayscale (G)"},
	{"pureref.clickThrough", "穿透點擊 (鎖定)", "Click-through Lock"},
	{"pureref.borderless", "無邊框極簡", "Borderless"},

	// Zeron-style Frosted Glass (Acrylic / Vibrancy) Theme.
	{"settings.preset.glass", "毛玻璃", "Glass"},
	{"settings.glassMode", "透明毛玻璃背景 (Acrylic)", "Frosted Glass Backdrop (Acrylic)"},
	{"settings.glassOpacity", "毛玻璃濃度", "Glass Density"},
	{"view.glassMode", "透明毛玻璃", "Frosted Glass"},

	// Redesigned Preferences (Clear, Human-Friendly Descriptions & MCP Hub).
	{"settings.sub.general", "調整外觀主題、毛玻璃效果、語言與日常檢視習慣。", "Customize theme, frosted glass, language, and daily workspace preferences."},
	{"settings.sub.database", "查看目前資料庫容量、一鍵備份還原與檔案維護工具。", "Inspect library storage, run backups, and maintain thumbnails or links."},
	{"settings.sub.import", "決定檔案進入資料庫的方式（保留原位索引或獨立複製）與自動化規則。", "Choose how files enter your library (index in place vs. copy) and import rules."},
	{"settings.sub.sharing", "區網裝置互傳、瀏覽器擴充連線與內建網頁圖庫。", "Share over LAN, connect the browser extension, or browse via built-in web gallery."},
	{"settings.sub.ai", "連接 Claude / Cursor / Antigravity MCP AI 助手，並管理本機視覺相似與 Laya 分類。", "Connect Claude, Cursor & Antigravity via MCP, plus local visual search and Laya tagging."},
	{"settings.sub.plugins", "管理安裝於本機資料庫中的擴充插件。", "Manage community plugins installed inside your library."},
	{"settings.glassDesc", "讓主視窗透出桌面柔和模糊背景，彈窗與右鍵選單維持清晰實心底色。", "Soft blurred desktop backdrop behind chrome while popups and menus stay crisp and solid."},
	{"settings.customColorsToggle", "選擇「自訂」風格即可微調畫布、面板、強調色與文字色。", "Pick the Custom preset above to fine-tune canvas, surface, accent, and text colors."},
	{"settings.kpiAssets", "總素材", "Assets"},
	{"settings.kpiTags", "標籤", "Tags"},
	{"settings.kpiProjects", "專案", "Projects"},
	{"settings.kpiBoards", "白板", "Boards"},
	{"settings.impAutoSelectDesc", "匯入完成後自動選取新項目並展開右側資訊欄。", "Select newly imported assets automatically and open the inspector."},
	{"settings.impAutoTagDesc", "若目前正在檢視某個標籤，新匯入素材會自動套用該標籤。", "Automatically apply the active tag filter to newly imported files."},
	{"settings.impHiddenDesc", "略過 .DS_Store、Thumbs.db 與隱藏資料夾，保持資料庫乾淨。", "Skip .DS_Store, Thumbs.db, and hidden dotfiles during import."},
	{"settings.impRecursiveDesc", "匯入資料夾時一併掃描所有子資料夾中的素材。", "Include nested subfolders when importing a folder."},
	{"settings.impDupDesc", "遇到內容雜湊相同的重複檔案時自動略過並顯示提醒。", "Skip byte-identical duplicates and show a summary toast."},
	{"mcp.section", "MCP AI 助手整合（17 項工具）", "MCP AI Assistant Integration (17 Tools)"},
	{"mcp.desc", "開啟內建伺服器後，Claude Desktop、Cursor、Antigravity 等 AI 代理就能直接搜尋你的靈感庫、匯入網頁書籤、管理專案，甚至在白板上自動建立分區框與箭頭。", "Enable the local server so Claude Desktop, Cursor, or Antigravity can search assets, save web bookmarks, organize projects, and build whiteboards with sections and arrows."},
	{"mcp.running", "MCP 服務運行中 · %s/mcp", "MCP Server Running · %s/mcp"},
	{"mcp.stopped", "MCP 服務未啟動（開啟下方開關即可連線）", "MCP Server Stopped (toggle on below to connect)"},
	{"mcp.toolsSummary", "支援工具：素材搜尋 / 詳細資訊 / 編輯評分與備註 / 以圖搜圖 / 匯入檔案與網頁 / 標籤與批量命名 / 專案管理 / 讀取與繪製白板元素 / Laya 分類", "Tools: Search, Get & Update Assets · Visual Search · Import Files & URLs · Tags & Batch Rename · Projects · Read & Draw Board Elements · Laya AI"},
	{"mcp.stepHint", "使用方式：1. 啟動伺服器 → 2. 點擊「複製 MCP 設定 JSON」→ 3. 貼入 Claude / Cursor / Antigravity 的 MCP 設定檔即可。", "How to connect: 1. Enable server → 2. Click Copy MCP Config JSON → 3. Paste into Claude / Cursor / Antigravity MCP settings."},

	// Interactive Hands-On Guided Tour (Step-by-step live walkthrough).
	{"tour.badge", "互動新手引導", "Interactive Guide"},
	{"tour.stepOf", "步驟 %d / %d", "Step %d of %d"},
	{"tour.skip", "略過教學", "Skip Tour"},
	{"tour.prev", "上一步", "Back"},
	{"tour.next", "下一步", "Next"},
	{"tour.done", "開始使用 Raglex", "Start Using Raglex"},
	{"tour.tryIt", "立即體驗", "Try It Now"},
	{"tour.tried", "已完成體驗", "Tried!"},

	{"tour.s1.title", "1. 匯入素材或網頁書籤", "1. Import Assets or Web Bookmarks"},
	{"tour.s1.desc", "Raglex 預設使用「索引模式」：只記錄原檔位置，不重複佔硬碟空間。你可以直接拖曳圖片/PSD/影片進來，或貼上網頁連結存成可雙擊開啟的視覺書籤。", "Raglex defaults to Index mode—tracking original files without duplicating disk space. Drop images, PSDs, or videos, or save live clickable web bookmarks."},
	{"tour.s1.act1", "匯入本機檔案…", "Import Files…"},
	{"tour.s1.act2", "匯入網頁連結…", "Import Web URL…"},

	{"tour.s2.title", "2. 點選檢視詳情，無選取時極簡收合", "2. Click to Inspect, Clean When Idle"},
	{"tour.s2.desc", "點選任一張素材卡片，右側資訊欄就會滑出顯示完整比例預覽、色彩分析、評分與標籤；按 Esc 或點右上角 × 即可收合。按 Space 進入縮放預覽，按 Ctrl+Alt+G 快速黑白對照。", "Click any card to slide open the right inspector with fitted preview, color palette, rating, and tags; press Esc or × to tuck it away. Press Space to zoom/pan or Ctrl+Alt+G for grayscale."},
	{"tour.s2.act1", "幫我選取第一張素材", "Select First Asset"},
	{"tour.s2.act2", "開啟縮放預覽 (Space)", "Open Zoom Preview (Space)"},

	{"tour.s3.title", "3. 右鍵選單與 Ctrl+T 快速跳轉", "3. Context Menu & Ctrl+T Quick Jump"},
	{"tour.s3.desc", "在任何素材上按右鍵可快速加入專案、送到白板、切換透明圖背景（黑/灰/白/網格）或尋找相似圖；按 Ctrl+T 隨時秒搜標籤、專案、白板與指令。", "Right-click any item to add to projects, send to whiteboards, switch transparent backgrounds, or find similar art. Press Ctrl+T anytime to jump anywhere instantly."},
	{"tour.s3.act1", "開啟快速跳轉 (Ctrl+T)", "Open Quick Jump (Ctrl+T)"},
	{"tour.s3.act2", "開啟標籤管理", "Open Tag Manager"},

	{"tour.s4.title", "4. Excalidraw 風格無限白板 & 透明 Ref 浮動視窗", "4. Infinite Whiteboard & Floating Ref Window"},
	{"tour.s4.desc", "白板支援圖片排板、向量箭頭（Shift 吸附角度）、可拖帶內部項目的分區框（Section）、無框文字、Ctrl+Z 還原與 Delete 刪除；點頂部 Ref 按鈕還可彈出可調背景透明度的置頂參考視窗！", "Arrange references on an infinite whiteboard with vector arrows, Section frames that move contained items, frameless text, Ctrl+Z undo, and a transparent always-on-top Ref window."},
	{"tour.s4.act1", "帶我打開白板試試看", "Open a Whiteboard Now"},

	{"tour.s5.title", "5. 挑選喜歡的主題風格與 AI / MCP", "5. Pick Your Look & Connect AI / MCP"},
	{"tour.s5.desc", "立刻試試毛玻璃（Acrylic）或深淺色主題！如果平常用 Claude、Cursor 或 Antigravity，也可以直接在偏好設定開啟內建 MCP 伺服器讓 AI 幫你整理靈感庫。", "Try Frosted Glass (Acrylic) or Dark/Light themes right here! You can also enable the built-in MCP server in Preferences so Claude, Cursor, or Antigravity can help organize your library."},
	{"tour.s5.themeDark", "深色", "Dark"},
	{"tour.s5.themeLight", "淺色", "Light"},
	{"tour.s5.themeGlass", "毛玻璃", "Frosted Glass"},
	{"tour.s5.openMCP", "前往 AI & MCP 設定", "Open AI & MCP Settings"},
}

var (
	zh map[string]string
	en map[string]string
)

func init() {
	zh = make(map[string]string, len(pairs))
	en = make(map[string]string, len(pairs))
	for _, p := range pairs {
		zh[p.key] = p.zh
		en[p.key] = p.en
	}
}

// T returns the string for key in lang. Unknown languages use English.
func T(lang, key string) string {
	table := en
	if lang == ZhHant {
		table = zh
	}
	if s, ok := table[key]; ok {
		return s
	}
	if s, ok := en[key]; ok {
		return s
	}
	return key
}

// Parity reports an empty string, a duplicate key, or a map that drifted
// from the pair list.
func Parity() error {
	seen := map[string]bool{}
	for _, p := range pairs {
		if p.key == "" || p.zh == "" || p.en == "" {
			return fmt.Errorf("i18n: empty catalog entry %q", p.key)
		}
		if seen[p.key] {
			return fmt.Errorf("i18n: duplicate key %s", p.key)
		}
		seen[p.key] = true
		if zh[p.key] != p.zh || en[p.key] != p.en {
			return fmt.Errorf("i18n: map mismatch %s", p.key)
		}
	}
	if len(zh) != len(pairs) || len(en) != len(pairs) {
		return fmt.Errorf("i18n: catalog length zh=%d en=%d pairs=%d", len(zh), len(en), len(pairs))
	}
	return nil
}

// Default follows the operating system. Chinese locales use Traditional Chinese.
func Default() string {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if isChinese(os.Getenv(env)) {
			return ZhHant
		}
	}
	if isChinese(osLocale()) {
		return ZhHant
	}
	return En
}

func isChinese(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "c" || s == "posix" {
		return false
	}
	return strings.HasPrefix(s, "zh") || strings.Contains(s, "chinese")
}
