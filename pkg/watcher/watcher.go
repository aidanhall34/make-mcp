package watcher

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Event is emitted after the debounce window settles.
type Event struct {
	Path string
}

// Watcher wraps fsnotify and coalesces rapid writes into a single reload event.
type Watcher struct {
	raw    *fsnotify.Watcher
	settle time.Duration

	events chan Event
	errors chan error
	done   chan struct{}

	mu      sync.Mutex
	pending map[string]time.Time
	timer   *time.Timer
}

// New creates a file watcher for the given paths.
func New(paths []string, settle time.Duration) (*Watcher, error) {
	raw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if settle <= 0 {
		settle = 200 * time.Millisecond
	}

	w := &Watcher{
		raw:     raw,
		settle:  settle,
		events:  make(chan Event, 16),
		errors:  make(chan error, 16),
		done:    make(chan struct{}),
		pending: map[string]time.Time{},
	}

	for _, path := range paths {
		if err := w.raw.Add(path); err != nil {
			raw.Close()
			return nil, err
		}
	}

	go w.loop()
	return w, nil
}

// Events returns debounced reload events.
func (w *Watcher) Events() <-chan Event { return w.events }

// Errors returns watcher errors.
func (w *Watcher) Errors() <-chan error { return w.errors }

// Add starts watching an additional path.
func (w *Watcher) Add(path string) error {
	return w.raw.Add(path)
}

// Remove stops watching a path.
func (w *Watcher) Remove(path string) error {
	return w.raw.Remove(path)
}

// Close stops the watcher and closes the output channels.
func (w *Watcher) Close() error {
	close(w.done)
	return w.raw.Close()
}

func (w *Watcher) loop() {
	defer func() {
		// Hold mu so flush() can't race on w.events between its done-check
		// and the actual channel close.
		w.mu.Lock()
		close(w.events)
		close(w.errors)
		w.mu.Unlock()
	}()

	for {
		select {
		case <-w.done:
			return
		case err, ok := <-w.raw.Errors:
			if !ok {
				return
			}
			select {
			case w.errors <- err:
			default:
			}
		case event, ok := <-w.raw.Events:
			if !ok {
				return
			}
			if !shouldHandle(event) {
				continue
			}
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				// Re-add watch in case the file was replaced (atomic save)
				_ = w.raw.Add(event.Name)
			}
			w.enqueue(event.Name)
		}
	}
}

func shouldHandle(event fsnotify.Event) bool {
	return event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename)
}

func (w *Watcher) enqueue(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending[filepath.Clean(path)] = time.Now().Add(w.settle)
	if w.timer == nil {
		w.timer = time.AfterFunc(w.settle, w.flush)
		return
	}
	w.timer.Reset(w.settle)
}

func (w *Watcher) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check whether the watcher has been stopped. If so, w.events is about to
	// be (or has been) closed under w.mu in loop's defer; skip the send.
	select {
	case <-w.done:
		return
	default:
	}

	var nextSettle time.Duration
	now := time.Now()

	for path, readyAt := range w.pending {
		if now.After(readyAt) || now.Equal(readyAt) {
			delete(w.pending, path)
			select {
			case w.events <- Event{Path: path}:
			default:
				// If channel is full, we still want to try sending later or
				// just drop it if we must, but for reload, we should try to
				// keep it. However, a full buffer usually means the consumer
				// is stuck. We'll re-add it to pending to try again.
				w.pending[path] = now.Add(w.settle)
			}
			continue
		}

		// File not ready yet, track when it will be
		wait := time.Until(readyAt)
		if nextSettle == 0 || wait < nextSettle {
			nextSettle = wait
		}
	}

	if nextSettle > 0 {
		if w.timer == nil {
			w.timer = time.AfterFunc(nextSettle, w.flush)
		} else {
			w.timer.Reset(nextSettle)
		}
	}
}
