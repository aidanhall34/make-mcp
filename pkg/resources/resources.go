// Package resources implements MCP file resources. It expands configured
// globs, directories, and file paths into concrete MCP resources, serves their
// content, and routes file-system change events into list-changed and
// resource-updated MCP notifications.
package resources

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/auth"
	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/telemetry"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// ResourceManager tracks and serves file-backed MCP resources. It is safe for
// concurrent use from multiple goroutines.
type ResourceManager struct {
	authCfg config.OAuthConfig
	srv     *mcpserver.MCPServer

	mu           sync.RWMutex
	pathToGroup  map[string]string   // abs file path → ResourcePath.Name
	fileToURI    map[string]string   // abs file path → file:// URI
	groupAllowed map[string][]string // ResourcePath.Name → allowed OAuth group names

	subMu         sync.RWMutex
	subscriptions map[string][]string // URI → []sessionID
}

// New expands all configured resource paths into concrete file URIs, registers
// them with srv, and returns the manager and the list of absolute paths/dirs
// that should be added to the file watcher.
func New(cfg config.ResourcesConfig, authCfg config.OAuthConfig, srv *mcpserver.MCPServer) (*ResourceManager, []string, error) {
	rm := &ResourceManager{
		authCfg:       authCfg,
		srv:           srv,
		pathToGroup:   make(map[string]string),
		fileToURI:     make(map[string]string),
		groupAllowed:  buildGroupAllowed(authCfg),
		subscriptions: make(map[string][]string),
	}

	var watchPaths []string

	for _, rp := range cfg.Paths {
		recursive := cfg.Recursive
		if rp.Recursive != nil {
			recursive = *rp.Recursive
		}

		files, dirs, err := expandPath(rp.Path, recursive)
		if err != nil {
			return nil, nil, fmt.Errorf("resources: expand path %q: %w", rp.Name, err)
		}

		for _, absPath := range files {
			rm.registerFile(absPath, rp.Name, rp.Description)
			telemetry.RecordFilesWatched(context.Background(), 1)
		}
		watchPaths = append(watchPaths, files...)
		watchPaths = append(watchPaths, dirs...)
	}

	return rm, watchPaths, nil
}

// buildGroupAllowed constructs a map from ResourcePath.Name → allowed OAuth
// group names from the OAuthConfig groups bindings.
func buildGroupAllowed(authCfg config.OAuthConfig) map[string][]string {
	m := make(map[string][]string)
	for _, g := range authCfg.Groups {
		for _, res := range g.Resources {
			m[res] = append(m[res], g.Name)
		}
	}
	return m
}

// fileMetadata returns the sha512 hex digest, byte size, and last-modified
// time for absPath. Errors are non-fatal: callers may proceed with zero
// values when the file is temporarily unreadable.
func fileMetadata(absPath string) (sha512hex string, fileSize int64, lastMod time.Time, err error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		// Fall back to stat-only info for size and time.
		if info, serr := os.Stat(absPath); serr == nil {
			fileSize = info.Size()
			lastMod = info.ModTime()
		}
		return "", fileSize, lastMod, err
	}
	sum := sha512.Sum512(data)
	sha512hex = hex.EncodeToString(sum[:])
	fileSize = int64(len(data))
	if info, serr := os.Stat(absPath); serr == nil {
		lastMod = info.ModTime()
	}
	return sha512hex, fileSize, lastMod, nil
}

// registerFile adds a single file to the MCP server as a resource.
// Callers must hold no locks; this method acquires mu.
func (rm *ResourceManager) registerFile(absPath, groupName, description string) {
	uri := pathToURI(absPath)
	mimeType := inferMIMEType(absPath)

	sha512hex, fileSize, lastMod, _ := fileMetadata(absPath)

	opts := []mcp.ResourceOption{mcp.WithMIMEType(mimeType)}
	if description != "" {
		opts = append(opts, mcp.WithResourceDescription(description))
	}
	if !lastMod.IsZero() {
		opts = append(opts, mcp.WithLastModified(lastMod.UTC().Format(time.RFC3339)))
	}
	resource := mcp.NewResource(uri, filepath.Base(absPath), opts...)
	// Store sha512 and size in _meta since the MCP Resource struct has no
	// dedicated size field and Annotations has no hash field.
	if sha512hex != "" || fileSize > 0 {
		meta := map[string]any{}
		if sha512hex != "" {
			meta["sha512"] = sha512hex
		}
		if fileSize > 0 {
			meta["size"] = fileSize
		}
		resource.Meta = mcp.NewMetaFromMap(meta)
	}

	rm.mu.Lock()
	rm.pathToGroup[absPath] = groupName
	rm.fileToURI[absPath] = uri
	rm.mu.Unlock()

	rm.srv.AddResource(resource, rm.makeHandler(groupName))
}

