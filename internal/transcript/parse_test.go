package transcript

import (
	"os"
	"strings"
	"testing"
	"time"
)

func mustOpen(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestParseSingleTurn(t *testing.T) {
	f := mustOpen(t, "testdata/single_turn.jsonl")
	turns, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}

	turn := turns[0]
	if turn.SessionID != "sess-001" {
		t.Errorf("SessionID = %q, want %q", turn.SessionID, "sess-001")
	}
	if turn.Index != 0 {
		t.Errorf("Index = %d, want 0", turn.Index)
	}
	if turn.DurationMs != 5000 {
		t.Errorf("DurationMs = %d, want 5000", turn.DurationMs)
	}

	wantTime, _ := time.Parse(time.RFC3339, "2026-01-15T10:00:00Z")
	if !turn.StartedAt.Equal(wantTime) {
		t.Errorf("StartedAt = %v, want %v", turn.StartedAt, wantTime)
	}

	if len(turn.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(turn.Steps))
	}

	step := turn.Steps[0]
	if step.ToolName != "Read" {
		t.Errorf("ToolName = %q, want %q", step.ToolName, "Read")
	}
	if step.ToolUseID != "toolu_001" {
		t.Errorf("ToolUseID = %q, want %q", step.ToolUseID, "toolu_001")
	}
	if step.Sequence != 0 {
		t.Errorf("Sequence = %d, want 0", step.Sequence)
	}
	if step.IsParallel {
		t.Error("IsParallel should be false for single step")
	}
	if step.IsError {
		t.Error("IsError should be false")
	}
}

func TestParseMultiTurn(t *testing.T) {
	f := mustOpen(t, "testdata/multi_turn.jsonl")
	turns, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}

	// Turn 0: one Read step.
	t0 := turns[0]
	if t0.Index != 0 {
		t.Errorf("turn 0 Index = %d, want 0", t0.Index)
	}
	if t0.DurationMs != 4000 {
		t.Errorf("turn 0 DurationMs = %d, want 4000", t0.DurationMs)
	}
	if len(t0.Steps) != 1 {
		t.Fatalf("turn 0 got %d steps, want 1", len(t0.Steps))
	}
	if t0.Steps[0].ToolName != "Read" {
		t.Errorf("turn 0 step 0 ToolName = %q, want %q", t0.Steps[0].ToolName, "Read")
	}

	// Turn 1: Read then Edit.
	t1 := turns[1]
	if t1.Index != 1 {
		t.Errorf("turn 1 Index = %d, want 1", t1.Index)
	}
	if t1.DurationMs != 6000 {
		t.Errorf("turn 1 DurationMs = %d, want 6000", t1.DurationMs)
	}
	if len(t1.Steps) != 2 {
		t.Fatalf("turn 1 got %d steps, want 2", len(t1.Steps))
	}
	if t1.Steps[0].ToolName != "Read" {
		t.Errorf("turn 1 step 0 ToolName = %q, want %q", t1.Steps[0].ToolName, "Read")
	}
	if t1.Steps[0].Sequence != 0 {
		t.Errorf("turn 1 step 0 Sequence = %d, want 0", t1.Steps[0].Sequence)
	}
	if t1.Steps[1].ToolName != "Edit" {
		t.Errorf("turn 1 step 1 ToolName = %q, want %q", t1.Steps[1].ToolName, "Edit")
	}
	if t1.Steps[1].Sequence != 1 {
		t.Errorf("turn 1 step 1 Sequence = %d, want 1", t1.Steps[1].Sequence)
	}
}

func TestParseParallelTools(t *testing.T) {
	f := mustOpen(t, "testdata/parallel_tools.jsonl")
	turns, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}

	turn := turns[0]
	if len(turn.Steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(turn.Steps))
	}

	// Steps 0 and 1 share the same parent (a1) → parallel.
	if turn.Steps[0].ToolName != "Grep" {
		t.Errorf("step 0 ToolName = %q, want %q", turn.Steps[0].ToolName, "Grep")
	}
	if !turn.Steps[0].IsParallel {
		t.Error("step 0 should be parallel (shares parent with step 1)")
	}
	if turn.Steps[1].ToolName != "Glob" {
		t.Errorf("step 1 ToolName = %q, want %q", turn.Steps[1].ToolName, "Glob")
	}
	if !turn.Steps[1].IsParallel {
		t.Error("step 1 should be parallel (shares parent with step 0)")
	}

	// Step 2 has a different parent → not parallel.
	if turn.Steps[2].ToolName != "Read" {
		t.Errorf("step 2 ToolName = %q, want %q", turn.Steps[2].ToolName, "Read")
	}
	if turn.Steps[2].IsParallel {
		t.Error("step 2 should not be parallel")
	}
}

func TestParseErrorStep(t *testing.T) {
	f := mustOpen(t, "testdata/error_step.jsonl")
	turns, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}

	turn := turns[0]
	if len(turn.Steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(turn.Steps))
	}

	// First step should be an error.
	if !turn.Steps[0].IsError {
		t.Error("step 0 should be an error")
	}
	if !strings.Contains(turn.Steps[0].Error, "No rule to make target") {
		t.Errorf("step 0 Error = %q, want to contain 'No rule to make target'", turn.Steps[0].Error)
	}

	// Second step should succeed.
	if turn.Steps[1].IsError {
		t.Error("step 1 should not be an error")
	}
}

