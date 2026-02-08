package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"

	_ "modernc.org/sqlite"
)

func TestRecordInvocationRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)

	inv := model.Invocation{
		ID:         "inv-1",
		Source:     "claude-code",
		InstanceID: "inst-abc",
		HostID:     "host-xyz",
		ToolName:   "Read",
		IsError:    false,
		Error:      "",
		CWD:        "/home/user/project",
		Timestamp:  ts,
		Metadata:   json.RawMessage(`{"model":"opus","tokens":150}`),
	}

	if err := s.RecordInvocation(ctx, inv); err != nil {
		t.Fatalf("RecordInvocation: %v", err)
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(got))
	}

	g := got[0]
	if g.ID != inv.ID {
		t.Errorf("ID: got %q, want %q", g.ID, inv.ID)
	}
	if g.Source != inv.Source {
		t.Errorf("Source: got %q, want %q", g.Source, inv.Source)
	}
	if g.InstanceID != inv.InstanceID {
		t.Errorf("InstanceID: got %q, want %q", g.InstanceID, inv.InstanceID)
	}
	if g.HostID != inv.HostID {
		t.Errorf("HostID: got %q, want %q", g.HostID, inv.HostID)
	}
	if g.ToolName != inv.ToolName {
		t.Errorf("ToolName: got %q, want %q", g.ToolName, inv.ToolName)
	}
	if g.IsError != inv.IsError {
		t.Errorf("IsError: got %v, want %v", g.IsError, inv.IsError)
	}
	if g.Error != inv.Error {
		t.Errorf("Error: got %q, want %q", g.Error, inv.Error)
	}
	if g.CWD != inv.CWD {
		t.Errorf("CWD: got %q, want %q", g.CWD, inv.CWD)
	}
	if !g.Timestamp.Equal(inv.Timestamp) {
		t.Errorf("Timestamp: got %v, want %v", g.Timestamp, inv.Timestamp)
	}
	if string(g.Metadata) != string(inv.Metadata) {
		t.Errorf("Metadata: got %s, want %s", g.Metadata, inv.Metadata)
	}
}

func TestRecordInvocationErrorCase(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	inv := model.Invocation{
		ID:        "inv-err-1",
		Source:    "claude-code",
		ToolName:  "Bash",
		IsError:   true,
		Error:     "command not found: foobar",
		Timestamp: ts,
	}

	if err := s.RecordInvocation(ctx, inv); err != nil {
		t.Fatalf("RecordInvocation: %v", err)
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if !got[0].IsError {
		t.Error("expected IsError=true")
	}
	if got[0].Error != inv.Error {
		t.Errorf("Error: got %q, want %q", got[0].Error, inv.Error)
	}
}

func TestRecordInvocationDuplicateID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	inv := model.Invocation{
		ID:        "dup-inv-1",
		Source:    "claude-code",
		ToolName:  "Read",
		Timestamp: ts,
	}
	if err := s.RecordInvocation(ctx, inv); err != nil {
		t.Fatalf("first RecordInvocation: %v", err)
	}

	err := s.RecordInvocation(ctx, inv)
	if err == nil {
		t.Fatal("expected error on duplicate ID, got nil")
	}
}

func TestRecordInvocationMinimalFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	inv := model.Invocation{
		ID:        "min-inv-1",
		Source:    "gemini-cli",
		ToolName:  "search",
		Timestamp: time.Now().UTC(),
	}
	if err := s.RecordInvocation(ctx, inv); err != nil {
		t.Fatalf("RecordInvocation: %v", err)
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].InstanceID != "" {
		t.Errorf("InstanceID: got %q, want empty", got[0].InstanceID)
	}
	if got[0].HostID != "" {
		t.Errorf("HostID: got %q, want empty", got[0].HostID)
	}
	if got[0].CWD != "" {
		t.Errorf("CWD: got %q, want empty", got[0].CWD)
	}
	if got[0].Metadata != nil {
		t.Errorf("Metadata: got %s, want nil", got[0].Metadata)
	}
	if got[0].IsError {
		t.Error("expected IsError=false")
	}
}

