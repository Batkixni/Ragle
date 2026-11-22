// Package ui is the Ragle window: welcome, library, project, and board.
package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	gui "github.com/egoist/mygo/ui"

	"ragle/internal/canvas"
	"ragle/internal/config"
	"ragle/internal/db"
	"ragle/internal/i18n"
	"ragle/internal/library"
	"ragle/internal/model"
	"ragle/internal/project"
	"ragle/internal/share"
	"ragle/internal/thumbnail"
	"ragle/internal/watcher"
)

type screen int

const (
	screenWelcome screen = iota
	screenLibrary
	screenProject
	screenBoard
)

const (
	sortNewest = iota
	sortOldest
	sortName
	sortRating
)

// assetDrag is the value dragged from a library card or the board tray.
type assetDrag struct {
	ID string
}

var storageModes = []string{
	model.StorageIndex,
	model.StorageSymlink,
	model.StorageMove,
	model.StorageCopy,
}

// App is the state of the single window. The view reads it on the main thread.
type App struct {
	cfg       config.Config
	lang      string
	themeMode string // "auto", "light", "dark"
	win       *mygo.Window

	mu     sync.Mutex
	closed bool

	screen screen
	status string

	lib      *library.Library
	projects *project.Store
	thumbs   *thumbnail.Service
	watch    *watcher.Watcher
	ffmpeg   bool

	node          *share.Node
	shareDB       atomic.Pointer[db.DB]
	shareOpen     bool
	shareFiles    bool
	shareDown     bool
	shareSending  bool
	shareAsking   bool
	signalURL     string
	pairCode      string
	pairCancel    context.CancelFunc
	pairRecvOpen  bool
	pairRecvCode  string
	pairReceiving bool

	openErr     error
	pendingRoot string
	canRestore  bool

	// Cached catalog data updated in reload() so View never hits bbolt per frame.
	tags           []model.Tag
	allAssets      []model.Asset
	assets         []model.Asset
	assetByID      map[string]model.Asset
	assetTags      map[string][]model.Tag
	tagCounts      map[string]int
	allAssetsCount int
	untaggedCount  int
	missingCount   int
	projectTags    map[string]bool
	boardCounts    map[string]int

	// Navigation, search, sort & library display state.
	tagID         string
	navFilter     string // "", "untagged", "missing"
	searchQuery   string
	searchHandle  gui.Handle
	kindFilter    int
	minRating     int
	sortBy        int
	grid          gui.GridState
	assetIndex    int
	cardSize      float64
	inspectorOpen bool
	filterBarOpen bool
	revealGen     int
	storageMode   int
	importing     bool

	// Inspector persistent bound state (avoids local-variable loss on rebuild).
	inspectorAssetID  string
	inspectorRating   int
	inspectorFileSize int64
	inspectorProjects []model.Project
	inspectorPlaces   int
	quickTagName      string

	// Quick Preview Lightbox state.
	previewOpen    bool
	previewAssetID string
	previewBitmap  *gui.Bitmap
	previewLoading bool

	// Project & board state.
	projectList   []model.Project
	projectID     string
	project       model.Project
	projectAssets []model.Asset
	boards        []model.Board
	referenceMode bool

	// Inline & modal creation/editing forms.
	newTagOpen     bool
	newTagName     string
	newProjOpen    bool
	editProjOpen   bool
	newProjectName string
	newProjectDesc string
	newProjTags    map[string]bool
	newBoardName   string

	// Board state.
	boardID            string
	boardName          string
	boardProject       string
	session            *canvas.Session
	tool               int
	tray               int
	boardDrawerOpen    bool
	boardInspectorOpen bool
	gesture            gesture
	spaceDown          bool
	focusBoard         bool
	focusBody          bool
	boundID            string
	notesBuf           string
	editText           string
	fontSizeBuf        float64
	flushArmed         bool

	bitmaps map[string]*gui.Bitmap
	loading map[string]bool
	queued  map[string]bool

	askOpen    bool
	askTitle   string
	askBody    string
	askConfirm string
	askFn      func()
	askCancel  func()

	sidebarWidth   float32
	libraryPercent float32
	selectedNav    string
	lastNav        string

	dark   bool
	pal    palette
	paper  gui.Color
	ink    gui.Color
	accent gui.Color
	muted  gui.Color
	line   gui.Color
}

