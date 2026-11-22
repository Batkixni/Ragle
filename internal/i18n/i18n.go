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
