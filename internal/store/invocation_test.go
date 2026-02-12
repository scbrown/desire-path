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

func TestRecordInvocationTurnFieldsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)

	inv := model.Invocation{
		ID:           "inv-turn-1",
		Source:       "claude-code",
		InstanceID:   "sess-xyz",
		ToolName:     "Read",
		Timestamp:    ts,
		TurnID:       "sess-xyz:3",
		TurnSequence: 2,
		TurnLength:   5,
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
	if g.TurnID != "sess-xyz:3" {
		t.Errorf("TurnID: got %q, want %q", g.TurnID, "sess-xyz:3")
	}
	if g.TurnSequence != 2 {
		t.Errorf("TurnSequence: got %d, want 2", g.TurnSequence)
	}
	if g.TurnLength != 5 {
		t.Errorf("TurnLength: got %d, want 5", g.TurnLength)
	}
}

func TestRecordInvocationTurnFieldsDefaults(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	inv := model.Invocation{
		ID:        "inv-noturn",
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

	g := got[0]
	if g.TurnID != "" {
		t.Errorf("TurnID: got %q, want empty", g.TurnID)
	}
	if g.TurnSequence != 0 {
		t.Errorf("TurnSequence: got %d, want 0", g.TurnSequence)
	}
	if g.TurnLength != 0 {
		t.Errorf("TurnLength: got %d, want 0", g.TurnLength)
	}
}

func TestMigrateV5TurnColumns(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "v5-test.db")

	// Create a v4 database manually.
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)

	v4Stmts := []string{
		`CREATE TABLE schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version (version) VALUES (4)`,
		`CREATE TABLE desires (
			id TEXT PRIMARY KEY, tool_name TEXT NOT NULL, tool_input TEXT,
			error TEXT NOT NULL, category TEXT, source TEXT, session_id TEXT,
			cwd TEXT, timestamp TEXT NOT NULL, metadata TEXT
		)`,
		`CREATE TABLE invocations (
			id TEXT PRIMARY KEY, source TEXT NOT NULL, instance_id TEXT,
			host_id TEXT, tool_name TEXT NOT NULL, is_error INTEGER NOT NULL DEFAULT 0,
			error TEXT, cwd TEXT, timestamp TEXT NOT NULL, metadata TEXT
		)`,
		`CREATE TABLE aliases (
			from_name TEXT NOT NULL, to_name TEXT NOT NULL, tool TEXT NOT NULL DEFAULT '',
			param TEXT NOT NULL DEFAULT '', command TEXT NOT NULL DEFAULT '',
			match_kind TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			PRIMARY KEY (from_name, tool, param, command, match_kind)
		)`,
		// Insert a pre-existing invocation to verify it survives migration.
		fmt.Sprintf(`INSERT INTO invocations (id, source, tool_name, timestamp) VALUES ('pre-v5', 'cc', 'Read', '%s')`,
			time.Now().UTC().Format(time.RFC3339Nano)),
	}
	for _, stmt := range v4Stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("v4 setup: %v", err)
		}
	}
	db.Close()

	// Open with New() - should run v5 migration.
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New on v4 DB: %v", err)
	}
	defer s.Close()

	// Verify version.
	var ver int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&ver); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if ver != 5 {
		t.Errorf("schema version: got %d, want 5", ver)
	}

	ctx := context.Background()

	// Pre-existing invocation should have default turn values.
	got, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].TurnID != "" {
		t.Errorf("pre-v5 TurnID: got %q, want empty", got[0].TurnID)
	}
	if got[0].TurnSequence != 0 {
		t.Errorf("pre-v5 TurnSequence: got %d, want 0", got[0].TurnSequence)
	}
	if got[0].TurnLength != 0 {
		t.Errorf("pre-v5 TurnLength: got %d, want 0", got[0].TurnLength)
	}

	// New invocation with turn fields should work.
	inv := model.Invocation{
		ID:           "post-v5",
		Source:       "cc",
		ToolName:     "Bash",
		Timestamp:    time.Now().UTC(),
		TurnID:       "abc:2",
		TurnSequence: 1,
		TurnLength:   4,
	}
	if err := s.RecordInvocation(ctx, inv); err != nil {
		t.Fatalf("RecordInvocation: %v", err)
	}

	all, err := s.ListInvocations(ctx, InvocationOpts{})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
	// Newest first.
	if all[0].TurnID != "abc:2" {
		t.Errorf("post-v5 TurnID: got %q, want %q", all[0].TurnID, "abc:2")
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

	// Verify schema_version is current.
	var ver int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&ver); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if ver != schemaVersion {
		t.Errorf("schema version: got %d, want %d", ver, schemaVersion)
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

	// Verify version upgraded to current.
	var ver int
	if err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&ver); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if ver != schemaVersion {
		t.Errorf("schema version after upgrade: got %d, want %d", ver, schemaVersion)
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

func TestGetTurnsBasic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)

	// Two turns: turn sess1:0 has 3 tools, turn sess1:1 has 6 tools.
	invocations := []model.Invocation{
		{ID: "t1-0", Source: "cc", ToolName: "Grep", Timestamp: base, TurnID: "sess1:0", TurnSequence: 0, TurnLength: 3},
		{ID: "t1-1", Source: "cc", ToolName: "Read", Timestamp: base.Add(time.Second), TurnID: "sess1:0", TurnSequence: 1, TurnLength: 3},
		{ID: "t1-2", Source: "cc", ToolName: "Edit", Timestamp: base.Add(2 * time.Second), TurnID: "sess1:0", TurnSequence: 2, TurnLength: 3},
		{ID: "t2-0", Source: "cc", ToolName: "Grep", Timestamp: base.Add(time.Minute), TurnID: "sess1:1", TurnSequence: 0, TurnLength: 6},
		{ID: "t2-1", Source: "cc", ToolName: "Read", Timestamp: base.Add(time.Minute + time.Second), TurnID: "sess1:1", TurnSequence: 1, TurnLength: 6},
		{ID: "t2-2", Source: "cc", ToolName: "Read", Timestamp: base.Add(time.Minute + 2*time.Second), TurnID: "sess1:1", TurnSequence: 2, TurnLength: 6},
		{ID: "t2-3", Source: "cc", ToolName: "Read", Timestamp: base.Add(time.Minute + 3*time.Second), TurnID: "sess1:1", TurnSequence: 3, TurnLength: 6},
		{ID: "t2-4", Source: "cc", ToolName: "Edit", Timestamp: base.Add(time.Minute + 4*time.Second), TurnID: "sess1:1", TurnSequence: 4, TurnLength: 6},
		{ID: "t2-5", Source: "cc", ToolName: "Read", Timestamp: base.Add(time.Minute + 5*time.Second), TurnID: "sess1:1", TurnSequence: 5, TurnLength: 6},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation %s: %v", inv.ID, err)
		}
	}

	// MinLength=5 should only return the 6-tool turn.
	turns, err := s.GetTurns(ctx, TurnOpts{MinLength: 5})
	if err != nil {
		t.Fatalf("GetTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].TurnID != "sess1:1" {
		t.Errorf("TurnID: got %q, want sess1:1", turns[0].TurnID)
	}
	if turns[0].SessionID != "sess1" {
		t.Errorf("SessionID: got %q, want sess1", turns[0].SessionID)
	}
	if turns[0].TurnIndex != 1 {
		t.Errorf("TurnIndex: got %d, want 1", turns[0].TurnIndex)
	}
	if turns[0].Length != 6 {
		t.Errorf("Length: got %d, want 6", turns[0].Length)
	}
	if len(turns[0].Tools) != 6 {
		t.Fatalf("Tools: expected 6, got %d", len(turns[0].Tools))
	}
	expectedTools := []string{"Grep", "Read", "Read", "Read", "Edit", "Read"}
	for i, tool := range expectedTools {
		if turns[0].Tools[i] != tool {
			t.Errorf("Tools[%d]: got %q, want %q", i, turns[0].Tools[i], tool)
		}
	}

	// MinLength=1 should return both turns (newest first).
	turns, err = s.GetTurns(ctx, TurnOpts{MinLength: 1})
	if err != nil {
		t.Fatalf("GetTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if turns[0].TurnID != "sess1:1" {
		t.Errorf("first turn: got %q, want sess1:1", turns[0].TurnID)
	}
	if turns[1].TurnID != "sess1:0" {
		t.Errorf("second turn: got %q, want sess1:0", turns[1].TurnID)
	}
}

func TestGetTurnsFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)

	// Session 1: 1 turn of length 4.
	for i := 0; i < 4; i++ {
		if err := s.RecordInvocation(ctx, model.Invocation{
			ID:           fmt.Sprintf("s1t0-%d", i),
			Source:       "cc",
			ToolName:     "Read",
			Timestamp:    base.Add(time.Duration(i) * time.Second),
			TurnID:       "sess1:0",
			TurnSequence: i,
			TurnLength:   4,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Session 2: 1 turn of length 5, 1 hour later.
	for i := 0; i < 5; i++ {
		if err := s.RecordInvocation(ctx, model.Invocation{
			ID:           fmt.Sprintf("s2t0-%d", i),
			Source:       "cc",
			ToolName:     "Bash",
			Timestamp:    base.Add(time.Hour + time.Duration(i)*time.Second),
			TurnID:       "sess2:0",
			TurnSequence: i,
			TurnLength:   5,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Filter by session.
	turns, err := s.GetTurns(ctx, TurnOpts{MinLength: 1, SessionID: "sess1"})
	if err != nil {
		t.Fatalf("GetTurns session filter: %v", err)
	}
	if len(turns) != 1 {
		t.Errorf("session filter: expected 1, got %d", len(turns))
	}

	// Filter by since.
	turns, err = s.GetTurns(ctx, TurnOpts{MinLength: 1, Since: base.Add(30 * time.Minute)})
	if err != nil {
		t.Fatalf("GetTurns since filter: %v", err)
	}
	if len(turns) != 1 {
		t.Errorf("since filter: expected 1, got %d", len(turns))
	}
	if len(turns) > 0 && turns[0].TurnID != "sess2:0" {
		t.Errorf("since filter: got %q, want sess2:0", turns[0].TurnID)
	}

	// Limit.
	turns, err = s.GetTurns(ctx, TurnOpts{MinLength: 1, Limit: 1})
	if err != nil {
		t.Fatalf("GetTurns limit: %v", err)
	}
	if len(turns) != 1 {
		t.Errorf("limit: expected 1, got %d", len(turns))
	}
}

func TestGetTurnsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	turns, err := s.GetTurns(ctx, TurnOpts{MinLength: 5})
	if err != nil {
		t.Fatalf("GetTurns: %v", err)
	}
	if len(turns) != 0 {
		t.Errorf("expected 0 turns, got %d", len(turns))
	}
}

func TestGetTurnsNoTurnData(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Invocations without turn data should not appear.
	if err := s.RecordInvocation(ctx, model.Invocation{
		ID: "noturn", Source: "cc", ToolName: "Read", Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	turns, err := s.GetTurns(ctx, TurnOpts{MinLength: 1})
	if err != nil {
		t.Fatalf("GetTurns: %v", err)
	}
	if len(turns) != 0 {
		t.Errorf("expected 0 turns for data without turn_id, got %d", len(turns))
	}
}

func TestGetPathTurnStatsBasic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)

	// Turn 1: length 3, tools: Grep, Read, Edit
	invocations := []model.Invocation{
		{ID: "ts1-0", Source: "cc", ToolName: "Grep", Timestamp: base, TurnID: "s:0", TurnSequence: 0, TurnLength: 3},
		{ID: "ts1-1", Source: "cc", ToolName: "Read", Timestamp: base.Add(time.Second), TurnID: "s:0", TurnSequence: 1, TurnLength: 3},
		{ID: "ts1-2", Source: "cc", ToolName: "Edit", Timestamp: base.Add(2 * time.Second), TurnID: "s:0", TurnSequence: 2, TurnLength: 3},
		// Turn 2: length 7, tools: Grep, Read, Read, Read, Edit, Read, Edit
		{ID: "ts2-0", Source: "cc", ToolName: "Grep", Timestamp: base.Add(time.Minute), TurnID: "s:1", TurnSequence: 0, TurnLength: 7},
		{ID: "ts2-1", Source: "cc", ToolName: "Read", Timestamp: base.Add(time.Minute + time.Second), TurnID: "s:1", TurnSequence: 1, TurnLength: 7},
		{ID: "ts2-2", Source: "cc", ToolName: "Read", Timestamp: base.Add(time.Minute + 2*time.Second), TurnID: "s:1", TurnSequence: 2, TurnLength: 7},
		{ID: "ts2-3", Source: "cc", ToolName: "Read", Timestamp: base.Add(time.Minute + 3*time.Second), TurnID: "s:1", TurnSequence: 3, TurnLength: 7},
		{ID: "ts2-4", Source: "cc", ToolName: "Edit", Timestamp: base.Add(time.Minute + 4*time.Second), TurnID: "s:1", TurnSequence: 4, TurnLength: 7},
		{ID: "ts2-5", Source: "cc", ToolName: "Read", Timestamp: base.Add(time.Minute + 5*time.Second), TurnID: "s:1", TurnSequence: 5, TurnLength: 7},
		{ID: "ts2-6", Source: "cc", ToolName: "Edit", Timestamp: base.Add(time.Minute + 6*time.Second), TurnID: "s:1", TurnSequence: 6, TurnLength: 7},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation %s: %v", inv.ID, err)
		}
	}

	stats, err := s.GetPathTurnStats(ctx, 5, time.Time{})
	if err != nil {
		t.Fatalf("GetPathTurnStats: %v", err)
	}

	statsMap := make(map[string]ToolTurnStats)
	for _, s := range stats {
		statsMap[s.ToolName] = s
	}

	// Grep: appears in turn of length 3 and turn of length 7. Avg = (3+7)/2 = 5.0.
	// Long turn (>5): 1 out of 2 = 50%.
	grep := statsMap["Grep"]
	if grep.AvgTurnLen != 5.0 {
		t.Errorf("Grep AvgTurnLen: got %.1f, want 5.0", grep.AvgTurnLen)
	}
	if grep.LongTurnPct != 50.0 {
		t.Errorf("Grep LongTurnPct: got %.1f, want 50.0", grep.LongTurnPct)
	}

	// Read: 1 in turn 3, 4 in turn 7. Avg = (3 + 7*4) / 5 = 31/5 = 6.2.
	// Long turns: 4 out of 5 = 80%.
	read := statsMap["Read"]
	expectedAvg := (3.0 + 7.0*4) / 5.0
	if read.AvgTurnLen < expectedAvg-0.1 || read.AvgTurnLen > expectedAvg+0.1 {
		t.Errorf("Read AvgTurnLen: got %.1f, want %.1f", read.AvgTurnLen, expectedAvg)
	}
	if read.LongTurnPct != 80.0 {
		t.Errorf("Read LongTurnPct: got %.1f, want 80.0", read.LongTurnPct)
	}
}

func TestGetPathTurnStatsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	stats, err := s.GetPathTurnStats(ctx, 5, time.Time{})
	if err != nil {
		t.Fatalf("GetPathTurnStats: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected 0 stats, got %d", len(stats))
	}
}

func TestParseTurnID(t *testing.T) {
	tests := []struct {
		turnID    string
		sessionID string
		index     int
	}{
		{"sess1:3", "sess1", 3},
		{"abc-def-ghi:0", "abc-def-ghi", 0},
		{"simple:10", "simple", 10},
		{"no-colon", "no-colon", 0},
	}
	for _, tt := range tests {
		sid, idx := parseTurnID(tt.turnID)
		if sid != tt.sessionID {
			t.Errorf("parseTurnID(%q) sessionID: got %q, want %q", tt.turnID, sid, tt.sessionID)
		}
		if idx != tt.index {
			t.Errorf("parseTurnID(%q) index: got %d, want %d", tt.turnID, idx, tt.index)
		}
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
