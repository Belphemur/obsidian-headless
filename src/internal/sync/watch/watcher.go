package watch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

const eventBufSize = 1024

const renameDetectWindow = 150 * time.Millisecond

type pendingRename struct {
	ino   uint64
	path  string
	timer *time.Timer
}

type Watcher struct {
	root                 string
	fw                   *fsnotify.Watcher
	agg                  *Aggregator
	scanner              *Scanner
	excludes             []string
	logger               zerolog.Logger
	Out                  chan ScanEvent
	rescanInterval       time.Duration
	rescanning           atomic.Bool // guards against concurrent full-rescans
	closing              atomic.Bool
	wg                   sync.WaitGroup
	renameMu             sync.Mutex
	inodeTrackingEnabled bool
	pendingRenames       map[uint64]*pendingRename
	pendingRenamePaths   map[string]uint64
	ignoreMu             sync.Mutex
	ignoredOld           map[string]bool // old paths to suppress events for
	ignoredNew           map[string]bool // new paths to suppress events for
}

func New(root string, excludes []string, logger zerolog.Logger, rescanInterval time.Duration) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	out := make(chan ScanEvent, eventBufSize)
	watcher := &Watcher{
		root:           root,
		fw:             fw,
		agg:            NewAggregator(out),
		scanner:        NewScanner(),
		excludes:       excludes,
		logger:         logger,
		Out:            out,
		rescanInterval: rescanInterval,
	}
	watcher.inodeTrackingEnabled = runtime.GOOS != "windows"
	watcher.pendingRenames = make(map[uint64]*pendingRename)
	watcher.pendingRenamePaths = make(map[string]uint64)
	watcher.ignoredOld = make(map[string]bool)
	watcher.ignoredNew = make(map[string]bool)
	if err := watcher.addDirsRecursive(root); err != nil {
		_ = fw.Close()
		return nil, err
	}
	return watcher, nil
}

func (w *Watcher) Run(ctx context.Context) {
	var ticker *time.Ticker
	var tickerCh <-chan time.Time
	if w.rescanInterval > 0 {
		ticker = time.NewTicker(w.rescanInterval)
		defer ticker.Stop()
		tickerCh = ticker.C
	}
	for {
		select {
		case <-ctx.Done():
			w.shutdown(ctx)
			return
		case event, ok := <-w.fw.Events:
			if !ok {
				w.shutdown(ctx)
				return
			}
			w.handle(event)
		case err, ok := <-w.fw.Errors:
			if !ok {
				w.shutdown(ctx)
				return
			}
			w.logger.Error().Err(err).Msg("fsnotify error")
		case <-tickerCh:
			w.startBackground(w.fullRescan)
		}
	}
}

func (w *Watcher) handle(event fsnotify.Event) {
	w.logger.Debug().Str("path", event.Name).Str("ops", event.Op.String()).Msg("fs event")
	if w.isExcluded(event.Name) {
		return
	}

	// Suppress events for paths added via AddIgnorePaths (remote rename aftermath)
	if w.isIgnored(event.Name) {
		w.logger.Debug().Str("path", event.Name).Msg("suppressing event for ignored path")
		return
	}

	// Skip events for paths currently in a pending rename
	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) || event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod) {
		w.renameMu.Lock()
		_, pending := w.pendingRenamePaths[event.Name]
		w.renameMu.Unlock()
		if pending {
			w.logger.Debug().Str("path", event.Name).Msg("skipping event for pending rename path")
			return
		}
	}

	if event.Has(fsnotify.Create) {
		info, err := os.Lstat(event.Name)
		if err == nil && info.IsDir() {
			// Directories: emit create events for all files inside
			w.logger.Info().Str("path", event.Name).Msg("directory created, adding watch")
			w.startBackground(func() {
				path := event.Name
				if err := w.addDirsRecursive(path); err != nil {
					w.logger.Error().Err(err).Str("path", path).Msg("failed to watch new directory")
					return
				}
				_ = filepath.WalkDir(path, func(p string, d os.DirEntry, werr error) error {
					if werr != nil || d.IsDir() || w.isExcluded(p) {
						return nil
					}
					w.logger.Debug().Str("path", p).Msg("file in new directory")
					w.handleFileCreate(p, nil)
					return nil
				})
			})
			return
		}
		w.handleFileCreate(event.Name, info)
		return
	}

	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		w.logger.Info().Str("path", event.Name).Msg("file removed/renamed")
		_ = w.fw.Remove(event.Name)

		var ino uint64
		var hasIno bool
		if w.inodeTrackingEnabled {
			ino, hasIno = w.scanner.GetInode(event.Name)
		}
		w.scanner.Remove(event.Name)

		if w.inodeTrackingEnabled && hasIno && ino != 0 {
			w.deferDeletion(event.Name, ino)
			return
		}
		w.agg.Push(event.Name, EventRemove)
		return
	}

	if changed, eventType := w.scanner.HasChanged(event.Name); changed {
		w.logger.Debug().Str("path", event.Name).Str("type", eventType.String()).Msg("file changed")
		w.agg.Push(event.Name, eventType)
	}
}

