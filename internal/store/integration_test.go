package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
)

// TestRecordListPathsRoundTrip is the integration test required by dp-8u6:
// record desires, list them back, then verify paths aggregation matches.
func TestRecordListPathsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)

	// Phase 1: Record a set of desires with known tool names.
	desires := []model.Desire{
		{ID: "rt-1", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"/a"}`), Error: "unknown tool", Source: "claude-code", Timestamp: base},
		{ID: "rt-2", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"/b"}`), Error: "unknown tool", Source: "claude-code", Timestamp: base.Add(time.Hour)},
		{ID: "rt-3", ToolName: "write_file", ToolInput: json.RawMessage(`{"path":"/c"}`), Error: "not found", Source: "cursor", Timestamp: base.Add(2 * time.Hour)},
		{ID: "rt-4", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"/d"}`), Error: "permission denied", Source: "cursor", Timestamp: base.Add(3 * time.Hour)},
		{ID: "rt-5", ToolName: "run_tests", Error: "tool not available", Source: "claude-code", Timestamp: base.Add(4 * time.Hour)},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire(%s): %v", d.ID, err)
		}
	}

	// Phase 2: List all desires back and verify complete round-trip.
	listed, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(listed) != len(desires) {
		t.Fatalf("ListDesires: got %d, want %d", len(listed), len(desires))
	}

	// Verify newest-first ordering.
	for i := 1; i < len(listed); i++ {
		if listed[i].Timestamp.After(listed[i-1].Timestamp) {
			t.Errorf("ListDesires not ordered DESC: [%d]=%v after [%d]=%v",
				i, listed[i].Timestamp, i-1, listed[i-1].Timestamp)
		}
	}

	// Verify all recorded desires are present by building an ID set.
	idSet := make(map[string]bool)
	for _, d := range listed {
		idSet[d.ID] = true
	}
	for _, d := range desires {
		if !idSet[d.ID] {
			t.Errorf("desire %s not found in listed results", d.ID)
		}
	}

	// Phase 3: Get paths and verify aggregation.
	paths, err := s.GetPaths(ctx, PathOpts{})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("GetPaths: got %d paths, want 3 (read_file, write_file, run_tests)", len(paths))
	}

	// Build path map for easier assertion.
	pathMap := make(map[string]model.Path)
	for _, p := range paths {
		pathMap[p.Pattern] = p
	}

	// read_file should have count=3 and be first (most frequent).
	if paths[0].Pattern != "read_file" {
		t.Errorf("most frequent path = %q, want read_file", paths[0].Pattern)
	}
	if paths[0].Count != 3 {
		t.Errorf("read_file count = %d, want 3", paths[0].Count)
	}

	// write_file should have count=1.
	wf, ok := pathMap["write_file"]
	if !ok {
		t.Fatal("write_file path not found")
	}
	if wf.Count != 1 {
		t.Errorf("write_file count = %d, want 1", wf.Count)
	}

	// run_tests should have count=1.
	rt, ok := pathMap["run_tests"]
	if !ok {
		t.Fatal("run_tests path not found")
	}
	if rt.Count != 1 {
		t.Errorf("run_tests count = %d, want 1", rt.Count)
	}

	// Verify path timestamps match recorded data.
	rf := pathMap["read_file"]
	if !rf.FirstSeen.Equal(base) {
		t.Errorf("read_file FirstSeen = %v, want %v", rf.FirstSeen, base)
	}
	if !rf.LastSeen.Equal(base.Add(3 * time.Hour)) {
		t.Errorf("read_file LastSeen = %v, want %v", rf.LastSeen, base.Add(3*time.Hour))
	}

	// Phase 4: Filtered list should match filtered paths.
	claudeOnly, err := s.ListDesires(ctx, ListOpts{Source: "claude-code"})
	if err != nil {
		t.Fatalf("ListDesires(claude-code): %v", err)
	}
	if len(claudeOnly) != 3 {
		t.Errorf("claude-code desires: got %d, want 3", len(claudeOnly))
	}

	readOnly, err := s.ListDesires(ctx, ListOpts{ToolName: "read_file"})
	if err != nil {
		t.Fatalf("ListDesires(read_file): %v", err)
	}
	if len(readOnly) != 3 {
		t.Errorf("read_file desires: got %d, want 3", len(readOnly))
	}

	// Phase 5: Paths with Top filter.
	topPaths, err := s.GetPaths(ctx, PathOpts{Top: 1})
	if err != nil {
		t.Fatalf("GetPaths(Top=1): %v", err)
	}
	if len(topPaths) != 1 {
		t.Fatalf("GetPaths(Top=1): got %d, want 1", len(topPaths))
	}
	if topPaths[0].Pattern != "read_file" {
		t.Errorf("top path = %q, want read_file", topPaths[0].Pattern)
	}
}

// TestRecordListPathsWithAliasRoundTrip verifies that aliases appear in paths.
func TestRecordListPathsWithAliasRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	if err := s.RecordDesire(ctx, model.Desire{
		ID: "a1", ToolName: "read_file", Error: "unknown tool", Timestamp: ts,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(ctx, "read_file", "Read"); err != nil {
		t.Fatal(err)
	}

	paths, err := s.GetPaths(ctx, PathOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("got %d paths, want 1", len(paths))
	}
	if paths[0].AliasTo != "Read" {
		t.Errorf("AliasTo = %q, want Read", paths[0].AliasTo)
	}

	// Inspect should also show the alias.
	result, err := s.InspectPath(ctx, InspectOpts{Pattern: "read_file"})
	if err != nil {
		t.Fatal(err)
	}
	if result.AliasTo != "Read" {
		t.Errorf("InspectPath AliasTo = %q, want Read", result.AliasTo)
	}
}

// TestVeryLargeToolInput ensures the store handles very large JSON payloads.
func TestVeryLargeToolInput(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a ~100KB tool_input JSON payload.
	largeValue := strings.Repeat("x", 100_000)
	input := fmt.Sprintf(`{"command":"%s"}`, largeValue)

	d := model.Desire{
		ID:        "large-1",
		ToolName:  "Bash",
		ToolInput: json.RawMessage(input),
		Error:     "failed",
		Timestamp: time.Now().UTC(),
	}

	if err := s.RecordDesire(ctx, d); err != nil {
		t.Fatalf("RecordDesire large input: %v", err)
	}

	listed, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1, got %d", len(listed))
	}
	if len(listed[0].ToolInput) != len(d.ToolInput) {
		t.Errorf("ToolInput length: got %d, want %d", len(listed[0].ToolInput), len(d.ToolInput))
	}
}

// TestVeryLargeMetadata ensures the store handles large metadata.
func TestVeryLargeMetadata(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Build a metadata object with many keys.
	meta := make(map[string]string)
	for i := 0; i < 500; i++ {
		meta[fmt.Sprintf("key_%d", i)] = strings.Repeat("v", 200)
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}

	d := model.Desire{
		ID:        "meta-large-1",
		ToolName:  "test",
		Error:     "err",
		Timestamp: time.Now().UTC(),
		Metadata:  metaBytes,
	}

	if err := s.RecordDesire(ctx, d); err != nil {
		t.Fatalf("RecordDesire: %v", err)
	}

	listed, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1, got %d", len(listed))
	}

	var gotMeta map[string]string
	if err := json.Unmarshal(listed[0].Metadata, &gotMeta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if len(gotMeta) != 500 {
		t.Errorf("metadata keys: got %d, want 500", len(gotMeta))
	}
}

// TestVeryLargeErrorString ensures very long error messages survive round-trip.
func TestVeryLargeErrorString(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	longError := strings.Repeat("error text ", 10_000)
	d := model.Desire{
		ID:        "longerr-1",
		ToolName:  "test",
		Error:     longError,
		Timestamp: time.Now().UTC(),
	}

	if err := s.RecordDesire(ctx, d); err != nil {
		t.Fatalf("RecordDesire: %v", err)
	}

	listed, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if listed[0].Error != longError {
		t.Errorf("Error length: got %d, want %d", len(listed[0].Error), len(longError))
	}
}

// TestManyDesires verifies bulk operations work correctly.
func TestManyDesires(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Insert 200 desires across 10 tool names.
	for i := 0; i < 200; i++ {
		tool := fmt.Sprintf("tool_%d", i%10)
		if err := s.RecordDesire(ctx, model.Desire{
			ID:        fmt.Sprintf("bulk-%d", i),
			ToolName:  tool,
			Error:     "err",
			Source:    "bulk-test",
			Timestamp: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("RecordDesire(%d): %v", i, err)
		}
	}

	// List all.
	all, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(all) != 200 {
		t.Errorf("ListDesires: got %d, want 200", len(all))
	}

	// Paths should show 10 unique tools.
	paths, err := s.GetPaths(ctx, PathOpts{})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != 10 {
		t.Errorf("GetPaths: got %d, want 10", len(paths))
	}

	// Each tool should have count=20.
	for _, p := range paths {
		if p.Count != 20 {
			t.Errorf("path %q count = %d, want 20", p.Pattern, p.Count)
		}
	}

	// Stats should reflect totals.
	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalDesires != 200 {
		t.Errorf("TotalDesires = %d, want 200", st.TotalDesires)
	}
	if st.UniquePaths != 10 {
		t.Errorf("UniquePaths = %d, want 10", st.UniquePaths)
	}
}

// TestEmptyDBAllOperations verifies all operations on an empty database.
func TestEmptyDBAllOperations(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// ListDesires on empty DB.
	desires, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(desires) != 0 {
		t.Errorf("ListDesires: got %d, want 0", len(desires))
	}

	// GetPaths on empty DB.
	paths, err := s.GetPaths(ctx, PathOpts{})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("GetPaths: got %d, want 0", len(paths))
	}

	// Stats on empty DB.
	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalDesires != 0 {
		t.Errorf("TotalDesires = %d, want 0", st.TotalDesires)
	}
	if st.UniquePaths != 0 {
		t.Errorf("UniquePaths = %d, want 0", st.UniquePaths)
	}

	// GetAliases on empty DB.
	aliases, err := s.GetAliases(ctx)
	if err != nil {
		t.Fatalf("GetAliases: %v", err)
	}
	if len(aliases) != 0 {
		t.Errorf("GetAliases: got %d, want 0", len(aliases))
	}

	// DeleteAlias on empty DB.
	deleted, err := s.DeleteAlias(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("DeleteAlias: %v", err)
	}
	if deleted {
		t.Error("DeleteAlias: expected false for empty DB")
	}

	// InspectPath on empty DB.
	result, err := s.InspectPath(ctx, InspectOpts{Pattern: "anything"})
	if err != nil {
		t.Fatalf("InspectPath: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("InspectPath.Total = %d, want 0", result.Total)
	}
}

// TestSpecialCharactersInToolName ensures tool names with special chars work.
func TestSpecialCharactersInToolName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	names := []string{
		"mcp__memory__search",     // Double underscores
		"tool-with-hyphens",       // Hyphens
		"tool.with.dots",          // Dots
		"tool/with/slashes",       // Slashes
		"tool with spaces",        // Spaces
		"tool'with\"quotes",       // Quotes
		"日本語ツール",                  // Unicode
		"tool\twith\ttabs",        // Tabs
		"a",                       // Single character
		strings.Repeat("x", 1000), // Very long name
	}

	for i, name := range names {
		if err := s.RecordDesire(ctx, model.Desire{
			ID:        fmt.Sprintf("special-%d", i),
			ToolName:  name,
			Error:     "err",
			Timestamp: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("RecordDesire(%q): %v", name, err)
		}
	}

	listed, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(listed) != len(names) {
		t.Fatalf("got %d desires, want %d", len(listed), len(names))
	}

	// Verify each name round-trips.
	nameSet := make(map[string]bool)
	for _, d := range listed {
		nameSet[d.ToolName] = true
	}
	for _, name := range names {
		if !nameSet[name] {
			t.Errorf("tool name %q not found after round-trip", name)
		}
	}

	// Paths should have one entry per unique name.
	paths, err := s.GetPaths(ctx, PathOpts{})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != len(names) {
		t.Errorf("GetPaths: got %d, want %d", len(paths), len(names))
	}
}