func TestParseNoSystemEvents(t *testing.T) {
	f := mustOpen(t, "testdata/no_system_events.jsonl")
	turns, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}

	turn := turns[0]
	if turn.DurationMs != 0 {
		t.Errorf("DurationMs = %d, want 0 (no turn_duration event)", turn.DurationMs)
	}
	if len(turn.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(turn.Steps))
	}
}

func TestParseEmptyInput(t *testing.T) {
	turns, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if turns != nil {
		t.Errorf("expected nil turns for empty input, got %d", len(turns))
	}
}

func TestParseMalformedJSON(t *testing.T) {
	_, err := Parse(strings.NewReader("not json\n"))
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("error %q should mention line 1", err.Error())
	}
}

func TestParseNonTranscriptEvents(t *testing.T) {
	// file-history-snapshot and progress events should be silently skipped.
	input := `{"type":"file-history-snapshot","uuid":"fh1","timestamp":"2026-01-15T10:00:00Z"}
{"type":"progress","uuid":"p1","timestamp":"2026-01-15T10:00:01Z"}
{"type":"user","uuid":"u1","parentUuid":null,"sessionId":"sess-006","timestamp":"2026-01-15T10:00:02Z","message":{"role":"user","content":"Hello"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"sess-006","timestamp":"2026-01-15T10:00:03Z","message":{"role":"assistant","content":[{"type":"text","text":"Hi"}]}}
`
	turns, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if len(turns[0].Steps) != 0 {
		t.Errorf("got %d steps, want 0 (no tool calls)", len(turns[0].Steps))
	}
}

func TestParseSessionIDFromFirstEvent(t *testing.T) {
	input := `{"type":"file-history-snapshot","uuid":"fh1","timestamp":"2026-01-15T10:00:00Z"}
{"type":"user","uuid":"u1","parentUuid":null,"sessionId":"my-session","timestamp":"2026-01-15T10:00:01Z","message":{"role":"user","content":"Hi"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"my-session","timestamp":"2026-01-15T10:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}]}}
`
	turns, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if turns[0].SessionID != "my-session" {
		t.Errorf("SessionID = %q, want %q", turns[0].SessionID, "my-session")
	}
}

func TestParseTurnWithOnlyTextNoTools(t *testing.T) {
	input := `{"type":"user","uuid":"u1","parentUuid":null,"sessionId":"sess-007","timestamp":"2026-01-15T10:00:00Z","message":{"role":"user","content":"What is Go?"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"sess-007","timestamp":"2026-01-15T10:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"Go is a programming language."}]}}
{"type":"system","uuid":"s1","parentUuid":"a1","sessionId":"sess-007","timestamp":"2026-01-15T10:00:02Z","subtype":"stop_hook_summary","hookCount":0,"hookErrors":[],"preventedContinuation":false}
{"type":"system","uuid":"s2","parentUuid":"s1","sessionId":"sess-007","timestamp":"2026-01-15T10:00:02Z","subtype":"turn_duration","durationMs":2000}
`
	turns, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if len(turns[0].Steps) != 0 {
		t.Errorf("got %d steps, want 0 (text-only turn)", len(turns[0].Steps))
	}
	if turns[0].DurationMs != 2000 {
		t.Errorf("DurationMs = %d, want 2000", turns[0].DurationMs)
	}
}

func TestParseRealisticTranscript(t *testing.T) {
	f := mustOpen(t, "testdata/realistic.jsonl")
	turns, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}

	turn := turns[0]
	if turn.SessionID != "real-sess-001" {
		t.Errorf("SessionID = %q, want %q", turn.SessionID, "real-sess-001")
	}
	if turn.DurationMs != 10000 {
		t.Errorf("DurationMs = %d, want 10000", turn.DurationMs)
	}

	// Should have 4 steps: Grep‖, Glob‖, Read, Edit.
	if len(turn.Steps) != 4 {
		t.Fatalf("got %d steps, want 4", len(turn.Steps))
	}

	wantTools := []string{"Grep", "Glob", "Read", "Edit"}
	for i, want := range wantTools {
		if turn.Steps[i].ToolName != want {
			t.Errorf("step %d ToolName = %q, want %q", i, turn.Steps[i].ToolName, want)
		}
	}

	// Grep and Glob should be parallel (same parent a0).
	if !turn.Steps[0].IsParallel {
		t.Error("step 0 (Grep) should be parallel")
	}
	if !turn.Steps[1].IsParallel {
		t.Error("step 1 (Glob) should be parallel")
	}
	// Read and Edit should not be parallel.
	if turn.Steps[2].IsParallel {
		t.Error("step 2 (Read) should not be parallel")
	}
	if turn.Steps[3].IsParallel {
		t.Error("step 3 (Edit) should not be parallel")
	}

	// No errors in this transcript.
	for i, s := range turn.Steps {
		if s.IsError {
			t.Errorf("step %d should not be an error", i)
		}
	}
}

func TestParseStepInput(t *testing.T) {
	f := mustOpen(t, "testdata/single_turn.jsonl")
	turns, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(turns) != 1 || len(turns[0].Steps) != 1 {
		t.Fatal("expected 1 turn with 1 step")
	}

	step := turns[0].Steps[0]
	if step.Input == nil {
		t.Fatal("Input should not be nil")
	}

	// Input should be valid JSON containing file_path.
	got := string(step.Input)
	if !strings.Contains(got, "file_path") {
		t.Errorf("Input %s should contain file_path", got)
	}
}