func (w *Watcher) handleFileCreate(path string, info os.FileInfo) {
	// Try to match this create against a pending deletion (rename detection)
	if w.inodeTrackingEnabled && info != nil {
		ino := getInode(info)
		if ino != 0 {
			w.renameMu.Lock()
			if pending, ok := w.pendingRenames[ino]; ok {
				// Found matching inode → it's a rename!
				pending.timer.Stop()
				delete(w.pendingRenames, ino)
				delete(w.pendingRenamePaths, pending.path)
				w.renameMu.Unlock()

				w.scanner.UpdateInfo(path, info)
				w.logger.Info().Str("oldPath", pending.path).Str("newPath", path).Msg("rename detected via inode match")
				w.agg.PushRename(path, pending.path)
				return
			}
			// Check for path match with different inode (delete+recreate)
			if oldIno, exists := w.pendingRenamePaths[path]; exists && oldIno != ino {
				if old, ok := w.pendingRenames[oldIno]; ok {
					old.timer.Stop()
					delete(w.pendingRenames, oldIno)
					delete(w.pendingRenamePaths, path)
					w.renameMu.Unlock()
					w.logger.Info().Str("path", path).Msg("new file created at pending deletion path, cancelling deferred deletion")
					// Fall through to normal create
				} else {
					w.renameMu.Unlock()
				}
			} else {
				w.renameMu.Unlock()
			}
		}
	}

	// No rename match: regular file create
	if info != nil {
		w.scanner.UpdateInfo(path, info)
	} else {
		w.scanner.Update(path)
	}
	w.agg.Push(path, EventCreate)
}

func (w *Watcher) deferDeletion(path string, ino uint64) {
	w.renameMu.Lock()
	// Defensive: cancel any existing pending rename for this inode
	if old, ok := w.pendingRenames[ino]; ok {
		old.timer.Stop()
		delete(w.pendingRenamePaths, old.path)
	}
	timer := time.AfterFunc(renameDetectWindow, func() {
		// Timer expired without a matching create → real deletion
		w.renameMu.Lock()
		if _, ok := w.pendingRenames[ino]; ok {
			delete(w.pendingRenames, ino)
			delete(w.pendingRenamePaths, path)
			w.renameMu.Unlock()
			w.agg.Push(path, EventRemove)
			w.logger.Info().Str("path", path).Msg("rename window expired, emitting remove")
		} else {
			w.renameMu.Unlock()
		}
	})
	w.pendingRenames[ino] = &pendingRename{ino: ino, path: path, timer: timer}
	w.pendingRenamePaths[path] = ino
	w.renameMu.Unlock()
}

func (w *Watcher) flushPendingRenames() {
	w.renameMu.Lock()
	defer w.renameMu.Unlock()
	for _, pending := range w.pendingRenames {
		pending.timer.Stop()
		w.agg.Push(pending.path, EventRemove)
	}
	clear(w.pendingRenames)
	clear(w.pendingRenamePaths)
}

// fullRescan walks the entire vault tree looking for metadata changes.
// An atomic guard ensures only one rescan can run at a time, and
// top-level subdirectories are walked concurrently for speed.
func (w *Watcher) fullRescan() {
	if w.closing.Load() {
		return
	}
	if !w.rescanning.CompareAndSwap(false, true) {
		return // another rescan is already in progress
	}
	defer w.rescanning.Store(false)

	entries, err := os.ReadDir(w.root)
	if err != nil {
		w.logger.Error().Err(err).Msg("fullRescan: cannot read root")
		return
	}

	var wg sync.WaitGroup
	for _, entry := range entries {
		child := filepath.Join(w.root, entry.Name())
		if w.isExcluded(child) {
			continue
		}
		if entry.IsDir() {
			wg.Add(1)
			go func(dir string) {
				defer wg.Done()
				w.rescanDir(dir)
			}(child)
		} else {
			if changed, eventType := w.scanner.HasChanged(child); changed {
				w.agg.Push(child, eventType)
			}
		}
	}
	wg.Wait()
}

