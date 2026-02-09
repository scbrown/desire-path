package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	_ "github.com/scbrown/desire-path/internal/source" // register source plugins
	"github.com/scbrown/desire-path/internal/store"
)

func testServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	srv := New(s)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)
	return srv, ts
}

func TestHealth(t *testing.T) {
	_, ts := testServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

func TestRecordAndListDesires(t *testing.T) {
	_, ts := testServer(t)

	d := model.Desire{
		ID:        "test-1",
		ToolName:  "read_file",
		Error:     "unknown tool",
		Source:    "claude-code",
		Timestamp: time.Now().UTC().Truncate(time.Second),
	}
	body, _ := json.Marshal(d)
	resp, err := http.Post(ts.URL+"/api/v1/desires", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST desire: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	// List desires.
	resp, err = http.Get(ts.URL + "/api/v1/desires")
	if err != nil {
		t.Fatalf("GET desires: %v", err)
	}
	defer resp.Body.Close()
	var desires []model.Desire
	if err := json.NewDecoder(resp.Body).Decode(&desires); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("got %d desires, want 1", len(desires))
	}
	if desires[0].ToolName != "read_file" {
		t.Errorf("tool_name = %q, want read_file", desires[0].ToolName)
	}
}

func TestRecordAndListInvocations(t *testing.T) {
	_, ts := testServer(t)

	inv := model.Invocation{
		ID:        "inv-1",
		Source:    "claude-code",
		ToolName:  "Read",
		IsError:   false,
		Timestamp: time.Now().UTC().Truncate(time.Second),
	}
	body, _ := json.Marshal(inv)
	resp, err := http.Post(ts.URL+"/api/v1/invocations", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST invocation: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	// List invocations.
	resp, err = http.Get(ts.URL + "/api/v1/invocations")
	if err != nil {
		t.Fatalf("GET invocations: %v", err)
	}
	defer resp.Body.Close()
	var invocations []model.Invocation
	if err := json.NewDecoder(resp.Body).Decode(&invocations); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("got %d invocations, want 1", len(invocations))
	}
	if invocations[0].ToolName != "Read" {
		t.Errorf("tool_name = %q, want Read", invocations[0].ToolName)
	}
}

func TestAliases(t *testing.T) {
	_, ts := testServer(t)

	// Set alias.
	body, _ := json.Marshal(map[string]string{"from": "read_file", "to": "Read"})
	resp, err := http.Post(ts.URL+"/api/v1/aliases", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST alias: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("set alias status = %d, want 201", resp.StatusCode)
	}

	// List aliases.
	resp, err = http.Get(ts.URL + "/api/v1/aliases")
	if err != nil {
		t.Fatalf("GET aliases: %v", err)
	}
	defer resp.Body.Close()
	var aliases []model.Alias
	if err := json.NewDecoder(resp.Body).Decode(&aliases); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("got %d aliases, want 1", len(aliases))
	}
	if aliases[0].From != "read_file" || aliases[0].To != "Read" {
		t.Errorf("alias = %+v, want read_file → Read", aliases[0])
	}

	// Delete alias.
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/aliases/read_file", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE alias: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete alias status = %d, want 200", resp.StatusCode)
	}

	// Delete nonexistent.
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/aliases/nonexistent", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE nonexistent: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete nonexistent status = %d, want 404", resp.StatusCode)
	}
}