func TestListInvocationsFilterBySource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	invocations := []model.Invocation{
		{ID: "s1", Source: "claude-code", ToolName: "Read", Timestamp: base},
		{ID: "s2", Source: "gemini-cli", ToolName: "Write", Timestamp: base.Add(time.Hour)},
		{ID: "s3", Source: "claude-code", ToolName: "Bash", Timestamp: base.Add(2 * time.Hour)},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation %s: %v", inv.ID, err)
		}
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{Source: "claude-code"})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("source filter: expected 2, got %d", len(got))
	}
	for _, inv := range got {
		if inv.Source != "claude-code" {
			t.Errorf("unexpected source %q", inv.Source)
		}
	}
}

func TestListInvocationsFilterByInstanceID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	invocations := []model.Invocation{
		{ID: "i1", Source: "claude-code", InstanceID: "inst-1", ToolName: "Read", Timestamp: base},
		{ID: "i2", Source: "claude-code", InstanceID: "inst-2", ToolName: "Write", Timestamp: base.Add(time.Hour)},
		{ID: "i3", Source: "claude-code", InstanceID: "inst-1", ToolName: "Bash", Timestamp: base.Add(2 * time.Hour)},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation %s: %v", inv.ID, err)
		}
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{InstanceID: "inst-1"})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("instance_id filter: expected 2, got %d", len(got))
	}
	for _, inv := range got {
		if inv.InstanceID != "inst-1" {
			t.Errorf("unexpected instance_id %q", inv.InstanceID)
		}
	}
}

func TestListInvocationsFilterByToolName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	invocations := []model.Invocation{
		{ID: "t1", Source: "claude-code", ToolName: "Read", Timestamp: base},
		{ID: "t2", Source: "claude-code", ToolName: "Write", Timestamp: base.Add(time.Hour)},
		{ID: "t3", Source: "claude-code", ToolName: "Read", Timestamp: base.Add(2 * time.Hour)},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation %s: %v", inv.ID, err)
		}
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{ToolName: "Read"})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("tool_name filter: expected 2, got %d", len(got))
	}
}

func TestListInvocationsFilterErrorsOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	invocations := []model.Invocation{
		{ID: "e1", Source: "claude-code", ToolName: "Read", IsError: false, Timestamp: base},
		{ID: "e2", Source: "claude-code", ToolName: "Bash", IsError: true, Error: "failed", Timestamp: base.Add(time.Hour)},
		{ID: "e3", Source: "claude-code", ToolName: "Write", IsError: false, Timestamp: base.Add(2 * time.Hour)},
		{ID: "e4", Source: "claude-code", ToolName: "Grep", IsError: true, Error: "timeout", Timestamp: base.Add(3 * time.Hour)},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation %s: %v", inv.ID, err)
		}
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{ErrorsOnly: true})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("errors_only filter: expected 2, got %d", len(got))
	}
	for _, inv := range got {
		if !inv.IsError {
			t.Errorf("expected IsError=true for %q", inv.ID)
		}
	}
}

func TestListInvocationsFilterBySince(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	invocations := []model.Invocation{
		{ID: "since1", Source: "cc", ToolName: "Read", Timestamp: base},
		{ID: "since2", Source: "cc", ToolName: "Write", Timestamp: base.Add(2 * time.Hour)},
		{ID: "since3", Source: "cc", ToolName: "Bash", Timestamp: base.Add(4 * time.Hour)},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation %s: %v", inv.ID, err)
		}
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{Since: base.Add(time.Hour)})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("since filter: expected 2, got %d", len(got))
	}
}

func TestListInvocationsFilterByLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		if err := s.RecordInvocation(ctx, model.Invocation{
			ID:        fmt.Sprintf("lim-%d", i),
			Source:    "cc",
			ToolName:  "Read",
			Timestamp: base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("RecordInvocation: %v", err)
		}
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{Limit: 3})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("limit filter: expected 3, got %d", len(got))
	}
}

func TestListInvocationsCombinedFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	invocations := []model.Invocation{
		{ID: "cf1", Source: "claude-code", InstanceID: "inst-1", ToolName: "Read", IsError: false, Timestamp: base},
		{ID: "cf2", Source: "claude-code", InstanceID: "inst-1", ToolName: "Read", IsError: true, Error: "err", Timestamp: base.Add(time.Hour)},
		{ID: "cf3", Source: "gemini-cli", InstanceID: "inst-1", ToolName: "Read", IsError: true, Error: "err", Timestamp: base.Add(2 * time.Hour)},
		{ID: "cf4", Source: "claude-code", InstanceID: "inst-1", ToolName: "Write", IsError: true, Error: "err", Timestamp: base.Add(3 * time.Hour)},
		{ID: "cf5", Source: "claude-code", InstanceID: "inst-2", ToolName: "Read", IsError: true, Error: "err", Timestamp: base.Add(4 * time.Hour)},
		{ID: "cf6", Source: "claude-code", InstanceID: "inst-1", ToolName: "Read", IsError: true, Error: "err", Timestamp: base.Add(5 * time.Hour)},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation %s: %v", inv.ID, err)
		}
	}

	// Combine: source + instance_id + tool_name + errors_only + since + limit.
	got, err := s.ListInvocations(ctx, InvocationOpts{
		Source:     "claude-code",
		InstanceID: "inst-1",
		ToolName:   "Read",
		ErrorsOnly: true,
		Since:      base.Add(30 * time.Minute),
		Limit:      1,
	})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("combined filter: expected 1, got %d", len(got))
	}
	// Should be cf6 (newest matching all filters).
	if got[0].ID != "cf6" {
		t.Errorf("ID = %q, want cf6", got[0].ID)
	}
}

func TestListInvocationsOrderDesc(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		if err := s.RecordInvocation(ctx, model.Invocation{
			ID:        fmt.Sprintf("ord-%d", i),
			Source:    "cc",
			ToolName:  "Read",
			Timestamp: base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("RecordInvocation: %v", err)
		}
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	// Newest first.
	if got[0].ID != "ord-2" {
		t.Errorf("first = %q, want ord-2", got[0].ID)
	}
	if got[2].ID != "ord-0" {
		t.Errorf("last = %q, want ord-0", got[2].ID)
	}
}

func TestListInvocationsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestInvocationStatsCorrectTotals(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	invocations := []model.Invocation{
		{ID: "st1", Source: "claude-code", ToolName: "Read", Timestamp: ts},
		{ID: "st2", Source: "claude-code", ToolName: "Read", Timestamp: ts},
		{ID: "st3", Source: "gemini-cli", ToolName: "Write", Timestamp: ts},
		{ID: "st4", Source: "claude-code", ToolName: "Bash", Timestamp: ts},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation: %v", err)
		}
	}

	st, err := s.InvocationStats(ctx)
	if err != nil {
		t.Fatalf("InvocationStats: %v", err)
	}
	if st.Total != 4 {
		t.Errorf("Total: got %d, want 4", st.Total)
	}
	if st.UniqueTools != 3 {
		t.Errorf("UniqueTools: got %d, want 3", st.UniqueTools)
	}
}

func TestInvocationStatsTopTools(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	// Read=3, Bash=2, Write=1
	tools := []string{"Read", "Read", "Read", "Bash", "Bash", "Write"}
	for i, tool := range tools {
		if err := s.RecordInvocation(ctx, model.Invocation{
			ID:        fmt.Sprintf("tt-%d", i),
			Source:    "cc",
			ToolName:  tool,
			Timestamp: ts,
		}); err != nil {
			t.Fatalf("RecordInvocation: %v", err)
		}
	}

	st, err := s.InvocationStats(ctx)
	if err != nil {
		t.Fatalf("InvocationStats: %v", err)
	}
	if len(st.TopTools) != 3 {
		t.Fatalf("TopTools: expected 3, got %d", len(st.TopTools))
	}
	if st.TopTools[0].Name != "Read" || st.TopTools[0].Count != 3 {
		t.Errorf("TopTools[0]: got %+v, want {Read, 3}", st.TopTools[0])
	}
	if st.TopTools[1].Name != "Bash" || st.TopTools[1].Count != 2 {
		t.Errorf("TopTools[1]: got %+v, want {Bash, 2}", st.TopTools[1])
	}
	if st.TopTools[2].Name != "Write" || st.TopTools[2].Count != 1 {
		t.Errorf("TopTools[2]: got %+v, want {Write, 1}", st.TopTools[2])
	}
}

