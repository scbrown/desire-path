//go:build integration

package integration

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/server"
	_ "github.com/scbrown/desire-path/internal/source" // register source plugins
	"github.com/scbrown/desire-path/internal/store"
)

// newRemoteEnv creates a dpEnv configured in remote mode. It starts an
// httptest.Server backed by a real SQLite store and configures the dp binary
// to connect to it via store_mode=remote and remote_url.
func newRemoteEnv(t *testing.T) (*dpEnv, *httptest.Server) {
	t.Helper()
	e := newEnv(t)

	// Create a server-side store (separate from the dpEnv's local db).
	serverDB := filepath.Join(t.TempDir(), "server.db")
	s, err := store.New(serverDB)
	if err != nil {
		t.Fatalf("open server store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	srv := server.New(s)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Configure dp to use remote mode pointing at the test server.
	e.writeConfig("store_mode = \"remote\"\nremote_url = \"" + ts.URL + "\"\n")

	return e, ts
}

// TestRemoteIngestAndList verifies the full ingest → list round-trip through
// the remote store backed by a real HTTP server.
func TestRemoteIngestAndList(t *testing.T) {
	t.Parallel()
	e, _ := newRemoteEnv(t)

	// Ingest a tool call through the CLI — this should go through RemoteStore → HTTP → server store.
	payload := e.fixture("read_file", "remote-session", "unknown tool")
	e.mustRun(payload, "ingest", "--source", "claude-code")

	// List should return our ingested data.
	stdout, _ := e.mustRun(nil, "list", "--json")
	if !strings.Contains(stdout, "read_file") {
		t.Errorf("remote list missing read_file:\n%s", stdout)
	}
}

// TestRemoteStatsRoundTrip verifies stats reflect data recorded via the
// remote store.
func TestRemoteStatsRoundTrip(t *testing.T) {
	t.Parallel()
	e, _ := newRemoteEnv(t)

	// Empty stats first.
	stdout, _ := e.mustRun(nil, "stats", "--json")
	var stats map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &stats); err != nil {
		t.Fatalf("parse empty stats: %v\noutput: %s", err, stdout)
	}
	if td, _ := stats["total_desires"].(float64); td != 0 {
		t.Errorf("expected 0 desires on empty remote, got %v", td)
	}

	// Ingest some data.
	e.mustRun(e.fixture("read_file", "s1", "err"), "ingest", "--source", "claude-code")
	e.mustRun(e.fixture("write_file", "s2", "err"), "ingest", "--source", "claude-code")

	// Stats should reflect the data.
	stdout, _ = e.mustRun(nil, "stats", "--json")
	if err := json.Unmarshal([]byte(stdout), &stats); err != nil {
		t.Fatalf("parse stats: %v\noutput: %s", err, stdout)
	}
	if td, _ := stats["total_desires"].(float64); td != 2 {
		t.Errorf("expected 2 desires, got %v", td)
	}
}

// TestRemotePathsRoundTrip verifies paths aggregation through the remote store.
func TestRemotePathsRoundTrip(t *testing.T) {
	t.Parallel()
	e, _ := newRemoteEnv(t)

	// Ingest multiple desires for the same tool.
	for i := 0; i < 3; i++ {
		e.mustRun(e.fixture("read_file", "s1", "err"), "ingest", "--source", "claude-code")
	}
	e.mustRun(e.fixture("write_file", "s2", "err"), "ingest", "--source", "claude-code")

	stdout, _ := e.mustRun(nil, "paths", "--json")

	var paths []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &paths); err != nil {
		t.Fatalf("parse paths: %v\noutput: %s", err, stdout)
	}
	if len(paths) < 2 {
		t.Fatalf("expected at least 2 paths, got %d", len(paths))
	}

	// First path should be read_file (most frequent).
	if paths[0]["pattern"] != "read_file" {
		t.Errorf("most frequent path = %v, want read_file", paths[0]["pattern"])
	}
}

// TestRemoteAliasRoundTrip verifies alias CRUD through the remote store.
func TestRemoteAliasRoundTrip(t *testing.T) {
	t.Parallel()
	e, _ := newRemoteEnv(t)

	// Set an alias via CLI → RemoteStore → server.
	e.mustRun(nil, "alias", "read_file", "Read")

	// List aliases — should show our alias.
	stdout, _ := e.mustRun(nil, "aliases", "--json")

	var aliases []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &aliases); err != nil {
		t.Fatalf("parse aliases: %v\noutput: %s", err, stdout)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if aliases[0]["from"] != "read_file" || aliases[0]["to"] != "Read" {
		t.Errorf("alias = %v, want read_file → Read", aliases[0])
	}

	// Delete the alias.
	e.mustRun(nil, "alias", "--delete", "read_file")

	// Verify it's gone.
	stdout, _ = e.mustRun(nil, "aliases", "--json")
	if err := json.Unmarshal([]byte(stdout), &aliases); err != nil {
		t.Fatalf("parse aliases after delete: %v\noutput: %s", err, stdout)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after delete, got %d", len(aliases))
	}
}

