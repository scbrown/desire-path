package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDesireRoundTrip(t *testing.T) {
	ts := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		d    Desire
	}{
		{
			name: "full desire",
			d: Desire{
				ID:        "abc-123",
				ToolName:  "read_file",
				ToolInput: json.RawMessage(`{"path":"/etc/hosts"}`),
				Error:     "unknown tool",
				Source:    "claude-code",
				SessionID: "sess-1",
				CWD:       "/home/user/project",
				Timestamp: ts,
				Metadata:  json.RawMessage(`{"model":"opus"}`),
			},
		},
		{
			name: "minimal desire",
			d: Desire{
				ID:        "def-456",
				ToolName:  "mcp__memory__search",
				Error:     "tool not found",
				Timestamp: ts,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.d)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got Desire
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.ID != tt.d.ID {
				t.Errorf("ID: got %q, want %q", got.ID, tt.d.ID)
			}
			if got.ToolName != tt.d.ToolName {
				t.Errorf("ToolName: got %q, want %q", got.ToolName, tt.d.ToolName)
			}
			if got.Error != tt.d.Error {
				t.Errorf("Error: got %q, want %q", got.Error, tt.d.Error)
			}
			if got.Source != tt.d.Source {
				t.Errorf("Source: got %q, want %q", got.Source, tt.d.Source)
			}
			if got.SessionID != tt.d.SessionID {
				t.Errorf("SessionID: got %q, want %q", got.SessionID, tt.d.SessionID)
			}
			if got.CWD != tt.d.CWD {
				t.Errorf("CWD: got %q, want %q", got.CWD, tt.d.CWD)
			}
			if !got.Timestamp.Equal(tt.d.Timestamp) {
				t.Errorf("Timestamp: got %v, want %v", got.Timestamp, tt.d.Timestamp)
			}
			if string(got.ToolInput) != string(tt.d.ToolInput) {
				t.Errorf("ToolInput: got %s, want %s", got.ToolInput, tt.d.ToolInput)
			}
			if string(got.Metadata) != string(tt.d.Metadata) {
				t.Errorf("Metadata: got %s, want %s", got.Metadata, tt.d.Metadata)
			}
		})
	}
}

func TestPathRoundTrip(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := Path{
		ID:        "path-1",
		Pattern:   "read_file",
		Count:     42,
		FirstSeen: ts,
		LastSeen:  ts.Add(24 * time.Hour),
		AliasTo:   "Read",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Path
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("ID: got %q, want %q", got.ID, p.ID)
	}
	if got.Pattern != p.Pattern {
		t.Errorf("Pattern: got %q, want %q", got.Pattern, p.Pattern)
	}
	if got.Count != p.Count {
		t.Errorf("Count: got %d, want %d", got.Count, p.Count)
	}
	if !got.FirstSeen.Equal(p.FirstSeen) {
		t.Errorf("FirstSeen: got %v, want %v", got.FirstSeen, p.FirstSeen)
	}
	if !got.LastSeen.Equal(p.LastSeen) {
		t.Errorf("LastSeen: got %v, want %v", got.LastSeen, p.LastSeen)
	}
	if got.AliasTo != p.AliasTo {
		t.Errorf("AliasTo: got %q, want %q", got.AliasTo, p.AliasTo)
	}
}

func TestAliasRoundTrip(t *testing.T) {
	a := Alias{
		From:      "read_file",
		To:        "Read",
		CreatedAt: time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Alias
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.From != a.From {
		t.Errorf("From: got %q, want %q", got.From, a.From)
	}
	if got.To != a.To {
		t.Errorf("To: got %q, want %q", got.To, a.To)
	}
	if !got.CreatedAt.Equal(a.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, a.CreatedAt)
	}
}