func (w *Watcher) rescanDir(root string) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if w.closing.Load() || err != nil || d.IsDir() || w.isExcluded(path) {
			return nil
		}
		if changed, eventType := w.scanner.HasChanged(path); changed {
			w.agg.Push(path, eventType)
		}
		return nil
	})
}

// addDirsRecursive registers fsnotify watches for root and all subdirectories,
// and pre-populates the scanner state for every file found.
func (w *Watcher) addDirsRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			w.logger.Warn().Err(err).Str("path", path).Msg("cannot walk path")
			if path == root {
				return err
			}
			return nil
		}
		if w.isExcluded(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if addErr := w.fw.Add(path); addErr != nil {
				w.logger.Warn().Err(addErr).Str("path", path).Msg("cannot watch path")
			}
			return nil
		}
		w.scanner.Update(path)
		return nil
	})
}

func (w *Watcher) startBackground(fn func()) {
	if w.closing.Load() {
		return
	}
	w.wg.Go(func() {
		if w.closing.Load() {
			return
		}
		fn()
	})
}

func (w *Watcher) shutdown(ctx context.Context) {
	if !w.closing.CompareAndSwap(false, true) {
		return
	}
	_ = w.fw.Close()
	w.wg.Wait()
	w.flushPendingRenames()
	// Use a fresh timeout context for aggregator shutdown so that pending
	// events are flushed even when the parent context has been cancelled.
	flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !w.agg.Shutdown(flushCtx) {
		w.logger.Warn().Msg("skipping final watcher flush during shutdown because cancellation fired first")
	}
	close(w.Out)
}

// isIgnored checks whether events for the given path should be suppressed.
// Must be called with ignoreMu NOT held (it acquires the lock internally).
func (w *Watcher) isIgnored(path string) bool {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return false
	}
	w.ignoreMu.Lock()
	defer w.ignoreMu.Unlock()
	// Check both old and new ignore sets
	if w.ignoredOld[rel] || w.ignoredNew[rel] {
		return true
	}
	return false
}

// AddIgnorePaths suppresses fsnotify events for the given rename pairs.
// After a remote rename is enacted on disk, the resulting filesystem events
// must be suppressed to prevent the watcher from interpreting them as new
// user-initiated renames in the next sync cycle.
//
// Paths are normalized to the same relative-slash form used by isIgnored,
// ensuring that equivalent paths (e.g., "./a/b.md", "a/b.md") match the
// suppression lookup regardless of caller format.
func (w *Watcher) AddIgnorePaths(pairs []RenamePair) {
	w.ignoreMu.Lock()
	defer w.ignoreMu.Unlock()
	for _, p := range pairs {
		if old := normalizeIgnoreKey(p.OldPath); old != "" {
			w.ignoredOld[old] = true
		}
		if newPath := normalizeIgnoreKey(p.NewPath); newPath != "" {
			w.ignoredNew[newPath] = true
		}
	}
}

// normalizeIgnoreKey converts a relative path to the normalized form used
// by isIgnored for consistent map lookups: forward slashes, no leading "./"
// or "/", cleaned relative form.
func normalizeIgnoreKey(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	if path == "." {
		return ""
	}
	return path
}

// FlushIgnored clears all ignored paths. Called at the start of each
// doSync cycle so ignorations don't persist across sync cycles.
func (w *Watcher) FlushIgnored() {
	w.ignoreMu.Lock()
	defer w.ignoreMu.Unlock()
	clear(w.ignoredOld)
	clear(w.ignoredNew)
}

func (w *Watcher) isExcluded(path string) bool {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return false
	}
	for _, exclude := range w.excludes {
		exclude = strings.Trim(exclude, "/")
		if exclude == "" {
			continue
		}
		if rel == exclude || strings.HasPrefix(rel, exclude+"/") {
			return true
		}
	}
	return false
}
