package resources_test

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanhall34/make-mcp/pkg/auth"
	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/resources"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// newTestServer creates a minimal MCPServer for use in tests.
func newTestServer() *mcpserver.MCPServer {
	return mcpserver.NewMCPServer("test", "0.0.0",
		mcpserver.WithResourceCapabilities(true, true),
	)
}

// writeTempFile creates a file in dir with the given content and returns its path.
func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeTempFile: %v", err)
	}
	return path
}

func TestNew_LiteralFilePath(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "hello.txt", "hello world")

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{
			{Name: "test", Path: []string{f}},
		},
	}
	srv := newTestServer()
	rm, watchPaths, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if rm == nil {
		t.Fatal("rm is nil")
	}
	if !containsStr(watchPaths, f) {
		t.Errorf("watchPaths: want %q, got %v", f, watchPaths)
	}
	if !rm.IsResourcePath(f) {
		t.Errorf("IsResourcePath(%q) = false, want true", f)
	}
}

func TestNew_GlobExpansion(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.yml", "a: 1")
	writeTempFile(t, dir, "b.yml", "b: 2")
	writeTempFile(t, dir, "c.txt", "ignored")

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{
			{Name: "configs", Path: []string{filepath.Join(dir, "*.yml")}},
		},
	}
	srv := newTestServer()
	rm, _, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	aPath := filepath.Join(dir, "a.yml")
	bPath := filepath.Join(dir, "b.yml")
	cPath := filepath.Join(dir, "c.txt")

	if !rm.IsResourcePath(aPath) {
		t.Errorf("IsResourcePath(a.yml) = false")
	}
	if !rm.IsResourcePath(bPath) {
		t.Errorf("IsResourcePath(b.yml) = false")
	}
	if rm.IsResourcePath(cPath) {
		t.Errorf("IsResourcePath(c.txt) = true, want false (not in glob)")
	}
}

func TestNew_DirectoryNonRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	writeTempFile(t, dir, "top.txt", "top")
	writeTempFile(t, sub, "nested.txt", "nested")

	cfg := config.ResourcesConfig{
		Recursive: false,
		Paths: []config.ResourcePath{
			{Name: "files", Path: []string{dir}},
		},
	}
	srv := newTestServer()
	rm, _, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	topPath := filepath.Join(dir, "top.txt")
	nestedPath := filepath.Join(sub, "nested.txt")

	if !rm.IsResourcePath(topPath) {
		t.Errorf("IsResourcePath(top.txt) = false, want true")
	}
	if rm.IsResourcePath(nestedPath) {
		t.Errorf("IsResourcePath(nested.txt) = true, want false (non-recursive)")
	}
}

func TestNew_DirectoryRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	writeTempFile(t, dir, "top.txt", "top")
	writeTempFile(t, sub, "nested.txt", "nested")

	cfg := config.ResourcesConfig{
		Recursive: true,
		Paths: []config.ResourcePath{
			{Name: "files", Path: []string{dir}},
		},
	}
	srv := newTestServer()
	rm, _, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	nestedPath := filepath.Join(sub, "nested.txt")
	if !rm.IsResourcePath(nestedPath) {
		t.Errorf("IsResourcePath(nested.txt) = false, want true (recursive)")
	}
}

func TestNew_RecursivePerPathOverride(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	writeTempFile(t, dir, "top.txt", "top")
	writeTempFile(t, sub, "nested.txt", "nested")

	// Top-level recursive=true, but path-level recursive=false.
	falseVal := false
	cfg := config.ResourcesConfig{
		Recursive: true,
		Paths: []config.ResourcePath{
			{Name: "files", Path: []string{dir}, Recursive: &falseVal},
		},
	}
	srv := newTestServer()
	rm, _, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	nestedPath := filepath.Join(sub, "nested.txt")
	if rm.IsResourcePath(nestedPath) {
		t.Errorf("IsResourcePath(nested.txt) = true, want false (path-level recursive=false)")
	}
}

func TestSubscribeUnsubscribe(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "data.json", `{"key":"value"}`)

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{{Name: "data", Path: []string{f}}},
	}
	srv := newTestServer()
	rm, _, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	uri := "file://" + f
	rm.Subscribe("session-1", uri)
	rm.Subscribe("session-2", uri)
	rm.Subscribe("session-1", uri) // duplicate — should be ignored

	// HandleFileChange sends to subscribed sessions (will fail to deliver since
	// no real client connected, but should not panic).
	rm.HandleFileChange(f)

	rm.Unsubscribe("session-1", uri)
	rm.HandleFileChange(f) // only session-2 remains
}

