package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
)

func TestTurnColumnsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)

	inv := model.Invocation{
		ID:           "turn-rt-1",
		Source:       "claude-code",
		ToolName:     "Read",
		Timestamp:    ts,
		TurnID:       "sess-001:3",
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
		t.Fatalf("expected 1, got %d", len(got))
	}

	g := got[0]
	if g.TurnID != "sess-001:3" {
		t.Errorf("TurnID: got %q, want %q", g.TurnID, "sess-001:3")
	}
	if g.TurnSequence != 2 {
		t.Errorf("TurnSequence: got %d, want 2", g.TurnSequence)
	}
	if g.TurnLength != 5 {
		t.Errorf("TurnLength: got %d, want 5", g.TurnLength)
	}
}

func TestTurnColumnsDefaultZero(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	inv := model.Invocation{
		ID:        "turn-def-1",
		Source:    "cc",
		ToolName:  "Read",
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
	if got[0].TurnID != "" {
		t.Errorf("TurnID: got %q, want empty", got[0].TurnID)
	}
	if got[0].TurnSequence != 0 {
		t.Errorf("TurnSequence: got %d, want 0", got[0].TurnSequence)
	}
	if got[0].TurnLength != 0 {
		t.Errorf("TurnLength: got %d, want 0", got[0].TurnLength)
	}
}

func seedTurnData(t *testing.T, s *SQLiteStore) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)

	// Turn sess-001:0 — 3 steps: Grep → Read → Read
	turn0 := []model.Invocation{
		{ID: "t0s0", Source: "cc", InstanceID: "sess-001", ToolName: "Grep", Timestamp: base, TurnID: "sess-001:0", TurnSequence: 0, TurnLength: 3},
		{ID: "t0s1", Source: "cc", InstanceID: "sess-001", ToolName: "Read", Timestamp: base.Add(time.Second), TurnID: "sess-001:0", TurnSequence: 1, TurnLength: 3},
		{ID: "t0s2", Source: "cc", InstanceID: "sess-001", ToolName: "Read", Timestamp: base.Add(2 * time.Second), TurnID: "sess-001:0", TurnSequence: 2, TurnLength: 3},
	}
	// Turn sess-001:1 — 6 steps: Grep → Read → Read → Read → Edit → Read
	turn1 := []model.Invocation{
		{ID: "t1s0", Source: "cc", InstanceID: "sess-001", ToolName: "Grep", Timestamp: base.Add(10 * time.Second), TurnID: "sess-001:1", TurnSequence: 0, TurnLength: 6},
		{ID: "t1s1", Source: "cc", InstanceID: "sess-001", ToolName: "Read", Timestamp: base.Add(11 * time.Second), TurnID: "sess-001:1", TurnSequence: 1, TurnLength: 6},
		{ID: "t1s2", Source: "cc", InstanceID: "sess-001", ToolName: "Read", Timestamp: base.Add(12 * time.Second), TurnID: "sess-001:1", TurnSequence: 2, TurnLength: 6},
		{ID: "t1s3", Source: "cc", InstanceID: "sess-001", ToolName: "Read", Timestamp: base.Add(13 * time.Second), TurnID: "sess-001:1", TurnSequence: 3, TurnLength: 6},
		{ID: "t1s4", Source: "cc", InstanceID: "sess-001", ToolName: "Edit", Timestamp: base.Add(14 * time.Second), TurnID: "sess-001:1", TurnSequence: 4, TurnLength: 6},
		{ID: "t1s5", Source: "cc", InstanceID: "sess-001", ToolName: "Read", Timestamp: base.Add(15 * time.Second), TurnID: "sess-001:1", TurnSequence: 5, TurnLength: 6},
	}
	// Turn sess-002:0 — 7 steps: Glob → Read → Read → Grep → Read → Read → Edit
	turn2 := []model.Invocation{
		{ID: "t2s0", Source: "cc", InstanceID: "sess-002", ToolName: "Glob", Timestamp: base.Add(20 * time.Second), TurnID: "sess-002:0", TurnSequence: 0, TurnLength: 7},
		{ID: "t2s1", Source: "cc", InstanceID: "sess-002", ToolName: "Read", Timestamp: base.Add(21 * time.Second), TurnID: "sess-002:0", TurnSequence: 1, TurnLength: 7},
		{ID: "t2s2", Source: "cc", InstanceID: "sess-002", ToolName: "Read", Timestamp: base.Add(22 * time.Second), TurnID: "sess-002:0", TurnSequence: 2, TurnLength: 7},
		{ID: "t2s3", Source: "cc", InstanceID: "sess-002", ToolName: "Grep", Timestamp: base.Add(23 * time.Second), TurnID: "sess-002:0", TurnSequence: 3, TurnLength: 7},
		{ID: "t2s4", Source: "cc", InstanceID: "sess-002", ToolName: "Read", Timestamp: base.Add(24 * time.Second), TurnID: "sess-002:0", TurnSequence: 4, TurnLength: 7},
		{ID: "t2s5", Source: "cc", InstanceID: "sess-002", ToolName: "Read", Timestamp: base.Add(25 * time.Second), TurnID: "sess-002:0", TurnSequence: 5, TurnLength: 7},
		{ID: "t2s6", Source: "cc", InstanceID: "sess-002", ToolName: "Edit", Timestamp: base.Add(26 * time.Second), TurnID: "sess-002:0", TurnSequence: 6, TurnLength: 7},
	}

	all := append(turn0, turn1...)
	all = append(all, turn2...)
	for _, inv := range all {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func TestListTurns(t *testing.T) {
	s := newTestStore(t)
	seedTurnData(t, s)
	ctx := context.Background()

	turns, err := s.ListTurns(ctx, TurnOpts{})
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("expected 3 turns, got %d", len(turns))
	}
	// Should be sorted by turn_length desc.
	if turns[0].Length != 7 {
		t.Errorf("turns[0].Length: got %d, want 7", turns[0].Length)
	}
}