// makeHandler returns a ResourceHandlerFunc for the given group. The handler
// checks OAuth group membership and reads the file.
func (rm *ResourceManager) makeHandler(groupName string) mcpserver.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		start := time.Now()
		uri := req.Params.URI
		bytesIn := int64(len(uri))

		// Group-based access control.
		if rm.authCfg.Enabled {
			callerGroups := auth.GroupsFromContext(ctx)
			if !rm.isGroupAllowed(callerGroups, groupName) {
				telemetry.RecordResourcesRead(ctx, telemetry.StatusFailure, time.Since(start), 0, bytesIn, 0)
				return nil, &resourceError{code: -32001, msg: "forbidden: insufficient group membership for resource " + groupName}
			}
		}

		absPath := uriToPath(uri)
		data, err := os.ReadFile(absPath)
		if err != nil {
			telemetry.RecordResourcesRead(ctx, telemetry.StatusFailure, time.Since(start), 0, bytesIn, 0)
			return nil, fmt.Errorf("resources: read %s: %w", absPath, err)
		}

		mimeType := inferMIMEType(absPath)
		elapsed := time.Since(start)
		bytesOut := int64(len(data))
		telemetry.RecordResourcesRead(ctx, telemetry.StatusSuccess, elapsed, 1, bytesIn, bytesOut)

		// Include sha512 of the content actually served so clients can verify
		// integrity without a separate stat call.
		sum := sha512.Sum512(data)
		responseMeta := map[string]any{
			"sha512": hex.EncodeToString(sum[:]),
			"size":   int64(len(data)),
		}

		if isTextMIME(mimeType) {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{URI: uri, MIMEType: mimeType, Text: string(data), Meta: responseMeta},
			}, nil
		}
		return []mcp.ResourceContents{
			mcp.BlobResourceContents{URI: uri, MIMEType: mimeType, Blob: base64.StdEncoding.EncodeToString(data), Meta: responseMeta},
		}, nil
	}
}

// isGroupAllowed returns true when at least one of callerGroups is in the
// allowed group list for groupName, or when no groups are configured for
// groupName (open access within the server).
func (rm *ResourceManager) isGroupAllowed(callerGroups []string, groupName string) bool {
	allowed, hasConfig := rm.groupAllowed[groupName]
	if !hasConfig || len(allowed) == 0 {
		return true // no group restriction configured for this path
	}
	for _, cg := range callerGroups {
		for _, ag := range allowed {
			if cg == ag {
				return true
			}
		}
	}
	return false
}

// IsResourcePath returns true when absPath is a registered resource file.
func (rm *ResourceManager) IsResourcePath(absPath string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	_, ok := rm.fileToURI[absPath]
	return ok
}

// HandleFileChange is called by the watcher when a registered resource file's
// content changes.  It sends notifications/resources/updated to subscribed
// sessions.
func (rm *ResourceManager) HandleFileChange(absPath string) {
	rm.mu.RLock()
	uri, ok := rm.fileToURI[absPath]
	rm.mu.RUnlock()
	if !ok {
		return
	}

	rm.subMu.RLock()
	sessions := rm.subscriptions[uri]
	rm.subMu.RUnlock()

	for _, sessionID := range sessions {
		start := time.Now()
		err := rm.srv.SendNotificationToSpecificClient(
			sessionID,
			mcp.MethodNotificationResourceUpdated,
			map[string]any{"uri": uri},
		)
		status := telemetry.StatusSuccess
		if err != nil {
			status = telemetry.StatusFailure
			slog.Warn("resource notification failed",
				"uri", uri,
				"session", sessionID,
				"error", err,
			)
		}
		telemetry.RecordResourceNotification(context.Background(), status, time.Since(start))
	}
}

// HandleFileAdded registers a newly-created file and lets the mcp-go library
// broadcast notifications/resources/list_changed automatically.
func (rm *ResourceManager) HandleFileAdded(absPath string) {
	rm.mu.RLock()
	groupName := rm.pathToGroup[absPath] // may be empty for brand-new files
	rm.mu.RUnlock()

	rm.registerFile(absPath, groupName, "")
	telemetry.RecordFilesWatched(context.Background(), 1)
	slog.Info("resource file added", "path", absPath)
}

