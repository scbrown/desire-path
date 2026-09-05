package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

// The whole point of the deferred path is that these two hooks are the same
// mechanism split across two events, so it is tested as ONE handoff: a failure
// parks a correction, and the agent's NEXT PreToolUse call delivers it.
func TestCorrectionIsDeliveredOnTheNextPreToolUse(t *testing.T) {
	t.Setenv("DP_PAVE_PENDING_DIR", t.TempDir())
	const session = "sess-handoff"

	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "python", To: "python3", Tool: "Bash", Param: "command", Command: "python", MatchKind: "command",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	old := dbPath
	dbPath = db
	defer func() { dbPath = old }()

	// 1. A failed command parks a correction.
	failure := `{"session_id":"` + session + `","tool_name":"Bash","tool_input":{"command":"python -c 'print(1)'"},"error":"python: command not found"}`
	if err := runPaveCorrect(strings.NewReader(failure)); err != nil {
		t.Fatalf("pave-correct: %v", err)
	}

	// 2. The next PreToolUse call — ANY command, not a search — delivers it.
	pre := `{"session_id":"` + session + `","tool_use_id":"t1","tool_name":"Bash","tool_input":{"command":"ls -la"}}`
	out := capturePrefetch(t, pre)
	if out == "" {
		t.Fatal("the parked correction was not delivered on the next PreToolUse")
	}
	var got struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not hook JSON: %q", out)
	}
	if got.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("event name = %q", got.HookSpecificOutput.HookEventName)
	}
	if !strings.Contains(got.HookSpecificOutput.AdditionalContext, "python3") {
		t.Errorf("correction did not survive the handoff: %q", got.HookSpecificOutput.AdditionalContext)
	}
	// It names the command it is about, since the agent has moved on by now.
	if !strings.Contains(got.HookSpecificOutput.AdditionalContext, "python -c") {
		t.Errorf("correction did not name the failed command: %q", got.HookSpecificOutput.AdditionalContext)
	}

	// 3. Delivered once.
	if second := capturePrefetch(t, pre); second != "" {
		t.Fatalf("the correction was delivered twice: %q", second)
	}
}

// With nothing parked the hook must be silent — this runs before every Bash
// call on every pane, and noise there is how a channel gets ignored.
func TestPreToolUseIsSilentWithNothingParked(t *testing.T) {
	t.Setenv("DP_PAVE_PENDING_DIR", t.TempDir())
	t.Setenv("DP_SIGNPOST_PREFETCH", "0")
	if out := capturePrefetch(t, `{"session_id":"quiet","tool_use_id":"t","tool_name":"Bash","tool_input":{"command":"ls"}}`); out != "" {
		t.Fatalf("hook spoke with nothing to say: %q", out)
	}
}

func capturePrefetch(t *testing.T, payload string) string {
	t.Helper()
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = stdinR, outW
	defer func() { os.Stdin, os.Stdout = oldIn, oldOut }()

	go func() { _, _ = stdinW.WriteString(payload); stdinW.Close() }()
	if err := runSignpostPrefetch(nil, nil); err != nil {
		t.Fatalf("prefetch: %v", err)
	}
	outW.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := outR.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return strings.TrimSpace(sb.String())
}