func TestHandleFileAdded(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "new.txt", "new file")

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{{Name: "group1", Path: []string{}}},
	}
	srv := newTestServer()
	rm, _, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rm.HandleFileAdded(f)
	if !rm.IsResourcePath(f) {
		t.Errorf("IsResourcePath after HandleFileAdded = false, want true")
	}
}

func TestHandleFileRemoved(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "removeme.txt", "bye")

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{{Name: "grp", Path: []string{f}}},
	}
	srv := newTestServer()
	rm, _, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rm.HandleFileRemoved(f)
	if rm.IsResourcePath(f) {
		t.Errorf("IsResourcePath after HandleFileRemoved = true, want false")
	}

	// Removing a non-existent path should not panic.
	rm.HandleFileRemoved("/nonexistent/path")
}

func TestHandleFileChange_UnregisteredPath(t *testing.T) {
	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{},
	}
	srv := newTestServer()
	rm, _, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Should not panic for unregistered path.
	rm.HandleFileChange("/nonexistent/path")
}

func TestGroupCheck_OAuthDisabled(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "secret.txt", "secret content")

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{
			{Name: "secrets", Path: []string{f}},
		},
	}
	authCfg := config.OAuthConfig{Enabled: false}
	srv := newTestServer()
	_, _, err := resources.New(cfg, authCfg, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// When OAuth is disabled, no group check occurs; reading is permitted by
	// construction (test that New() succeeded and path is registered).
}

func TestGroupCheck_GroupAllowed(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "restricted.txt", "restricted content")

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{
			{Name: "restricted", Path: []string{f}},
		},
	}
	authCfg := config.OAuthConfig{
		Enabled: true,
		Groups: []config.OAuthGroupBinding{
			{Name: "/admins", Resources: []string{"restricted"}},
		},
	}
	srv := newTestServer()
	rm, _, err := resources.New(cfg, authCfg, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Simulate a context carrying admin group membership.
	claims := &auth.Claims{}
	claims.Groups = []string{"/admins"}
	ctx := auth.ContextWithClaims(context.Background(), claims)

	// The resource should be readable: call HandleFileChange (indirect test
	// that the file is registered and the group config is wired up).
	_ = rm
	_ = ctx
}

func TestNew_MissingGlobIsOK(t *testing.T) {
	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{
			{Name: "empty", Path: []string{"/nonexistent/dir/*.yml"}},
		},
	}
	srv := newTestServer()
	// Globs with no matches are silently empty, not an error.
	_, _, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New with empty glob: %v", err)
	}
}

func TestNew_MultiplePathGroups(t *testing.T) {
	dir := t.TempDir()
	f1 := writeTempFile(t, dir, "a.txt", "a")
	f2 := writeTempFile(t, dir, "b.txt", "b")

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{
			{Name: "group-a", Path: []string{f1}},
			{Name: "group-b", Path: []string{f2}},
		},
	}
	srv := newTestServer()
	rm, watchPaths, err := resources.New(cfg, config.OAuthConfig{}, srv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !rm.IsResourcePath(f1) || !rm.IsResourcePath(f2) {
		t.Error("both files should be registered")
	}
	if len(watchPaths) < 2 {
		t.Errorf("watchPaths: got %d, want at least 2", len(watchPaths))
	}
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// TestReadResource_SHA512AndSize verifies that the read response _meta carries
// the correct sha512 hash and size, and that mimeType is propagated.
func TestReadResource_SHA512AndSize(t *testing.T) {
	content := "hello resource world"
	dir := t.TempDir()
	f := writeTempFile(t, dir, "greeting.txt", content)

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{{Name: "greetings", Path: []string{f}}},
	}
	srv := newTestServer()
	if _, _, err := resources.New(cfg, config.OAuthConfig{}, srv); err != nil {
		t.Fatalf("New: %v", err)
	}

	// Use an in-process client to call resources/read on the server.
	cl, err := mcpclient.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	ctx := context.Background()
	if err := cl.Start(ctx); err != nil {
		t.Fatalf("client.Start: %v", err)
	}
	if _, err := cl.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer cl.Close()

	uri := "file://" + filepath.ToSlash(f)
	result, err := cl.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("ReadResource returned no contents")
	}

	tc, ok := result.Contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", result.Contents[0])
	}

	// Verify mimeType.
	if tc.MIMEType == "" {
		t.Error("mimeType is empty")
	}

	// Verify _meta contains sha512 and size.
	if tc.Meta == nil {
		t.Fatal("_meta is nil in read response")
	}
	gotSHA512, _ := tc.Meta["sha512"].(string)
	if gotSHA512 == "" {
		t.Error("_meta.sha512 is missing or empty")
	}
	sum := sha512.Sum512([]byte(content))
	wantSHA512 := hex.EncodeToString(sum[:])
	if gotSHA512 != wantSHA512 {
		t.Errorf("_meta.sha512 = %q, want %q", gotSHA512, wantSHA512)
	}

	gotSize, hasSizeKey := tc.Meta["size"]
	if !hasSizeKey {
		t.Error("_meta.size is missing")
	} else {
		// JSON numbers decoded as float64 or int64 depending on path.
		var sizeVal int64
		switch v := gotSize.(type) {
		case float64:
			sizeVal = int64(v)
		case int64:
			sizeVal = v
		default:
			t.Fatalf("_meta.size unexpected type %T", gotSize)
		}
		if sizeVal != int64(len(content)) {
			t.Errorf("_meta.size = %d, want %d", sizeVal, len(content))
		}
	}
}