// HandleFileRemoved removes a registered resource and lets the mcp-go library
// broadcast notifications/resources/list_changed automatically.
func (rm *ResourceManager) HandleFileRemoved(absPath string) {
	rm.mu.Lock()
	uri, ok := rm.fileToURI[absPath]
	if ok {
		delete(rm.pathToGroup, absPath)
		delete(rm.fileToURI, absPath)
	}
	rm.mu.Unlock()

	if !ok {
		return
	}

	rm.srv.RemoveResource(uri)
	telemetry.RecordFilesWatched(context.Background(), -1)
	slog.Info("resource file removed", "path", absPath, "uri", uri)
}

// Subscribe records sessionID as interested in updates to uri.
func (rm *ResourceManager) Subscribe(sessionID, uri string) {
	rm.subMu.Lock()
	defer rm.subMu.Unlock()
	// Avoid duplicate subscriptions.
	for _, id := range rm.subscriptions[uri] {
		if id == sessionID {
			return
		}
	}
	rm.subscriptions[uri] = append(rm.subscriptions[uri], sessionID)
	telemetry.RecordResourceSubscription(context.Background(), "subscribe", 1)
}

// Unsubscribe removes sessionID's subscription for uri.
func (rm *ResourceManager) Unsubscribe(sessionID, uri string) {
	rm.subMu.Lock()
	defer rm.subMu.Unlock()
	sessions := rm.subscriptions[uri]
	for i, id := range sessions {
		if id == sessionID {
			rm.subscriptions[uri] = append(sessions[:i], sessions[i+1:]...)
			telemetry.RecordResourceSubscription(context.Background(), "unsubscribe", -1)
			return
		}
	}
}

// expandPath resolves a list of path patterns (globs, directories, or literal
// file paths) into concrete absolute file paths and the directory paths that
// should be watched for new files.
func expandPath(patterns []string, recursive bool) (files, dirs []string, err error) {
	seen := make(map[string]struct{})
	add := func(p string) {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			files = append(files, p)
		}
	}

	for _, pattern := range patterns {
		// Check if the pattern is a directory (no glob metacharacters and is a dir).
		if !containsGlobMeta(pattern) {
			abs, err := filepath.Abs(pattern)
			if err != nil {
				return nil, nil, err
			}
			info, statErr := os.Stat(abs)
			if statErr == nil && info.IsDir() {
				dirFiles, err := walkDir(abs, recursive)
				if err != nil {
					return nil, nil, err
				}
				for _, f := range dirFiles {
					add(f)
				}
				dirs = append(dirs, abs)
				continue
			}
			// Literal file path.
			if statErr == nil {
				add(abs)
			}
			continue
		}

		// Glob pattern.
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, nil, fmt.Errorf("glob %q: %w", pattern, err)
		}
		for _, m := range matches {
			abs, err := filepath.Abs(m)
			if err != nil {
				return nil, nil, err
			}
			info, err := os.Stat(abs)
			if err != nil {
				continue
			}
			if info.IsDir() {
				dirFiles, err := walkDir(abs, recursive)
				if err != nil {
					return nil, nil, err
				}
				for _, f := range dirFiles {
					add(f)
				}
				dirs = append(dirs, abs)
			} else {
				add(abs)
			}
		}
	}
	return files, dirs, nil
}

// walkDir lists all regular files in dir. When recursive is true, sub-
// directories are traversed; otherwise only the immediate children are listed.
func walkDir(dir string, recursive bool) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		files = append(files, abs)
		return nil
	})
	return files, err
}

// containsGlobMeta returns true when s contains a glob metacharacter.
func containsGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// pathToURI converts an absolute filesystem path to a file:// URI.
func pathToURI(absPath string) string {
	return "file://" + filepath.ToSlash(absPath)
}

// uriToPath converts a file:// URI to an absolute filesystem path.
func uriToPath(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	return filepath.FromSlash(path)
}

// inferMIMEType returns the MIME type for path. It uses the file extension
// first, then falls back to sniffing the first 512 bytes of content, then to
// "application/octet-stream".
func inferMIMEType(path string) string {
	if t := mime.TypeByExtension(filepath.Ext(path)); t != "" {
		return t
	}
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n > 0 {
		return http.DetectContentType(buf[:n])
	}
	return "application/octet-stream"
}

// isTextMIME returns true for MIME types whose content is human-readable text.
func isTextMIME(mimeType string) bool {
	mt, _, _ := mime.ParseMediaType(mimeType)
	switch {
	case strings.HasPrefix(mt, "text/"):
		return true
	case mt == "application/json",
		mt == "application/xml",
		mt == "application/yaml",
		mt == "application/javascript",
		mt == "application/x-sh":
		return true
	default:
		return false
	}
}

// resourceError is a JSON-RPC error returned from resource handlers.
type resourceError struct {
	code int
	msg  string
}

func (e *resourceError) Error() string { return e.msg }
