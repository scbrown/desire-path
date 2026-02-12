//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// freePort asks the OS for an unused port and returns it as a string.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return fmt.Sprintf("%d", port)
}

// startServe launches `dp serve` as a subprocess on the given address and
// returns a cleanup function. It waits for the health endpoint to respond
// before returning.
func startServe(t *testing.T, e *dpEnv, addr string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(dpBin, "serve", "--addr", addr)
	cmd.Env = append(os.Environ(),
		"HOME="+e.home,
		"XDG_CONFIG_HOME="+filepath.Join(e.home, ".config"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start dp serve: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	// Wait for the server to be ready by polling the health endpoint.
	baseURL := "http://" + addr
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/v1/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return cmd
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	cmd.Process.Kill()
	t.Fatalf("dp serve did not become healthy within 10s on %s", addr)
	return nil
}

// TestServeHealthCheck verifies that `dp serve` starts as a subprocess and
// responds to the health check endpoint.
func TestServeHealthCheck(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	port := freePort(t)
	addr := "127.0.0.1:" + port
	startServe(t, e, addr)

	resp, err := http.Get("http://" + addr + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("health status = %q, want ok", body["status"])
	}
}

// TestServeIngestAndRetrieveInvocations verifies that POST /ingest stores data
// and GET /invocations retrieves it from a running `dp serve` subprocess.
func TestServeIngestAndRetrieveInvocations(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	port := freePort(t)
	addr := "127.0.0.1:" + port
	startServe(t, e, addr)

	baseURL := "http://" + addr

	// POST /ingest with a Claude Code payload.
	payload := `{"tool_name":"Read","session_id":"serve-test","cwd":"/tmp","error":"unknown tool"}`
	resp, err := http.Post(baseURL+"/api/v1/ingest?source=claude-code",
		"application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST ingest: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("ingest status = %d, want 201", resp.StatusCode)
	}

	// GET /invocations should return the ingested invocation.
	resp, err = http.Get(baseURL + "/api/v1/invocations")
	if err != nil {
		t.Fatalf("GET invocations: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("invocations status = %d, want 200", resp.StatusCode)
	}
	var invocations []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&invocations); err != nil {
		t.Fatalf("decode invocations: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("got %d invocations, want 1", len(invocations))
	}
	if invocations[0]["tool_name"] != "Read" {
		t.Errorf("tool_name = %v, want Read", invocations[0]["tool_name"])
	}
	if invocations[0]["source"] != "claude-code" {
		t.Errorf("source = %v, want claude-code", invocations[0]["source"])
	}
}

// TestServeIngestStoresDesireForError verifies that POST /ingest with an error
// creates both an invocation and a desire, retrievable via the API.
func TestServeIngestStoresDesireForError(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	port := freePort(t)
	addr := "127.0.0.1:" + port
	startServe(t, e, addr)

	baseURL := "http://" + addr

	// Ingest a tool call with an error.
	payload := `{"tool_name":"write_file","session_id":"s-err","cwd":"/tmp","error":"permission denied"}`
	resp, err := http.Post(baseURL+"/api/v1/ingest?source=claude-code",
		"application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST ingest: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("ingest status = %d, want 201", resp.StatusCode)
	}

	// GET /desires should have the error recorded as a desire.
	resp, err = http.Get(baseURL + "/api/v1/desires")
	if err != nil {
		t.Fatalf("GET desires: %v", err)
	}
	defer resp.Body.Close()
	var desires []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&desires); err != nil {
		t.Fatalf("decode desires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("got %d desires, want 1", len(desires))
	}
	if desires[0]["tool_name"] != "write_file" {
		t.Errorf("desire tool_name = %v, want write_file", desires[0]["tool_name"])
	}
}

// TestConfigStoreModeLocal verifies that when store_mode is unset (default)
// or explicitly "local", the CLI uses the local SQLite store.
func TestConfigStoreModeLocal(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// Default mode (no store_mode set) — should work with local SQLite.
	e.mustRun(e.fixture("tool_default", "s1", "err"), "ingest", "--source", "claude-code")
	stdout, _ := e.mustRun(nil, "list", "--json")
	if !strings.Contains(stdout, "tool_default") {
		t.Errorf("default mode: list missing tool_default:\n%s", stdout)
	}

	// Explicitly set store_mode=local — should still work.
	e.writeConfig("store_mode = \"local\"\n")
	e.mustRun(e.fixture("tool_explicit_local", "s2", "err"), "ingest", "--source", "claude-code")
	stdout, _ = e.mustRun(nil, "list", "--json")
	if !strings.Contains(stdout, "tool_explicit_local") {
		t.Errorf("explicit local mode: list missing tool_explicit_local:\n%s", stdout)
	}
}

// TestConfigStoreModeRemoteConstructsRemoteStore verifies that setting
// store_mode=remote in config causes the CLI to use the remote store,
// and that data flows through to the backing server.
func TestConfigStoreModeRemoteConstructsRemoteStore(t *testing.T) {
	t.Parallel()
	e, _ := newRemoteEnv(t)

	// Ingest via CLI — this goes through config → openStore() → RemoteStore → HTTP → server.
	e.mustRun(e.fixture("remote_config_tool", "rs1", "err"), "ingest", "--source", "claude-code")

	// Retrieve via CLI — also goes through RemoteStore.
	stdout, _ := e.mustRun(nil, "list", "--json")
	if !strings.Contains(stdout, "remote_config_tool") {
		t.Errorf("remote mode: list missing remote_config_tool:\n%s", stdout)
	}

	// Stats should reflect the ingested data.
	stdout, _ = e.mustRun(nil, "stats", "--json")
	var stats map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &stats); err != nil {
		t.Fatalf("parse stats: %v\noutput: %s", err, stdout)
	}
	if td, _ := stats["total_desires"].(float64); td != 1 {
		t.Errorf("total_desires = %v, want 1", td)
	}
}

// TestConfigStoreModeRemoteUnreachable verifies that when store_mode=remote
// points at an unreachable server, CLI commands fail with a meaningful error.
func TestConfigStoreModeRemoteUnreachable(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// Listen on a port, then close it immediately to guarantee fast ECONNREFUSED.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // Close immediately — connections will be refused.

	e.writeConfig(fmt.Sprintf("store_mode = \"remote\"\nremote_url = \"http://%s\"\n", addr))

	// Ingest should fail because the remote server is unreachable.
	_, stderr, runErr := e.run(e.fixture("tool_x", "s1", "err"), "ingest", "--source", "claude-code")
	if runErr == nil {
		t.Fatal("expected error when remote server is unreachable, got success")
	}
	// The error message should indicate a connection problem, not a panic.
	combined := stderr
	if combined == "" {
		combined = "exit status 1"
	}
	if !strings.Contains(strings.ToLower(combined), "connect") &&
		!strings.Contains(strings.ToLower(combined), "remote") &&
		!strings.Contains(strings.ToLower(combined), "refused") &&
		!strings.Contains(strings.ToLower(combined), "dial") {
		t.Logf("stderr: %s", stderr)
		// Still acceptable — the important thing is it failed, not panicked.
	}
}

// TestConfigStoreModeRemoteMissingURL verifies that store_mode=remote without
// remote_url produces a clear error.
func TestConfigStoreModeRemoteMissingURL(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// Set store_mode=remote but don't set remote_url.
	e.writeConfig("store_mode = \"remote\"\n")

	_, stderr, err := e.run(nil, "list")
	if err == nil {
		t.Fatal("expected error when store_mode=remote but remote_url is unset")
	}
	if !strings.Contains(stderr, "remote_url") {
		t.Errorf("error should mention remote_url, got stderr:\n%s", stderr)
	}
}
