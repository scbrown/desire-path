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
