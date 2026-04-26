package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

const (
	rescanInterval = 60 * time.Second
	eventBufSize   = 1024
)

type Watcher struct {
	root           string
	fw             *fsnotify.Watcher
	agg            *Aggregator
	scanner        *Scanner
	excludes       []string
	logger         zerolog.Logger
	Out            chan ScanEvent
	rescanInterval time.Duration
	rescanning     atomic.Bool // guards against concurrent full-rescans
	closing        atomic.Bool
	wg             sync.WaitGroup
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
		w.logger.Debug().Str("path", event.Name).Msg("excluding event")
		return
	}
	if event.Has(fsnotify.Create) {
		info, err := os.Lstat(event.Name)
		if err == nil && info.IsDir() {
			w.logger.Info().Str("path", event.Name).Msg("directory created, adding watch")
			w.startBackground(func() {
				path := event.Name
				if err := w.addDirsRecursive(path); err != nil {
					w.logger.Error().Err(err).Str("path", path).Msg("failed to watch new directory")
					return
				}
				// Emit create events for any files that already existed inside
				// the newly-appeared directory (e.g. a directory rename/move).
				_ = filepath.WalkDir(path, func(p string, d os.DirEntry, werr error) error {
					if werr != nil || d.IsDir() || w.isExcluded(p) {
						return nil
					}
					w.logger.Debug().Str("path", p).Msg("file in new directory")
					w.agg.Push(p, EventCreate)
					return nil
				})
			})
			return
		}
		w.logger.Debug().Str("path", event.Name).Msg("file created")
	}
	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		w.logger.Info().Str("path", event.Name).Msg("file removed/renamed")
		_ = w.fw.Remove(event.Name)
		w.scanner.Remove(event.Name)
		w.agg.Push(event.Name, EventRemove)
		return
	}
	if changed, eventType := w.scanner.HasChanged(event.Name); changed {
		w.logger.Debug().Str("path", event.Name).Str("type", eventType.String()).Msg("file changed")
		w.agg.Push(event.Name, eventType)
	}
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
	if !w.agg.Shutdown(ctx) {
		w.logger.Warn().Msg("skipping final watcher flush during shutdown because cancellation fired first")
	}
	close(w.Out)
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