func TestInvocationStatsTopSources(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	sources := []string{"claude-code", "claude-code", "claude-code", "gemini-cli", "kiro"}
	for i, src := range sources {
		if err := s.RecordInvocation(ctx, model.Invocation{
			ID:        fmt.Sprintf("ts-%d", i),
			Source:    src,
			ToolName:  "Read",
			Timestamp: ts,
		}); err != nil {
			t.Fatalf("RecordInvocation: %v", err)
		}
	}

	st, err := s.InvocationStats(ctx)
	if err != nil {
		t.Fatalf("InvocationStats: %v", err)
	}
	if len(st.TopSources) != 3 {
		t.Fatalf("TopSources: expected 3, got %d", len(st.TopSources))
	}
	if st.TopSources[0].Name != "claude-code" || st.TopSources[0].Count != 3 {
		t.Errorf("TopSources[0]: got %+v, want {claude-code, 3}", st.TopSources[0])
	}
}

func TestInvocationStatsTimeWindows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	invocations := []model.Invocation{
		{ID: "tw1", Source: "cc", ToolName: "a", Timestamp: now.Add(-2 * time.Hour)},          // Within 24h.
		{ID: "tw2", Source: "cc", ToolName: "b", Timestamp: now.Add(-3 * 24 * time.Hour)},     // Within 7d.
		{ID: "tw3", Source: "cc", ToolName: "c", Timestamp: now.Add(-15 * 24 * time.Hour)},    // Within 30d.
		{ID: "tw4", Source: "cc", ToolName: "d", Timestamp: now.Add(-60 * 24 * time.Hour)},    // Outside 30d.
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation: %v", err)
		}
	}

	st, err := s.InvocationStats(ctx)
	if err != nil {
		t.Fatalf("InvocationStats: %v", err)
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

func TestInvocationStatsDateRange(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	earliest := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	latest := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	invocations := []model.Invocation{
		{ID: "dr1", Source: "cc", ToolName: "a", Timestamp: earliest},
		{ID: "dr2", Source: "cc", ToolName: "b", Timestamp: latest},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation: %v", err)
		}
	}

	st, err := s.InvocationStats(ctx)
	if err != nil {
		t.Fatalf("InvocationStats: %v", err)
	}
	if !st.Earliest.Equal(earliest) {
		t.Errorf("Earliest: got %v, want %v", st.Earliest, earliest)
	}
	if !st.Latest.Equal(latest) {
		t.Errorf("Latest: got %v, want %v", st.Latest, latest)
	}
}

func TestInvocationStatsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	st, err := s.InvocationStats(ctx)
	if err != nil {
		t.Fatalf("InvocationStats: %v", err)
	}
	if st.Total != 0 {
		t.Errorf("Total: got %d, want 0", st.Total)
	}
	if st.UniqueTools != 0 {
		t.Errorf("UniqueTools: got %d, want 0", st.UniqueTools)
	}
	if len(st.TopSources) != 0 {
		t.Errorf("TopSources: expected empty, got %v", st.TopSources)
	}
	if len(st.TopTools) != 0 {
		t.Errorf("TopTools: expected empty, got %v", st.TopTools)
	}
}

func TestInvocationStatsTopToolsLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	// Create 7 different tools - only top 5 should be returned.
	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("tool_%d", i)
		for j := 0; j <= i; j++ {
			if err := s.RecordInvocation(ctx, model.Invocation{
				ID:        fmt.Sprintf("tl-%d-%d", i, j),
				Source:    "cc",
				ToolName:  name,
				Timestamp: ts,
			}); err != nil {
				t.Fatalf("RecordInvocation: %v", err)
			}
		}
	}

	st, err := s.InvocationStats(ctx)
	if err != nil {
		t.Fatalf("InvocationStats: %v", err)
	}
	if len(st.TopTools) != 5 {
		t.Errorf("TopTools: expected 5, got %d", len(st.TopTools))
	}
	if st.TopTools[0].Name != "tool_6" || st.TopTools[0].Count != 7 {
		t.Errorf("TopTools[0]: got %+v, want {tool_6, 7}", st.TopTools[0])
	}
}

