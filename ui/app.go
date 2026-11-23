// Package ui is the Raglex window: welcome, library, project, and board.
package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	gui "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"

	"raglex/internal/ai/laya"
	"raglex/internal/canvas"
	"raglex/internal/config"
	"raglex/internal/db"
	"raglex/internal/i18n"
	"raglex/internal/library"
	"raglex/internal/model"
	"raglex/internal/plugin"
	"raglex/internal/preview"
	"raglex/internal/project"
	"raglex/internal/server"
	"raglex/internal/share"
	"raglex/internal/thumbnail"
	"raglex/internal/updater"
	"raglex/internal/watcher"
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

const (
	layoutBento   = "bento"
	layoutMasonry = "masonry"
	layoutGrid    = "grid"
)

// assetDrag is the value dragged from the board tray onto the canvas.
type assetDrag struct {
	ID string
}

// assetBatchDrag is the value dragged from the library grid/bento/masonry onto
// a sidebar project or tag for single or multi-asset batch organization.
type assetBatchDrag struct {
	IDs []string
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
	tags               []model.Tag
	tagByID            map[string]model.Tag
	tagChildren        map[string][]model.Tag
	tagRecursiveCounts map[string]int
	expandedTags       map[string]bool
	allAssets          []model.Asset
	allAssetIdx        map[string]int
	assets             []model.Asset
	assetIdx           map[string]int
	assetByID          map[string]model.Asset
	assetSearchKey     map[string]string
	assetTags          map[string][]model.Tag
	tagCounts          map[string]int
	allAssetsCount     int
	untaggedCount      int
	missingCount       int
	projectTags        map[string]bool
	boardCounts        map[string]int
	boardPlacements    map[string][]model.Placement

	// Navigation, search, sort, multi-select & library display state.
	tagID           string
	navFilter       string // "", "untagged", "missing"
	searchQuery     string
	searchHandle    gui.Handle
	kindFilter      int
	minRating       int
	sortBy          int
	layoutMode      string // "bento", "masonry", "grid"
	grid            gui.GridState
	bentoScroll     gui.ScrollState
	masonryScroll   gui.ScrollState
	layoutGen       uint64
	masonryCache    masonryLayoutCache
	bentoCache      bentoLayoutCache
	assetIndex      int
	anchorIndex     int
	selectedIDs     map[string]bool
	multiSelectMode bool
	cardBounds      map[string]gui.Rect
	stageBounds     gui.Rect
	marqueeActive   bool
	marqueeDragging bool
	marqueeX0       float32
	marqueeY0       float32
	marqueeX1       float32
	marqueeY1       float32
	marqueeAdditive bool
	marqueeBase     map[string]bool
	cardSize        float64
	inspectorOpen   bool
	filterBarOpen   bool
	revealGen       int
	storageMode     int
	importing       bool
	importStage     string // "scanning", "importing", "canceling"
	importDone      int
	importTotal     int
	importAdded     int
	importExisting  int
	importFailed    int
	importCurrent   string
	importCancel    context.CancelFunc

	// Tag management popup & edit modal state.
	tagManagerOpen   bool
	sidebarFilter    string
	tagSearch        string
	newRootTagName   string
	newRootTagColor  string
	newChildParentID string
	newChildTagName  string
	editTagOpen      bool
	editTagID        string
	editTagName      string
	editTagParentID  string
	editTagColor     string

	// Inspector persistent bound state (avoids local-variable loss on rebuild).
	inspectorAssetID  string
	inspectorRating   int
	inspectorFileSize int64
	inspectorProjects []model.Project
	inspectorPlaces   int
	quickTagName      string
	inspectorNotes    string
	inspectorSrcURL    string
	inspectorMetaID    string
	inspFileMenuOpen       bool
	inspFileMenuPinned     bool
	inspFilePrevMode       bool
	inspFileSuppressHover  bool
	inspFileSuppressFrames int

	// Stage 12: Batch Rename & Batch Export modals.
	renameOpen       bool
	renameIDs        []string
	renameTemplate   string
	renameFind       string
	renameReplace    string
	renameRegex      bool
	renameIgnoreCase bool
	renamePrefix     string
	renameSuffix     string
	renameCaseMode   string
	renameStartIdx   string
	renameDiskFile   bool
	renameIncludeExt bool
	exportOpen       bool
	exportIDs        []string
	exportFormat     string
	exportQuality    int
	exportMaxEdge    int

	// Stage 14: Visual search (OKLab color filter, reverse image search) & Built-in Embedding + Laya AI.
	embedEnabled      bool
	layaEnabled       bool
	aiIndexBusy       bool
	aiIndexedCount    int
	colorFilter       string
	similarQueryLabel string
	similarScores     map[string]float64
	layaURL           string
	layaModel         string
	layaSuggestions   map[string][]laya.TagSuggestion
	layaRuntime       *laya.RuntimeManager
	layaStatus        laya.RuntimeStatus
	layaDeployBusy    bool
	layaDeployText    string
	layaDeployPct     float64

	// Stage 15 & 16: Built-in Server (Web Gallery, Companion API, MCP, Signaling).
	localSrv        *server.Server
	serverRunning   bool
	serverPortStr   string
	serverBindAll   bool
	serverAutoStart bool
	serverToken     string

	// Stage 18: Plugin manager.
	plugins *plugin.Manager

	// Quick Preview Inline Viewer, Quick Look Pop-up & Native Designer Formats state.
	previewOpen         bool
	quickLookOpen       bool
	previewAssetID      string
	previewBitmap       *gui.Bitmap
	previewVideo        *gui.VideoPlayer
	previewVideoErr     string
	previewLoop         bool
	previewLoading      bool
	previewZoom         float64
	previewSVG          *gui.SVG
	previewSVGChecker   bool
	previewFontInfo     preview.FontInfo
	previewFontBitmap   *gui.Bitmap
	previewFontText     string
	previewFontSize     float64
	previewFontGen      int
	previewAudio        *preview.AudioPlayer
	previewAudioWave    preview.Waveform
	previewAudioPos     float64
	previewAudioDur     float64
	previewAudioVol     float64
	previewAudioPlaying bool
	previewAudioCancel  func()
	previewWaveW        float32
	previewPDFBitmap    *gui.Bitmap
	previewPDFPage      int
	previewPDFPages     int
	previewPDFGen       int
	previewPanX         float64
	previewPanY         float64
	previewDragging     bool
	previewDragX        float64
	previewDragY        float64
	previewDragStartX   float64
	previewDragStartY   float64
	previewStageW       float32
	previewStageH       float32
	previewMiniDrag     bool
	previewVideoVol     float64
	previewVideoMuted   bool
	previewVideoRate    float64
	previewScrubbing    bool
	previewScrubVal     float64

	// Asset Stacks state.
	stacks             []model.Stack
	stackByID          map[string]model.Stack
	stackVisibleCounts map[string]int
	openStackID        string

	// PureRef-style Always-on-Top Floating Reference Window state.
	boardAlwaysOnTop    bool
	pureRefWin          *mygo.Window
	pureRefPinned       bool
	pureRefOpacity      float64
	pureRefBgOpacity    float64
	pureRefImgOpacity   float64
	pureRefFlipH        bool
	pureRefGray         bool
	pureRefClickThrough bool
	pureRefBorderless   bool
	pureRefPanX         float64
	pureRefPanY         float64
	pureRefZoom         float64
	pureRefDragging     bool
	pureRefDragX        float64
	pureRefDragY        float64
	pureRefStartPanX    float64
	pureRefStartPanY    float64
	pureRefViewW        float32
	pureRefViewH        float32

	// Inspector Info / Similar tabs.
	inspectorTab         int // 0 = Info, 1 = Similar
	inspectorSimilarID   string
	inspectorSimilar     []library.VisualMatch
	inspectorSimilarBusy bool

	// Ctrl+T Go to / Command Palette state.
	gotoOpen  bool
	gotoQuery string
	gotoIndex int

	// Random discovery seed.
	randomSeed int64

	// Custom theme overrides & Zeron-style Frosted Glass (Acrylic / Vibrancy).
	themePreset   string
	customCanvas  string
	customSurface string
	customAccent  string
	customText    string
	glassMode     bool
	glassOpacity  float64
	glassActive   bool

	// First-run interactive guided tour state.
	onboardingDone     bool
	onboardingOpen     bool
	onboardingStep     int
	onboardingPrevStep int
	onboardingTried    [5]bool

	// Preferences / Settings modal state & live database diagnostics.
	settingsOpen         bool
	settingsTab          int
	settingsPrevTab      int
	showSidebarCounts    bool
	showTagColors        bool
	showCardCaption      bool
	showCardExt          bool
	showCardRating       bool
	thumbBg              string // "", "black", "gray", "white", "checker"
	grayscalePreview     bool
	displayPopoverOpen   bool
	webImportOpen        bool
	webImportURL         string
	webImportBusy        bool
	autoSelectImported   bool
	autoTagOnImport      bool
	ignoreHiddenOnImport bool
	importRecursive      bool
	notifyDuplicates     bool
	backupBeforeDelete   bool
	deviceName           string
	dbStats              dbStatsInfo
	dbStatsBusy          bool
	dbStatsLoaded        bool

	// Project & board state.
	projectList            []model.Project
	standaloneBoards       []model.Board
	newStandaloneBoardOpen bool
	newStandaloneBoardName string
	projectID              string
	project                model.Project
	projectAssets          []model.Asset
	projSearchQuery        string
	projKindFilter         int
	boards                 []model.Board
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
	boardShowMinimap   bool
	boardStrokeColor   string
	boardStrokeWidth   float64
	boardMiniDrag      bool
	gesture            gesture
	spaceDown          bool
	focusBoard         bool
	focusBody          bool
	boundID            string
	notesBuf           string
	editText           string
	fontSizeBuf        float64
	flushArmed         bool

	bitmaps      map[string]*gui.Bitmap
	bitmapOrder  []string
	bitmapSem    chan struct{}
	bitmapWake   chan struct{}
	bitmapGen    map[string]uint64
	bitmapPaths  map[string]string
	viewFrameSeq uint64
	loading      map[string]bool
	queued       map[string]bool

	askOpen    bool
	askTitle   string
	askBody    string
	askConfirm string
	askFn      func()
	askCancel  func()

	sidebarOpen       bool
	sidebarWidth      float32
	sidebarResizing   bool
	sidebarGripX      float32
	sidebarDragStartX float32
	sidebarDragStartW float32
	inspectorWidth    float32
	inspectorResizing bool
	inspectorGripX    float32
	inspectorDragX    float32
	inspectorDragW    float32
	libraryPercent    float32
	selectedNav       string
	lastNav           string

	// GitHub Auto-Updater state.
	autoCheckUpdates  bool
	updateBetaChannel bool
	updateChecking    bool
	updateApplying    bool
	updateAvailable   bool
	updateInstalled   bool
	updatePct         float64
	updateMsg         string
	updateErr         string
	updateRelease     *updater.Release

	// Auto-dismissing floating toast popup state.
	toastSeq     uint64
	toastMsg     string
	toastVisible bool

	dark   bool
	pal    palette
	paper  gui.Color
	ink    gui.Color
	accent gui.Color
	muted  gui.Color
	line   gui.Color
}