// NewApp starts on the welcome screen. Language follows the OS until a
// preference is loaded.
func NewApp() *App {
	return &App{
		lang:               i18n.Default(),
		themeMode:          "auto",
		screen:             screenWelcome,
		shareFiles:         true,
		assetIndex:         -1,
		cardSize:           184,
		inspectorOpen:      true,
		filterBarOpen:      false,
		boardDrawerOpen:    true,
		boardInspectorOpen: true,
		bitmaps:            map[string]*gui.Bitmap{},
		loading:            map[string]bool{},
		queued:             map[string]bool{},
		assetByID:          map[string]model.Asset{},
		assetTags:          map[string][]model.Tag{},
		tagCounts:          map[string]int{},
		projectTags:        map[string]bool{},
		boardCounts:        map[string]int{},
		newProjTags:        map[string]bool{},
		sidebarWidth:       232,
		libraryPercent:     74,
		selectedNav:        "tag-all",
		lastNav:            "tag-all",
	}
}

// Run opens the window and, when a library path was saved, opens that folder.
func Run() error {
	a := NewApp()
	if cfg, err := config.Load(); err == nil {
		a.cfg = cfg
		if cfg.Language == i18n.ZhHant || cfg.Language == i18n.En {
			a.lang = cfg.Language
		}
		if cfg.SignalingURL != "" {
			a.signalURL = cfg.SignalingURL
		}
		if cfg.ThemeMode == "auto" || cfg.ThemeMode == "light" || cfg.ThemeMode == "dark" {
			a.themeMode = cfg.ThemeMode
		}
		if cfg.CardSize >= 120 && cfg.CardSize <= 320 {
			a.cardSize = cfg.CardSize
		}
	}
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Ragle",
			Width:     1360,
			Height:    860,
			MinWidth:  980,
			MinHeight: 640,
			StateKey:  "main",
			Content:   gui.View(a.View),
		})
		a.win.OnClosed(func() { a.shutdown() })
		a.startShare()
		if a.cfg.LibraryPath != "" {
			root := a.cfg.LibraryPath
			go a.openRoot(root)
		}
	})
	return mygo.App.Run()
}

func (a *App) t(key string) string { return i18n.T(a.lang, key) }

func (a *App) setLang(lang string) {
	if lang != i18n.ZhHant && lang != i18n.En {
		return
	}
	a.lang = lang
	a.cfg.Language = lang
	a.saveConfig()
}

func (a *App) setThemeMode(mode string) {
	if mode != "auto" && mode != "light" && mode != "dark" {
		return
	}
	a.themeMode = mode
	a.cfg.ThemeMode = mode
	a.saveConfig()
}

func (a *App) saveConfig() {
	if a.win != nil {
		_ = config.Save(a.cfg)
	}
}

func (a *App) shutdown() {
	a.mu.Lock()
	a.closed = true
	node := a.node
	a.node = nil
	pairCancel := a.pairCancel
	a.pairCancel = nil
	a.mu.Unlock()
	if pairCancel != nil {
		pairCancel()
	}
	a.shareDB.Store(nil)
	if node != nil {
		_ = node.Close()
	}
	a.flushBoard(true)
	watch, thumbs, lib := a.watch, a.thumbs, a.lib
	a.watch, a.thumbs, a.lib = nil, nil, nil
	a.session = nil
	if watch != nil {
		_ = watch.Close()
	}
	if thumbs != nil {
		thumbs.Close()
	}
	if lib != nil {
		_ = lib.Close()
	}
}

func (a *App) openRoot(root string) {
	lib, err := library.Open(root)
	if a.win == nil {
		if lib != nil {
			_ = lib.Close()
		}
		return
	}
	a.win.Update(func() {
		a.mu.Lock()
		closed := a.closed
		a.mu.Unlock()
		if closed {
			if lib != nil {
				_ = lib.Close()
			}
			return
		}
		if err != nil {
			a.openErr = err
			a.pendingRoot = root
			a.canRestore = errors.Is(err, db.ErrCorrupt) && db.HasBackup(root)
			a.screen = screenWelcome
			if errors.Is(err, db.ErrBusy) {
				a.status = a.t("welcome.busy")
			} else {
				a.status = a.t("welcome.openFailed")
			}
			return
		}
		a.attach(lib, root)
	})
}

