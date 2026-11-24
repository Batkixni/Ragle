// Package watcher notices moved library files, tries to relink them by hash,
// and monitors user-configured Watch Folders to auto-import new files live.
package watcher

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"raglex/internal/library"
	"raglex/internal/model"
)

const debounce = 500 * time.Millisecond

// WatchRule configures a local folder source that automatically imports new files.
type WatchRule struct {
	Path           string
	TagName        string
	StorageMode    string // default: model.StorageIndex
	Recursive      bool
	Enabled        bool
	IsClipperInbox bool // also ingests .raglex-clip.json offline browser clipper sidecars
}

// Watcher watches the library's files directory, indexed asset folders, and
// user-configured Watch Folders.
type Watcher struct {
	lib      *library.Library
	fs       *fsnotify.Watcher
	onChange func()
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	mu          sync.Mutex
	timers      map[string]*time.Timer
	watched     map[string]bool
	rules       []WatchRule
	onClipRoute func(ClipRouteEvent)
}

// SetClipRouteHandler registers an optional callback invoked when an offline
// browser clip specifies a target ProjectID or BoardID.
func (w *Watcher) SetClipRouteHandler(fn func(ClipRouteEvent)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.onClipRoute = fn
	w.mu.Unlock()
}

// Start begins watching. onChange runs after a record changes; it is called
// from the watcher goroutine.
func Start(lib *library.Library, onChange func()) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watcher{
		lib:      lib,
		fs:       fsw,
		onChange: onChange,
		ctx:      ctx,
		cancel:   cancel,
		timers:   map[string]*time.Timer{},
		watched:  map[string]bool{},
	}
	if err := w.Sync(); err != nil {
		cancel()
		_ = fsw.Close()
		return nil, err
	}
	w.wg.Add(1)
	go w.loop()
	return w, nil
}

// SetWatchRules updates the active Watch Folders and immediately attaches
// fsnotify watches for every enabled folder (and subdirectories when recursive).
func (w *Watcher) SetWatchRules(rules []WatchRule) {
	if w == nil {
		return
	}
	cleaned := make([]WatchRule, 0, len(rules))
	for _, r := range rules {
		p := strings.TrimSpace(r.Path)
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		r.Path = filepath.Clean(p)
		if r.StorageMode == "" {
			r.StorageMode = model.StorageIndex
		}
		cleaned = append(cleaned, r)
	}
	w.mu.Lock()
	w.rules = cleaned
	w.mu.Unlock()
	_ = w.Sync()
}

// ScanWatchRules performs a one-shot scan across all enabled Watch Folders,
// importing any files not already in the library (deduplicated by SHA-256).
func (w *Watcher) ScanWatchRules(ctx context.Context) (int, error) {
	if w == nil || w.lib == nil {
		return 0, nil
	}
	w.mu.Lock()
	rules := append([]WatchRule(nil), w.rules...)
	w.mu.Unlock()

	totalAdded := 0
	for _, rule := range rules {
		if !rule.Enabled || rule.Path == "" {
			continue
		}
		added, err := w.scanSingleRule(ctx, rule)
		if err != nil {
			continue
		}
		totalAdded += added
	}
	if totalAdded > 0 && w.onChange != nil {
		w.onChange()
	}
	return totalAdded, nil
}

func (w *Watcher) scanSingleRule(ctx context.Context, rule WatchRule) (int, error) {
	st, err := os.Stat(rule.Path)
	if err != nil || !st.IsDir() {
		return 0, err
	}
	var files []string
	var sidecars []string
	collect := func(p, base string) {
		if rule.IsClipperInbox && isClipSidecar(base) {
			sidecars = append(sidecars, p)
			return
		}
		if isIgnoredWatchFile(base) {
			return
		}
		files = append(files, p)
	}
	if rule.Recursive {
		_ = filepath.WalkDir(rule.Path, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			base := d.Name()
			if d.IsDir() {
				if p != rule.Path && strings.HasPrefix(base, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			collect(p, base)
			return nil
		})
	} else {
		entries, err := os.ReadDir(rule.Path)
		if err != nil {
			return 0, err
		}
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			collect(filepath.Join(rule.Path, ent.Name()), ent.Name())
		}
	}

	totalAdded := 0
	// Process offline browser clipper sidecars first so paired images receive their
	// exact tags, source URLs, notes, and project/board routing immediately.
	for _, sc := range sidecars {
		if n, _ := w.ingestClipSidecar(ctx, sc, rule); n > 0 {
			totalAdded += n
		}
	}

	if len(files) == 0 {
		return totalAdded, nil
	}
	var tagIDs []string
	if rule.TagName != "" {
		if tid := w.lib.EnsureTagID(rule.TagName); tid != "" {
			tagIDs = []string{tid}
		}
	}
	res, err := w.lib.Import(ctx, files, rule.StorageMode, tagIDs, nil)
	if err != nil {
		return totalAdded, err
	}
	return totalAdded + res.Added, nil
}