// dbStatsInfo holds live disk & catalog metrics shown in Database Preferences.
type dbStatsInfo struct {
	DBBytes     int64
	FilesBytes  int64
	FilesCount  int
	ThumbsBytes int64
	ThumbsCount int
	HasBackup   bool
	BackupBytes int64
	BackupTime  time.Time
	TotalBoards int
}

// NewApp starts on the welcome screen. Language follows the OS until a
// preference is loaded.
func NewApp() *App {
	return &App{
		lang:                 i18n.Default(),
		themeMode:            "auto",
		layoutMode:           layoutBento,
		screen:               screenWelcome,
		shareFiles:           true,
		showSidebarCounts:    true,
		showTagColors:        true,
		showCardCaption:      true,
		showCardExt:          true,
		showCardRating:       true,
		previewVideoVol:      1.0,
		previewVideoRate:     1.0,
		pureRefBgOpacity:     0.88,
		pureRefImgOpacity:    1.0,
		boardShowMinimap:     true,
		boardStrokeColor:     "#FF5C33",
		boardStrokeWidth:     3.0,
		glassOpacity:         0.85,
		autoSelectImported:   true,
		autoTagOnImport:      true,
		ignoreHiddenOnImport: true,
		importRecursive:      true,
		notifyDuplicates:     true,
		backupBeforeDelete:   true,
		previewLoop:          true,
		onboardingDone:       true,
		assetIndex:           -1,
		anchorIndex:          -1,
		selectedIDs:          map[string]bool{},
		cardBounds:           map[string]gui.Rect{},
		cardSize:             184,
		inspectorOpen:        true,
		filterBarOpen:        false,
		boardDrawerOpen:      true,
		boardInspectorOpen:   true,
		bitmaps:              map[string]*gui.Bitmap{},
		bitmapSem:            make(chan struct{}, 6),
		bitmapWake:           make(chan struct{}, 1),
		bitmapGen:            map[string]uint64{},
		bitmapPaths:          map[string]string{},
		loading:              map[string]bool{},
		queued:               map[string]bool{},
		allAssetIdx:          map[string]int{},
		assetIdx:             map[string]int{},
		assetSearchKey:       map[string]string{},
		tagByID:              map[string]model.Tag{},
		tagChildren:          map[string][]model.Tag{},
		tagRecursiveCounts:   map[string]int{},
		expandedTags:         map[string]bool{},
		assetByID:            map[string]model.Asset{},
		assetTags:            map[string][]model.Tag{},
		tagCounts:            map[string]int{},
		projectTags:          map[string]bool{},
		boardCounts:          map[string]int{},
		boardPlacements:      map[string][]model.Placement{},
		newProjTags:          map[string]bool{},
		stackByID:            map[string]model.Stack{},
		stackVisibleCounts:   map[string]int{},
		previewSVGChecker:    true,
		previewFontSize:      44,
		previewAudioVol:      0.85,
		pureRefPinned:        true,
		pureRefOpacity:       0.96,
		pureRefZoom:          1,
		sidebarOpen:          true,
		sidebarWidth:         240,
		inspectorWidth:       316,
		libraryPercent:       74,
		autoCheckUpdates:     true,
		updateBetaChannel:    false,
		selectedNav:          "tag-all",
		lastNav:              "tag-all",
	}
}

// Run opens the window and, when a library path was saved, opens that folder.
func Run() error {
	updater.CleanupOldBinary()
	a := NewApp()
	if cfg, err := config.Load(); err == nil {
		a.applyConfig(cfg)
	} else {
		a.onboardingDone = false
	}
	if !a.onboardingDone {
		a.onboardingOpen = true
	}
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:             "Raglex",
			Width:             1360,
			Height:            860,
			MinWidth:          980,
			MinHeight:         640,
			TitleBarStyle:     mygo.TitleBarHidden,
			HideWindowButtons: true,
			Vibrancy:          mygo.VibrancyAcrylic,
			StateKey:          "main",
			Content:           gui.View(a.View),
		})
		a.syncWindowVibrancy()
		a.win.OnClosed(func() { a.shutdown() })
		a.startShare()
		a.startQuietUpdateCheck()
		if a.cfg.LibraryPath != "" {
			root := a.cfg.LibraryPath
			go a.openRoot(root)
		}
	})
	return mygo.App.Run()
}

func (a *App) applyConfig(cfg config.Config) {
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
	a.themePreset = cfg.ThemePreset
	a.customCanvas = cfg.CustomCanvas
	a.customSurface = cfg.CustomSurface
	a.customAccent = cfg.CustomAccent
	a.customText = cfg.CustomText
	a.glassMode = config.BoolOr(cfg.GlassMode, cfg.ThemePreset == "glass")
	if cfg.GlassOpacity != nil && *cfg.GlassOpacity >= 0.25 && *cfg.GlassOpacity <= 0.96 {
		a.glassOpacity = *cfg.GlassOpacity
	} else {
		a.glassOpacity = 0.85
	}
	a.onboardingDone = cfg.OnboardingDone
	if cfg.CardSize >= 120 && cfg.CardSize <= 320 {
		a.cardSize = cfg.CardSize
	}
	if cfg.DefaultSort >= sortNewest && cfg.DefaultSort <= sortRating {
		a.sortBy = cfg.DefaultSort
	}
	if cfg.LayoutMode == layoutBento || cfg.LayoutMode == layoutMasonry || cfg.LayoutMode == layoutGrid {
		a.layoutMode = cfg.LayoutMode
	}
	for i, m := range storageModes {
		if m == cfg.DefaultStorageMode {
			a.storageMode = i
			break
		}
	}
	a.showSidebarCounts = config.BoolOr(cfg.ShowSidebarCounts, true)
	a.showTagColors = config.BoolOr(cfg.ShowTagColors, true)
	a.showCardCaption = config.BoolOr(cfg.ShowCardCaption, true)
	a.showCardExt = config.BoolOr(cfg.ShowCardExt, true)
	a.showCardRating = config.BoolOr(cfg.ShowCardRating, true)
	switch cfg.ThumbBg {
	case "", "black", "gray", "white", "checker":
		a.thumbBg = cfg.ThumbBg
	}
	a.grayscalePreview = cfg.GrayscalePreview
	if cfg.PureRefBgOpacity != nil {
		v := *cfg.PureRefBgOpacity
		if v >= 0 && v <= 1 {
			a.pureRefBgOpacity = v
		}
	}
	a.autoSelectImported = config.BoolOr(cfg.AutoSelectImported, true)
	a.autoTagOnImport = config.BoolOr(cfg.AutoTagOnImport, true)
	a.ignoreHiddenOnImport = config.BoolOr(cfg.IgnoreHiddenOnImport, true)
	a.importRecursive = config.BoolOr(cfg.ImportRecursive, true)
	a.notifyDuplicates = config.BoolOr(cfg.NotifyDuplicates, true)
	a.backupBeforeDelete = config.BoolOr(cfg.BackupBeforeDelete, true)
	a.shareFiles = config.BoolOr(cfg.ShareIncludeFiles, true)
	a.deviceName = cfg.DeviceName
	a.embedEnabled = config.BoolOr(cfg.EmbedEnabled, true)
	a.layaEnabled = config.BoolOr(cfg.LayaEnabled, true)
	a.layaURL = cfg.LayaURL
	if a.layaURL == "" {
		a.layaURL = laya.DefaultBaseURL
	}
	a.layaModel = cfg.LayaModel
	if a.layaModel == "" {
		a.layaModel = laya.DefaultModel
	}
	port := cfg.ServerPort
	if port <= 0 || port > 65535 {
		port = server.DefaultPort
	}
	a.serverPortStr = strconv.Itoa(port)
	a.serverBindAll = cfg.ServerBindAll
	a.serverAutoStart = cfg.ServerAutoStart
	a.serverToken = cfg.ServerToken
	a.autoCheckUpdates = config.BoolOr(cfg.AutoCheckUpdates, true)
	a.updateBetaChannel = config.BoolOr(cfg.UpdateBetaChannel, false)
}

