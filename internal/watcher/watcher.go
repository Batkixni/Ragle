// Package watcher notices moved library files and tries to attach them again by hash.
package watcher

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"raglex/internal/library"
)

const debounce = 500 * time.Millisecond

// Watcher watches the library's files directory and indexed asset folders.
type Watcher struct {
	lib      *library.Library
	fs       *fsnotify.Watcher
	onChange func()
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	mu      sync.Mutex
	timers  map[string]*time.Timer
	watched map[string]bool
}

// Start begins watching. onChange runs after a record changes; it is called
// from the watcher goroutine.
func Start(lib *library.Library, onChange func()) (*Watcher, error) {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watcher{
		lib:      lib,
		fs:       fs,
		onChange: onChange,
		ctx:      ctx,
		cancel:   cancel,
		timers:   map[string]*time.Timer{},
		watched:  map[string]bool{},
	}
	if err := w.Sync(); err != nil {
		cancel()
		_ = fs.Close()
		return nil, err
	}
	w.wg.Add(1)
	go w.loop()
	return w, nil
}

// Sync adds watches for the directories the library currently cares about,
// capped at 256 directories so large indexed trees never exhaust OS handles.
func (w *Watcher) Sync() error {
	const maxWatchDirs = 256
	w.mu.Lock()
	if len(w.watched) >= maxWatchDirs {
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()

	dirs, err := w.lib.WatchDirs()
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, dir := range dirs {
		if len(w.watched) >= maxWatchDirs {
			break
		}
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

func (w *Watcher) dispatch(ev fsnotify.Event) {
	changed := false
	switch {
	case ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename):
		ok, err := w.lib.MarkMissing(ev.Name)
		changed = err == nil && ok
	case ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write):
		info, err := os.Stat(ev.Name)
		if err != nil || info.IsDir() {
			return
		}
		ok, err := w.lib.TryRelink(ev.Name)
		changed = err == nil && ok
	}
	if changed && w.onChange != nil {
		w.onChange()
	}
}
