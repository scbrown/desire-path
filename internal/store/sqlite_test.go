package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewCreatesDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")
	s, err := New(filepath.Join(nested, "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("expected directory %s to exist: %v", nested, err)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	s1.Close()

	// Opening again should not fail (migration is idempotent).
	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	s2.Close()
}

func TestRecordAndListDesires(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)

	d := model.Desire{
		ID:        "test-1",
		ToolName:  "read_file",
		ToolInput: json.RawMessage(`{"path":"/etc/hosts"}`),
		Error:     "unknown tool",
		Source:    "claude-code",
		SessionID: "sess-1",
		CWD:       "/home/user",
		Timestamp: ts,
		Metadata:  json.RawMessage(`{"model":"opus"}`),
	}

	if err := s.RecordDesire(ctx, d); err != nil {
		t.Fatalf("RecordDesire: %v", err)
	}

	desires, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("expected 1 desire, got %d", len(desires))
	}

	got := desires[0]
	if got.ID != d.ID {
		t.Errorf("ID: got %q, want %q", got.ID, d.ID)
	}
	if got.ToolName != d.ToolName {
		t.Errorf("ToolName: got %q, want %q", got.ToolName, d.ToolName)
	}
	if got.Error != d.Error {
		t.Errorf("Error: got %q, want %q", got.Error, d.Error)
	}
	if got.Source != d.Source {
		t.Errorf("Source: got %q, want %q", got.Source, d.Source)
	}
	if got.SessionID != d.SessionID {
		t.Errorf("SessionID: got %q, want %q", got.SessionID, d.SessionID)
	}
	if got.CWD != d.CWD {
		t.Errorf("CWD: got %q, want %q", got.CWD, d.CWD)
	}
	if !got.Timestamp.Equal(d.Timestamp) {
		t.Errorf("Timestamp: got %v, want %v", got.Timestamp, d.Timestamp)
	}
	if string(got.ToolInput) != string(d.ToolInput) {
		t.Errorf("ToolInput: got %s, want %s", got.ToolInput, d.ToolInput)
	}
	if string(got.Metadata) != string(d.Metadata) {
		t.Errorf("Metadata: got %s, want %s", got.Metadata, d.Metadata)
	}
}

func TestListDesiresMinimal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	d := model.Desire{
		ID:        "min-1",
		ToolName:  "foo",
		Error:     "not found",
		Timestamp: time.Now().UTC(),
	}
	if err := s.RecordDesire(ctx, d); err != nil {
		t.Fatalf("RecordDesire: %v", err)
	}

	desires, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("expected 1, got %d", len(desires))
	}
	if desires[0].Source != "" {
		t.Errorf("expected empty Source, got %q", desires[0].Source)
	}
	if desires[0].ToolInput != nil {
		t.Errorf("expected nil ToolInput, got %s", desires[0].ToolInput)
	}
}

func TestListDesiresFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	desires := []model.Desire{
		{ID: "d1", ToolName: "read_file", Error: "e1", Source: "claude-code", Timestamp: base},
		{ID: "d2", ToolName: "write_file", Error: "e2", Source: "cursor", Timestamp: base.Add(time.Hour)},
		{ID: "d3", ToolName: "read_file", Error: "e3", Source: "claude-code", Timestamp: base.Add(2 * time.Hour)},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire %s: %v", d.ID, err)
		}
	}

	// Filter by tool name.
	got, err := s.ListDesires(ctx, ListOpts{ToolName: "read_file"})
	if err != nil {
		t.Fatalf("ListDesires by tool: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("tool filter: expected 2, got %d", len(got))
	}

	// Filter by source.
	got, err = s.ListDesires(ctx, ListOpts{Source: "cursor"})
	if err != nil {
		t.Fatalf("ListDesires by source: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("source filter: expected 1, got %d", len(got))
	}

	// Filter by since.
	got, err = s.ListDesires(ctx, ListOpts{Since: base.Add(30 * time.Minute)})
	if err != nil {
		t.Fatalf("ListDesires by since: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("since filter: expected 2, got %d", len(got))
	}

	// Limit.
	got, err = s.ListDesires(ctx, ListOpts{Limit: 1})
	if err != nil {
		t.Fatalf("ListDesires with limit: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("limit: expected 1, got %d", len(got))
	}
}

func TestGetPaths(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	desires := []model.Desire{
		{ID: "d1", ToolName: "read_file", Error: "e1", Timestamp: base},
		{ID: "d2", ToolName: "read_file", Error: "e2", Timestamp: base.Add(time.Hour)},
		{ID: "d3", ToolName: "write_file", Error: "e3", Timestamp: base.Add(2 * time.Hour)},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	paths, err := s.GetPaths(ctx, PathOpts{})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	// read_file should be first (count=2).
	if paths[0].Pattern != "read_file" {
		t.Errorf("expected read_file first, got %q", paths[0].Pattern)
	}
	if paths[0].Count != 2 {
		t.Errorf("expected count 2, got %d", paths[0].Count)
	}
	if paths[1].Pattern != "write_file" {
		t.Errorf("expected write_file second, got %q", paths[1].Pattern)
	}
}

func TestGetPathsWithTop(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC()

	for i, name := range []string{"a", "b", "c"} {
		if err := s.RecordDesire(ctx, model.Desire{
			ID: name, ToolName: name, Error: "e", Timestamp: base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	paths, err := s.GetPaths(ctx, PathOpts{Top: 2})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("expected 2 paths with Top=2, got %d", len(paths))
	}
}

func TestGetPathsWithAlias(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.RecordDesire(ctx, model.Desire{
		ID: "d1", ToolName: "read_file", Error: "e1", Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("RecordDesire: %v", err)
	}
	if err := s.SetAlias(ctx, model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}

	paths, err := s.GetPaths(ctx, PathOpts{})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if paths[0].AliasTo != "Read" {
		t.Errorf("AliasTo: got %q, want %q", paths[0].AliasTo, "Read")
	}
}

func TestSetAliasUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetAlias(ctx, model.Alias{From: "foo", To: "bar"}); err != nil {
		t.Fatalf("first SetAlias: %v", err)
	}
	if err := s.SetAlias(ctx, model.Alias{From: "foo", To: "baz"}); err != nil {
		t.Fatalf("second SetAlias: %v", err)
	}

	aliases, err := s.GetAliases(ctx)
	if err != nil {
		t.Fatalf("GetAliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias after upsert, got %d", len(aliases))
	}
	if aliases[0].To != "baz" {
		t.Errorf("To: got %q, want %q", aliases[0].To, "baz")
	}
}

func TestGetAliases(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetAlias(ctx, model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}
	if err := s.SetAlias(ctx, model.Alias{From: "write_file", To: "Write"}); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}

	aliases, err := s.GetAliases(ctx)
	if err != nil {
		t.Fatalf("GetAliases: %v", err)
	}
	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(aliases))
	}
	// Ordered by from_name.
	if aliases[0].From != "read_file" {
		t.Errorf("first alias From: got %q, want %q", aliases[0].From, "read_file")
	}
	if aliases[1].From != "write_file" {
		t.Errorf("second alias From: got %q, want %q", aliases[1].From, "write_file")
	}
}

func TestStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	desires := []model.Desire{
		{ID: "d1", ToolName: "read_file", Error: "e1", Source: "claude-code", Timestamp: ts},
		{ID: "d2", ToolName: "read_file", Error: "e2", Source: "claude-code", Timestamp: ts},
		{ID: "d3", ToolName: "write_file", Error: "e3", Source: "cursor", Timestamp: ts},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalDesires != 3 {
		t.Errorf("TotalDesires: got %d, want 3", st.TotalDesires)
	}
	if st.UniquePaths != 2 {
		t.Errorf("UniquePaths: got %d, want 2", st.UniquePaths)
	}
	if st.TopSources["claude-code"] != 2 {
		t.Errorf("TopSources[claude-code]: got %d, want 2", st.TopSources["claude-code"])
	}
	if st.TopSources["cursor"] != 1 {
		t.Errorf("TopSources[cursor]: got %d, want 1", st.TopSources["cursor"])
	}

	// TopDesires: read_file=2 should be first.
	if len(st.TopDesires) != 2 {
		t.Fatalf("TopDesires: expected 2, got %d", len(st.TopDesires))
	}
	if st.TopDesires[0].Name != "read_file" || st.TopDesires[0].Count != 2 {
		t.Errorf("TopDesires[0]: got %+v, want {read_file, 2}", st.TopDesires[0])
	}
	if st.TopDesires[1].Name != "write_file" || st.TopDesires[1].Count != 1 {
		t.Errorf("TopDesires[1]: got %+v, want {write_file, 1}", st.TopDesires[1])
	}

	// All desires were just recorded, so time-window counts should match.
	if st.Last24h != 3 {
		t.Errorf("Last24h: got %d, want 3", st.Last24h)
	}
	if st.Last7d != 3 {
		t.Errorf("Last7d: got %d, want 3", st.Last7d)
	}
	if st.Last30d != 3 {
		t.Errorf("Last30d: got %d, want 3", st.Last30d)
	}
}

func TestStatsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalDesires != 0 {
		t.Errorf("TotalDesires: got %d, want 0", st.TotalDesires)
	}
	if st.UniquePaths != 0 {
		t.Errorf("UniquePaths: got %d, want 0", st.UniquePaths)
	}
	if len(st.TopSources) != 0 {
		t.Errorf("TopSources: expected empty, got %v", st.TopSources)
	}
}

func TestRecordDesireDuplicateID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	d := model.Desire{
		ID:        "dup-1",
		ToolName:  "foo",
		Error:     "e1",
		Timestamp: ts,
	}
	if err := s.RecordDesire(ctx, d); err != nil {
		t.Fatalf("first RecordDesire: %v", err)
	}

	// Inserting same ID should fail (PRIMARY KEY constraint).
	err := s.RecordDesire(ctx, d)
	if err == nil {
		t.Fatal("expected error on duplicate ID, got nil")
	}
}

func TestListDesiresEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	desires, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(desires) != 0 {
		t.Errorf("expected 0 desires, got %d", len(desires))
	}
}

func TestListDesiresCombinedFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	desires := []model.Desire{
		{ID: "cf1", ToolName: "read_file", Error: "e1", Source: "claude-code", Timestamp: base},
		{ID: "cf2", ToolName: "read_file", Error: "e2", Source: "cursor", Timestamp: base.Add(time.Hour)},
		{ID: "cf3", ToolName: "write_file", Error: "e3", Source: "claude-code", Timestamp: base.Add(2 * time.Hour)},
		{ID: "cf4", ToolName: "read_file", Error: "e4", Source: "claude-code", Timestamp: base.Add(3 * time.Hour)},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire %s: %v", d.ID, err)
		}
	}

	// Combine: source + tool_name + since + limit.
	got, err := s.ListDesires(ctx, ListOpts{
		Source:   "claude-code",
		ToolName: "read_file",
		Since:    base.Add(30 * time.Minute),
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	// Should be cf4 (newest, matches all filters).
	if got[0].ID != "cf4" {
		t.Errorf("ID = %q, want %q", got[0].ID, "cf4")
	}
}

func TestListDesiresOrderDesc(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		if err := s.RecordDesire(ctx, model.Desire{
			ID:        fmt.Sprintf("ord-%d", i),
			ToolName:  "t",
			Error:     "e",
			Timestamp: base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	got, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	// Should be newest first.
	if got[0].ID != "ord-2" {
		t.Errorf("first = %q, want ord-2", got[0].ID)
	}
	if got[2].ID != "ord-0" {
		t.Errorf("last = %q, want ord-0", got[2].ID)
	}
}

func TestGetPathsWithSince(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	desires := []model.Desire{
		{ID: "ps1", ToolName: "old_tool", Error: "e1", Timestamp: base},
		{ID: "ps2", ToolName: "new_tool", Error: "e2", Timestamp: base.Add(2 * time.Hour)},
		{ID: "ps3", ToolName: "new_tool", Error: "e3", Timestamp: base.Add(3 * time.Hour)},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	paths, err := s.GetPaths(ctx, PathOpts{Since: base.Add(time.Hour)})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path (only new_tool after since), got %d", len(paths))
	}
	if paths[0].Pattern != "new_tool" {
		t.Errorf("Pattern = %q, want %q", paths[0].Pattern, "new_tool")
	}
	if paths[0].Count != 2 {
		t.Errorf("Count = %d, want 2", paths[0].Count)
	}
}

func TestGetPathsTimestamps(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	desires := []model.Desire{
		{ID: "pt1", ToolName: "read_file", Error: "e1", Timestamp: first},
		{ID: "pt2", ToolName: "read_file", Error: "e2", Timestamp: last},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	paths, err := s.GetPaths(ctx, PathOpts{})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if !paths[0].FirstSeen.Equal(first) {
		t.Errorf("FirstSeen = %v, want %v", paths[0].FirstSeen, first)
	}
	if !paths[0].LastSeen.Equal(last) {
		t.Errorf("LastSeen = %v, want %v", paths[0].LastSeen, last)
	}
	if paths[0].ID != paths[0].Pattern {
		t.Errorf("ID (%q) should equal Pattern (%q)", paths[0].ID, paths[0].Pattern)
	}
}

func TestGetPathsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	paths, err := s.GetPaths(ctx, PathOpts{})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
	}
}

func TestGetAliasesEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	aliases, err := s.GetAliases(ctx)
	if err != nil {
		t.Fatalf("GetAliases: %v", err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases, got %d", len(aliases))
	}
}

func TestSetAliasTimestamp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	before := time.Now().UTC()
	if err := s.SetAlias(ctx, model.Alias{From: "hallucinated", To: "real_tool"}); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}
	after := time.Now().UTC()

	aliases, err := s.GetAliases(ctx)
	if err != nil {
		t.Fatalf("GetAliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1, got %d", len(aliases))
	}
	if aliases[0].CreatedAt.Before(before) || aliases[0].CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not between %v and %v", aliases[0].CreatedAt, before, after)
	}
}

func TestStatsWithNoSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	// Desires with no source should not appear in TopSources.
	if err := s.RecordDesire(ctx, model.Desire{
		ID: "ns1", ToolName: "foo", Error: "e", Timestamp: ts,
	}); err != nil {
		t.Fatalf("RecordDesire: %v", err)
	}

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalDesires != 1 {
		t.Errorf("TotalDesires = %d, want 1", st.TotalDesires)
	}
	if len(st.TopSources) != 0 {
		t.Errorf("TopSources should be empty for desires with no source, got %v", st.TopSources)
	}
}

func TestRecordDesireNullableFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	// Record with all nullable fields empty.
	d := model.Desire{
		ID:        "nullable-1",
		ToolName:  "test_tool",
		Error:     "test error",
		Timestamp: ts,
	}
	if err := s.RecordDesire(ctx, d); err != nil {
		t.Fatalf("RecordDesire: %v", err)
	}

	got, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}

	if got[0].Source != "" {
		t.Errorf("Source = %q, want empty", got[0].Source)
	}
	if got[0].SessionID != "" {
		t.Errorf("SessionID = %q, want empty", got[0].SessionID)
	}
	if got[0].CWD != "" {
		t.Errorf("CWD = %q, want empty", got[0].CWD)
	}
	if got[0].ToolInput != nil {
		t.Errorf("ToolInput = %s, want nil", got[0].ToolInput)
	}
	if got[0].Metadata != nil {
		t.Errorf("Metadata = %s, want nil", got[0].Metadata)
	}
}