func (a *App) syncConfigFromState() {
	a.cfg.Language = a.lang
	a.cfg.ThemeMode = a.themeMode
	a.cfg.ThemePreset = a.themePreset
	a.cfg.CustomCanvas = strings.TrimSpace(a.customCanvas)
	a.cfg.CustomSurface = strings.TrimSpace(a.customSurface)
	a.cfg.CustomAccent = strings.TrimSpace(a.customAccent)
	a.cfg.CustomText = strings.TrimSpace(a.customText)
	a.cfg.GlassMode = config.BoolPtr(a.glassMode)
	goOp := a.glassOpacity
	a.cfg.GlassOpacity = &goOp
	a.cfg.OnboardingDone = a.onboardingDone
	a.cfg.CardSize = a.cardSize
	a.cfg.DefaultSort = a.sortBy
	a.cfg.LayoutMode = a.layoutMode
	if a.storageMode >= 0 && a.storageMode < len(storageModes) {
		a.cfg.DefaultStorageMode = storageModes[a.storageMode]
	}
	a.cfg.ShowSidebarCounts = config.BoolPtr(a.showSidebarCounts)
	a.cfg.ShowTagColors = config.BoolPtr(a.showTagColors)
	a.cfg.ShowCardCaption = config.BoolPtr(a.showCardCaption)
	a.cfg.ShowCardExt = config.BoolPtr(a.showCardExt)
	a.cfg.ShowCardRating = config.BoolPtr(a.showCardRating)
	a.cfg.ThumbBg = a.thumbBg
	a.cfg.GrayscalePreview = a.grayscalePreview
	bgOp := a.pureRefBgOpacity
	a.cfg.PureRefBgOpacity = &bgOp
	a.cfg.AutoSelectImported = config.BoolPtr(a.autoSelectImported)
	a.cfg.AutoTagOnImport = config.BoolPtr(a.autoTagOnImport)
	a.cfg.IgnoreHiddenOnImport = config.BoolPtr(a.ignoreHiddenOnImport)
	a.cfg.ImportRecursive = config.BoolPtr(a.importRecursive)
	a.cfg.NotifyDuplicates = config.BoolPtr(a.notifyDuplicates)
	a.cfg.BackupBeforeDelete = config.BoolPtr(a.backupBeforeDelete)
	a.cfg.ShareIncludeFiles = config.BoolPtr(a.shareFiles)
	a.cfg.DeviceName = strings.TrimSpace(a.deviceName)
	a.cfg.SignalingURL = strings.TrimSpace(a.signalURL)
	a.cfg.EmbedEnabled = config.BoolPtr(a.embedEnabled)
	a.cfg.LayaEnabled = config.BoolPtr(a.layaEnabled)
	a.cfg.LayaURL = strings.TrimSpace(a.layaURL)
	a.cfg.LayaModel = strings.TrimSpace(a.layaModel)
	a.cfg.ServerPort = a.serverPortValue()
	a.cfg.ServerBindAll = a.serverBindAll
	a.cfg.ServerAutoStart = a.serverAutoStart
	a.cfg.ServerToken = strings.TrimSpace(a.serverToken)
	a.cfg.AutoCheckUpdates = config.BoolPtr(a.autoCheckUpdates)
	a.cfg.UpdateBetaChannel = config.BoolPtr(a.updateBetaChannel)
	a.saveConfig()
}

func (a *App) setStorageMode(idx int) {
	if idx < 0 || idx >= len(storageModes) {
		return
	}
	a.storageMode = idx
	a.syncConfigFromState()
}

func (a *App) setLayoutMode(mode string) {
	if mode != layoutBento && mode != layoutMasonry && mode != layoutGrid {
		return
	}
	if a.layoutMode == mode {
		return
	}
	a.layoutMode = mode
	a.revealGen++
	a.syncConfigFromState()
}

func (a *App) syncWindowVibrancy() {
	if a.win == nil || a.win.IsDestroyed() {
		return
	}
	if a.glassMode {
		a.win.SetVibrancy(mygo.VibrancyAcrylic)
	} else {
		a.win.SetVibrancy(mygo.VibrancyNone)
	}
}

func (a *App) applyThemePreset(preset string) {
	a.themePreset = preset
	if preset == "" {
		a.customCanvas = ""
		a.customSurface = ""
		a.customAccent = ""
		a.customText = ""
		a.glassMode = false
	} else {
		a.glassMode = preset == "glass"
		for _, pr := range themePresets {
			if pr.id == preset {
				a.customCanvas = pr.canvas
				a.customSurface = pr.surface
				a.customAccent = pr.accent
				a.customText = pr.text
				break
			}
		}
	}
	a.syncWindowVibrancy()
	a.syncConfigFromState()
}