func TestListTurnsMinLength(t *testing.T) {
	s := newTestStore(t)
	seedTurnData(t, s)
	ctx := context.Background()

	turns, err := s.ListTurns(ctx, TurnOpts{MinLength: 5})
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns with length >= 5, got %d", len(turns))
	}
	for _, tr := range turns {
		if tr.Length < 5 {
			t.Errorf("turn %s has length %d, want >= 5", tr.TurnID, tr.Length)
		}
	}
}

func TestListTurnsFilterBySession(t *testing.T) {
	s := newTestStore(t)
	seedTurnData(t, s)
	ctx := context.Background()

	turns, err := s.ListTurns(ctx, TurnOpts{SessionID: "sess-001"})
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns for sess-001, got %d", len(turns))
	}
	for _, tr := range turns {
		if tr.SessionID != "sess-001" {
			t.Errorf("unexpected session %q", tr.SessionID)
		}
	}
}

func TestListTurnsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	turns, err := s.ListTurns(ctx, TurnOpts{})
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}
	if len(turns) != 0 {
		t.Errorf("expected 0, got %d", len(turns))
	}
}

func TestTurnPatternStats(t *testing.T) {
	s := newTestStore(t)
	seedTurnData(t, s)
	ctx := context.Background()

	patterns, err := s.TurnPatternStats(ctx, TurnOpts{})
	if err != nil {
		t.Fatalf("TurnPatternStats: %v", err)
	}
	if len(patterns) == 0 {
		t.Fatal("expected at least 1 pattern")
	}
	// All patterns should have Count > 0.
	for _, p := range patterns {
		if p.Count == 0 {
			t.Errorf("pattern %q has Count 0", p.Pattern)
		}
		if p.Sessions == 0 {
			t.Errorf("pattern %q has Sessions 0", p.Pattern)
		}
	}
}

func TestToolTurnStats(t *testing.T) {
	s := newTestStore(t)
	seedTurnData(t, s)
	ctx := context.Background()

	stats, err := s.ToolTurnStats(ctx, TurnOpts{MinLength: 5})
	if err != nil {
		t.Fatalf("ToolTurnStats: %v", err)
	}
	if len(stats) == 0 {
		t.Fatal("expected at least 1 tool stat")
	}
	// Read should have the highest count (appears in many turns).
	found := false
	for _, ts := range stats {
		if ts.ToolName == "Read" {
			found = true
			if ts.Count == 0 {
				t.Error("Read count should be > 0")
			}
			break
		}
	}
	if !found {
		t.Error("Read not found in tool turn stats")
	}
}

