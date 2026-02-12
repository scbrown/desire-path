package analyze

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

// mockStore implements store.Store for surface testing. Only the methods
// used by SurfaceTurnPatternDesires need real implementations.
type mockStore struct {
	patterns []store.TurnPattern
	desires  []model.Desire
	recorded []model.Desire
}

func (m *mockStore) TurnPatternStats(_ context.Context, _ store.TurnOpts) ([]store.TurnPattern, error) {
	return m.patterns, nil
}

func (m *mockStore) ListDesires(_ context.Context, opts store.ListOpts) ([]model.Desire, error) {
	if opts.Category == "" {
		return m.desires, nil
	}
	var filtered []model.Desire
	for _, d := range m.desires {
		if d.Category == opts.Category {
			filtered = append(filtered, d)
		}
	}
	return filtered, nil
}

func (m *mockStore) RecordDesire(_ context.Context, d model.Desire) error {
	m.recorded = append(m.recorded, d)
	m.desires = append(m.desires, d)
	return nil
}

// Unused Store methods — satisfy the interface.
func (m *mockStore) GetPaths(context.Context, store.PathOpts) ([]model.Path, error)            { return nil, nil }
func (m *mockStore) SetAlias(context.Context, model.Alias) error                               { return nil }
func (m *mockStore) GetAlias(context.Context, string, string, string, string, string) (*model.Alias, error) {
	return nil, nil
}
func (m *mockStore) GetAliases(context.Context) ([]model.Alias, error)                            { return nil, nil }
func (m *mockStore) DeleteAlias(context.Context, string, string, string, string, string) (bool, error) {
	return false, nil
}
func (m *mockStore) GetRulesForTool(context.Context, string) ([]model.Alias, error)               { return nil, nil }
func (m *mockStore) Stats(context.Context) (store.Stats, error)                                   { return store.Stats{}, nil }
func (m *mockStore) InspectPath(context.Context, store.InspectOpts) (*store.InspectResult, error) { return nil, nil }
func (m *mockStore) RecordInvocation(context.Context, model.Invocation) error                     { return nil }
func (m *mockStore) ListInvocations(context.Context, store.InvocationOpts) ([]model.Invocation, error) {
	return nil, nil
}
func (m *mockStore) InvocationStats(context.Context) (store.InvocationStatsResult, error) {
	return store.InvocationStatsResult{}, nil
}
func (m *mockStore) ListTurns(context.Context, store.TurnOpts) ([]store.TurnRow, error)        { return nil, nil }
func (m *mockStore) ToolTurnStats(context.Context, store.TurnOpts) ([]store.ToolTurnStat, error) { return nil, nil }
func (m *mockStore) Close() error                                                                { return nil }

func TestSurfaceTurnPatternDesires_CreatesDesires(t *testing.T) {
	ms := &mockStore{
		patterns: []store.TurnPattern{
			{Pattern: "Grep → Read{3+} → Edit", Count: 12, AvgLength: 5.3, Sessions: 4},
			{Pattern: "Glob → Read{2+}", Count: 8, AvgLength: 4.8, Sessions: 3},
		},
	}

	created, err := SurfaceTurnPatternDesires(context.Background(), ms, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 desires, got %d", len(created))
	}

	// Check first desire.
	d := created[0]
	if d.ToolName != "Grep" {
		t.Errorf("expected ToolName=Grep, got %q", d.ToolName)
	}
	if d.Category != model.CategoryTurnPattern {
		t.Errorf("expected Category=%q, got %q", model.CategoryTurnPattern, d.Category)
	}
	if d.Source != "transcript-analysis" {
		t.Errorf("expected Source=transcript-analysis, got %q", d.Source)
	}
	if d.Error == "" {
		t.Error("expected non-empty Error")
	}

	// Verify metadata contains the pattern.
	var meta struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(d.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta.Pattern != "Grep → Read{3+} → Edit" {
		t.Errorf("expected pattern in metadata, got %q", meta.Pattern)
	}

	// Check second desire.
	d2 := created[1]
	if d2.ToolName != "Glob" {
		t.Errorf("expected ToolName=Glob, got %q", d2.ToolName)
	}
}

func TestSurfaceTurnPatternDesires_SkipsBelowSessionThreshold(t *testing.T) {
	ms := &mockStore{
		patterns: []store.TurnPattern{
			{Pattern: "Grep → Read{2+}", Count: 5, AvgLength: 3.5, Sessions: 2}, // only 2 sessions
		},
	}

	created, err := SurfaceTurnPatternDesires(context.Background(), ms, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("expected 0 desires (below session threshold), got %d", len(created))
	}
}

func TestSurfaceTurnPatternDesires_Idempotent(t *testing.T) {
	ms := &mockStore{
		patterns: []store.TurnPattern{
			{Pattern: "Grep → Read{3+} → Edit", Count: 12, AvgLength: 5.3, Sessions: 4},
		},
	}

	// First call creates the desire.
	created1, err := SurfaceTurnPatternDesires(context.Background(), ms, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created1) != 1 {
		t.Fatalf("expected 1 desire on first call, got %d", len(created1))
	}

	// Second call should skip (already exists in ms.desires via RecordDesire).
	created2, err := SurfaceTurnPatternDesires(context.Background(), ms, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created2) != 0 {
		t.Fatalf("expected 0 desires on second call (idempotent), got %d", len(created2))
	}

	// Total recorded should still be 1.
	if len(ms.recorded) != 1 {
		t.Fatalf("expected 1 total recorded desire, got %d", len(ms.recorded))
	}
}

func TestSurfaceTurnPatternDesires_SkipsExistingPattern(t *testing.T) {
	// Pre-populate with an existing turn-pattern desire.
	existingMeta, _ := json.Marshal(map[string]any{"pattern": "Grep → Read{3+} → Edit"})
	ms := &mockStore{
		patterns: []store.TurnPattern{
			{Pattern: "Grep → Read{3+} → Edit", Count: 12, AvgLength: 5.3, Sessions: 4},
			{Pattern: "Bash{2+} → Read", Count: 6, AvgLength: 3.5, Sessions: 5},
		},
		desires: []model.Desire{
			{
				ID:       "existing-1",
				Category: model.CategoryTurnPattern,
				Metadata: existingMeta,
			},
		},
	}

	created, err := SurfaceTurnPatternDesires(context.Background(), ms, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the new pattern should be created.
	if len(created) != 1 {
		t.Fatalf("expected 1 new desire, got %d", len(created))
	}
	if created[0].ToolName != "Bash" {
		t.Errorf("expected ToolName=Bash, got %q", created[0].ToolName)
	}
}

func TestFirstTool(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"Grep → Read{3+} → Edit", "Grep"},
		{"Glob → Read{2+}", "Glob"},
		{"Bash{2+} → Read", "Bash"},
		{"Read", "Read"},
		{"Read{5+}", "Read"},
	}
	for _, tt := range tests {
		got := firstTool(tt.pattern)
		if got != tt.want {
			t.Errorf("firstTool(%q) = %q, want %q", tt.pattern, got, tt.want)
		}
	}
}

