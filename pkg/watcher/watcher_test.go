package watcher

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestShouldHandle(t *testing.T) {
	tests := []struct {
		name  string
		event fsnotify.Event
		want  bool
	}{
		{name: "write", event: fsnotify.Event{Op: fsnotify.Write}, want: true},
		{name: "create", event: fsnotify.Event{Op: fsnotify.Create}, want: true},
		{name: "rename", event: fsnotify.Event{Op: fsnotify.Rename}, want: true},
		{name: "chmod", event: fsnotify.Event{Op: fsnotify.Chmod}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldHandle(tt.event); got != tt.want {
				t.Fatalf("shouldHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWatcherDebouncesPaths(t *testing.T) {
	w := &Watcher{
		settle:  20 * time.Millisecond,
		events:  make(chan Event, 4),
		pending: map[string]time.Time{},
	}

	path := filepath.Clean("/tmp/example")
	w.enqueue(path)
	w.enqueue(path)

	select {
	case event := <-w.events:
		if event.Path != path {
			t.Fatalf("event path = %q, want %q", event.Path, path)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for debounced event")
	}

	select {
	case event := <-w.events:
		t.Fatalf("unexpected extra event: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestNew_ValidPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(path, []byte("hello:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := New([]string{path}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	if w.Events() == nil {
		t.Error("Events() returned nil channel")
	}
	if w.Errors() == nil {
		t.Error("Errors() returned nil channel")
	}
}

func TestNew_InvalidPath(t *testing.T) {
	_, err := New([]string{"/nonexistent/path/Makefile"}, 10*time.Millisecond)
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
}

func TestNew_DefaultSettleTime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Makefile")
	os.WriteFile(path, []byte(""), 0644)

	// settle <= 0 should use the default (200ms)
	w, err := New([]string{path}, 0)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()
	if w.settle != 200*time.Millisecond {
		t.Errorf("settle = %v, want 200ms", w.settle)
	}
}

func TestWatcher_FileChangeEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(path, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := New([]string{path}, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	if err := os.WriteFile(path, []byte("v2\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	select {
	case event, ok := <-w.Events():
		if !ok {
			t.Fatal("events channel closed unexpectedly")
		}
		if event.Path != filepath.Clean(path) {
			t.Errorf("event path = %q, want %q", event.Path, filepath.Clean(path))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for file change event")
	}
}

func TestWatcher_AddRemove(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "a")
	path2 := filepath.Join(dir, "b")
	os.WriteFile(path1, []byte(""), 0644)
	os.WriteFile(path2, []byte(""), 0644)

	w, err := New([]string{path1}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	if err := w.Add(path2); err != nil {
		t.Errorf("Add() error = %v", err)
	}
	if err := w.Remove(path1); err != nil {
		t.Errorf("Remove() error = %v", err)
	}
}

func TestLoop_ChmodNotHandled(t *testing.T) {
	rawW, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify not available: %v", err)
	}
	w := &Watcher{
		raw:     rawW,
		settle:  10 * time.Millisecond,
		events:  make(chan Event, 16),
		errors:  make(chan error, 16),
		done:    make(chan struct{}),
		pending: map[string]time.Time{},
	}
	done := make(chan struct{})
	go func() {
		w.loop()
		close(done)
	}()

	// Send a Chmod event — shouldHandle returns false, so loop continues without
	// enqueueing. No event should appear in w.events.
	rawW.Events <- fsnotify.Event{Name: "/tmp/test-chmod", Op: fsnotify.Chmod}

	select {
	case event := <-w.events:
		t.Errorf("unexpected event from chmod: %+v", event)
	case <-time.After(80 * time.Millisecond):
		// expected: chmod is ignored
	}

	close(w.done)
	<-done
}

func TestLoop_RawWatcherClose(t *testing.T) {
	rawW, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify not available: %v", err)
	}
	w := &Watcher{
		raw:     rawW,
		settle:  10 * time.Millisecond,
		events:  make(chan Event, 16),
		errors:  make(chan error, 16),
		done:    make(chan struct{}),
		pending: map[string]time.Time{},
	}
	done := make(chan struct{})
	go func() {
		w.loop()
		close(done)
	}()

	// Close the raw watcher (closes raw.Events and raw.Errors) without
	// closing w.done — loop should detect !ok and return.
	rawW.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(w.done) // force cleanup
		t.Fatal("loop did not return after raw watcher close")
	}
}

func TestLoop_ErrorForwarded(t *testing.T) {
	rawW, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify not available: %v", err)
	}
	w := &Watcher{
		raw:     rawW,
		settle:  10 * time.Millisecond,
		events:  make(chan Event, 16),
		errors:  make(chan error, 16),
		done:    make(chan struct{}),
		pending: map[string]time.Time{},
	}
	done := make(chan struct{})
	go func() {
		w.loop()
		close(done)
	}()

	want := errors.New("injected watcher error")
	rawW.Errors <- want

	select {
	case got := <-w.errors:
		if got.Error() != want.Error() {
			t.Errorf("errors = %v, want %v", got, want)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for forwarded error")
	}

	close(w.done)
	<-done
}

func TestFlush_FullChannel(t *testing.T) {
	w := &Watcher{
		settle:  10 * time.Millisecond,
		events:  make(chan Event, 1), // small buffer
		pending: map[string]time.Time{},
	}
	path1 := filepath.Clean("/tmp/f1")
	path2 := filepath.Clean("/tmp/f2")
	now := time.Now()
	w.pending[path1] = now.Add(-time.Second) // ready
	w.pending[path2] = now.Add(-time.Second) // ready

	// Fill the buffer with one event
	w.flush()

	// One should be in events, one should be back in pending because buffer was full
	select {
	case <-w.events:
		// good
	default:
		t.Fatal("expected one event in channel")
	}

	w.mu.Lock()
	if len(w.pending) != 1 {
		t.Errorf("len(pending) = %d, want 1 (due to full buffer)", len(w.pending))
	}
	w.mu.Unlock()
}

func TestLoop_ReAddWatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Clean(filepath.Join(dir, "Makefile"))
	os.WriteFile(path, []byte(""), 0644)

	w, err := New([]string{path}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.Close()

	// Send a Create event to simulate atomic save
	w.raw.Events <- fsnotify.Event{Name: path, Op: fsnotify.Create}

	select {
	case event := <-w.events:
		if event.Path != path {
			t.Errorf("path = %q, want %q", event.Path, path)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event after re-add")
	}
}

func TestFlush_MultipleFiles(t *testing.T) {
	w := &Watcher{
		settle:  20 * time.Millisecond,
		events:  make(chan Event, 10),
		pending: map[string]time.Time{},
	}
	now := time.Now()
	path1 := filepath.Clean("/tmp/a")
	path2 := filepath.Clean("/tmp/b")

	w.pending[path1] = now.Add(-time.Second) // ready
	w.pending[path2] = now.Add(time.Second)  // not ready

	w.flush()

	// path1 should be sent
	select {
	case event := <-w.events:
		if event.Path != path1 {
			t.Errorf("got %q, want %q", event.Path, path1)
		}
	default:
		t.Fatal("expected event for path1")
	}

	// path2 should still be pending
	w.mu.Lock()
	if _, ok := w.pending[path2]; !ok {
		t.Error("path2 should still be pending")
	}
	w.mu.Unlock()
}

func TestFlush_RescheduleWithTimer(t *testing.T) {
	w := &Watcher{
		settle:  10 * time.Millisecond,
		events:  make(chan Event, 4),
		pending: map[string]time.Time{},
	}
	path := filepath.Clean("/tmp/reschedule-timer-test")
	w.pending[path] = time.Now().Add(500 * time.Millisecond)
	// Set a real timer so flush hits the w.timer.Reset branch.
	w.timer = time.AfterFunc(10*time.Second, func() {})

	w.flush()

	select {
	case event := <-w.events:
		t.Errorf("unexpected event before path is ready: %+v", event)
	default:
	}
	w.mu.Lock()
	_, still := w.pending[path]
	w.mu.Unlock()
	if !still {
		t.Error("path should still be pending after reschedule")
	}
}

func TestFlush_Reschedule(t *testing.T) {
	w := &Watcher{
		settle:  10 * time.Millisecond,
		events:  make(chan Event, 4),
		pending: map[string]time.Time{},
	}
	path := filepath.Clean("/tmp/reschedule-test")
	// Set a readyAt far in the future so flush sees now.Before(readyAt).
	w.pending[path] = time.Now().Add(500 * time.Millisecond)

	// Call flush directly; path is not yet ready, so it reschedules (timer nil so Reset is skipped).
	w.flush()

	select {
	case event := <-w.events:
		t.Errorf("unexpected event before path is ready: %+v", event)
	default:
	}
	w.mu.Lock()
	_, still := w.pending[path]
	w.mu.Unlock()
	if !still {
		t.Error("path should still be pending after reschedule")
	}
}

func TestWatcher_CloseClosesChannels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Makefile")
	os.WriteFile(path, []byte(""), 0644)

	w, err := New([]string{path}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	select {
	case _, ok := <-w.Events():
		if ok {
			t.Error("expected events channel to be closed after Close()")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for events channel to close")
	}
}