func (a *App) attach(lib *library.Library, root string) {
	a.detach()
	a.lib = lib
	a.shareDB.Store(lib.DB)
	a.projects = project.New(lib.DB)
	a.cfg.LibraryPath = root
	a.saveConfig()
	win := a.win
	a.thumbs = thumbnail.Start(lib.DB, func(id string) {
		if win == nil {
			return
		}
		win.Update(func() { a.onThumb(id) })
	})
	w, err := watcher.Start(lib, func() {
		if win == nil {
			return
		}
		win.Update(func() {
			if a.isClosed() || a.lib == nil {
				return
			}
			a.reload()
		})
	})
	if err == nil {
		a.watch = w
	}
	a.ffmpeg = thumbnail.Available()
	a.screen = screenLibrary
	a.openErr = nil
	a.canRestore = false
	a.tagID = ""
	a.navFilter = ""
	a.searchQuery = ""
	a.assetIndex = -1
	a.projectID = ""
	a.project = model.Project{}
	a.previewOpen = false
	a.bitmaps = map[string]*gui.Bitmap{}
	a.loading = map[string]bool{}
	a.queued = map[string]bool{}
	a.reload()
	if !a.ffmpeg {
		a.status = a.t("library.noFfmpeg")
	} else if a.shareDown {
		a.status = a.t("share.unavailable")
	} else {
		a.status = ""
	}
}

func (a *App) detach() {
	a.shareDB.Store(nil)
	if a.watch != nil {
		_ = a.watch.Close()
		a.watch = nil
	}
	if a.thumbs != nil {
		a.thumbs.Close()
		a.thumbs = nil
	}
	if a.lib != nil {
		_ = a.lib.Close()
		a.lib = nil
	}
	a.projects = nil
	a.session = nil
}

func (a *App) isClosed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}

func (a *App) reload() {
	if a.lib == nil {
		return
	}
	tags, err := a.lib.Tags()
	if err != nil {
		a.status = err.Error()
		return
	}
	a.tags = tags

	all, err := a.lib.List(library.Filter{})
	if err != nil {
		a.status = err.Error()
		return
	}
	a.allAssets = all
	a.allAssetsCount = len(all)
	a.assetByID = make(map[string]model.Asset, len(all))
	a.assetTags = make(map[string][]model.Tag, len(all))
	a.tagCounts = make(map[string]int, len(tags))
	a.untaggedCount = 0
	a.missingCount = 0

	for _, asset := range all {
		a.assetByID[asset.ID] = asset
		if asset.Missing {
			a.missingCount++
		}
		atags, _ := a.lib.AssetTags(asset.ID)
		a.assetTags[asset.ID] = atags
		if len(atags) == 0 {
			a.untaggedCount++
		}
		for _, tg := range atags {
			a.tagCounts[tg.ID]++
		}
		if asset.ThumbnailPath != "" {
			a.ensureBitmap(asset.ID, asset.ThumbnailPath)
		} else {
			a.queueThumb(asset)
		}
	}

	a.applyFilters()

	if a.projects != nil {
		if a.tagID == "" {
			a.projectList, err = a.projects.List()
		} else {
			a.projectList, err = a.projects.ListByTag(a.tagID)
		}
		if err != nil {
			a.status = err.Error()
			return
		}
	}

	a.projectTags = map[string]bool{}
	a.boardCounts = map[string]int{}
	if a.projectID != "" && a.projects != nil {
		projects, err := a.projects.List()
		if err != nil {
			a.status = err.Error()
			return
		}
		found := false
		for _, p := range projects {
			if p.ID == a.projectID {
				a.project = p
				found = true
				break
			}
		}
		if !found {
			a.projectID = ""
			a.project = model.Project{}
			a.projectAssets = nil
			a.boards = nil
			if a.screen == screenProject || a.screen == screenBoard {
				a.screen = screenLibrary
				a.session = nil
			}
		} else {
			if ids, err := a.projects.TagIDs(a.projectID); err == nil {
				for _, id := range ids {
					a.projectTags[id] = true
				}
			}
			a.projectAssets, err = a.projects.Assets(a.projectID)
			if err != nil {
				a.status = err.Error()
				return
			}
			a.boards, err = a.projects.Boards(a.projectID)
			if err != nil {
				a.status = err.Error()
				return
			}
			for _, b := range a.boards {
				if items, err := a.projects.LoadPlacements(b.ID); err == nil {
					a.boardCounts[b.ID] = len(items)
				}
			}
		}
	}
	if a.watch != nil {
		_ = a.watch.Sync()
	}
	a.syncInspectorSelection(true)
	a.syncNavSelection()
}