// Sync adds watches for Watch Folders and the directories the library cares about,
// capped at 256 directories so large indexed trees never exhaust OS handles.
func (w *Watcher) Sync() error {
	const maxWatchDirs = 256
	w.mu.Lock()
	rules := append([]WatchRule(nil), w.rules...)
	w.mu.Unlock()

	var priorityDirs []string
	for _, rule := range rules {
		if !rule.Enabled || rule.Path == "" {
			continue
		}
		if rule.Recursive {
			_ = filepath.WalkDir(rule.Path, func(p string, d fs.DirEntry, err error) error {
				if err != nil || !d.IsDir() {
					return nil
				}
				if p != rule.Path && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				priorityDirs = append(priorityDirs, p)
				if len(priorityDirs) >= 128 {
					return fs.SkipAll
				}
				return nil
			})
		} else {
			priorityDirs = append(priorityDirs, rule.Path)
		}
	}

	libDirs, err := w.lib.WatchDirs()
	if err != nil {
		return err
	}
	allDirs := append(priorityDirs, libDirs...)

	w.mu.Lock()
	defer w.mu.Unlock()
	for _, dir := range allDirs {
		if len(w.watched) >= maxWatchDirs {
			break
		}
		dir = filepath.Clean(dir)
		if w.watched[dir] {
			continue
		}
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		if err := w.fs.Add(dir); err != nil {
			continue
		}
		w.watched[dir] = true
	}
	return nil
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	if w == nil {
		return nil
	}
	w.cancel()
	err := w.fs.Close()
	w.wg.Wait()
	w.mu.Lock()
	for _, timer := range w.timers {
		timer.Stop()
	}
	w.mu.Unlock()
	return err
}

func (w *Watcher) loop() {
	defer w.wg.Done()
	for {
		select {
		case <-w.ctx.Done():
			return
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			w.schedule(ev)
		case _, ok := <-w.fs.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Watcher) schedule(ev fsnotify.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if timer := w.timers[ev.Name]; timer != nil {
		timer.Stop()
	}
	w.timers[ev.Name] = time.AfterFunc(debounce, func() {
		w.dispatch(ev)
	})
}

func (w *Watcher) matchWatchRule(filePath string) (WatchRule, bool) {
	clean := filepath.Clean(filePath)
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, rule := range w.rules {
		if !rule.Enabled || rule.Path == "" {
			continue
		}
		rel, err := filepath.Rel(rule.Path, clean)
		if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
			continue
		}
		if !rule.Recursive && strings.ContainsRune(rel, filepath.Separator) {
			continue
		}
		return rule, true
	}
	return WatchRule{}, false
}

func (w *Watcher) dispatch(ev fsnotify.Event) {
	changed := false
	switch {
	case ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename):
		ok, err := w.lib.MarkMissing(ev.Name)
		changed = err == nil && ok
	case ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write):
		info, err := os.Stat(ev.Name)
		if err != nil {
			return
		}
		if info.IsDir() {
			// If a new subdirectory is created inside a recursive Watch Folder, watch it too.
			if rule, ok := w.matchWatchRule(ev.Name); ok && rule.Recursive {
				_ = w.Sync()
			}
			return
		}
		// 1. Try relinking a previously moved asset first.
		if ok, err := w.lib.TryRelink(ev.Name); err == nil && ok {
			changed = true
		} else if rule, ok := w.matchWatchRule(ev.Name); ok {
			base := filepath.Base(ev.Name)
			if rule.IsClipperInbox && isClipSidecar(base) {
				if _, handled := w.ingestClipSidecar(w.ctx, ev.Name, rule); handled {
					changed = true
				}
			} else if !isIgnoredWatchFile(base) {
				if rule.IsClipperInbox {
					scPath := ev.Name + ClipSidecarSuffix
					if _, err := os.Stat(scPath); err == nil {
						if _, handled := w.ingestClipSidecar(w.ctx, scPath, rule); handled {
							changed = true
							break
						}
					}
				}
				// 2. Auto-import new file from an active Watch Folder!
				var tagIDs []string
				if rule.TagName != "" {
					if tid := w.lib.EnsureTagID(rule.TagName); tid != "" {
						tagIDs = []string{tid}
					}
				}
				if res, err := w.lib.Import(w.ctx, []string{ev.Name}, rule.StorageMode, tagIDs, nil); err == nil && res.Added > 0 {
					changed = true
				}
			}
		}
	}
	if changed && w.onChange != nil {
		w.onChange()
	}
}

func isIgnoredWatchFile(base string) bool {
	if base == "" || strings.HasPrefix(base, ".") {
		return true
	}
	low := strings.ToLower(base)
	switch {
	case low == "thumbs.db", low == "desktop.ini",
		strings.HasSuffix(low, ClipSidecarSuffix),
		strings.HasSuffix(low, ".tmp"),
		strings.HasSuffix(low, ".part"),
		strings.HasSuffix(low, ".crdownload"),
		strings.HasSuffix(low, ".download"):
		return true
	}
	return library.IsAuxiliarySidecarFile(base)
}
