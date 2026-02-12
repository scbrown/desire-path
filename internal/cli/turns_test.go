package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

func TestAbstractPattern(t *testing.T) {
	tests := []struct {
		tools []string
		want  string
	}{
		{nil, ""},
		{[]string{"Read"}, "Read"},
		{[]string{"Grep", "Read", "Edit"}, "Grep → Read → Edit"},
		{[]string{"Grep", "Read", "Read", "Read", "Edit"}, "Grep → Read{3+} → Edit"},
		{[]string{"Read", "Read"}, "Read{2+}"},
		{[]string{"Read", "Read", "Read", "Read", "Read"}, "Read{5+}"},
		{[]string{"Bash", "Bash", "Read", "Bash", "Read"}, "Bash{2+} → Read → Bash → Read"},
	}
	for _, tt := range tests {
		got := abstractPattern(tt.tools)
		if got != tt.want {
			t.Errorf("abstractPattern(%v) = %q, want %q", tt.tools, got, tt.want)
		}
	}
}

func TestPatternKey(t *testing.T) {
	// patternKey normalizes all consecutive runs to {2+}.
	tests := []struct {
		tools []string
		want  string
	}{
		{[]string{"Grep", "Read", "Read", "Read", "Edit"}, "Grep → Read{2+} → Edit"},
		{[]string{"Grep", "Read", "Read", "Read", "Read", "Read", "Edit"}, "Grep → Read{2+} → Edit"},
		{[]string{"Read", "Read"}, "Read{2+}"},
		{[]string{"Read", "Read", "Read", "Read", "Read"}, "Read{2+}"},
		{[]string{"Grep", "Read", "Edit"}, "Grep → Read → Edit"},
	}
	for _, tt := range tests {
		got := patternKey(tt.tools)
		if got != tt.want {
			t.Errorf("patternKey(%v) = %q, want %q", tt.tools, got, tt.want)
		}
	}
}