// applyFilters filters & sorts a.allAssets into a.assets in memory (instant search/sort).
func (a *App) applyFilters() {
	var selectedID string
	if cur, ok := a.selectedAsset(); ok {
		selectedID = cur.ID
	}

	kind := a.kindValue()
	q := strings.ToLower(strings.TrimSpace(a.searchQuery))
	out := make([]model.Asset, 0, len(a.allAssets))

	for _, asset := range a.allAssets {
		if a.navFilter == "missing" && !asset.Missing {
			continue
		}
		if a.navFilter == "untagged" && len(a.assetTags[asset.ID]) > 0 {
			continue
		}
		if a.tagID != "" && !a.hasTag(asset.ID, a.tagID) {
			continue
		}
		if a.minRating > 0 && asset.Rating < a.minRating {
			continue
		}
		if kind != "" && model.MediaKind(asset.Mime) != kind {
			continue
		}
		if q != "" && !a.matchesQuery(asset, q) {
			continue
		}
		out = append(out, asset)
	}

	switch a.sortBy {
	case sortOldest:
		sort.Slice(out, func(i, j int) bool {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		})
	case sortName:
		sort.Slice(out, func(i, j int) bool {
			return strings.ToLower(a.assetName(out[i])) < strings.ToLower(a.assetName(out[j]))
		})
	case sortRating:
		sort.Slice(out, func(i, j int) bool {
			if out[i].Rating != out[j].Rating {
				return out[i].Rating > out[j].Rating
			}
			return out[i].CreatedAt.After(out[j].CreatedAt)
		})
	default: // sortNewest
		sort.Slice(out, func(i, j int) bool {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		})
	}

	a.assets = out
	if selectedID != "" {
		a.assetIndex = -1
		for i, item := range a.assets {
			if item.ID == selectedID {
				a.assetIndex = i
				break
			}
		}
	} else if a.assetIndex >= len(a.assets) {
		a.assetIndex = -1
	}
	a.syncInspectorSelection(false)
}

func (a *App) hasTag(assetID, tagID string) bool {
	for _, tg := range a.assetTags[assetID] {
		if tg.ID == tagID {
			return true
		}
	}
	return false
}

func (a *App) matchesQuery(asset model.Asset, q string) bool {
	name := strings.ToLower(a.assetName(asset))
	if strings.Contains(name, q) {
		return true
	}
	if strings.Contains(strings.ToLower(asset.Mime), q) {
		return true
	}
	if strings.Contains(strings.ToLower(asset.Path), q) {
		return true
	}
	for _, tg := range a.assetTags[asset.ID] {
		if strings.Contains(strings.ToLower(tg.Name), q) {
			return true
		}
	}
	return false
}

func (a *App) syncInspectorSelection(force bool) {
	asset, ok := a.selectedAsset()
	if !ok {
		a.inspectorAssetID = ""
		a.inspectorRating = 0
		a.inspectorFileSize = 0
		a.inspectorProjects = nil
		a.inspectorPlaces = 0
		return
	}
	if !force && a.inspectorAssetID == asset.ID {
		return
	}
	a.inspectorAssetID = asset.ID
	a.inspectorRating = asset.Rating
	a.inspectorFileSize = 0
	if !asset.Missing && asset.Path != "" {
		if fi, err := os.Stat(asset.Path); err == nil {
			a.inspectorFileSize = fi.Size()
		}
	}
	a.inspectorProjects = nil
	a.inspectorPlaces = 0
	if a.projects != nil {
		if projs, places, err := a.projects.UsageOfAsset(asset.ID); err == nil {
			a.inspectorProjects = projs
			a.inspectorPlaces = places
		}
	}
	if a.bitmaps[asset.ID] == nil && !asset.Missing && model.MediaKind(asset.Mime) == model.MediaImage && asset.Path != "" {
		a.ensureBitmap(asset.ID, asset.Path)
	}
}