func TestExtractPattern(t *testing.T) {
	tests := []struct {
		name     string
		metadata json.RawMessage
		want     string
	}{
		{
			name:     "valid metadata",
			metadata: json.RawMessage(`{"pattern":"Grep → Read{2+} → Edit"}`),
			want:     "Grep → Read{2+} → Edit",
		},
		{
			name:     "empty metadata",
			metadata: nil,
			want:     "",
		},
		{
			name:     "no pattern field",
			metadata: json.RawMessage(`{"foo":"bar"}`),
			want:     "",
		},
		{
			name:     "invalid json",
			metadata: json.RawMessage(`{broken`),
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPattern(tt.metadata)
			if got != tt.want {
				t.Errorf("extractPattern(%s) = %q, want %q", tt.metadata, got, tt.want)
			}
		})
	}
}

func TestSurfaceTurnPatternDesires_ErrorDescriptionFormat(t *testing.T) {
	ms := &mockStore{
		patterns: []store.TurnPattern{
			{Pattern: "Grep → Read{2+} → Edit", Count: 10, AvgLength: 5.0, Sessions: 3},
		},
	}

	created, err := SurfaceTurnPatternDesires(context.Background(), ms, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 desire, got %d", len(created))
	}

	want := "Repeated pattern: Grep → Read{2+} → Edit (avg 5.0 calls, seen 10 times across 3 sessions)"
	if created[0].Error != want {
		t.Errorf("error description mismatch:\n  got:  %q\n  want: %q", created[0].Error, want)
	}
}

// TestSurfaceTurnPatternDesires_SQLiteIntegration exercises the full flow with
// a real SQLite store: insert invocations forming recurring turn patterns, then
// verify SurfaceTurnPatternDesires creates the expected desire records.
func TestSurfaceTurnPatternDesires_SQLiteIntegration(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now()

	// Insert invocations forming the pattern "Grep → Read → Read → Read → Edit"
	// across 3 different sessions. Each session has 1 turn with 5 tools.
	sessions := []string{"session-a", "session-b", "session-c"}
	tools := []string{"Grep", "Read", "Read", "Read", "Edit"}

	for _, sess := range sessions {
		turnID := sess + ":0"
		for seq, tool := range tools {
			inv := model.Invocation{
				ID:           uuid.New().String(),
				Source:       "claude-code",
				InstanceID:   sess,
				ToolName:     tool,
				Timestamp:    now,
				TurnID:       turnID,
				TurnSequence: seq,
				TurnLength:   len(tools),
			}
			if err := s.RecordInvocation(ctx, inv); err != nil {
				t.Fatalf("record invocation: %v", err)
			}
		}
	}

	// Run surfacing with threshold=5 (matches our 5-tool turns).
	created, err := SurfaceTurnPatternDesires(ctx, s, 5)
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 desire, got %d", len(created))
	}

	d := created[0]
	if d.ToolName != "Grep" {
		t.Errorf("ToolName = %q, want Grep", d.ToolName)
	}
	if d.Category != model.CategoryTurnPattern {
		t.Errorf("Category = %q, want %q", d.Category, model.CategoryTurnPattern)
	}
	if d.Source != "transcript-analysis" {
		t.Errorf("Source = %q, want transcript-analysis", d.Source)
	}

	// Verify the desire was persisted.
	desires, err := s.ListDesires(ctx, store.ListOpts{Category: model.CategoryTurnPattern})
	if err != nil {
		t.Fatalf("list desires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("expected 1 persisted desire, got %d", len(desires))
	}

	// Run again — should be idempotent.
	created2, err := SurfaceTurnPatternDesires(ctx, s, 5)
	if err != nil {
		t.Fatalf("surface (second call): %v", err)
	}
	if len(created2) != 0 {
		t.Errorf("expected 0 new desires on second call, got %d", len(created2))
	}
}
