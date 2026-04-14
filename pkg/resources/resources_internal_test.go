package resources

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResourceError_Error(t *testing.T) {
	err := &resourceError{code: 1, msg: "test error"}
	if err.Error() != "test error" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test error")
	}
}

func TestInferMIMEType(t *testing.T) {
	dir := t.TempDir()

	// Case 1: Known extension.
	f1 := filepath.Join(dir, "test.txt")
	os.WriteFile(f1, []byte("hello"), 0644)
	if got := inferMIMEType(f1); got != "text/plain; charset=utf-8" {
		t.Errorf("inferMIMEType(test.txt) = %q", got)
	}

	// Case 2: Unknown extension, sniff content.
	f2 := filepath.Join(dir, "test.unknown")
	os.WriteFile(f2, []byte("<html><body>hi</body></html>"), 0644)
	if got := inferMIMEType(f2); got != "text/html; charset=utf-8" {
		t.Errorf("inferMIMEType(test.unknown) = %q", got)
	}

	// Case 3: Empty file, no extension.
	f3 := filepath.Join(dir, "empty")
	os.WriteFile(f3, nil, 0644)
	if got := inferMIMEType(f3); got != "application/octet-stream" {
		t.Errorf("inferMIMEType(empty) = %q", got)
	}

	// Case 4: Non-existent file.
	if got := inferMIMEType("/nonexistent"); got != "application/octet-stream" {
		t.Errorf("inferMIMEType(/nonexistent) = %q", got)
	}
}

func TestIsTextMIME(t *testing.T) {
	tests := []struct {
		mime string
		want bool
	}{
		{"text/plain", true},
		{"text/html", true},
		{"application/json", true},
		{"application/yaml", true},
		{"application/javascript", true},
		{"application/x-sh", true},
		{"image/png", false},
		{"application/pdf", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		if got := isTextMIME(tt.mime); got != tt.want {
			t.Errorf("isTextMIME(%q) = %v, want %v", tt.mime, got, tt.want)
		}
	}
}

func TestFileMetadata_Errors(t *testing.T) {
	// Test error reading file (permissions).
	// Skip this part if running as root, because root can read anything.
	if os.Getuid() != 0 {
		dir := t.TempDir()
		f := filepath.Join(dir, "unreadable.txt")
		os.WriteFile(f, []byte("no read"), 0000)

		_, _, _, err := fileMetadata(f)
		if err == nil {
			t.Error("expected error for unreadable file, got nil")
		}
	}

	// Test non-existent file.
	_, _, _, err := fileMetadata("/nonexistent")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}