func (a *App) hasActiveFilters() bool {
	return a.searchQuery != "" || a.kindFilter != 0 || a.minRating != 0
}

func (a *App) clearFilters() {
	a.searchQuery = ""
	a.kindFilter = 0
	a.minRating = 0
	a.applyFilters()
}

func (a *App) kindValue() string {
	switch a.kindFilter {
	case 1:
		return model.MediaImage
	case 2:
		return model.MediaVideo
	case 3:
		return model.MediaOther
	default:
		return ""
	}
}

func (a *App) queueThumb(asset model.Asset) {
	if a.thumbs == nil || asset.ThumbnailPath != "" || asset.Missing || a.queued[asset.ID] {
		return
	}
	if model.MediaKind(asset.Mime) == model.MediaOther {
		return
	}
	a.queued[asset.ID] = true
	a.thumbs.Enqueue(asset.ID)
}

func (a *App) onThumb(id string) {
	if a.lib == nil {
		return
	}
	asset, err := a.lib.Get(id)
	if err != nil {
		return
	}
	a.assetByID[id] = asset
	for i := range a.allAssets {
		if a.allAssets[i].ID == id {
			a.allAssets[i] = asset
		}
	}
	for i := range a.assets {
		if a.assets[i].ID == id {
			a.assets[i] = asset
		}
	}
	for i := range a.projectAssets {
		if a.projectAssets[i].ID == id {
			a.projectAssets[i] = asset
		}
	}
	if asset.ThumbnailPath != "" {
		a.ensureBitmap(id, asset.ThumbnailPath)
	}
}

func (a *App) ensureBitmap(id, path string) {
	if path == "" || a.bitmaps[id] != nil || a.loading[id] || a.win == nil {
		return
	}
	a.loading[id] = true
	win := a.win
	go func() {
		data, err := os.ReadFile(path)
		win.Update(func() {
			delete(a.loading, id)
			if a.lib == nil || err != nil {
				return
			}
			bm, decErr := gui.DecodeBitmap(data)
			if decErr != nil {
				return
			}
			a.bitmaps[id] = bm
		})
	}()
}

// openPreview opens the full-resolution Quick Preview lightbox for asset.
func (a *App) openPreview(asset model.Asset) {
	if asset.ID == "" {
		return
	}
	a.previewOpen = true
	if a.previewAssetID == asset.ID && (a.previewBitmap != nil || a.previewLoading) {
		return
	}
	a.previewAssetID = asset.ID
	a.previewBitmap = nil
	a.previewLoading = false

	if asset.Missing || model.MediaKind(asset.Mime) != model.MediaImage || asset.Path == "" {
		return
	}
	if a.win == nil {
		a.previewBitmap = a.bitmaps[asset.ID]
		return
	}
	a.previewLoading = true
	win := a.win
	targetID := asset.ID
	path := asset.Path
	go func() {
		data, err := os.ReadFile(path)
		var bm *gui.Bitmap
		if err == nil {
			bm, _ = gui.DecodeBitmap(data)
		}
		win.Update(func() {
			if a.previewAssetID != targetID {
				return
			}
			a.previewLoading = false
			if bm != nil {
				a.previewBitmap = bm
			}
		})
	}()
}