func TestMetadataJSONPreserved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		metadata json.RawMessage
	}{
		{"simple object", json.RawMessage(`{"key":"value"}`)},
		{"nested object", json.RawMessage(`{"model":"opus","context":{"tokens":1500,"turns":3}}`)},
		{"array value", json.RawMessage(`{"tools":["Read","Write","Bash"]}`)},
		{"numeric values", json.RawMessage(`{"tokens":1500,"cost":0.03,"success":true}`)},
		{"empty object", json.RawMessage(`{}`)},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := model.Invocation{
				ID:        fmt.Sprintf("meta-%d", i),
				Source:    "cc",
				ToolName:  "Read",
				Timestamp: time.Now().UTC(),
				Metadata:  tt.metadata,
			}
			if err := s.RecordInvocation(ctx, inv); err != nil {
				t.Fatalf("RecordInvocation: %v", err)
			}

			got, err := s.ListInvocations(ctx, InvocationOpts{ToolName: "Read", Limit: 1})
			if err != nil {
				t.Fatalf("ListInvocations: %v", err)
			}
			if len(got) == 0 {
				t.Fatal("expected at least 1 invocation")
			}

			// Parse both to compare structured JSON (not string equality).
			var wantParsed, gotParsed any
			if err := json.Unmarshal(tt.metadata, &wantParsed); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}
			if err := json.Unmarshal(got[0].Metadata, &gotParsed); err != nil {
				t.Fatalf("unmarshal got: %v", err)
			}

			wantBytes, _ := json.Marshal(wantParsed)
			gotBytes, _ := json.Marshal(gotParsed)
			if string(wantBytes) != string(gotBytes) {
				t.Errorf("metadata: got %s, want %s", gotBytes, wantBytes)
			}
		})
	}
}

func TestMetadataNilPreserved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	inv := model.Invocation{
		ID:        "meta-nil",
		Source:    "cc",
		ToolName:  "Read",
		Timestamp: time.Now().UTC(),
		Metadata:  nil,
	}
	if err := s.RecordInvocation(ctx, inv); err != nil {
		t.Fatalf("RecordInvocation: %v", err)
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].Metadata != nil {
		t.Errorf("Metadata: got %s, want nil", got[0].Metadata)
	}
}

func TestMetadataLargePayload(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Build a large metadata object.
	meta := make(map[string]string)
	for i := 0; i < 500; i++ {
		meta[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("value_%d_padding", i)
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}

	inv := model.Invocation{
		ID:        "meta-large",
		Source:    "cc",
		ToolName:  "Read",
		Timestamp: time.Now().UTC(),
		Metadata:  metaBytes,
	}
	if err := s.RecordInvocation(ctx, inv); err != nil {
		t.Fatalf("RecordInvocation: %v", err)
	}

	got, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}

	var gotMeta map[string]string
	if err := json.Unmarshal(got[0].Metadata, &gotMeta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(gotMeta) != 500 {
		t.Errorf("metadata keys: got %d, want 500", len(gotMeta))
	}
}

func TestMigrateV2FreshDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fresh.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// Verify schema_version is 2.
	var ver int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&ver); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if ver != 2 {
		t.Errorf("schema version: got %d, want 2", ver)
	}

	// Verify invocations table exists by inserting.
	ctx := context.Background()
	inv := model.Invocation{
		ID:        "fresh-1",
		Source:    "cc",
		ToolName:  "Read",
		Timestamp: time.Now().UTC(),
	}
	if err := s.RecordInvocation(ctx, inv); err != nil {
		t.Fatalf("RecordInvocation on fresh DB: %v", err)
	}

	// Verify desires table also exists (from v1).
	d := model.Desire{
		ID:        "fresh-d1",
		ToolName:  "read_file",
		Error:     "unknown tool",
		Timestamp: time.Now().UTC(),
	}
	if err := s.RecordDesire(ctx, d); err != nil {
		t.Fatalf("RecordDesire on fresh DB: %v", err)
	}
}