func TestGetAlias(t *testing.T) {
	_, ts := testServer(t)

	// Get nonexistent alias.
	resp, err := http.Get(ts.URL + "/api/v1/aliases/nonexistent")
	if err != nil {
		t.Fatalf("GET alias: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}

	// Set alias.
	body, _ := json.Marshal(map[string]string{"from": "read_file", "to": "Read"})
	resp, err = http.Post(ts.URL+"/api/v1/aliases", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST alias: %v", err)
	}
	resp.Body.Close()

	// Get existing alias.
	resp, err = http.Get(ts.URL + "/api/v1/aliases/read_file")
	if err != nil {
		t.Fatalf("GET alias: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var alias model.Alias
	if err := json.NewDecoder(resp.Body).Decode(&alias); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if alias.From != "read_file" || alias.To != "Read" {
		t.Errorf("alias = %+v, want read_file → Read", alias)
	}
}

func TestStats(t *testing.T) {
	_, ts := testServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/stats")
	if err != nil {
		t.Fatalf("GET stats: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var stats store.Stats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.TotalDesires != 0 {
		t.Errorf("total = %d, want 0", stats.TotalDesires)
	}
}

func TestInvocationStats(t *testing.T) {
	_, ts := testServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/invocations/stats")
	if err != nil {
		t.Fatalf("GET invocation stats: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var stats store.InvocationStatsResult
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.Total != 0 {
		t.Errorf("total = %d, want 0", stats.Total)
	}
}

func TestPaths(t *testing.T) {
	_, ts := testServer(t)

	// Record a desire so paths has data.
	d := model.Desire{
		ID:        "p-1",
		ToolName:  "read_file",
		Error:     "unknown tool",
		Source:    "test",
		Timestamp: time.Now().UTC().Truncate(time.Second),
	}
	body, _ := json.Marshal(d)
	resp, err := http.Post(ts.URL+"/api/v1/desires", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST desire: %v", err)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/v1/paths")
	if err != nil {
		t.Fatalf("GET paths: %v", err)
	}
	defer resp.Body.Close()
	var paths []model.Path
	if err := json.NewDecoder(resp.Body).Decode(&paths); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("got %d paths, want 1", len(paths))
	}
	if paths[0].Pattern != "read_file" {
		t.Errorf("pattern = %q, want read_file", paths[0].Pattern)
	}
}

func TestInspect(t *testing.T) {
	_, ts := testServer(t)

	// Inspect with no data.
	resp, err := http.Get(ts.URL + "/api/v1/inspect?pattern=read_file")
	if err != nil {
		t.Fatalf("GET inspect: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var result store.InspectResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0", result.Total)
	}
}

func TestInspectMissingPattern(t *testing.T) {
	_, ts := testServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/inspect")
	if err != nil {
		t.Fatalf("GET inspect: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestListDesiresWithFilters(t *testing.T) {
	_, ts := testServer(t)

	// Record two desires.
	for i, d := range []model.Desire{
		{ID: "f-1", ToolName: "Read", Error: "err1", Source: "s1", Timestamp: time.Now().UTC()},
		{ID: "f-2", ToolName: "Write", Error: "err2", Source: "s2", Timestamp: time.Now().UTC()},
	} {
		body, _ := json.Marshal(d)
		resp, _ := http.Post(ts.URL+"/api/v1/desires", "application/json", bytes.NewReader(body))
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("desire %d: status = %d", i, resp.StatusCode)
		}
	}

	// Filter by source.
	resp, err := http.Get(ts.URL + "/api/v1/desires?source=s1")
	if err != nil {
		t.Fatalf("GET desires: %v", err)
	}
	defer resp.Body.Close()
	var desires []model.Desire
	if err := json.NewDecoder(resp.Body).Decode(&desires); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("got %d desires, want 1", len(desires))
	}
	if desires[0].Source != "s1" {
		t.Errorf("source = %q, want s1", desires[0].Source)
	}
}

func TestIngest(t *testing.T) {
	_, ts := testServer(t)

	// Send a Claude Code hook payload through the ingest endpoint.
	payload := `{"tool_name":"Read","session_id":"s-123","cwd":"/tmp","error":"unknown tool"}`
	resp, err := http.Post(ts.URL+"/api/v1/ingest?source=claude-code", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST ingest: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var inv model.Invocation
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inv.ToolName != "Read" {
		t.Errorf("tool_name = %q, want Read", inv.ToolName)
	}
	if inv.Source != "claude-code" {
		t.Errorf("source = %q, want claude-code", inv.Source)
	}
	if !inv.IsError {
		t.Error("expected IsError=true for payload with error field")
	}

	// The ingest pipeline should also have created a desire for the error.
	dresp, err := http.Get(ts.URL + "/api/v1/desires")
	if err != nil {
		t.Fatalf("GET desires: %v", err)
	}
	defer dresp.Body.Close()
	var desires []model.Desire
	if err := json.NewDecoder(dresp.Body).Decode(&desires); err != nil {
		t.Fatalf("decode desires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("got %d desires, want 1 (ingest should create desire for errors)", len(desires))
	}
	if desires[0].ToolName != "Read" {
		t.Errorf("desire tool_name = %q, want Read", desires[0].ToolName)
	}
}

func TestIngestMissingSource(t *testing.T) {
	_, ts := testServer(t)

	resp, err := http.Post(ts.URL+"/api/v1/ingest", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("POST ingest: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestIngestUnknownSource(t *testing.T) {
	_, ts := testServer(t)

	resp, err := http.Post(ts.URL+"/api/v1/ingest?source=nonexistent", "application/json", bytes.NewBufferString(`{"tool_name":"X"}`))
	if err != nil {
		t.Fatalf("POST ingest: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
}

func TestIngestSuccessNoDesire(t *testing.T) {
	_, ts := testServer(t)

	// A successful tool call (no error) should create invocation but NOT desire.
	payload := `{"tool_name":"Read","session_id":"s-456","cwd":"/tmp"}`
	resp, err := http.Post(ts.URL+"/api/v1/ingest?source=claude-code", "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST ingest: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var inv model.Invocation
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inv.IsError {
		t.Error("expected IsError=false for payload without error")
	}

	// No desire should be created for a success.
	dresp, err := http.Get(ts.URL + "/api/v1/desires")
	if err != nil {
		t.Fatalf("GET desires: %v", err)
	}
	defer dresp.Body.Close()
	var desires []model.Desire
	if err := json.NewDecoder(dresp.Body).Decode(&desires); err != nil {
		t.Fatalf("decode desires: %v", err)
	}
	if len(desires) != 0 {
		t.Fatalf("got %d desires, want 0 (success should not create desire)", len(desires))
	}
}

func TestShutdown(t *testing.T) {
	srv, _ := testServer(t)
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}