func (a *App) stepPreview(delta int) {
	if !a.previewOpen || len(a.assets) == 0 {
		return
	}
	idx := -1
	for i, item := range a.assets {
		if item.ID == a.previewAssetID {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	} else {
		idx = (idx + delta + len(a.assets)) % len(a.assets)
	}
	a.assetIndex = idx
	a.syncInspectorSelection(true)
	a.openPreview(a.assets[idx])
}

func (a *App) chooseLibrary() {
	if a.win == nil {
		return
	}
	win := a.win
	title := a.t("welcome.choose")
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    win,
			Title:     title,
			Directory: true,
		})
		if err != nil || len(paths) == 0 {
			if err != nil && win != nil {
				win.Update(func() {
					if !a.isClosed() {
						a.status = err.Error()
					}
				})
			}
			return
		}
		a.openRoot(paths[0])
	}()
}

func (a *App) restoreLibrary() {
	root := a.pendingRoot
	if root == "" || a.win == nil {
		return
	}
	go func() {
		err := db.Restore(root)
		if err != nil {
			a.win.Update(func() { a.status = err.Error() })
			return
		}
		a.openRoot(root)
	}()
}

func (a *App) startImport(directory bool) {
	if a.importing || a.win == nil || a.lib == nil {
		return
	}
	title := a.t("library.files")
	if directory {
		title = a.t("library.folder")
	}
	win := a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    win,
			Title:     title,
			Directory: directory,
			Multiple:  !directory,
		})
		if err != nil || len(paths) == 0 {
			if err != nil {
				win.Update(func() {
					if !a.isClosed() {
						a.status = err.Error()
					}
				})
			}
			return
		}
		win.Update(func() {
			if !a.isClosed() {
				a.importPaths(paths)
			}
		})
	}()
}

// importPaths imports files or directories from either the file picker or OS drag-and-drop.
func (a *App) importPaths(paths []string) {
	if a.importing || a.lib == nil || len(paths) == 0 {
		return
	}
	a.importing = true
	lib := a.lib
	mode := storageModes[a.storageMode]
	var tags []string
	if a.tagID != "" {
		tags = []string{a.tagID}
	}
	win := a.win
	go func() {
		res, importErr := lib.Import(nilContext(), paths, mode, tags, func(done, total int) {
			if win == nil {
				return
			}
			d, n := done, total
			win.Update(func() {
				if a.isClosed() {
					return
				}
				a.status = fmt.Sprintf(a.t("library.importing"), d, n)
			})
		})
		if win == nil {
			return
		}
		win.Update(func() {
			a.importing = false
			if a.isClosed() || a.lib == nil {
				return
			}
			if importErr != nil {
				a.status = importErr.Error()
			} else {
				a.status = fmt.Sprintf(a.t("library.imported"), res.Added, res.Existing, res.Failed)
				for _, item := range res.Items {
					if item.Status == "added" && a.thumbs != nil {
						a.thumbs.Enqueue(item.AssetID)
						a.queued[item.AssetID] = true
					}
					if item.Err != nil && errors.Is(item.Err, library.ErrSymlink) {
						a.status = a.t("library.symlinkFailed")
					}
				}
			}
			a.reload()
		})
	}()
}

func (a *App) relinkMissing(directory bool) {
	if a.win == nil || a.lib == nil {
		return
	}
	win := a.win
	lib := a.lib
	title := a.t("inspector.relink")
	if !directory {
		title = a.t("inspector.relinkFile")
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    win,
			Title:     title,
			Directory: directory,
			Multiple:  !directory,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		count, relinkErr := lib.RelinkPaths(context.Background(), paths)
		win.Update(func() {
			if a.isClosed() || a.lib == nil {
				return
			}
			if relinkErr != nil {
				a.status = relinkErr.Error()
				return
			}
			if count > 0 {
				a.status = fmt.Sprintf(a.t("inspector.relinkResult"), count)
			} else {
				a.status = a.t("inspector.relinkNone")
			}
			a.reload()
		})
	}()
}

func (a *App) ask(title, body, confirm string, fn func()) {
	a.askBoth(title, body, confirm, fn, nil)
}

func (a *App) askBoth(title, body, confirm string, yes, no func()) {
	a.askTitle = title
	a.askBody = body
	a.askConfirm = confirm
	a.askFn = yes
	a.askCancel = no
	a.askOpen = true
}