func TestMigrateV2OnExistingV1DB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "upgrade.db")

	// Step 1: Create a v1-only database manually.
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)

	v1Stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS desires (
			id         TEXT PRIMARY KEY,
			tool_name  TEXT NOT NULL,
			tool_input TEXT,
			error      TEXT NOT NULL,
			source     TEXT,
			session_id TEXT,
			cwd        TEXT,
			timestamp  TEXT NOT NULL,
			metadata   TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_desires_tool_name ON desires(tool_name)`,
		`CREATE INDEX IF NOT EXISTS idx_desires_source ON desires(source)`,
		`CREATE INDEX IF NOT EXISTS idx_desires_timestamp ON desires(timestamp)`,
		`CREATE TABLE IF NOT EXISTS aliases (
			from_name  TEXT PRIMARY KEY,
			to_name    TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`INSERT INTO schema_version (version) VALUES (1)`,
	}
	for _, stmt := range v1Stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("v1 setup: %v", err)
		}
	}

	// Insert some v1 data.
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(
		`INSERT INTO desires (id, tool_name, error, source, timestamp) VALUES (?, ?, ?, ?, ?)`,
		"v1-desire-1", "read_file", "unknown tool", "claude-code", ts,
	); err != nil {
		t.Fatalf("insert v1 desire: %v", err)
	}
	db.Close()

	// Step 2: Open with New() which should run migration v2.
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New on v1 DB: %v", err)
	}
	defer s.Close()

	// Verify version upgraded to 2.
	var ver int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&ver); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if ver != 2 {
		t.Errorf("schema version after upgrade: got %d, want 2", ver)
	}

	ctx := context.Background()

	// Verify existing v1 data survived.
	desires, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("expected 1 desire from v1, got %d", len(desires))
	}
	if desires[0].ID != "v1-desire-1" {
		t.Errorf("v1 desire ID: got %q, want v1-desire-1", desires[0].ID)
	}

	// Verify invocations table works.
	inv := model.Invocation{
		ID:        "v2-inv-1",
		Source:    "cc",
		ToolName:  "Read",
		Timestamp: time.Now().UTC(),
	}
	if err := s.RecordInvocation(ctx, inv); err != nil {
		t.Fatalf("RecordInvocation on upgraded DB: %v", err)
	}

	invocations, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(invocations))
	}
	if invocations[0].ID != "v2-inv-1" {
		t.Errorf("invocation ID: got %q, want v2-inv-1", invocations[0].ID)
	}
}

func TestMigrateV2Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "idempotent.db")

	// Open twice - migration should be idempotent.
	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	s1.Close()

	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	defer s2.Close()

	// Should still work.
	ctx := context.Background()
	if err := s2.RecordInvocation(ctx, model.Invocation{
		ID:        "idem-1",
		Source:    "cc",
		ToolName:  "Read",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("RecordInvocation: %v", err)
	}
}

func TestInvocationAndDesireIndependence(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Now().UTC()

	// Record both types.
	if err := s.RecordDesire(ctx, model.Desire{
		ID: "d1", ToolName: "read_file", Error: "unknown", Timestamp: ts,
	}); err != nil {
		t.Fatalf("RecordDesire: %v", err)
	}
	if err := s.RecordInvocation(ctx, model.Invocation{
		ID: "i1", Source: "cc", ToolName: "Read", Timestamp: ts,
	}); err != nil {
		t.Fatalf("RecordInvocation: %v", err)
	}

	// Desires should only show desires.
	desires, err := s.ListDesires(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListDesires: %v", err)
	}
	if len(desires) != 1 {
		t.Errorf("desires: got %d, want 1", len(desires))
	}

	// Invocations should only show invocations.
	invocations, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(invocations) != 1 {
		t.Errorf("invocations: got %d, want 1", len(invocations))
	}

	// Stats should be separate.
	dStats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if dStats.TotalDesires != 1 {
		t.Errorf("TotalDesires: got %d, want 1", dStats.TotalDesires)
	}

	iStats, err := s.InvocationStats(ctx)
	if err != nil {
		t.Fatalf("InvocationStats: %v", err)
	}
	if iStats.Total != 1 {
		t.Errorf("invocation Total: got %d, want 1", iStats.Total)
	}
}