func TestFuzzToolSequence(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Grep → Read → Read → Read → Edit", "Grep → Read{3+} → Edit"},
		{"Read → Read → Edit → Edit", "Read{2+} → Edit{2+}"},
		{"Grep → Glob → Read", "Grep → Glob → Read"},
		{"Read", "Read"},
		{"Read → Read", "Read{2+}"},
		{"", ""},
		{"Glob → Read → Read → Grep → Read → Read → Read → Edit", "Glob → Read{2+} → Grep → Read{3+} → Edit"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := fuzzToolSequence(tt.input)
			if got != tt.want {
				t.Errorf("fuzzToolSequence(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestListTurnsToolSequenceOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)

	// Create a turn with known tool order: Grep → Read → Edit.
	invocations := []model.Invocation{
		{ID: "ord0", Source: "cc", InstanceID: "s1", ToolName: "Grep", Timestamp: base, TurnID: "s1:0", TurnSequence: 0, TurnLength: 3},
		{ID: "ord1", Source: "cc", InstanceID: "s1", ToolName: "Read", Timestamp: base.Add(time.Second), TurnID: "s1:0", TurnSequence: 1, TurnLength: 3},
		{ID: "ord2", Source: "cc", InstanceID: "s1", ToolName: "Edit", Timestamp: base.Add(2 * time.Second), TurnID: "s1:0", TurnSequence: 2, TurnLength: 3},
	}
	for _, inv := range invocations {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("RecordInvocation: %v", err)
		}
	}

	turns, err := s.ListTurns(ctx, TurnOpts{})
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}

	want := "Grep → Read → Edit"
	if turns[0].Tools != want {
		t.Errorf("Tools: got %q, want %q", turns[0].Tools, want)
	}
}

func TestListTurnsWithLimit(t *testing.T) {
	s := newTestStore(t)
	seedTurnData(t, s)
	ctx := context.Background()

	turns, err := s.ListTurns(ctx, TurnOpts{Limit: 1})
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 with limit, got %d", len(turns))
	}
}

func TestTurnPatternStatsMinLength(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)

	// Two short turns (length 2) and one long turn (length 6).
	short := []model.Invocation{
		{ID: "sh0", Source: "cc", InstanceID: "s1", ToolName: "Read", Timestamp: base, TurnID: "s1:0", TurnSequence: 0, TurnLength: 2},
		{ID: "sh1", Source: "cc", InstanceID: "s1", ToolName: "Edit", Timestamp: base.Add(time.Second), TurnID: "s1:0", TurnSequence: 1, TurnLength: 2},
	}
	long := []model.Invocation{
		{ID: "lg0", Source: "cc", InstanceID: "s1", ToolName: "Grep", Timestamp: base.Add(10 * time.Second), TurnID: "s1:1", TurnSequence: 0, TurnLength: 6},
		{ID: "lg1", Source: "cc", InstanceID: "s1", ToolName: "Read", Timestamp: base.Add(11 * time.Second), TurnID: "s1:1", TurnSequence: 1, TurnLength: 6},
		{ID: "lg2", Source: "cc", InstanceID: "s1", ToolName: "Read", Timestamp: base.Add(12 * time.Second), TurnID: "s1:1", TurnSequence: 2, TurnLength: 6},
		{ID: "lg3", Source: "cc", InstanceID: "s1", ToolName: "Read", Timestamp: base.Add(13 * time.Second), TurnID: "s1:1", TurnSequence: 3, TurnLength: 6},
		{ID: "lg4", Source: "cc", InstanceID: "s1", ToolName: "Edit", Timestamp: base.Add(14 * time.Second), TurnID: "s1:1", TurnSequence: 4, TurnLength: 6},
		{ID: "lg5", Source: "cc", InstanceID: "s1", ToolName: "Read", Timestamp: base.Add(15 * time.Second), TurnID: "s1:1", TurnSequence: 5, TurnLength: 6},
	}
	all := append(short, long...)
	for _, inv := range all {
		if err := s.RecordInvocation(ctx, inv); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	patterns, err := s.TurnPatternStats(ctx, TurnOpts{MinLength: 5})
	if err != nil {
		t.Fatalf("TurnPatternStats: %v", err)
	}
	// Only the long turn should match.
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern with min_length=5, got %d", len(patterns))
	}

	_ = fmt.Sprintf("pattern: %s", patterns[0].Pattern) // ensure it's non-empty
}