func (a *App) confirm(c *gui.Context) {
	if a.askConfirm == "" {
		a.askConfirm = a.t("common.delete")
	}
	switch gui.AlertDialog(c, &a.askOpen, a.askTitle, a.askBody, a.t("common.cancel"), a.askConfirm) {
	case 1:
		fn := a.askFn
		a.askFn = nil
		a.askCancel = nil
		if fn != nil {
			fn()
		}
	case 0:
		if a.askOpen {
			return
		}
		a.dismissAsk()
	case gui.AlertDismissed:
		a.dismissAsk()
	}
}

func (a *App) dismissAsk() {
	cancel := a.askCancel
	a.askFn = nil
	a.askCancel = nil
	if cancel != nil {
		cancel()
	}
}

func (a *App) selectedAsset() (model.Asset, bool) {
	if a.assetIndex < 0 || a.assetIndex >= len(a.assets) {
		return model.Asset{}, false
	}
	return a.assets[a.assetIndex], true
}

func (a *App) createTag() {
	name := trim(a.newTagName)
	if name == "" {
		a.status = a.t("tag.needName")
		return
	}
	tag, err := a.lib.CreateTag(name)
	if err != nil {
		if errors.Is(err, library.ErrDuplicateName) {
			a.status = a.t("tag.exists")
			return
		}
		a.status = err.Error()
		return
	}
	a.newTagName = ""
	a.newTagOpen = false
	a.tagID = tag.ID
	a.navFilter = ""
	a.reload()
}

func (a *App) quickAddAssetTag(assetID string) {
	name := trim(a.quickTagName)
	if name == "" || a.lib == nil {
		return
	}
	var targetID string
	for _, tg := range a.tags {
		if strings.EqualFold(tg.Name, name) {
			targetID = tg.ID
			break
		}
	}
	if targetID == "" {
		created, err := a.lib.CreateTag(name)
		if err != nil {
			a.status = err.Error()
			return
		}
		targetID = created.ID
	}
	if err := a.lib.SetAssetTag(assetID, targetID, true); err != nil {
		a.status = err.Error()
		return
	}
	a.quickTagName = ""
	a.reload()
}

func (a *App) openNewProjectModal() {
	a.newProjectName = ""
	a.newProjectDesc = ""
	a.newProjTags = map[string]bool{}
	if a.tagID != "" {
		a.newProjTags[a.tagID] = true
	} else if len(a.tags) > 0 {
		a.newProjTags[a.tags[0].ID] = true
	}
	a.newProjOpen = true
}

func (a *App) openEditProjectModal() {
	if a.projectID == "" {
		return
	}
	a.newProjectName = a.project.Name
	a.newProjectDesc = a.project.Description
	a.editProjOpen = true
}

func (a *App) saveProjectFromModal() {
	name := trim(a.newProjectName)
	if name == "" {
		a.status = a.t("project.needName")
		return
	}
	if a.projects == nil || a.projectID == "" {
		return
	}
	if _, err := a.projects.Update(a.projectID, name, trim(a.newProjectDesc)); err != nil {
		a.status = err.Error()
		return
	}
	a.editProjOpen = false
	a.reload()
}

func (a *App) createProjectFromModal() {
	name := trim(a.newProjectName)
	if name == "" {
		a.status = a.t("project.needName")
		return
	}
	var tagIDs []string
	for _, tg := range a.tags {
		if a.newProjTags[tg.ID] {
			tagIDs = append(tagIDs, tg.ID)
		}
	}
	if len(tagIDs) == 0 {
		a.status = a.t("project.needTag")
		return
	}
	p, err := a.projects.Create(name, trim(a.newProjectDesc), tagIDs)
	if err != nil {
		a.status = err.Error()
		return
	}
	a.newProjOpen = false
	a.newProjectName = ""
	a.newProjectDesc = ""
	a.openProject(p.ID)
}

func (a *App) openProject(id string) {
	a.flushBoard(true)
	a.session = nil
	a.boardID = ""
	a.boardProject = ""
	a.projectID = id
	a.referenceMode = false
	a.screen = screenProject
	a.reload()
}