func TestDesireOmitsEmptyFields(t *testing.T) {
	d := Desire{
		ID:        "test-1",
		ToolName:  "foo",
		Error:     "bar",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	for _, key := range []string{"tool_input", "source", "session_id", "cwd", "metadata"} {
		if _, ok := m[key]; ok {
			t.Errorf("expected %q to be omitted for zero value", key)
		}
	}
}

func TestPathOmitsEmptyAliasTo(t *testing.T) {
	p := Path{
		ID:        "path-1",
		Pattern:   "foo",
		Count:     1,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := m["alias_to"]; ok {
		t.Error("expected alias_to to be omitted when empty")
	}
}

func TestDesireFromExternalJSON(t *testing.T) {
	// Simulate receiving JSON from an external source (e.g., Claude Code hook).
	raw := `{
		"id": "ext-1",
		"tool_name": "mcp__memory__search",
		"tool_input": {"query": "test"},
		"error": "tool not available",
		"source": "claude-code",
		"session_id": "sess-ext",
		"cwd": "/home/user/project",
		"timestamp": "2026-02-07T12:00:00Z",
		"metadata": {"hook": "PostToolUseFailure"}
	}`
	var d Desire
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.ID != "ext-1" {
		t.Errorf("ID = %q, want %q", d.ID, "ext-1")
	}
	if d.ToolName != "mcp__memory__search" {
		t.Errorf("ToolName = %q, want %q", d.ToolName, "mcp__memory__search")
	}

	// Re-marshal and verify round-trip.
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var d2 Desire
	if err := json.Unmarshal(data, &d2); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}
	if d2.ToolName != d.ToolName {
		t.Errorf("round-trip ToolName = %q, want %q", d2.ToolName, d.ToolName)
	}
	if !d2.Timestamp.Equal(d.Timestamp) {
		t.Errorf("round-trip Timestamp = %v, want %v", d2.Timestamp, d.Timestamp)
	}
}

func TestDesireNullToolInput(t *testing.T) {
	// ToolInput and Metadata are json.RawMessage; verify null handling.
	raw := `{"id":"n-1","tool_name":"foo","error":"e","timestamp":"2026-01-01T00:00:00Z","tool_input":null,"metadata":null}`
	var d Desire
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// null unmarshals to nil for json.RawMessage.
	if d.ToolInput != nil && string(d.ToolInput) != "null" {
		t.Errorf("ToolInput = %s, want nil or null", d.ToolInput)
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var d2 Desire
	if err := json.Unmarshal(data, &d2); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}
	if d2.ToolName != "foo" {
		t.Errorf("ToolName = %q, want %q", d2.ToolName, "foo")
	}
}

func TestDesireWithComplexToolInput(t *testing.T) {
	// Verify nested JSON in ToolInput survives round-trip.
	input := `{"command":"ls -la","args":["--color","auto"],"nested":{"key":"value"}}`
	d := Desire{
		ID:        "complex-1",
		ToolName:  "Bash",
		ToolInput: json.RawMessage(input),
		Error:     "failed",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Desire
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Parse both to verify structural equality.
	var origInput, gotInput map[string]interface{}
	if err := json.Unmarshal([]byte(input), &origInput); err != nil {
		t.Fatalf("parse original input: %v", err)
	}
	if err := json.Unmarshal(got.ToolInput, &gotInput); err != nil {
		t.Fatalf("parse got input: %v", err)
	}
	if origInput["command"] != gotInput["command"] {
		t.Errorf("command = %v, want %v", gotInput["command"], origInput["command"])
	}
}

func TestPathZeroCount(t *testing.T) {
	p := Path{
		ID:        "path-z",
		Pattern:   "zero_tool",
		Count:     0,
		FirstSeen: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastSeen:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Path
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Count != 0 {
		t.Errorf("Count = %d, want 0", got.Count)
	}
}

func TestAliasEmptyTo(t *testing.T) {
	// An alias with empty To field should still round-trip.
	a := Alias{
		From:      "some_tool",
		To:        "",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Alias
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.From != a.From {
		t.Errorf("From = %q, want %q", got.From, a.From)
	}
	if got.To != "" {
		t.Errorf("To = %q, want empty", got.To)
	}
}
