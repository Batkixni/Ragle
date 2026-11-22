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
	{"welcome.restore", "從備份還原", "Restore backup"},
	{"welcome.lead", "選擇一個資料夾當作母庫。資料庫、縮圖，以及複製、移動或連結進來的檔案都會放在這裡。", "Pick a folder for the library. The database, thumbnails, and files you copy, move, or link live there."},
	{"welcome.openFailed", "這個資料夾的資料庫打不開。", "This library database could not be opened."},
	{"welcome.busy", "資料庫正在被別的程式使用。", "The database is in use by another process."},

	{"shell.library", "母庫", "Library"},
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
	{"library.deleteCopy", "這會刪除母庫裡的檔案，也會拿掉所有專案與白版上的引用。", "This deletes the file in the library and removes every reference on projects and boards."},
	{"library.deleteIndex", "這只會移除索引。你的原檔會留在原地。", "This removes the index only. Your original file stays where it is."},
	{"library.deleteLink", "這只會刪除母庫裡的連結。你的原檔會留在原地。", "This removes the library link only. Your original file stays where it is."},
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
	{"project.deleteNote", "會刪除這個專案的白版與放置。母庫裡的素材檔案會留下。", "This deletes the project's boards and placements. Library files stay."},
	{"project.needTag", "先選一個標籤，專案會建在它下面。", "Select a tag first. The project is created under it."},
	{"project.needName", "請輸入專案名稱。", "Enter a project name."},
	{"project.assets", "引用的素材", "Referenced assets"},
	{"project.boards", "白版", "Boards"},
	{"project.newBoard", "新白版", "New board"},
	{"project.emptyAssets", "還沒有引用素材。", "No assets referenced yet."},
	{"project.emptyBoards", "還沒有白版。", "No boards yet."},
	{"project.unreference", "移除引用", "Remove reference"},
	{"project.unreferenceNote", "會拿掉這個專案白版上該素材的放置。母庫檔案與其他專案會留下。", "This removes that asset's placements on this project's boards. The library file and other projects stay."},
	{"project.description", "說明", "Description"},
	{"project.fromLibrary", "從母庫引用", "Reference from library"},
	{"project.openBoard", "開啟", "Open"},
	{"project.deleteBoard", "刪除白版", "Delete board"},
	{"project.deleteBoardNote", "會刪除這面白版上的放置。母庫裡的素材會留下。", "This deletes the placements on this board. Library assets stay."},

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
	{"board.deleteNote", "只會從白版移除這些物件。母庫裡的素材會留下。", "This removes the objects from the board. Library assets stay."},
	{"board.leave", "返回專案", "Back to project"},
	{"board.trayProject", "專案", "Project"},
	{"board.trayLibrary", "母庫", "Library"},
	{"board.empty", "把素材拖到白版上，或用文字工具寫一句話。", "Drag an asset onto the board, or use the text tool."},

	{"share.send", "分享", "Share"},
	{"share.sendTo", "傳送", "Send"},
	{"share.title", "區域網", "On this network"},
	{"share.files", "包含素材檔案", "Include asset files"},
	{"share.none", "還沒有發現同一網路上的 Ragle。", "No other Ragle on this network yet."},
	{"share.sending", "正在傳送…", "Sending…"},
	{"share.receiving", "正在接收…", "Receiving…"},
	{"share.sent", "已送出。", "Sent."},
	{"share.incoming", "接收專案", "Incoming project"},
	{"share.incomingFiles", "%s 想傳送專案「%s」，包含 %d 個檔案。", "%s wants to send project %s, including %d files."},
	{"share.incomingMeta", "%s 想傳送專案「%s」的結構，不含檔案。", "%s wants to send the structure of project %s, without files."},
	{"share.accept", "接收", "Receive"},
	{"share.received", "已接收專案。", "Project received."},
	{"share.unavailable", "區域網分享現在不可用。母庫仍可離線使用。", "Sharing on this network is unavailable. The library still works offline."},
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
	{"inspector.placements", "白版放置 %d 次", "%d board placements"},
	{"inspector.actions", "操作", "Actions"},
	{"inspector.toggle", "切換資訊欄", "Toggle inspector"},
	{"sidebar.toggle", "切換側邊欄 (Ctrl+B)", "Toggle sidebar (Ctrl+B)"},
	{"inspector.quickTag", "新增並套用標籤…", "Add & apply tag…"},
	{"inspector.relink", "掃描資料夾重連…", "Relink from folder…"},
	{"inspector.relinkFile", "選取檔案重連…", "Relink file…"},
	{"inspector.relinkResult", "已重新連結 %d 個遺失檔案。", "Relinked %d missing files."},
	{"inspector.relinkNone", "沒有找到符合雜湊的遺失檔案。", "No missing files matched by hash."},

	{"shell.allAssets", "全部素材", "All Assets"},
	{"shell.untagged", "未分類", "Untagged"},
	{"shell.missing", "遺失檔案", "Missing"},
	{"shell.switchLib", "切換母庫資料夾…", "Switch library folder…"},
	{"shell.theme", "外觀主題", "Appearance"},
	{"shell.themeAuto", "跟隨系統", "System"},
	{"shell.themeLight", "淺色", "Light"},
	{"shell.themeDark", "深色", "Dark"},
	{"shell.language", "語言", "Language"},
	{"shell.localFirst", "本地母庫 · 離線可用", "Local-first · Offline ready"},

	{"library.search", "搜尋檔名、格式、標籤… (Ctrl+F)", "Search name, format, tags… (Ctrl+F)"},
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
	{"library.emptyHint", "拖曳圖片或資料夾到這裡，或點擊上方「匯入」開始建立你的參考母庫。", "Drag images or folders here, or click Import above to build your reference library."},
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
	{"project.emptyAssetsHint", "點擊上方「從母庫引用」將母庫素材加入此專案。", "Click “Reference from library” above to bring assets into this project."},

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
	{"settings.backupDone", "資料庫已備份至 ragle.db.bak。", "Database backed up to ragle.db.bak."},
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
	{"settings.shareSignalDesc", "同一區域網內會自動透過 mDNS 發現裝置，無需任何伺服器。跨網配對碼分享則使用您自架的 ragle-signal 服務。", "Devices on the same LAN discover each other via mDNS with no server. Pairing codes use your self-hosted ragle-signal server."},
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
	{"batch.deleteConfirm", "確定要刪除已選取的 %d 個素材嗎？此動作將依各自的存放方式處理檔案並移除白版引用。", "Delete %d selected assets? Files follow their storage mode and board references will be removed."},
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
	{"onboarding.start", "開始使用 Ragle", "Start using Ragle"},

	{"onboarding.s1.badge", "01 · 核心理念", "01 · CORE CONCEPT"},
	{"onboarding.s1.title", "歡迎使用 Ragle", "Welcome to Ragle"},
	{"onboarding.s1.lead", "為創作者與設計師打造的本地優先參考母庫與靈感白板。所有檔案與資料庫都留在你的電腦裡，完全離線運作。", "A local-first reference library and moodboard studio. Everything stays on your machine and works 100% offline."},
	{"onboarding.s1.c1.title", "本地母庫 · 四種匯入模式", "Local Library · 4 Storage Modes"},
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
	{"onboarding.k.import", "匯入檔案至母庫", "Import files"},
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

	// Stage 13: Standalone Boards.
	{"shell.boards", "獨立白板", "Boards"},
	{"board.newStandalone", "新獨立白板", "New standalone board"},
	{"board.leaveToLib", "返回母庫", "Back to library"},
	{"board.sendTo", "送到白板", "Send to board"},
	{"board.sendToNew", "+ 建立新白板並放置", "+ New board with selection"},
	{"board.addedTo", "已將 %d 個素材加入白板「%s」", "Added %d assets to board “%s”"},
	{"board.moveToProject", "歸類到專案", "Move to project"},
	{"board.movedToProject", "已將白板移入專案「%s」", "Moved board to project “%s”"},
	{"board.deleteBoard", "刪除白板", "Delete board"},
	{"board.deleteBoardConfirm", "確定要刪除獨立白板「%s」嗎？母庫中的原始素材不會被刪除。", "Delete standalone board “%s”? Original assets in your library will not be deleted."},
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