func (a *App) openBoard(b model.Board) {
	a.flushBoard(true)
	items, err := a.projects.LoadPlacements(b.ID)
	if err != nil {
		a.status = err.Error()
		return
	}
	a.session = canvas.NewSession(items)
	a.boardID = b.ID
	a.boardName = b.Name
	a.boardProject = b.ProjectID
	a.projectID = b.ProjectID
	a.screen = screenBoard
	a.gesture = gesture{}
	a.boundID = ""
	a.focusBoard = true
	a.reload()
}

func (a *App) referenceAssetToProject(assetID, projectID string) {
	if assetID == "" || projectID == "" || a.projects == nil {
		return
	}
	if err := a.projects.Reference(projectID, assetID); err != nil {
		a.status = err.Error()
		return
	}
	for _, p := range a.projectList {
		if p.ID == projectID {
			a.status = fmt.Sprintf(a.t("library.referencedIn"), p.Name)
			break
		}
	}
	a.reload()
}

func (a *App) referenceSelected() {
	asset, ok := a.selectedAsset()
	if !ok || a.projectID == "" {
		return
	}
	a.referenceAssetToProject(asset.ID, a.projectID)
}

func (a *App) deleteAssetPrompt(asset model.Asset) {
	id := asset.ID
	a.ask(a.t("library.delete"), a.deleteAssetCopy(asset), a.t("common.delete"), func() {
		if a.session != nil {
			var drop []string
			for _, item := range a.session.Items {
				if item.Kind == model.KindImage && item.AssetID == id {
					drop = append(drop, item.ID)
				}
			}
			if len(drop) > 0 {
				a.session.Delete(drop)
				a.flushBoard(true)
			}
		}
		if err := a.lib.Delete(id); err != nil {
			a.status = err.Error()
			return
		}
		delete(a.bitmaps, id)
		if a.previewAssetID == id {
			a.previewOpen = false
			a.previewBitmap = nil
		}
		a.assetIndex = -1
		a.reload()
	})
}

func (a *App) assetName(asset model.Asset) string {
	name := filepath.Base(asset.Path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = asset.ID
	}
	return name
}

func (a *App) assetExt(asset model.Asset) string {
	ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(asset.Path), "."))
	if ext == "" {
		return "FILE"
	}
	return ext
}

func (a *App) modeLabel(mode string) string {
	switch mode {
	case model.StorageSymlink:
		return a.t("library.mode.symlink")
	case model.StorageMove:
		return a.t("library.mode.move")
	case model.StorageCopy:
		return a.t("library.mode.copy")
	default:
		return a.t("library.mode.index")
	}
}

func (a *App) deleteAssetCopy(asset model.Asset) string {
	switch asset.StorageMode {
	case model.StorageCopy, model.StorageMove:
		return a.t("library.deleteCopy")
	case model.StorageSymlink:
		return a.t("library.deleteLink")
	default:
		return a.t("library.deleteIndex")
	}
}

func (a *App) handleNav(nav string) {
	if nav == "" {
		return
	}
	a.selectedNav = nav
	a.lastNav = nav
	a.revealGen++
	switch {
	case nav == "library" || nav == "tag-all":
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = ""
		a.navFilter = ""
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case nav == "filter-untagged":
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = ""
		a.navFilter = "untagged"
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case nav == "filter-missing":
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = ""
		a.navFilter = "missing"
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case strings.HasPrefix(nav, "tag:"):
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = strings.TrimPrefix(nav, "tag:")
		a.navFilter = ""
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case strings.HasPrefix(nav, "project:"):
		pid := strings.TrimPrefix(nav, "project:")
		a.openProject(pid)
	}
}

func (a *App) syncNavSelection() {
	if a.screen == screenProject || a.screen == screenBoard {
		if a.projectID != "" {
			a.selectedNav = "project:" + a.projectID
			a.lastNav = a.selectedNav
		}
	} else if a.screen == screenLibrary {
		switch {
		case a.navFilter == "untagged":
			a.selectedNav = "filter-untagged"
		case a.navFilter == "missing":
			a.selectedNav = "filter-missing"
		case a.tagID != "":
			a.selectedNav = "tag:" + a.tagID
		default:
			a.selectedNav = "tag-all"
		}
		a.lastNav = a.selectedNav
	}
}