func (a *App) resetCustomTheme() {
	a.themePreset = ""
	a.customCanvas = ""
	a.customSurface = ""
	a.customAccent = ""
	a.customText = ""
	a.glassMode = false
	a.glassOpacity = 0.85
	a.syncWindowVibrancy()
	a.syncConfigFromState()
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
	if a.importCancel != nil {
		a.importCancel()
		a.importCancel = nil
	}
	a.shareDB.Store(nil)
	if node != nil {
		_ = node.Close()
	}
	a.closePreview()
	a.closePureRefWindow()
	a.stopBuiltInServer()
	if a.layaRuntime != nil {
		a.layaRuntime.Stop()
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
	a.plugins = plugin.NewManager(root)
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
	a.bitmapOrder = nil
	a.loading = map[string]bool{}
	a.queued = map[string]bool{}
	if purged, err := lib.PurgeAuxiliarySidecars(); err == nil && purged > 0 {
		a.notify(fmt.Sprintf("已自動清理 %d 個輔助附屬檔 (metadata.json / thumbnail)", purged))
	}
	a.reload()
	if a.serverAutoStart {
		a.startBuiltInServer()
	}
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
	snap, err := a.lib.LoadCatalogSnapshot()
	if err != nil {
		a.status = err.Error()
		return
	}
	a.tags = snap.Tags
	a.rebuildTagTree()

	all := snap.Assets
	a.allAssets = all
	a.allAssetsCount = len(all)
	a.allAssetIdx = make(map[string]int, len(all))
	a.assetByID = make(map[string]model.Asset, len(all))
	a.assetSearchKey = make(map[string]string, len(all))
	a.assetTags = snap.AssetTags
	if a.assetTags == nil {
		a.assetTags = make(map[string][]model.Tag)
	}
	a.tagCounts = make(map[string]int, len(a.tags))
	a.tagRecursiveCounts = make(map[string]int, len(a.tags))
	a.untaggedCount = 0
	a.missingCount = 0

	for i, asset := range all {
		a.allAssetIdx[asset.ID] = i
		a.assetByID[asset.ID] = asset
		if asset.Missing {
			a.missingCount++
		}
		atags := a.assetTags[asset.ID]
		a.assetSearchKey[asset.ID] = a.buildAssetSearchKey(asset, atags)
		if len(atags) == 0 {
			a.untaggedCount++
		}
		countedAncestors := map[string]bool{}
		for _, tg := range atags {
			a.tagCounts[tg.ID]++
			walk := tg.ID
			for walk != "" && !countedAncestors[walk] {
				countedAncestors[walk] = true
				a.tagRecursiveCounts[walk]++
				walk = a.tagByID[walk].ParentID
			}
		}
		// Only queue background thumbnail generation for assets missing a thumbnail
		// or visual features; actual bitmap decoding into RAM happens lazily when a card is visible!
		if !asset.Missing && !model.IsSVG(asset.Mime, asset.Path) {
			a.queueThumb(asset)
		}
	}

	// Load Asset Stacks metadata from the same snapshot.
	a.stacks = snap.Stacks
	a.stackByID = make(map[string]model.Stack, len(snap.Stacks))
	for _, st := range snap.Stacks {
		a.stackByID[st.ID] = st
	}
	if a.openStackID != "" {
		if _, ok := a.stackByID[a.openStackID]; !ok {
			a.openStackID = ""
		}
	}

	a.applyFilters()

	if a.projects != nil {
		// Always show all projects in the sidebar regardless of selected tag.
		a.projectList, err = a.projects.List()
		if err != nil {
			a.status = err.Error()
			return
		}
	}

	a.projectTags = map[string]bool{}
	a.boardCounts = map[string]int{}
	a.boardPlacements = map[string][]model.Placement{}
	if a.projects != nil {
		if sb, err := a.projects.StandaloneBoards(); err == nil {
			a.standaloneBoards = sb
			for _, b := range sb {
				if items, err := a.projects.LoadPlacements(b.ID); err == nil {
					a.boardCounts[b.ID] = len(items)
					a.boardPlacements[b.ID] = items
				}
			}
		}
	}
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
					a.boardPlacements[b.ID] = items
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

	var tagSet map[string]bool
	if a.tagID != "" {
		tagSet = a.tagDescendantSet(a.tagID)
	}

	colorDists := map[string]float64{}
	recentCutoff := time.Now().Add(-14 * 24 * time.Hour)
	for _, asset := range a.allAssets {
		if a.navFilter == "missing" && !asset.Missing {
			continue
		}
		if a.navFilter == "untagged" && len(a.assetTags[asset.ID]) > 0 {
			continue
		}
		if a.navFilter == "recent" && !asset.CreatedAt.IsZero() && asset.CreatedAt.Before(recentCutoff) {
			continue
		}
		if tagSet != nil && !a.hasAnyTag(asset.ID, tagSet) {
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
		if a.colorFilter != "" {
			d, ok := thumbnail.ColorDistanceToPalette(a.colorFilter, asset.Palette)
			if !ok || d > 0.24 {
				continue
			}
			colorDists[asset.ID] = d
		}
		if len(a.similarScores) > 0 {
			if _, ok := a.similarScores[asset.ID]; !ok {
				continue
			}
		}
		out = append(out, asset)
	}

	if len(a.similarScores) > 0 {
		sort.Slice(out, func(i, j int) bool {
			return a.similarScores[out[i].ID] > a.similarScores[out[j].ID]
		})
	} else if a.colorFilter != "" {
		sort.Slice(out, func(i, j int) bool {
			return colorDists[out[i].ID] < colorDists[out[j].ID]
		})
	} else if a.navFilter == "random" {
		seed := a.randomSeed
		if seed == 0 {
			seed = 1
		}
		sort.Slice(out, func(i, j int) bool {
			return hashOrderKey(out[i].ID, seed) < hashOrderKey(out[j].ID, seed)
		})
	} else {
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
	}

	a.stackVisibleCounts = map[string]int{}
	if a.openStackID != "" {
		// Inside an expanded stack: show every member in stack order.
		if st, ok := a.stackByID[a.openStackID]; ok {
			byID := make(map[string]model.Asset, len(out))
			for _, it := range out {
				byID[it.ID] = it
			}
			expanded := make([]model.Asset, 0, len(st.AssetIDs))
			for _, id := range st.AssetIDs {
				if it, exists := byID[id]; exists {
					expanded = append(expanded, it)
				}
			}
			out = expanded
		}
	} else if len(a.stackByID) > 0 {
		// Collapse stacked versions into one representative cover card.
		membersByStack := map[string][]model.Asset{}
		for _, it := range out {
			if it.StackID != "" {
				if _, ok := a.stackByID[it.StackID]; ok {
					membersByStack[it.StackID] = append(membersByStack[it.StackID], it)
				}
			}
		}
		collapsed := make([]model.Asset, 0, len(out))
		emittedStack := map[string]bool{}
		for _, it := range out {
			if it.StackID == "" {
				collapsed = append(collapsed, it)
				continue
			}
			st, ok := a.stackByID[it.StackID]
			if !ok {
				collapsed = append(collapsed, it)
				continue
			}
			if emittedStack[st.ID] {
				continue
			}
			emittedStack[st.ID] = true
			mems := membersByStack[st.ID]
			a.stackVisibleCounts[st.ID] = len(mems)
			rep := mems[0]
			for _, m := range mems {
				if m.ID == st.CoverID {
					rep = m
					break
				}
			}
			collapsed = append(collapsed, rep)
		}
		out = collapsed
	}

	a.assets = out
	a.layoutGen++
	a.assetIdx = make(map[string]int, len(out))
	for i, item := range out {
		a.assetIdx[item.ID] = i
	}
	if selectedID != "" {
		if idx, ok := a.assetIdx[selectedID]; ok {
			a.assetIndex = idx
		} else {
			a.assetIndex = -1
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

func (a *App) hasAnyTag(assetID string, set map[string]bool) bool {
	for _, tg := range a.assetTags[assetID] {
		if set[tg.ID] {
			return true
		}
	}
	return false
}

func (a *App) rebuildTagTree() {
	a.tagByID = make(map[string]model.Tag, len(a.tags))
	a.tagChildren = make(map[string][]model.Tag)
	for _, tg := range a.tags {
		a.tagByID[tg.ID] = tg
	}
	for _, tg := range a.tags {
		pid := tg.ParentID
		if pid != "" {
			if _, ok := a.tagByID[pid]; !ok {
				pid = ""
			}
		}
		a.tagChildren[pid] = append(a.tagChildren[pid], tg)
	}
	for pid := range a.tagChildren {
		list := a.tagChildren[pid]
		sort.Slice(list, func(i, j int) bool {
			return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
		})
		a.tagChildren[pid] = list
	}
}

func (a *App) ensureTagTree() {
	if len(a.tags) > 0 && len(a.tagByID) != len(a.tags) {
		a.rebuildTagTree()
	}
}

// tagDescendantSet returns tagID plus all of its nested child tag IDs.
func (a *App) tagDescendantSet(tagID string) map[string]bool {
	out := map[string]bool{tagID: true}
	var walk func(id string)
	walk = func(id string) {
		for _, ch := range a.tagChildren[id] {
			if !out[ch.ID] {
				out[ch.ID] = true
				walk(ch.ID)
			}
		}
	}
	walk(tagID)
	return out
}

func (a *App) tagDisplayCount(tagID string) int {
	if c, ok := a.tagRecursiveCounts[tagID]; ok {
		return c
	}
	return a.tagCounts[tagID]
}

func (a *App) buildAssetSearchKey(asset model.Asset, tags []model.Tag) string {
	var sb strings.Builder
	sb.Grow(len(asset.Path) + len(asset.Mime) + len(asset.Notes) + 24)
	sb.WriteString(strings.ToLower(filepath.Base(asset.Path)))
	sb.WriteByte('\n')
	sb.WriteString(strings.ToLower(asset.Mime))
	if asset.Notes != "" {
		sb.WriteByte('\n')
		sb.WriteString(strings.ToLower(asset.Notes))
	}
	for _, tg := range tags {
		sb.WriteByte('\n')
		sb.WriteString(strings.ToLower(tg.Name))
	}
	return sb.String()
}

func (a *App) matchesQuery(asset model.Asset, q string) bool {
	if key, ok := a.assetSearchKey[asset.ID]; ok {
		return strings.Contains(key, q)
	}
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
	a.inspFileMenuOpen = false
	a.inspFileMenuPinned = false
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
	return a.searchQuery != "" || a.kindFilter != 0 || a.minRating != 0 || a.colorFilter != "" || len(a.similarScores) > 0
}

func (a *App) clearFilters() {
	a.searchQuery = ""
	a.kindFilter = 0
	a.minRating = 0
	a.colorFilter = ""
	a.similarQueryLabel = ""
	a.similarScores = nil
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
	if a.thumbs == nil || asset.Missing || a.queued[asset.ID] {
		return
	}
	hasThumb := asset.ThumbnailPath != ""
	hasVisual := len(asset.Palette) > 0 && (asset.PHash != 0 || asset.DHash != 0)
	if hasThumb && hasVisual {
		return
	}
	if model.IsSVG(asset.Mime, asset.Path) {
		a.loadSVGThumbAsync(asset.ID, asset.Path)
		return
	}
	if model.MediaKind(asset.Mime) == model.MediaOther &&
		!model.IsFont(asset.Mime, asset.Path) &&
		!model.IsAudio(asset.Mime, asset.Path) &&
		!model.IsPDF(asset.Mime, asset.Path) {
		return
	}
	a.queued[asset.ID] = true
	a.thumbs.Enqueue(asset.ID)
}

// updateAssetInMemory mutates a single asset's record in all in-memory slices
// and maps in O(1) time without running a full bbolt reload().
func (a *App) updateAssetInMemory(asset model.Asset) {
	a.assetByID[asset.ID] = asset
	if idx, ok := a.allAssetIdx[asset.ID]; ok && idx >= 0 && idx < len(a.allAssets) && a.allAssets[idx].ID == asset.ID {
		a.allAssets[idx] = asset
	}
	if idx, ok := a.assetIdx[asset.ID]; ok && idx >= 0 && idx < len(a.assets) && a.assets[idx].ID == asset.ID {
		a.assets[idx] = asset
	}
	for i := range a.projectAssets {
		if a.projectAssets[i].ID == asset.ID {
			a.projectAssets[i] = asset
			break
		}
	}
	a.assetSearchKey[asset.ID] = a.buildAssetSearchKey(asset, a.assetTags[asset.ID])
}

// setAssetRatingFast updates an asset's star rating in bbolt and in memory in O(1).
func (a *App) setAssetRatingFast(assetID string, rating int) {
	if a.lib == nil || assetID == "" {
		return
	}
	if err := a.lib.SetRating(assetID, rating); err != nil {
		a.status = err.Error()
		return
	}
	if cur, ok := a.assetByID[assetID]; ok {
		cur.Rating = rating
		a.updateAssetInMemory(cur)
	}
	if a.inspectorAssetID == assetID {
		a.inspectorRating = rating
	}
	if a.minRating > 0 || a.sortBy == sortRating {
		a.applyFilters()
	}
}

// setAssetTagFast links or unlinks a tag on an asset in O(1) without reloading 100k records.
func (a *App) setAssetTagFast(assetID, tagID string, on bool) {
	if a.lib == nil || assetID == "" || tagID == "" {
		return
	}
	if err := a.lib.SetAssetTag(assetID, tagID, on); err != nil {
		a.status = err.Error()
		return
	}
	tg, ok := a.tagByID[tagID]
	if !ok {
		a.reload()
		return
	}
	curTags := a.assetTags[assetID]
	hadTag := false
	for _, existing := range curTags {
		if existing.ID == tagID {
			hadTag = true
			break
		}
	}
	if on && !hadTag {
		wasUntagged := len(curTags) == 0
		curTags = append(curTags, tg)
		a.assetTags[assetID] = curTags
		if wasUntagged && a.untaggedCount > 0 {
			a.untaggedCount--
		}
	} else if !on && hadTag {
		next := make([]model.Tag, 0, len(curTags)-1)
		for _, existing := range curTags {
			if existing.ID != tagID {
				next = append(next, existing)
			}
		}
		a.assetTags[assetID] = next
		if len(next) == 0 {
			a.untaggedCount++
		}
	}
	a.recomputeTagCountsForAsset(assetID)
	if cur, ok := a.assetByID[assetID]; ok {
		a.assetSearchKey[assetID] = a.buildAssetSearchKey(cur, a.assetTags[assetID])
	}
	if a.tagID != "" || a.navFilter == "untagged" || a.searchQuery != "" {
		a.applyFilters()
	}
}

func (a *App) recomputeTagCountsForAsset(_ string) {
	a.tagCounts = make(map[string]int, len(a.tags))
	a.tagRecursiveCounts = make(map[string]int, len(a.tags))
	for _, atags := range a.assetTags {
		if len(atags) == 0 {
			continue
		}
		countedAncestors := map[string]bool{}
		for _, tg := range atags {
			a.tagCounts[tg.ID]++
			walk := tg.ID
			for walk != "" && !countedAncestors[walk] {
				countedAncestors[walk] = true
				a.tagRecursiveCounts[walk]++
				walk = a.tagByID[walk].ParentID
			}
		}
	}
}

func (a *App) onThumb(id string) {
	if a.lib == nil {
		return
	}
	asset, err := a.lib.Get(id)
	if err != nil {
		return
	}
	a.updateAssetInMemory(asset)
}

const maxCachedBitmaps = 480

func (a *App) storeBitmap(id string, bm *gui.Bitmap) {
	if bm == nil || id == "" {
		return
	}
	if _, exists := a.bitmaps[id]; !exists {
		if len(a.bitmapOrder) >= maxCachedBitmaps {
			dropCount := 64
			if dropCount > len(a.bitmapOrder) {
				dropCount = len(a.bitmapOrder)
			}
			for i := 0; i < dropCount; i++ {
				oldID := a.bitmapOrder[i]
				if oldID != a.previewAssetID && oldID != a.inspectorAssetID && !a.selectedIDs[oldID] {
					delete(a.bitmaps, oldID)
				}
			}
			a.bitmapOrder = append([]string(nil), a.bitmapOrder[dropCount:]...)
		}
		a.bitmapOrder = append(a.bitmapOrder, id)
	}
	a.bitmaps[id] = bm
}

// requestCardBitmap lazily loads or prioritizes thumbnail generation for a
// visible card inside the viewport.
func (a *App) requestCardBitmap(asset model.Asset) *gui.Bitmap {
	if bm := a.bitmaps[asset.ID]; bm != nil {
		return bm
	}
	if asset.Missing {
		return nil
	}
	if asset.ThumbnailPath != "" {
		// If Thumbnail Background is active and this alpha-capable asset has a legacy
		// opaque .jpg thumbnail, load from source path so transparency shows cleanly.
		p := asset.ThumbnailPath
		if a.thumbBg != "" && hasAlphaFormat(asset.Mime, asset.Path) && strings.HasSuffix(strings.ToLower(p), ".jpg") && asset.Path != "" {
			p = asset.Path
		}
		a.ensureBitmap(asset.ID, p)
		return nil
	}
	if model.IsSVG(asset.Mime, asset.Path) {
		a.loadSVGThumbAsync(asset.ID, asset.Path)
		return nil
	}
	if a.thumbs != nil {
		a.thumbs.Prioritize(asset.ID)
	}
	return nil
}

func (a *App) ensureBitmap(id, path string) {
	if path == "" || a.bitmaps[id] != nil || a.loading[id] || a.win == nil {
		return
	}
	if a.bitmapSem == nil {
		a.bitmapSem = make(chan struct{}, 6)
	}
	// Non-blocking concurrency guard: only decode up to 6 thumbnails at once.
	// Visible cards still waiting will automatically re-request on subsequent frames;
	// cards scrolled past are never queued at all!
	select {
	case a.bitmapSem <- struct{}{}:
	default:
		return
	}
	a.loading[id] = true
	win := a.win
	go func() {
		defer func() {
			<-a.bitmapSem
			// Wake the UI loop so any remaining visible cards waiting for a slot
			// immediately claim the freed worker on the next frame.
			if win != nil {
				win.Update(func() {})
			}
		}()
		data, err := os.ReadFile(path)
		var bm *gui.Bitmap
		var decErr error
		if err == nil {
			bm, decErr = gui.DecodeBitmap(data)
		}
		win.Update(func() {
			delete(a.loading, id)
			if a.lib == nil || err != nil || decErr != nil || bm == nil {
				return
			}
			a.storeBitmap(id, bm)
		})
	}()
}

// closePreview shuts down any active media players and closes both Quick Look and Inline Viewer.
func (a *App) closePreview() {
	if a.previewVideo != nil {
		a.previewVideo.Close()
		a.previewVideo = nil
	}
	a.resetNativePreviewState()
	a.previewVideoErr = ""
	a.previewLoading = false
	a.previewOpen = false
	a.quickLookOpen = false
}

// openQuickLook opens the centered Quick Look Pop-up (1st Space press).
func (a *App) openQuickLook(asset model.Asset) {
	if asset.ID == "" {
		return
	}
	a.quickLookOpen = true
	a.previewOpen = false
	a.loadPreviewAsset(asset)
}

// promoteQuickLookToFullPreview promotes Quick Look Pop-up into the full-stage
// inline preview viewer without reloading already-decoded media (2nd Space press).
func (a *App) promoteQuickLookToFullPreview() {
	if !a.quickLookOpen {
		return
	}
	a.quickLookOpen = false
	a.previewOpen = true
	if asset, ok := a.assetByID[a.previewAssetID]; ok {
		a.loadPreviewAsset(asset)
	}
}

// handleSpacePreview implements the 2-stage Space workflow:
// 1st Space in grid -> Quick Look Pop-up
// 2nd Space in Quick Look -> Full Inline Preview
// Space in Full Inline Preview -> Play/Pause media (or close for static assets)
func (a *App) handleSpacePreview() {
	if a.quickLookOpen {
		a.promoteQuickLookToFullPreview()
		return
	}
	if a.previewOpen {
		if a.previewVideo != nil {
			if a.previewVideo.Playing() {
				a.previewVideo.Pause()
			} else {
				a.previewVideo.Play()
			}
			return
		}
		if a.previewAudio != nil {
			a.toggleAudioPlayback()
			return
		}
		a.closePreview()
		return
	}
	if a.canRunLibraryShortcut() {
		if asset, ok := a.selectedAsset(); ok {
			a.openQuickLook(asset)
		}
	}
}

// openPreview opens the full-resolution inline viewer directly (double-click / button).
func (a *App) openPreview(asset model.Asset) {
	if asset.ID == "" {
		return
	}
	a.quickLookOpen = false
	a.previewOpen = true
	a.loadPreviewAsset(asset)
}

func (a *App) loadPreviewAsset(asset model.Asset) {
	a.previewZoom = 1.0
	a.previewPanX = 0
	a.previewPanY = 0
	a.previewDragging = false
	a.previewMiniDrag = false
	a.previewScrubbing = false
	if a.previewAssetID == asset.ID && (a.previewBitmap != nil || a.previewVideo != nil || a.previewSVG != nil || a.previewFontBitmap != nil || a.previewPDFBitmap != nil || a.previewAudio != nil || a.previewLoading) {
		return
	}
	if a.previewVideo != nil {
		a.previewVideo.Close()
		a.previewVideo = nil
	}
	a.resetNativePreviewState()
	a.previewAssetID = asset.ID
	// Keep inspector synced with the viewed asset.
	for i, item := range a.assets {
		if item.ID == asset.ID {
			a.assetIndex = i
			a.selectedIDs = map[string]bool{asset.ID: true}
			break
		}
	}
	if a.inspectorTab == 1 {
		a.ensureInspectorSimilar(asset)
	}
	a.previewBitmap = nil
	a.previewVideoErr = ""
	a.previewLoading = false

	if asset.Missing || asset.Path == "" {
		return
	}

	if a.loadNativeFormatPreview(asset) {
		return
	}

	kind := model.MediaKind(asset.Mime)
	isGIF := strings.EqualFold(asset.Mime, "image/gif") || strings.EqualFold(filepath.Ext(asset.Path), ".gif")

	if kind == model.MediaVideo {
		if a.win == nil {
			a.previewBitmap = a.bitmaps[asset.ID]
			return
		}
		// Prefer MyGo's native OS media framework (Media Foundation / AVFoundation / GStreamer)
		// for GPU-accelerated video + synchronized audio + instant timeline scrubbing!
		if player, err := video.Open(asset.Path); err == nil && player != nil {
			player.SetLoop(a.previewLoop)
			if a.previewVideoVol <= 0 {
				a.previewVideoVol = 1.0
			}
			player.SetVolume(a.previewVideoVol)
			player.SetMuted(a.previewVideoMuted)
			if a.previewVideoRate > 0 {
				player.SetRate(a.previewVideoRate)
			}
			a.previewVideo = player
			return
		}
		if !thumbnail.Available() {
			a.previewVideoErr = a.t("preview.videoNoFfmpeg")
			return
		}
		a.previewLoading = true
		win := a.win
		targetID := asset.ID
		path := asset.Path
		w, h := asset.Width, asset.Height
		loop := a.previewLoop
		go func() {
			dec, err := newFFmpegVideoDecoder(path, w, h)
			var player *gui.VideoPlayer
			if err == nil {
				player, err = gui.NewVideoPlayer(dec)
				if err != nil {
					_ = dec.Close()
				} else {
					player.SetLoop(loop)
				}
			}
			win.Update(func() {
				if a.previewAssetID != targetID || (!a.previewOpen && !a.quickLookOpen) || a.isClosed() {
					if player != nil {
						player.Close()
					}
					return
				}
				a.previewLoading = false
				if err != nil || player == nil {
					a.previewVideoErr = a.t("preview.videoFailed")
					return
				}
				a.previewVideo = player
			})
		}()
		return
	}

	if kind != model.MediaImage && !model.IsExtraCreativeImage(asset.Mime, asset.Path) {
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
	mimeType := asset.Mime
	loop := a.previewLoop
	go func() {
		var bm *gui.Bitmap
		var player *gui.VideoPlayer
		if model.IsExtraCreativeImage(mimeType, path) {
			if img, err := preview.DecodeCreativeImage(path, 2400); err == nil && img != nil {
				bm, _ = imageToBitmap(img)
			}
		} else if data, err := os.ReadFile(path); err == nil {
			if isGIF {
				if dec, gerr := gui.NewGIFDecoder(data); gerr == nil {
					if p, perr := gui.NewVideoPlayer(dec); perr == nil {
						p.SetLoop(loop)
						player = p
					}
				}
			}
			if player == nil {
				bm, _ = gui.DecodeBitmap(data)
				if bm == nil {
					if img, derr := preview.DecodeCreativeImage(path, 2400); derr == nil && img != nil {
						bm, _ = imageToBitmap(img)
					}
				}
			}
		}
		win.Update(func() {
			if a.previewAssetID != targetID || (!a.previewOpen && !a.quickLookOpen) || a.isClosed() {
				if player != nil {
					player.Close()
				}
				return
			}
			a.previewLoading = false
			if player != nil {
				a.previewVideo = player
			} else if bm != nil {
				a.previewBitmap = bm
			}
		})
	}()
}

func (a *App) stepPreview(delta int) {
	if (!a.previewOpen && !a.quickLookOpen) || len(a.assets) == 0 {
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
	if a.quickLookOpen {
		a.openQuickLook(a.assets[idx])
	} else {
		a.openPreview(a.assets[idx])
	}
}

func (a *App) defaultInstallLibraryDir() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
			exe = resolved
		}
		dir := filepath.Dir(exe)
		// On macOS, if running inside Foo.app/Contents/MacOS, place the default library beside Foo.app.
		if strings.HasSuffix(filepath.ToSlash(dir), ".app/Contents/MacOS") {
			dir = filepath.Dir(filepath.Dir(filepath.Dir(dir)))
		}
		candidate := filepath.Join(dir, "Library")
		if err := os.MkdirAll(candidate, 0o755); err == nil {
			return candidate
		}
	}
	if cfgDir, err := os.UserConfigDir(); err == nil && cfgDir != "" {
		fallback := filepath.Join(cfgDir, "Raglex", "Library")
		_ = os.MkdirAll(fallback, 0o755)
		return fallback
	}
	return "Library"
}

func (a *App) useDefaultLibrary() {
	root := a.defaultInstallLibraryDir()
	go a.openRoot(root)
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
	ctx, cancel := context.WithCancel(context.Background())
	a.importing = true
	a.importStage = "scanning"
	a.importDone = 0
	a.importTotal = 0
	a.importAdded = 0
	a.importExisting = 0
	a.importFailed = 0
	a.importCurrent = filepath.Base(paths[0])
	a.importCancel = cancel

	lib := a.lib
	mode := storageModes[a.storageMode]
	var tags []string
	if a.autoTagOnImport && a.tagID != "" {
		tags = []string{a.tagID}
	}
	ignoreHidden := a.ignoreHiddenOnImport
	recursive := a.importRecursive
	autoSelect := a.autoSelectImported
	notifyDup := a.notifyDuplicates
	win := a.win
	go func() {
		defer cancel()
		resolved := expandImportPaths(paths, recursive, ignoreHidden)
		if len(resolved) == 0 || ctx.Err() != nil {
			if win != nil {
				win.Update(func() {
					a.importing = false
					a.importCancel = nil
					a.importStage = ""
				})
			}
			return
		}
		if win != nil {
			totalFiles := len(resolved)
			firstFile := filepath.Base(resolved[0])
			win.Update(func() {
				if a.isClosed() {
					return
				}
				a.importStage = "importing"
				a.importTotal = totalFiles
				a.importCurrent = firstFile
			})
		}
		res, importErr := lib.ImportWithProgress(ctx, resolved, mode, tags, func(p library.ImportProgress) {
			if win == nil {
				return
			}
			prog := p
			win.Update(func() {
				if a.isClosed() {
					return
				}
				if a.importStage != "canceling" {
					a.importStage = "importing"
				}
				a.importDone = prog.Done
				a.importTotal = prog.Total
				a.importAdded = prog.Added
				a.importExisting = prog.Existing
				a.importFailed = prog.Failed
				if prog.Current != "" {
					a.importCurrent = prog.Current
				}
				a.status = fmt.Sprintf(a.t("library.importing"), prog.Done, prog.Total)
			})
		})
		if win == nil {
			return
		}
		win.Update(func() {
			a.importing = false
			a.importCancel = nil
			a.importStage = ""
			if a.isClosed() || a.lib == nil {
				return
			}
			var firstAddedID string
			if importErr != nil && errors.Is(importErr, context.Canceled) {
				a.notify(fmt.Sprintf(a.t("import.canceled"), res.Added, res.Existing))
			} else if importErr != nil {
				a.status = importErr.Error()
				a.notify(importErr.Error())
			} else {
				if notifyDup || res.Added > 0 || res.Failed > 0 {
					a.notify(fmt.Sprintf(a.t("library.imported"), res.Added, res.Existing, res.Failed))
				}
			}
			for _, item := range res.Items {
				if item.Status == "added" {
					if firstAddedID == "" {
						firstAddedID = item.AssetID
					}
					if a.thumbs != nil {
						a.thumbs.Enqueue(item.AssetID)
						a.queued[item.AssetID] = true
					}
				}
				if item.Err != nil && errors.Is(item.Err, library.ErrSymlink) {
					a.notify(a.t("library.symlinkFailed"))
				}
			}
			a.reload()
			if autoSelect && firstAddedID != "" {
				for i, item := range a.assets {
					if item.ID == firstAddedID {
						a.assetIndex = i
						a.syncInspectorSelection(true)
						break
					}
				}
			}
		})
	}()
}

func (a *App) cancelImport() {
	if a.importCancel != nil {
		a.importStage = "canceling"
		a.importCancel()
	}
}

func isIgnoredImportFile(name string) bool {
	if name == "" {
		return true
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return library.IsAuxiliarySidecarFile(name)
}

func expandImportPaths(paths []string, recursive, ignoreHidden bool) []string {
	var out []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if ignoreHidden && isIgnoredImportFile(filepath.Base(p)) {
				continue
			}
			out = append(out, p)
			continue
		}
		if !recursive {
			entries, err := os.ReadDir(p)
			if err != nil {
				continue
			}
			for _, ent := range entries {
				if ent.IsDir() {
					continue
				}
				if ignoreHidden && isIgnoredImportFile(ent.Name()) {
					continue
				}
				out = append(out, filepath.Join(p, ent.Name()))
			}
			continue
		}
		_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != p && ignoreHidden && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if ignoreHidden && isIgnoredImportFile(d.Name()) {
				return nil
			}
			out = append(out, path)
			return nil
		})
	}
	return out
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
				a.notify(fmt.Sprintf(a.t("inspector.relinkResult"), count))
			} else {
				a.notify(a.t("inspector.relinkNone"))
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
	a.newProjTags = make(map[string]bool, len(a.projectTags))
	for id, on := range a.projectTags {
		if on {
			a.newProjTags[id] = true
		}
	}
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
	for _, tg := range a.tags {
		want := a.newProjTags[tg.ID]
		if want != a.projectTags[tg.ID] {
			_ = a.projects.SetTag(a.projectID, tg.ID, want)
		}
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
	a.closePreview()
	a.flushBoard(true)
	a.session = nil
	a.boardID = ""
	a.boardProject = ""
	a.projectID = id
	a.projSearchQuery = ""
	a.projKindFilter = 0
	a.referenceMode = false
	a.screen = screenProject
	a.reload()
}

func (a *App) openBoard(b model.Board) {
	a.closePreview()
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
	if b.ProjectID == "" {
		a.tray = 1 // Standalone boards default to Library tray.
	}
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
			a.notify(fmt.Sprintf(a.t("library.referencedIn"), p.Name))
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
				a.closePreview()
				a.flushBoard(true)
			}
		}
		if a.backupBeforeDelete && a.lib != nil {
			_ = a.lib.DB.Backup()
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
	name := asset.Label()
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
	case model.StorageMove:
		return a.t("library.deleteMove")
	case model.StorageCopy:
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
	a.clearSelection()
	switch {
	case nav == "manage-tags":
		a.tagManagerOpen = true
		return
	case nav == "library" || nav == "tag-all":
		a.closePreview()
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = ""
		a.navFilter = ""
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case nav == "filter-untagged":
		a.closePreview()
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = ""
		a.navFilter = "untagged"
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case nav == "filter-recent":
		a.closePreview()
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = ""
		a.navFilter = "recent"
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case nav == "filter-random":
		a.closePreview()
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = ""
		a.navFilter = "random"
		a.randomSeed = time.Now().UnixNano()
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case nav == "filter-missing":
		a.closePreview()
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		a.tagID = ""
		a.navFilter = "missing"
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case strings.HasPrefix(nav, "tag:"):
		a.closePreview()
		a.flushBoard(true)
		a.session = nil
		a.referenceMode = false
		tid := strings.TrimPrefix(nav, "tag:")
		if a.screen == screenLibrary && a.tagID == tid && len(a.tagChildren[tid]) > 0 {
			a.toggleTagExpand(tid)
		} else if len(a.tagChildren[tid]) > 0 {
			a.expandedTags[tid] = true
		}
		a.tagID = tid
		a.navFilter = ""
		a.assetIndex = -1
		a.screen = screenLibrary
		a.reload()
	case strings.HasPrefix(nav, "project:"):
		pid := strings.TrimPrefix(nav, "project:")
		a.openProject(pid)
	case strings.HasPrefix(nav, "board:"):
		bid := strings.TrimPrefix(nav, "board:")
		if a.projects != nil {
			if b, err := a.projects.GetBoard(bid); err == nil {
				a.openBoard(b)
			}
		}
	}
}

func (a *App) syncNavSelection() {
	if a.screen == screenBoard && a.boardProject == "" && a.boardID != "" {
		a.selectedNav = "board:" + a.boardID
		a.lastNav = a.selectedNav
	} else if a.screen == screenProject || a.screen == screenBoard {
		if a.projectID != "" {
			a.selectedNav = "project:" + a.projectID
			a.lastNav = a.selectedNav
		}
	} else if a.screen == screenLibrary {
		switch {
		case a.navFilter == "untagged":
			a.selectedNav = "filter-untagged"
		case a.navFilter == "recent":
			a.selectedNav = "filter-recent"
		case a.navFilter == "random":
			a.selectedNav = "filter-random"
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

// --- Multi-Select & Batch Operations ---

func (a *App) selectCard(index int, mods gui.Modifiers) {
	if index < 0 || index >= len(a.assets) {
		return
	}
	id := a.assets[index].ID
	if a.selectedIDs == nil {
		a.selectedIDs = map[string]bool{}
	}
	ctrl := mods&(gui.Cmd|gui.Ctrl) != 0
	shift := mods&gui.Shift != 0
	switch {
	case shift && a.anchorIndex >= 0 && a.anchorIndex < len(a.assets):
		lo, hi := a.anchorIndex, index
		if lo > hi {
			lo, hi = hi, lo
		}
		if !ctrl {
			a.selectedIDs = map[string]bool{}
		}
		for i := lo; i <= hi; i++ {
			a.selectedIDs[a.assets[i].ID] = true
		}
		a.assetIndex = index
	case ctrl || a.multiSelectMode:
		if a.selectedIDs[id] {
			delete(a.selectedIDs, id)
		} else {
			a.selectedIDs[id] = true
		}
		a.assetIndex = index
		a.anchorIndex = index
	default:
		a.selectedIDs = map[string]bool{id: true}
		a.assetIndex = index
		a.anchorIndex = index
	}
	a.inspectorOpen = true
	a.syncInspectorSelection(false)
}

func (a *App) toggleCardSelection(index int) {
	if index < 0 || index >= len(a.assets) {
		return
	}
	id := a.assets[index].ID
	if a.selectedIDs == nil {
		a.selectedIDs = map[string]bool{}
	}
	if a.selectedIDs[id] {
		delete(a.selectedIDs, id)
	} else {
		a.selectedIDs[id] = true
		a.assetIndex = index
		a.anchorIndex = index
	}
	a.syncInspectorSelection(false)
}

func (a *App) isCardSelected(index int, id string) bool {
	if a.selectedIDs[id] {
		return true
	}
	return len(a.selectedIDs) == 0 && index == a.assetIndex
}

func (a *App) selectedAssetIDs() []string {
	var out []string
	seen := map[string]bool{}
	for _, asset := range a.assets {
		if a.selectedIDs[asset.ID] {
			out = append(out, asset.ID)
			seen[asset.ID] = true
		}
	}
	if len(out) == 0 {
		if cur, ok := a.selectedAsset(); ok && !seen[cur.ID] {
			out = append(out, cur.ID)
		}
	}
	return out
}

func (a *App) dragIDsFor(assetID string) []string {
	if a.selectedIDs[assetID] && len(a.selectedIDs) > 1 {
		return a.selectedAssetIDs()
	}
	return []string{assetID}
}

func (a *App) selectAllVisible() {
	a.selectedIDs = make(map[string]bool, len(a.assets))
	for _, asset := range a.assets {
		a.selectedIDs[asset.ID] = true
	}
	if len(a.assets) > 0 && a.assetIndex < 0 {
		a.assetIndex = 0
		a.anchorIndex = 0
	}
	a.syncInspectorSelection(false)
}

func (a *App) clearSelection() {
	a.selectedIDs = map[string]bool{}
	a.multiSelectMode = false
	a.marqueeActive = false
	a.marqueeDragging = false
}

func (a *App) canRunLibraryShortcut() bool {
	return a.lib != nil &&
		a.screen == screenLibrary &&
		!a.gotoOpen &&
		!a.onboardingOpen &&
		!a.tagManagerOpen &&
		!a.previewOpen &&
		!a.quickLookOpen &&
		!a.settingsOpen &&
		!a.renameOpen &&
		!a.exportOpen &&
		!a.newProjOpen &&
		!a.editProjOpen &&
		!a.editTagOpen &&
		!a.shareOpen &&
		!a.pairRecvOpen &&
		!a.askOpen
}

func (a *App) stepAssetSelection(delta int, extend bool) {
	if len(a.assets) == 0 {
		return
	}
	next := a.assetIndex + delta
	if a.assetIndex < 0 {
		if delta >= 0 {
			next = 0
		} else {
			next = len(a.assets) - 1
		}
	}
	if next < 0 {
		next = 0
	}
	if next >= len(a.assets) {
		next = len(a.assets) - 1
	}
	var mods gui.Modifiers
	if extend {
		mods = gui.Shift
	}
	a.selectCard(next, mods)
}

func (a *App) batchSetRating(rating int) {
	ids := a.selectedAssetIDs()
	if len(ids) == 0 || a.lib == nil {
		return
	}
	for _, id := range ids {
		if err := a.lib.SetRating(id, rating); err == nil {
			if cur, ok := a.assetByID[id]; ok {
				cur.Rating = rating
				a.updateAssetInMemory(cur)
			}
			if a.inspectorAssetID == id {
				a.inspectorRating = rating
			}
		}
	}
	if a.minRating > 0 || a.sortBy == sortRating {
		a.applyFilters()
	}
}

func (a *App) notify(msg string) {
	a.status = msg
	if msg == "" {
		a.toastVisible = false
		a.toastMsg = ""
		return
	}
	a.toastSeq++
	seq := a.toastSeq
	a.toastMsg = msg
	a.toastVisible = true
	win := a.win
	if win == nil {
		return
	}
	time.AfterFunc(3500*time.Millisecond, func() {
		win.Update(func() {
			if a.isClosed() || a.toastSeq != seq {
				return
			}
			a.toastVisible = false
			if a.status == msg {
				a.status = ""
			}
		})
	})
}

func (a *App) registerWindowShortcuts(c *gui.Context) {
	if a.lib == nil {
		return
	}
	c.OnShortcut(gui.Cmd, gui.KeyB, func() {
		if a.screen == screenLibrary || a.screen == screenProject {
			a.sidebarOpen = !a.sidebarOpen
		}
	})
	c.OnShortcut(gui.Cmd, gui.KeyT, func() {
		if !a.gotoOpen {
			a.openGotoModal()
		} else {
			a.gotoOpen = false
		}
	})
	c.OnShortcut(gui.Cmd, gui.KeyComma, func() {
		a.openSettings(a.settingsTab)
	})
	c.OnShortcut(gui.Cmd, gui.KeyF, func() {
		if a.canRunLibraryShortcut() {
			a.searchHandle.Focus(c)
		}
	})
	c.OnShortcut(gui.Cmd, gui.KeyI, func() {
		if a.canRunLibraryShortcut() {
			a.startImport(false)
		}
	})
	c.OnShortcut(gui.Cmd, gui.KeyA, func() {
		if a.canRunLibraryShortcut() {
			a.selectAllVisible()
		}
	})
	c.OnShortcut(gui.Cmd|gui.Alt, gui.KeyG, func() {
		a.grayscalePreview = !a.grayscalePreview
		a.syncConfigFromState()
	})
	c.OnShortcut(gui.Cmd|gui.Shift, gui.KeyU, func() {
		a.webImportOpen = true
		a.webImportURL = ""
	})
	c.OnShortcut(gui.Cmd, gui.KeyG, func() {
		if a.canRunLibraryShortcut() {
			if ids := a.selectedAssetIDs(); len(ids) >= 2 {
				a.stackSelectedAssets(ids)
			}
		}
	})
	c.OnShortcut(gui.Cmd, gui.KeyC, func() {
		if a.canRunLibraryShortcut() {
			if ids := a.selectedAssetIDs(); len(ids) > 0 {
				a.copySelectedFilesToClipboard(ids)
			}
		}
	})
	c.OnShortcut(gui.Cmd, gui.KeyV, func() {
		if a.canRunLibraryShortcut() {
			a.pasteFromClipboard()
		}
	})
	c.OnShortcut(gui.Cmd, gui.KeyZ, func() {
		if a.screen == screenBoard && a.session != nil && !a.askOpen && !a.settingsOpen && !a.gotoOpen && !a.webImportOpen {
			a.boardUndo()
			return
		}
		if a.canRunLibraryShortcut() && a.lib.HasRenameUndo() {
			a.undoLastBatchRename()
		}
	})
	c.OnShortcut(gui.Cmd|gui.Shift, gui.KeyZ, func() {
		if a.screen == screenBoard && a.session != nil && !a.askOpen && !a.settingsOpen && !a.gotoOpen && !a.webImportOpen {
			a.boardRedo()
		}
	})
	c.OnShortcut(gui.Cmd, gui.KeyY, func() {
		if a.screen == screenBoard && a.session != nil && !a.askOpen && !a.settingsOpen && !a.gotoOpen && !a.webImportOpen {
			a.boardRedo()
		}
	})
	c.OnShortcut(0, gui.KeyF2, func() {
		if a.canRunLibraryShortcut() {
			if ids := a.selectedAssetIDs(); len(ids) > 0 {
				a.openRenameModal(ids)
			}
		}
	})
	c.OnShortcut(0, gui.KeyEscape, func() {
		if a.previewOpen || a.quickLookOpen {
			a.closePreview()
			return
		}
		if !a.canRunLibraryShortcut() {
			return
		}
		if a.openStackID != "" {
			a.closeStackView()
			return
		}
		if a.newTagOpen {
			a.newTagOpen = false
			a.newTagName = ""
			return
		}
		if len(a.selectedIDs) > 0 || a.multiSelectMode || a.assetIndex >= 0 {
			a.clearSelection()
			a.assetIndex = -1
			a.syncInspectorSelection(false)
		}
	})
	c.OnShortcut(0, gui.KeySpace, func() {
		a.handleSpacePreview()
	})
	c.OnShortcut(0, gui.KeyDelete, func() {
		if a.screen == screenBoard && a.session != nil && !c.FocusTakesText() && !a.askOpen && !a.settingsOpen && !a.gotoOpen {
			a.deleteBoardSelectionInstant()
			return
		}
		if a.canRunLibraryShortcut() {
			if ids := a.selectedAssetIDs(); len(ids) > 0 {
				a.batchDeletePrompt(ids)
			}
		}
	})
	c.OnShortcut(0, gui.KeyBackspace, func() {
		if a.screen == screenBoard && a.session != nil && !c.FocusTakesText() && !a.askOpen && !a.settingsOpen && !a.gotoOpen {
			a.deleteBoardSelectionInstant()
		}
	})
	c.OnShortcut(0, gui.KeyLeft, func() {
		if a.previewOpen || a.quickLookOpen {
			a.stepPreview(-1)
			return
		}
		if a.canRunLibraryShortcut() {
			a.stepAssetSelection(-1, false)
		}
	})
	c.OnShortcut(0, gui.KeyRight, func() {
		if a.previewOpen || a.quickLookOpen {
			a.stepPreview(1)
			return
		}
		if a.canRunLibraryShortcut() {
			a.stepAssetSelection(1, false)
		}
	})
	c.OnShortcut(gui.Shift, gui.KeyLeft, func() {
		if a.canRunLibraryShortcut() {
			a.stepAssetSelection(-1, true)
		}
	})
	c.OnShortcut(gui.Shift, gui.KeyRight, func() {
		if a.canRunLibraryShortcut() {
			a.stepAssetSelection(1, true)
		}
	})
	ratingKeys := []struct {
		key gui.Key
		val int
	}{
		{gui.Key0, 0},
		{gui.Key1, 1},
		{gui.Key2, 2},
		{gui.Key3, 3},
		{gui.Key4, 4},
		{gui.Key5, 5},
	}
	for _, rk := range ratingKeys {
		r := rk
		c.OnShortcut(0, r.key, func() {
			if a.canRunLibraryShortcut() {
				a.batchSetRating(r.val)
			}
		})
	}
}

func (a *App) batchReferenceToProject(ids []string, projectID string) {
	if len(ids) == 0 || projectID == "" || a.projects == nil {
		return
	}
	added := 0
	for _, id := range ids {
		if err := a.projects.Reference(projectID, id); err == nil {
			added++
		}
	}
	projName := projectID
	for _, p := range a.projectList {
		if p.ID == projectID {
			projName = p.Name
			break
		}
	}
	a.notify(fmt.Sprintf(a.t("batch.addedProject"), added, projName))
	a.reload()
}

func (a *App) batchApplyTag(ids []string, tagID string) {
	if len(ids) == 0 || tagID == "" || a.lib == nil {
		return
	}
	applied := 0
	for _, id := range ids {
		if err := a.lib.SetAssetTag(id, tagID, true); err == nil {
			applied++
		}
	}
	tagName := a.tagByID[tagID].Name
	a.notify(fmt.Sprintf(a.t("batch.appliedTag"), applied, tagName))
	a.reload()
}

func (a *App) batchDeleteMessage(ids []string) string {
	var copyCount, moveCount int
	for _, id := range ids {
		asset, ok := a.assetByID[id]
		if !ok {
			continue
		}
		switch asset.StorageMode {
		case model.StorageCopy:
			copyCount++
		case model.StorageMove:
			moveCount++
		}
	}
	total := len(ids)
	switch {
	case moveCount > 0:
		return fmt.Sprintf(a.t("batch.deleteWithMove"), total, moveCount, total-moveCount)
	case copyCount > 0:
		return fmt.Sprintf(a.t("batch.deleteSafeCopy"), total, copyCount)
	default:
		return fmt.Sprintf(a.t("batch.deleteSafeIndex"), total)
	}
}

func (a *App) batchDeletePrompt(ids []string) {
	if len(ids) == 0 || a.lib == nil {
		return
	}
	if len(ids) == 1 {
		if asset, ok := a.assetByID[ids[0]]; ok {
			a.deleteAssetPrompt(asset)
			return
		}
	}
	list := append([]string(nil), ids...)
	msg := a.batchDeleteMessage(list)
	a.ask(a.t("batch.delete"), msg, a.t("common.delete"), func() {
		if a.lib == nil {
			return
		}
		idSet := make(map[string]bool, len(list))
		for _, id := range list {
			idSet[id] = true
			delete(a.bitmaps, id)
			if a.previewAssetID == id {
				a.closePreview()
			}
		}
		if a.session != nil {
			var drop []string
			for _, item := range a.session.Items {
				if item.Kind == model.KindImage && idSet[item.AssetID] {
					drop = append(drop, item.ID)
				}
			}
			if len(drop) > 0 {
				a.session.Delete(drop)
				a.flushBoard(true)
			}
		}

		finish := func(deleted int, err error) {
			a.clearSelection()
			a.assetIndex = -1
			if err != nil {
				a.notify(err.Error())
			} else {
				a.notify(fmt.Sprintf(a.t("batch.deleted"), deleted))
			}
			a.reload()
		}

		if a.win == nil {
			deleted, err := a.lib.DeleteBatch(list)
			finish(deleted, err)
			return
		}

		lib := a.lib
		win := a.win
		a.notify(fmt.Sprintf(a.t("batch.deleting"), len(list)))
		go func() {
			deleted, err := lib.DeleteBatch(list)
			win.Update(func() {
				if a.isClosed() || a.lib != lib {
					return
				}
				finish(deleted, err)
			})
		}()
	})
}

// --- Hierarchical Tag Management & Color Helpers ---

type tagColorChoice struct {
	key string
	hex string
}

var tagColorPalette = []tagColorChoice{
	{key: "tags.color.red", hex: "#EF4444"},
	{key: "tags.color.orange", hex: "#F97316"},
	{key: "tags.color.amber", hex: "#EAB308"},
	{key: "tags.color.green", hex: "#22C55E"},
	{key: "tags.color.cyan", hex: "#06B6D4"},
	{key: "tags.color.blue", hex: "#3B82F6"},
	{key: "tags.color.purple", hex: "#A855F7"},
	{key: "tags.color.pink", hex: "#EC4899"},
}

// effectiveTagColor returns a tag's display color (inheriting its nearest colored
// ancestor so child folders stay visually cohesive, like Eagle), or false when
// ShowTagColors is disabled or no color is assigned.
func (a *App) effectiveTagColor(tagID string) (gui.Color, bool) {
	if !a.showTagColors || tagID == "" {
		return gui.Color{}, false
	}
	walk := tagID
	for depth := 0; walk != "" && depth < 16; depth++ {
		tg, ok := a.tagByID[walk]
		if !ok {
			break
		}
		if tg.Color != "" {
			if col, ok := parseHexColor(tg.Color); ok {
				return col, true
			}
		}
		walk = tg.ParentID
	}
	return gui.Color{}, false
}

func (a *App) setTagColor(id, hex string) {
	if a.lib == nil || id == "" {
		return
	}
	if err := a.lib.SetTagColor(id, hex); err != nil {
		a.status = err.Error()
		return
	}
	a.reload()
}

func (a *App) moveTagParent(id, parentID string) {
	if a.lib == nil || id == "" {
		return
	}
	tg, ok := a.tagByID[id]
	if !ok {
		return
	}
	if _, err := a.lib.UpdateTag(id, tg.Name, parentID); err != nil {
		if errors.Is(err, library.ErrTagCycle) {
			a.status = a.t("tags.cycleErr")
		} else {
			a.status = err.Error()
		}
		return
	}
	if parentID != "" {
		a.expandedTags[parentID] = true
	}
	a.reload()
}

func (a *App) expandAllTags(expand bool) {
	if a.expandedTags == nil {
		a.expandedTags = map[string]bool{}
	}
	if !expand {
		a.expandedTags = map[string]bool{}
		return
	}
	for _, tg := range a.tags {
		if len(a.tagChildren[tg.ID]) > 0 {
			a.expandedTags[tg.ID] = true
		}
	}
}

func (a *App) createTagWithParent(name, parentID string) {
	a.createTagWithParentAndColor(name, parentID, "")
}

func (a *App) createTagWithParentAndColor(name, parentID, color string) {
	name = trim(name)
	if name == "" {
		a.status = a.t("tag.needName")
		return
	}
	created, err := a.lib.CreateTagWithParent(name, parentID)
	if err != nil {
		if errors.Is(err, library.ErrDuplicateName) {
			a.status = a.t("tag.exists")
			return
		}
		a.status = err.Error()
		return
	}
	if color != "" {
		_ = a.lib.SetTagColor(created.ID, color)
	}
	if parentID != "" {
		a.expandedTags[parentID] = true
	}
	a.reload()
}

func (a *App) openEditTagModal(tg model.Tag) {
	a.editTagID = tg.ID
	a.editTagName = tg.Name
	a.editTagParentID = tg.ParentID
	a.editTagColor = tg.Color
	a.editTagOpen = true
}

func (a *App) saveEditTag() {
	if a.lib == nil || a.editTagID == "" {
		return
	}
	name := trim(a.editTagName)
	if name == "" {
		a.status = a.t("tag.needName")
		return
	}
	if _, err := a.lib.UpdateTag(a.editTagID, name, a.editTagParentID); err != nil {
		switch {
		case errors.Is(err, library.ErrDuplicateName):
			a.status = a.t("tag.exists")
		case errors.Is(err, library.ErrTagCycle):
			a.status = a.t("tags.cycleErr")
		default:
			a.status = err.Error()
		}
		return
	}
	_ = a.lib.SetTagColor(a.editTagID, a.editTagColor)
	if a.editTagParentID != "" {
		a.expandedTags[a.editTagParentID] = true
	}
	a.editTagOpen = false
	a.reload()
}

func (a *App) toggleTagExpand(id string) {
	if a.expandedTags == nil {
		a.expandedTags = map[string]bool{}
	}
	a.expandedTags[id] = !a.expandedTags[id]
}

// --- Stage 13: Standalone Board Actions ---

func (a *App) createStandaloneBoard() {
	if a.projects == nil {
		return
	}
	name := trim(a.newStandaloneBoardName)
	if name == "" {
		name = a.t("board.newStandalone")
	}
	b, err := a.projects.CreateStandaloneBoard(name)
	if err != nil {
		a.notify(err.Error())
		return
	}
	a.newStandaloneBoardOpen = false
	a.newStandaloneBoardName = ""
	a.openBoard(b)
}

func (a *App) sendAssetsToBoard(ids []string, boardID string) {
	if a.projects == nil || len(ids) == 0 || boardID == "" {
		return
	}
	n, err := a.projects.AddAssetsToBoard(boardID, ids)
	if err != nil {
		a.notify(err.Error())
		return
	}
	bName := boardID
	if b, err := a.projects.GetBoard(boardID); err == nil {
		bName = b.Name
	}
	a.reload()
	a.notify(fmt.Sprintf(a.t("board.addedTo"), n, bName))
}

func (a *App) sendAssetsToNewBoard(ids []string) {
	if a.projects == nil || len(ids) == 0 {
		return
	}
	name := fmt.Sprintf("%s %s", a.t("shell.boards"), time.Now().Format("01-02 15:04"))
	b, err := a.projects.CreateStandaloneBoard(name)
	if err != nil {
		a.notify(err.Error())
		return
	}
	if _, err := a.projects.AddAssetsToBoard(b.ID, ids); err != nil {
		a.notify(err.Error())
		return
	}
	a.openBoard(b)
}

func (a *App) moveBoardToProject(boardID, projectID string) {
	if a.projects == nil || boardID == "" || projectID == "" {
		return
	}
	if err := a.projects.MoveBoardToProject(boardID, projectID); err != nil {
		a.notify(err.Error())
		return
	}
	if a.boardID == boardID {
		a.boardProject = projectID
		a.projectID = projectID
	}
	projName := projectID
	for _, p := range a.projectList {
		if p.ID == projectID {
			projName = p.Name
			break
		}
	}
	a.reload()
	a.notify(fmt.Sprintf(a.t("board.movedToProject"), projName))
}

func hashOrderKey(id string, seed int64) uint64 {
	var h uint64 = 14695981039346656037 ^ uint64(seed)
	for i := 0; i < len(id); i++ {
		h ^= uint64(id[i])
		h *= 1099511628211
	}
	return h
}

func (a *App) deleteStandaloneBoardPrompt(b model.Board) {
	a.ask(
		a.t("board.deleteBoard"),
		fmt.Sprintf(a.t("board.deleteBoardConfirm"), b.Name),
		a.t("common.delete"),
		func() {
			if a.projects == nil {
				return
			}
			if a.boardID == b.ID {
				a.session = nil
				a.boardID = ""
				a.boardProject = ""
				a.screen = screenLibrary
			}
			if err := a.projects.DeleteBoard(b.ID); err != nil {
				a.notify(err.Error())
				return
			}
			a.reload()
		},
	)
}
