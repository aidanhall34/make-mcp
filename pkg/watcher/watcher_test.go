package watcher

import (
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