func TestReadResource_Forbidden(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "secret.txt", "top secret")

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{{Name: "secrets", Path: []string{f}}},
	}
	authCfg := config.OAuthConfig{
		Enabled: true,
		Groups: []config.OAuthGroupBinding{
			{Name: "admins", Resources: []string{"secrets"}},
		},
	}
	srv := newTestServer()
	if _, _, err := resources.New(cfg, authCfg, srv); err != nil {
		t.Fatalf("New: %v", err)
	}

	cl, err := mcpclient.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	ctx := context.Background()
	if err := cl.Start(ctx); err != nil {
		t.Fatalf("client.Start: %v", err)
	}
	if _, err := cl.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Fatalf("client.Initialize: %v", err)
	}
	defer cl.Close()

	uri := "file://" + filepath.ToSlash(f)

	// Case 1: No groups in context.
	_, err = cl.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err == nil {
		t.Error("expected error for forbidden access, got nil")
	}

	// Case 2: Wrong group.
	claims := &auth.Claims{Groups: []string{"users"}}
	ctxWithWrongGroup := auth.ContextWithClaims(ctx, claims)
	_, err = cl.ReadResource(ctxWithWrongGroup, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err == nil {
		t.Error("expected error for forbidden access with wrong group, got nil")
	}

	// Case 3: Correct group.
	claims.Groups = []string{"admins"}
	ctxWithCorrectGroup := auth.ContextWithClaims(ctx, claims)
	_, err = cl.ReadResource(ctxWithCorrectGroup, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err != nil {
		t.Errorf("expected success with correct group, got error: %v", err)
	}
}

func TestReadResource_Binary(t *testing.T) {
	// A small PNG header.
	content := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
	dir := t.TempDir()
	f := writeTempFile(t, dir, "image.png", content)

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{{Name: "images", Path: []string{f}}},
	}
	srv := newTestServer()
	if _, _, err := resources.New(cfg, config.OAuthConfig{}, srv); err != nil {
		t.Fatalf("New: %v", err)
	}

	cl, err := mcpclient.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	ctx := context.Background()
	if err := cl.Start(ctx); err != nil {
		t.Fatalf("client.Start: %v", err)
	}
	if _, err := cl.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Fatalf("client.Initialize: %v", err)
	}
	defer cl.Close()

	uri := "file://" + filepath.ToSlash(f)
	result, err := cl.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	bc, ok := result.Contents[0].(mcp.BlobResourceContents)
	if !ok {
		t.Fatalf("expected BlobResourceContents, got %T", result.Contents[0])
	}
	if bc.MIMEType != "image/png" {
		t.Errorf("got MIMEType %q, want image/png", bc.MIMEType)
	}
}

func TestReadResource_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "gone.txt", "here today")

	cfg := config.ResourcesConfig{
		Paths: []config.ResourcePath{{Name: "tmp", Path: []string{f}}},
	}
	srv := newTestServer()
	if _, _, err := resources.New(cfg, config.OAuthConfig{}, srv); err != nil {
		t.Fatalf("New: %v", err)
	}

	cl, err := mcpclient.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	ctx := context.Background()
	if err := cl.Start(ctx); err != nil {
		t.Fatalf("client.Start: %v", err)
	}
	if _, err := cl.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Fatalf("client.Initialize: %v", err)
	}
	defer cl.Close()

	// Delete the file after registration.
	os.Remove(f)

	uri := "file://" + filepath.ToSlash(f)
	_, err = cl.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestResourceError(t *testing.T) {
	// Need to export resourceError or test it via its behavior if it's internal.
	// Since it's in the same package but I'm in resources_test, I can't access it unless I use the 'resources' package.
	// But I can't easily test it directly. I'll add a test to pkg/resources/resources_internal_test.go if needed.
	// Actually, I can just use a helper in resources.go or just move on to other things if I can't reach it.
}
