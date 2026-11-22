// Package ui is the Ragle window: welcome, library, project, and board.
package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	cfg  config.Config
	lang string
	win  *mygo.Window

	mu     sync.Mutex
	closed bool

	screen screen
	status string

	lib      *library.Library
	projects *project.Store
	thumbs   *thumbnail.Service
	watch    *watcher.Watcher
	ffmpeg   bool

	node         *share.Node
	shareDB      atomic.Pointer[db.DB]
	shareOpen    bool
	shareFiles   bool
	shareDown    bool
	shareSending bool
	shareAsking  bool

	openErr     error
	pendingRoot string
	canRestore  bool

	tags        []model.Tag
	assets      []model.Asset
	assetByID   map[string]model.Asset
	tagID       string
	kindFilter  int
	minRating   int
	grid        gui.GridState
	assetIndex  int
	storageMode int
	importing   bool

	projectList   []model.Project
	projectID     string
	project       model.Project
	projectAssets []model.Asset
	boards        []model.Board
	referenceMode bool

	newTagName     string
	newProjectName string
	newProjectDesc string
	newBoardName   string

	boardID      string
	boardName    string
	boardProject string
	session      *canvas.Session
	tool         int
	tray         int
	gesture      gesture
	spaceDown    bool
	focusBoard   bool
	focusBody    bool
	boundID      string
	notesBuf     string
	editText     string
	flushArmed   bool

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
		lang:           i18n.Default(),
		screen:         screenWelcome,
		shareFiles:     true,
		assetIndex:     -1,
		bitmaps:        map[string]*gui.Bitmap{},
		loading:        map[string]bool{},
		queued:         map[string]bool{},
		assetByID:      map[string]model.Asset{},
		sidebarWidth:   220,
		libraryPercent: 72,
		selectedNav:    "tag-all",
		lastNav:        "tag-all",
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
	}
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Ragle",
			Width:     1280,
			Height:    800,
			MinWidth:  960,
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
	if a.win != nil {
		_ = config.Save(a.cfg)
	}
}

func (a *App) shutdown() {
	a.mu.Lock()
	a.closed = true
	node := a.node
	a.node = nil
	a.mu.Unlock()
	// Drop the library pointer before waiting, so a transfer blocked on the
	// accept dialog wakes up from the node context and does not touch the DB.
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
	if a.win != nil {
		_ = config.Save(a.cfg)
	}
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
	a.assetIndex = -1
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
	a.assetByID = make(map[string]model.Asset, len(all))
	for _, asset := range all {
		a.assetByID[asset.ID] = asset
		if asset.ThumbnailPath != "" {
			a.ensureBitmap(asset.ID, asset.ThumbnailPath)
		} else {
			a.queueThumb(asset)
		}
	}
	filtered, err := a.lib.List(library.Filter{
		TagID:     a.tagID,
		Kind:      a.kindValue(),
		MinRating: a.minRating,
	})
	if err != nil {
		a.status = err.Error()
		return
	}
	a.assets = filtered
	if a.assetIndex >= len(a.assets) {
		a.assetIndex = -1
	}
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
	if a.projectID != "" {
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
		}
	}
	if a.watch != nil {
		_ = a.watch.Sync()
	}
	a.syncNavSelection()
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
					a.mu.Lock()
					closed := a.closed
					a.mu.Unlock()
					if !closed {
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
	a.importing = true
	lib := a.lib
	mode := storageModes[a.storageMode]
	var tags []string
	if a.tagID != "" {
		tags = []string{a.tagID}
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
			win.Update(func() {
				a.importing = false
				if err != nil && !a.isClosed() {
					a.status = err.Error()
				}
			})
			return
		}
		if a.isClosed() {
			win.Update(func() { a.importing = false })
			return
		}
		res, importErr := lib.Import(nilContext(), paths, mode, tags, func(done, total int) {
			d, n := done, total
			win.Update(func() {
				if a.isClosed() {
					return
				}
				a.status = fmt.Sprintf(a.t("library.importing"), d, n)
			})
		})
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
					if item.Status == "added" {
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
		// A missing build context also returns 0, and it leaves the dialog open.
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
	a.tagID = tag.ID
	a.reload()
}

func (a *App) createProject() {
	if a.tagID == "" {
		a.status = a.t("project.needTag")
		return
	}
	name := trim(a.newProjectName)
	if name == "" {
		a.status = a.t("project.needName")
		return
	}
	p, err := a.projects.Create(name, trim(a.newProjectDesc), []string{a.tagID})
	if err != nil {
		a.status = err.Error()
		return
	}
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

func (a *App) referenceSelected() {
	asset, ok := a.selectedAsset()
	if !ok || a.projectID == "" || a.projects == nil {
		return
	}
	if err := a.projects.Reference(a.projectID, asset.ID); err != nil {
		a.status = err.Error()
		return
	}
	a.reload()
}

func (a *App) assetName(asset model.Asset) string {
	name := filepath.Base(asset.Path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = asset.ID
	}
	return name
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
	switch {
	case nav == "library" || nav == "tag-all":
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = ""
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case strings.HasPrefix(nav, "tag:"):
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = strings.TrimPrefix(nav, "tag:")
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
		if a.tagID != "" {
			a.selectedNav = "tag:" + a.tagID
		} else {
			a.selectedNav = "tag-all"
		}
		a.lastNav = a.selectedNav
	}
}