// TestRemoteInspectRoundTrip verifies the inspect command through the remote store.
func TestRemoteInspectRoundTrip(t *testing.T) {
	t.Parallel()
	e, _ := newRemoteEnv(t)

	// Ingest some data.
	e.mustRun(e.fixture("read_file", "s1", "unknown tool"), "ingest", "--source", "claude-code")
	e.mustRun(e.fixture("read_file", "s2", "permission denied"), "ingest", "--source", "claude-code")

	// Inspect read_file.
	stdout, _ := e.mustRun(nil, "inspect", "read_file", "--json")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse inspect: %v\noutput: %s", err, stdout)
	}

	total, _ := result["total"].(float64)
	if total != 2 {
		t.Errorf("inspect total = %v, want 2", total)
	}
	if result["pattern"] != "read_file" {
		t.Errorf("inspect pattern = %v, want read_file", result["pattern"])
	}
}

// TestRemoteGoldenPath exercises the full desire-path workflow via the remote
// store: ingest → alias → pave-check → suggest → inspect.
func TestRemoteGoldenPath(t *testing.T) {
	t.Parallel()
	e, _ := newRemoteEnv(t)

	// 1. Ingest a failing tool call.
	e.mustRun(e.fixture("read_file", "gp-session", "unknown tool"), "ingest", "--source", "claude-code")

	// 2. Verify it shows up in list.
	stdout, _ := e.mustRun(nil, "list", "--json")
	if !strings.Contains(stdout, "read_file") {
		t.Fatalf("list missing read_file after remote ingest")
	}

	// 3. Create alias.
	e.mustRun(nil, "alias", "read_file", "Read")

	// 4. Suggest should find the alias.
	stdout, _ = e.mustRun(nil, "suggest", "read_file", "--json")
	if !strings.Contains(stdout, "Read") {
		t.Errorf("suggest missing alias Read:\n%s", stdout)
	}

	// 5. Paths should show the pattern.
	stdout, _ = e.mustRun(nil, "paths", "--json")
	if !strings.Contains(stdout, "read_file") {
		t.Errorf("paths missing read_file:\n%s", stdout)
	}

	// 6. Stats should reflect our data.
	stdout, _ = e.mustRun(nil, "stats", "--json")
	var stats map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &stats); err != nil {
		t.Fatalf("parse stats: %v", err)
	}
	if td, _ := stats["total_desires"].(float64); td != 1 {
		t.Errorf("total_desires = %v, want 1", td)
	}
}

// TestRemoteMultipleClients verifies that multiple dp CLI invocations sharing
// the same remote server see each other's data.
func TestRemoteMultipleClients(t *testing.T) {
	t.Parallel()

	// Create a shared server.
	serverDB := filepath.Join(t.TempDir(), "shared.db")
	s, err := store.New(serverDB)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	srv := server.New(s)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Create two independent dpEnvs pointing at the same server.
	e1 := newEnv(t)
	e1.writeConfig("store_mode = \"remote\"\nremote_url = \"" + ts.URL + "\"\n")

	e2 := newEnv(t)
	e2.writeConfig("store_mode = \"remote\"\nremote_url = \"" + ts.URL + "\"\n")

	// Client 1 ingests data.
	e1.mustRun(e1.fixture("tool_from_client1", "s1", "err"), "ingest", "--source", "claude-code")

	// Client 2 ingests data.
	e2.mustRun(e2.fixture("tool_from_client2", "s2", "err"), "ingest", "--source", "claude-code")

	// Both clients should see both records.
	stdout1, _ := e1.mustRun(nil, "list", "--json")
	stdout2, _ := e2.mustRun(nil, "list", "--json")

	for _, stdout := range []string{stdout1, stdout2} {
		if !strings.Contains(stdout, "tool_from_client1") {
			t.Errorf("missing tool_from_client1:\n%s", stdout)
		}
		if !strings.Contains(stdout, "tool_from_client2") {
			t.Errorf("missing tool_from_client2:\n%s", stdout)
		}
	}
}
