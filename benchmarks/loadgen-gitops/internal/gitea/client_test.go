package gitea_test

import (
	"context"
	"os"
	"testing"

	"github.com/tech-comparison-lab/loadgen-gitops/internal/gitea"
)

// Integration tests against a real Gitea instance.
// Skip unless GITEA_URL and GITEA_PASS are set (or Gitea is running at default URL).
//
// Run with:
//   GITEA_URL=http://localhost:3000 GITEA_PASS=benchpass123 go test ./internal/gitea/ -v -run Integration

func newTestClient(t *testing.T) *gitea.Client {
	t.Helper()
	url := os.Getenv("GITEA_URL")
	if url == "" {
		url = "http://localhost:3000"
	}
	pass := os.Getenv("GITEA_PASS")
	if pass == "" {
		pass = "benchpass123"
	}

	c := gitea.NewClient(url, "benchadmin", "", pass)
	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Skipf("Gitea not reachable at %s: %v — set GITEA_URL/GITEA_PASS to run integration tests", url, err)
	}
	return c
}

func TestIntegration_PingOK(t *testing.T) {
	c := newTestClient(t)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestIntegration_EnsureRepo(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	const repo = "test-ensure-repo"
	if err := c.EnsureRepo(ctx, repo); err != nil {
		t.Fatalf("EnsureRepo (create): %v", err)
	}
	// Should be idempotent.
	if err := c.EnsureRepo(ctx, repo); err != nil {
		t.Fatalf("EnsureRepo (existing): %v", err)
	}
}

func TestIntegration_PutAndGetFile(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	const repo = "test-put-get-file"
	if err := c.EnsureRepo(ctx, repo); err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	content := []byte("hello: world\n")
	path := "test/hello.yaml"

	// Create new file (sha="").
	sha, err := c.PutFile(ctx, repo, path, "test: create", content, "")
	if err != nil {
		t.Fatalf("PutFile (create): %v", err)
	}
	if sha == "" {
		t.Fatal("PutFile returned empty SHA")
	}
	t.Logf("created file SHA: %s", sha)

	// Read it back.
	got, gotSHA, err := c.GetFile(ctx, repo, path)
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
	if gotSHA == "" {
		t.Error("GetFile returned empty SHA")
	}

	// Update the file.
	updated := []byte("hello: updated\n")
	sha2, err := c.PutFile(ctx, repo, path, "test: update", updated, gotSHA)
	if err != nil {
		t.Fatalf("PutFile (update): %v", err)
	}
	if sha2 == sha {
		t.Error("SHA should change after update")
	}
	t.Logf("updated file SHA: %s", sha2)

	// Verify update.
	got2, _, err := c.GetFile(ctx, repo, path)
	if err != nil {
		t.Fatalf("GetFile after update: %v", err)
	}
	if string(got2) != string(updated) {
		t.Errorf("content after update: got %q, want %q", got2, updated)
	}
}

func TestIntegration_GetFile_NotFound(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	const repo = "test-not-found"
	if err := c.EnsureRepo(ctx, repo); err != nil {
		t.Fatalf("EnsureRepo: %v", err)
	}

	data, sha, err := c.GetFile(ctx, repo, "nonexistent/file.yaml")
	if err != nil {
		t.Fatalf("GetFile nonexistent: %v", err)
	}
	if data != nil || sha != "" {
		t.Errorf("expected nil,\"\" for missing file, got len=%d sha=%q", len(data), sha)
	}
}