func TestStatsDateRange(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	earliest := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	latest := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	desires := []model.Desire{
		{ID: "dr1", ToolName: "a", Error: "e", Timestamp: earliest},
		{ID: "dr2", ToolName: "b", Error: "e", Timestamp: latest},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if !st.Earliest.Equal(earliest) {
		t.Errorf("Earliest: got %v, want %v", st.Earliest, earliest)
	}
	if !st.Latest.Equal(latest) {
		t.Errorf("Latest: got %v, want %v", st.Latest, latest)
	}
}

func TestStatsTimeWindows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	desires := []model.Desire{
		{ID: "tw1", ToolName: "a", Error: "e", Timestamp: now.Add(-2 * time.Hour)},  // Within 24h.
		{ID: "tw2", ToolName: "b", Error: "e", Timestamp: now.Add(-3 * 24 * time.Hour)},  // Within 7d.
		{ID: "tw3", ToolName: "c", Error: "e", Timestamp: now.Add(-15 * 24 * time.Hour)}, // Within 30d.
		{ID: "tw4", ToolName: "d", Error: "e", Timestamp: now.Add(-60 * 24 * time.Hour)}, // Outside 30d.
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Last24h != 1 {
		t.Errorf("Last24h: got %d, want 1", st.Last24h)
	}
	if st.Last7d != 2 {
		t.Errorf("Last7d: got %d, want 2", st.Last7d)
	}
	if st.Last30d != 3 {
		t.Errorf("Last30d: got %d, want 3", st.Last30d)
	}
}

func TestStatsTopDesiresLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	// Create 7 different tool names - only top 5 should be returned.
	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("tool_%d", i)
		for j := 0; j <= i; j++ {
			if err := s.RecordDesire(ctx, model.Desire{
				ID:        fmt.Sprintf("tl-%d-%d", i, j),
				ToolName:  name,
				Error:     "e",
				Timestamp: ts,
			}); err != nil {
				t.Fatalf("RecordDesire: %v", err)
			}
		}
	}

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(st.TopDesires) != 5 {
		t.Errorf("TopDesires: expected 5, got %d", len(st.TopDesires))
	}
	// Most frequent should be tool_6 (7 occurrences).
	if st.TopDesires[0].Name != "tool_6" {
		t.Errorf("TopDesires[0].Name = %q, want tool_6", st.TopDesires[0].Name)
	}
	if st.TopDesires[0].Count != 7 {
		t.Errorf("TopDesires[0].Count = %d, want 7", st.TopDesires[0].Count)
	}
}

func TestDeleteAlias(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetAlias(ctx, model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}

	deleted, err := s.DeleteAlias(ctx, "read_file", "", "", "", "")
	if err != nil {
		t.Fatalf("DeleteAlias: %v", err)
	}
	if !deleted {
		t.Error("expected deleted=true")
	}

	aliases, err := s.GetAliases(ctx)
	if err != nil {
		t.Fatalf("GetAliases: %v", err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after delete, got %d", len(aliases))
	}
}

func TestGetAlias(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetAlias(ctx, model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}

	alias, err := s.GetAlias(ctx, "read_file", "", "", "", "")
	if err != nil {
		t.Fatalf("GetAlias: %v", err)
	}
	if alias == nil {
		t.Fatal("expected alias, got nil")
	}
	if alias.From != "read_file" || alias.To != "Read" {
		t.Errorf("got %s->%s, want read_file->Read", alias.From, alias.To)
	}
}

func TestGetAliasNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	alias, err := s.GetAlias(ctx, "nonexistent", "", "", "", "")
	if err != nil {
		t.Fatalf("GetAlias: %v", err)
	}
	if alias != nil {
		t.Errorf("expected nil for nonexistent alias, got %+v", alias)
	}
}

func TestDeleteAliasNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	deleted, err := s.DeleteAlias(ctx, "nonexistent", "", "", "", "")
	if err != nil {
		t.Fatalf("DeleteAlias: %v", err)
	}
	if deleted {
		t.Error("expected deleted=false for nonexistent alias")
	}
}

func TestInspectPathBasic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	desires := []model.Desire{
		{ID: "ip1", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"/etc/hosts"}`), Error: "unknown tool", Timestamp: base},
		{ID: "ip2", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"/etc/passwd"}`), Error: "unknown tool", Timestamp: base.Add(time.Hour)},
		{ID: "ip3", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"/etc/hosts"}`), Error: "tool not found", Timestamp: base.Add(25 * time.Hour)},
		{ID: "ip4", ToolName: "write_file", ToolInput: json.RawMessage(`{"path":"/tmp/out"}`), Error: "not allowed", Timestamp: base.Add(2 * time.Hour)},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	result, err := s.InspectPath(ctx, InspectOpts{Pattern: "read_file"})
	if err != nil {
		t.Fatalf("InspectPath: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if !result.FirstSeen.Equal(base) {
		t.Errorf("FirstSeen = %v, want %v", result.FirstSeen, base)
	}
	if !result.LastSeen.Equal(base.Add(25 * time.Hour)) {
		t.Errorf("LastSeen = %v, want %v", result.LastSeen, base.Add(25*time.Hour))
	}

	// Histogram: should have 2 days.
	if len(result.Histogram) != 2 {
		t.Fatalf("Histogram: expected 2 days, got %d", len(result.Histogram))
	}
	if result.Histogram[0].Date != "2026-01-10" {
		t.Errorf("Histogram[0].Date = %q, want %q", result.Histogram[0].Date, "2026-01-10")
	}
	if result.Histogram[0].Count != 2 {
		t.Errorf("Histogram[0].Count = %d, want 2", result.Histogram[0].Count)
	}
	if result.Histogram[1].Date != "2026-01-11" {
		t.Errorf("Histogram[1].Date = %q, want %q", result.Histogram[1].Date, "2026-01-11")
	}
	if result.Histogram[1].Count != 1 {
		t.Errorf("Histogram[1].Count = %d, want 1", result.Histogram[1].Count)
	}

	// Top inputs: /etc/hosts (2) > /etc/passwd (1).
	if len(result.TopInputs) != 2 {
		t.Fatalf("TopInputs: expected 2, got %d", len(result.TopInputs))
	}
	if result.TopInputs[0].Count != 2 {
		t.Errorf("TopInputs[0].Count = %d, want 2", result.TopInputs[0].Count)
	}

	// Top errors: "unknown tool" (2) > "tool not found" (1).
	if len(result.TopErrors) != 2 {
		t.Fatalf("TopErrors: expected 2, got %d", len(result.TopErrors))
	}
	if result.TopErrors[0].Name != "unknown tool" {
		t.Errorf("TopErrors[0].Name = %q, want %q", result.TopErrors[0].Name, "unknown tool")
	}
	if result.TopErrors[0].Count != 2 {
		t.Errorf("TopErrors[0].Count = %d, want 2", result.TopErrors[0].Count)
	}
}

func TestInspectPathEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	result, err := s.InspectPath(ctx, InspectOpts{Pattern: "nonexistent"})
	if err != nil {
		t.Fatalf("InspectPath: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
	if len(result.Histogram) != 0 {
		t.Errorf("Histogram: expected empty, got %d entries", len(result.Histogram))
	}
}

func TestInspectPathWithAlias(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.RecordDesire(ctx, model.Desire{
		ID: "ipa1", ToolName: "read_file", Error: "e", Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("RecordDesire: %v", err)
	}
	if err := s.SetAlias(ctx, model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}

	result, err := s.InspectPath(ctx, InspectOpts{Pattern: "read_file"})
	if err != nil {
		t.Fatalf("InspectPath: %v", err)
	}
	if result.AliasTo != "Read" {
		t.Errorf("AliasTo = %q, want %q", result.AliasTo, "Read")
	}
}

func TestInspectPathWildcard(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	desires := []model.Desire{
		{ID: "ipw1", ToolName: "read_file", Error: "e1", Timestamp: ts},
		{ID: "ipw2", ToolName: "read_dir", Error: "e2", Timestamp: ts},
		{ID: "ipw3", ToolName: "write_file", Error: "e3", Timestamp: ts},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	result, err := s.InspectPath(ctx, InspectOpts{Pattern: "read%"})
	if err != nil {
		t.Fatalf("InspectPath: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("Total = %d, want 2 (read_file + read_dir)", result.Total)
	}
}

func TestInspectPathWithSince(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	desires := []model.Desire{
		{ID: "ips1", ToolName: "read_file", Error: "e1", Timestamp: base},
		{ID: "ips2", ToolName: "read_file", Error: "e2", Timestamp: base.Add(2 * time.Hour)},
		{ID: "ips3", ToolName: "read_file", Error: "e3", Timestamp: base.Add(4 * time.Hour)},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	result, err := s.InspectPath(ctx, InspectOpts{
		Pattern: "read_file",
		Since:   base.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("InspectPath: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("Total = %d, want 2 (only after since)", result.Total)
	}
}

func TestInspectPathTopN(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	// Create desires with 6 different error messages.
	for i := 0; i < 6; i++ {
		if err := s.RecordDesire(ctx, model.Desire{
			ID:       fmt.Sprintf("ipn-%d", i),
			ToolName: "test_tool",
			Error:    fmt.Sprintf("error_%d", i),
			Timestamp: ts,
		}); err != nil {
			t.Fatalf("RecordDesire: %v", err)
		}
	}

	result, err := s.InspectPath(ctx, InspectOpts{Pattern: "test_tool", TopN: 3})
	if err != nil {
		t.Fatalf("InspectPath: %v", err)
	}
	if len(result.TopErrors) != 3 {
		t.Errorf("TopErrors: expected 3 with TopN=3, got %d", len(result.TopErrors))
	}
}

func TestListDesiresCategoryFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	desires := []model.Desire{
		{ID: "d1", ToolName: "Bash", Error: "bash: cargo-insta: command not found", Category: model.CategoryEnvNeed, Timestamp: base},
		{ID: "d2", ToolName: "Bash", Error: "permission denied", Timestamp: base.Add(time.Hour)},
		{ID: "d3", ToolName: "Bash", Error: "bash: cargo-nextest: command not found", Category: model.CategoryEnvNeed, Timestamp: base.Add(2 * time.Hour)},
		{ID: "d4", ToolName: "read_file", Error: "tool not found", Timestamp: base.Add(3 * time.Hour)},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire %s: %v", d.ID, err)
		}
	}

	// Filter by env-need category.
	got, err := s.ListDesires(ctx, ListOpts{Category: model.CategoryEnvNeed})
	if err != nil {
		t.Fatalf("ListDesires by category: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("category filter: expected 2, got %d", len(got))
	}
	for _, d := range got {
		if d.Category != model.CategoryEnvNeed {
			t.Errorf("expected category %q, got %q", model.CategoryEnvNeed, d.Category)
		}
	}

	// All desires without category filter.
	all, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires all: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("all: expected 4, got %d", len(all))
	}
}

func TestRecordDesireCategoryRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	d := model.Desire{
		ID:        "cat-1",
		ToolName:  "Bash",
		Error:     "bash: rg: command not found",
		Category:  model.CategoryEnvNeed,
		Timestamp: time.Now().UTC(),
	}
	if err := s.RecordDesire(ctx, d); err != nil {
		t.Fatalf("RecordDesire: %v", err)
	}

	got, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].Category != model.CategoryEnvNeed {
		t.Errorf("category: got %q, want %q", got[0].Category, model.CategoryEnvNeed)
	}
}

// Verify SQLiteStore satisfies the Store interface at compile time.
var _ Store = (*SQLiteStore)(nil)