func TestWriteTurnsTable(t *testing.T) {
	turns := []store.TurnRow{
		{
			TurnID:    "abc123:3",
			SessionID: "abc123",
			TurnIndex: 3,
			Length:    7,
			Tools:     []string{"Grep", "Read", "Read", "Read", "Edit", "Read", "Edit"},
			Timestamp: time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC),
		},
		{
			TurnID:    "abc123:5",
			SessionID: "abc123",
			TurnIndex: 5,
			Length:    5,
			Tools:     []string{"Bash", "Bash", "Read", "Bash", "Read"},
			Timestamp: time.Date(2026, 2, 10, 12, 5, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	writeTurnsTable(&buf, turns)
	out := buf.String()

	if !strings.Contains(out, "SESSION") {
		t.Error("missing SESSION header")
	}
	if !strings.Contains(out, "TURN") {
		t.Error("missing TURN header")
	}
	if !strings.Contains(out, "LENGTH") {
		t.Error("missing LENGTH header")
	}
	if !strings.Contains(out, "TOOLS") {
		t.Error("missing TOOLS header")
	}
	if !strings.Contains(out, "abc123") {
		t.Error("missing session abc123")
	}
	if !strings.Contains(out, "Grep → Read → Read → Read → Edit → Read → Edit") {
		t.Error("missing full tool sequence")
	}
}

func TestWriteTurnsTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	writeTurnsTable(&buf, nil)
	out := buf.String()

	if !strings.Contains(out, "SESSION") {
		t.Error("missing header for empty table")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (header only), got %d", len(lines))
	}
}

func TestWriteTurnsJSON(t *testing.T) {
	turns := []store.TurnRow{
		{
			TurnID:    "sess:0",
			SessionID: "sess",
			TurnIndex: 0,
			Length:    3,
			Tools:     []string{"Grep", "Read", "Edit"},
			Timestamp: time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	if err := writeTurnsJSON(&buf, turns); err != nil {
		t.Fatalf("writeTurnsJSON: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `"turn_id": "sess:0"`) {
		t.Error("missing turn_id in JSON")
	}
	if !strings.Contains(out, `"length": 3`) {
		t.Error("missing length in JSON")
	}
}

func TestWriteTurnsJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := writeTurnsJSON(&buf, nil); err != nil {
		t.Fatalf("writeTurnsJSON: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "[]" {
		t.Errorf("expected [] for nil turns, got %q", out)
	}
}

func TestOutputPatterns(t *testing.T) {
	turns := []store.TurnRow{
		{TurnID: "s1:0", SessionID: "s1", Length: 5, Tools: []string{"Grep", "Read", "Read", "Read", "Edit"}},
		{TurnID: "s1:1", SessionID: "s1", Length: 7, Tools: []string{"Grep", "Read", "Read", "Read", "Read", "Read", "Edit"}},
		{TurnID: "s2:0", SessionID: "s2", Length: 5, Tools: []string{"Grep", "Read", "Read", "Read", "Edit"}},
		{TurnID: "s1:2", SessionID: "s1", Length: 3, Tools: []string{"Bash", "Bash", "Read"}},
	}

	var buf bytes.Buffer
	if err := outputPatterns(&buf, turns); err != nil {
		t.Fatalf("outputPatterns: %v", err)
	}
	out := buf.String()

	// "Grep → Read{2+} → Edit" should appear (3 turns match via patternKey).
	if !strings.Contains(out, "Grep") {
		t.Error("missing Grep pattern")
	}
	if !strings.Contains(out, "PATTERN") {
		t.Error("missing PATTERN header")
	}
	if !strings.Contains(out, "COUNT") {
		t.Error("missing COUNT header")
	}
	if !strings.Contains(out, "3") {
		t.Error("expected count of 3 for the Grep pattern cluster")
	}
}

func TestOutputPatternDrilldown(t *testing.T) {
	turns := []store.TurnRow{
		{TurnID: "s1:0", SessionID: "s1", Length: 5, Tools: []string{"Grep", "Read", "Read", "Read", "Edit"}},
		{TurnID: "s1:1", SessionID: "s1", Length: 3, Tools: []string{"Bash", "Bash", "Read"}},
		{TurnID: "s2:0", SessionID: "s2", Length: 7, Tools: []string{"Grep", "Read", "Read", "Read", "Read", "Read", "Edit"}},
	}

	var buf bytes.Buffer
	if err := outputPatternDrilldown(&buf, turns, "Grep → Read{2+} → Edit"); err != nil {
		t.Fatalf("outputPatternDrilldown: %v", err)
	}
	out := buf.String()

	// Should include both Grep → Read{N} → Edit turns.
	if !strings.Contains(out, "s1") {
		t.Error("missing session s1")
	}
	if !strings.Contains(out, "s2") {
		t.Error("missing session s2")
	}
	// Should NOT include the Bash turn.
	if strings.Contains(out, "Bash") {
		t.Error("should not include Bash turn")
	}
}

func TestWritePathsTurnsTable(t *testing.T) {
	paths := []model.Path{
		{
			ID:        "Grep",
			Pattern:   "Grep",
			Count:     42,
			FirstSeen: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
			LastSeen:  time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC),
		},
		{
			ID:        "Read",
			Pattern:   "Read",
			Count:     100,
			FirstSeen: time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC),
			LastSeen:  time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC),
		},
	}
	turnStats := []store.ToolTurnStats{
		{ToolName: "Grep", AvgTurnLen: 4.2, LongTurnPct: 35.0},
		{ToolName: "Read", AvgTurnLen: 3.1, LongTurnPct: 18.0},
	}

	var buf bytes.Buffer
	writePathsTurnsTable(&buf, paths, turnStats)
	out := buf.String()

	if !strings.Contains(out, "AVG_TURN_LEN") {
		t.Error("missing AVG_TURN_LEN header")
	}
	if !strings.Contains(out, "LONG_TURN_%") {
		t.Error("missing LONG_TURN_% header")
	}
	if !strings.Contains(out, "4.2") {
		t.Error("missing avg turn len 4.2")
	}
	if !strings.Contains(out, "35%") {
		t.Error("missing long turn pct 35%")
	}
	if !strings.Contains(out, "18%") {
		t.Error("missing long turn pct 18%")
	}
}

func TestWritePathsTurnsTableNoStats(t *testing.T) {
	paths := []model.Path{
		{ID: "Grep", Pattern: "Grep", Count: 5},
	}

	var buf bytes.Buffer
	writePathsTurnsTable(&buf, paths, nil)
	out := buf.String()

	// Should show "-" for missing stats.
	if !strings.Contains(out, "-") {
		t.Error("expected '-' for tools without turn stats")
	}
}

func TestSessionIDTruncation(t *testing.T) {
	turns := []store.TurnRow{
		{
			TurnID:    "very-long-session-id-12345:0",
			SessionID: "very-long-session-id-12345",
			TurnIndex: 0,
			Length:    3,
			Tools:     []string{"Read", "Read", "Read"},
		},
	}

	var buf bytes.Buffer
	writeTurnsTable(&buf, turns)
	out := buf.String()

	// Session ID should be truncated to 8 chars.
	if !strings.Contains(out, "very-lon") {
		t.Error("session ID not truncated to 8 chars")
	}
}
