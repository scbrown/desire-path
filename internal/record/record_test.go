package record

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

// fakeStore records calls to RecordDesire for test inspection.
type fakeStore struct {
	recorded []model.Desire
	err      error
}

func (f *fakeStore) RecordDesire(_ context.Context, d model.Desire) error {
	if f.err != nil {
		return f.err
	}
	f.recorded = append(f.recorded, d)
	return nil
}

func (f *fakeStore) ListDesires(context.Context, store.ListOpts) ([]model.Desire, error) {
	return nil, nil
}
func (f *fakeStore) GetPaths(context.Context, store.PathOpts) ([]model.Path, error) {
	return nil, nil
}
func (f *fakeStore) SetAlias(context.Context, string, string) error   { return nil }
func (f *fakeStore) GetAliases(context.Context) ([]model.Alias, error) { return nil, nil }
func (f *fakeStore) DeleteAlias(context.Context, string) (bool, error) { return false, nil }
func (f *fakeStore) Stats(context.Context) (store.Stats, error)        { return store.Stats{}, nil }
func (f *fakeStore) InspectPath(context.Context, store.InspectOpts) (*store.InspectResult, error) {
	return &store.InspectResult{}, nil
}
func (f *fakeStore) RecordInvocation(context.Context, model.Invocation) error {
	return nil
}
func (f *fakeStore) ListInvocations(context.Context, store.InvocationOpts) ([]model.Invocation, error) {
	return nil, nil
}
func (f *fakeStore) InvocationStats(context.Context) (store.InvocationStatsResult, error) {
	return store.InvocationStatsResult{}, nil
}
func (f *fakeStore) Close() error { return nil }

func TestRecord(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		source  string
		wantErr string
		check   func(t *testing.T, d model.Desire)
	}{
		{
			name:  "minimal input with only tool_name",
			input: `{"tool_name":"read_file"}`,
			check: func(t *testing.T, d model.Desire) {
				if d.ToolName != "read_file" {
					t.Errorf("ToolName = %q, want %q", d.ToolName, "read_file")
				}
				if d.ID == "" {
					t.Error("ID should be auto-generated")
				}
				if d.Timestamp.IsZero() {
					t.Error("Timestamp should be auto-generated")
				}
			},
		},
		{
			name:    "missing tool_name",
			input:   `{"error":"something failed"}`,
			wantErr: "missing required field: tool_name",
		},
		{
			name:    "empty tool_name",
			input:   `{"tool_name":""}`,
			wantErr: "missing required field: tool_name",
		},
		{
			name:    "invalid JSON",
			input:   `not json`,
			wantErr: "parsing JSON",
		},
		{
			name:  "full input with all known fields",
			input: `{"id":"abc-123","tool_name":"Bash","tool_input":{"command":"ls"},"error":"failed","source":"cursor","session_id":"sess-1","cwd":"/tmp","timestamp":"2025-01-15T10:30:00Z"}`,
			check: func(t *testing.T, d model.Desire) {
				if d.ID != "abc-123" {
					t.Errorf("ID = %q, want %q", d.ID, "abc-123")
				}
				if d.ToolName != "Bash" {
					t.Errorf("ToolName = %q, want %q", d.ToolName, "Bash")
				}
				if d.Error != "failed" {
					t.Errorf("Error = %q, want %q", d.Error, "failed")
				}
				if d.Source != "cursor" {
					t.Errorf("Source = %q, want %q", d.Source, "cursor")
				}
				if d.SessionID != "sess-1" {
					t.Errorf("SessionID = %q, want %q", d.SessionID, "sess-1")
				}
				if d.CWD != "/tmp" {
					t.Errorf("CWD = %q, want %q", d.CWD, "/tmp")
				}
				want := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
				if !d.Timestamp.Equal(want) {
					t.Errorf("Timestamp = %v, want %v", d.Timestamp, want)
				}
				// tool_input should be preserved as raw JSON.
				var ti map[string]string
				if err := json.Unmarshal(d.ToolInput, &ti); err != nil {
					t.Fatalf("unmarshaling tool_input: %v", err)
				}
				if ti["command"] != "ls" {
					t.Errorf("tool_input.command = %q, want %q", ti["command"], "ls")
				}
			},
		},
		{
			name:   "source flag overrides JSON source",
			input:  `{"tool_name":"read_file","source":"cursor"}`,
			source: "claude-code",
			check: func(t *testing.T, d model.Desire) {
				if d.Source != "claude-code" {
					t.Errorf("Source = %q, want %q", d.Source, "claude-code")
				}
			},
		},
		{
			name:  "source from JSON when flag is empty",
			input: `{"tool_name":"read_file","source":"cursor"}`,
			check: func(t *testing.T, d model.Desire) {
				if d.Source != "cursor" {
					t.Errorf("Source = %q, want %q", d.Source, "cursor")
				}
			},
		},
		{
			name:  "unknown fields go into metadata",
			input: `{"tool_name":"read_file","model":"claude-3","user":"alice","hook_event_name":"PostToolUseFailure"}`,
			check: func(t *testing.T, d model.Desire) {
				var meta map[string]string
				if err := json.Unmarshal(d.Metadata, &meta); err != nil {
					t.Fatalf("unmarshaling metadata: %v", err)
				}
				if meta["model"] != "claude-3" {
					t.Errorf("metadata.model = %q, want %q", meta["model"], "claude-3")
				}
				if meta["user"] != "alice" {
					t.Errorf("metadata.user = %q, want %q", meta["user"], "alice")
				}
				if meta["hook_event_name"] != "PostToolUseFailure" {
					t.Errorf("metadata.hook_event_name = %q, want %q", meta["hook_event_name"], "PostToolUseFailure")
				}
			},
		},
		{
			name:  "existing metadata merged with unknown fields",
			input: `{"tool_name":"read_file","metadata":{"existing":"value"},"extra_field":"extra"}`,
			check: func(t *testing.T, d model.Desire) {
				var meta map[string]json.RawMessage
				if err := json.Unmarshal(d.Metadata, &meta); err != nil {
					t.Fatalf("unmarshaling metadata: %v", err)
				}
				var existing string
				if err := json.Unmarshal(meta["existing"], &existing); err != nil {
					t.Fatalf("unmarshaling existing: %v", err)
				}
				if existing != "value" {
					t.Errorf("metadata.existing = %q, want %q", existing, "value")
				}
				var extra string
				if err := json.Unmarshal(meta["extra_field"], &extra); err != nil {
					t.Fatalf("unmarshaling extra_field: %v", err)
				}
				if extra != "extra" {
					t.Errorf("metadata.extra_field = %q, want %q", extra, "extra")
				}
			},
		},
		{
			name:  "metadata preserved when no unknown fields",
			input: `{"tool_name":"read_file","metadata":{"key":"val"}}`,
			check: func(t *testing.T, d model.Desire) {
				var meta map[string]string
				if err := json.Unmarshal(d.Metadata, &meta); err != nil {
					t.Fatalf("unmarshaling metadata: %v", err)
				}
				if meta["key"] != "val" {
					t.Errorf("metadata.key = %q, want %q", meta["key"], "val")
				}
			},
		},
		{
			name:  "claude code hook input",
			input: `{"session_id":"abc123","hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":{"command":"nonexistent-cmd"},"tool_use_id":"toolu_01xyz","error":"Command exited with non-zero status code 1","cwd":"/home/user/project","transcript_path":"/tmp/transcript","permission_mode":"default"}`,
			source: "claude-code",
			check: func(t *testing.T, d model.Desire) {
				if d.ToolName != "Bash" {
					t.Errorf("ToolName = %q, want %q", d.ToolName, "Bash")
				}
				if d.SessionID != "abc123" {
					t.Errorf("SessionID = %q, want %q", d.SessionID, "abc123")
				}
				if d.Source != "claude-code" {
					t.Errorf("Source = %q, want %q", d.Source, "claude-code")
				}
				if d.CWD != "/home/user/project" {
					t.Errorf("CWD = %q, want %q", d.CWD, "/home/user/project")
				}
				// Unknown hook fields should be in metadata.
				var meta map[string]json.RawMessage
				if err := json.Unmarshal(d.Metadata, &meta); err != nil {
					t.Fatalf("unmarshaling metadata: %v", err)
				}
				if _, ok := meta["hook_event_name"]; !ok {
					t.Error("metadata should contain hook_event_name")
				}
				if _, ok := meta["tool_use_id"]; !ok {
					t.Error("metadata should contain tool_use_id")
				}
				if _, ok := meta["transcript_path"]; !ok {
					t.Error("metadata should contain transcript_path")
				}
				if _, ok := meta["permission_mode"]; !ok {
					t.Error("metadata should contain permission_mode")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &fakeStore{}
			_, err := Record(context.Background(), fs, strings.NewReader(tt.input), tt.source)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(fs.recorded) != 1 {
				t.Fatalf("expected 1 recorded desire, got %d", len(fs.recorded))
			}

			tt.check(t, fs.recorded[0])
		})
	}
}

func TestRecordStoreError(t *testing.T) {
	fs := &fakeStore{err: fmt.Errorf("db connection failed")}
	_, err := Record(context.Background(), fs, strings.NewReader(`{"tool_name":"test"}`), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "storing desire") {
		t.Errorf("error %q should contain 'storing desire'", err.Error())
	}
}

func TestRecordMalformedInputs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "empty string",
			input:   "",
			wantErr: "parsing JSON",
		},
		{
			name:    "JSON array instead of object",
			input:   `[{"tool_name":"foo"}]`,
			wantErr: "parsing JSON",
		},
		{
			name:    "bare string",
			input:   `"just a string"`,
			wantErr: "parsing JSON",
		},
		{
			name:    "number",
			input:   `42`,
			wantErr: "parsing JSON",
		},
		{
			name:    "null",
			input:   `null`,
			wantErr: "tool_name",
		},
		{
			name:    "tool_name is number not string",
			input:   `{"tool_name":123}`,
			wantErr: "parsing tool_name",
		},
		{
			name:    "tool_name is array",
			input:   `{"tool_name":["a","b"]}`,
			wantErr: "parsing tool_name",
		},
		{
			name:    "tool_name is null",
			input:   `{"tool_name":null}`,
			wantErr: "missing required field: tool_name",
		},
		{
			name:    "tool_name is boolean",
			input:   `{"tool_name":true}`,
			wantErr: "parsing tool_name",
		},
		{
			name:    "id is number",
			input:   `{"tool_name":"foo","id":123}`,
			wantErr: "parsing id",
		},
		{
			name:    "error is number",
			input:   `{"tool_name":"foo","error":123}`,
			wantErr: "parsing error",
		},
		{
			name:    "session_id is object",
			input:   `{"tool_name":"foo","session_id":{"key":"val"}}`,
			wantErr: "parsing session_id",
		},
		{
			name:    "cwd is array",
			input:   `{"tool_name":"foo","cwd":["a"]}`,
			wantErr: "parsing cwd",
		},
		{
			name:    "timestamp is not valid format",
			input:   `{"tool_name":"foo","timestamp":"not-a-timestamp"}`,
			wantErr: "parsing timestamp",
		},
		{
			name:    "source is number",
			input:   `{"tool_name":"foo","source":42}`,
			wantErr: "parsing source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &fakeStore{}
			_, err := Record(context.Background(), fs, strings.NewReader(tt.input), "")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRecordNonObjectMetadataWithExtraFields(t *testing.T) {
	// When metadata is not a JSON object and there are extra fields,
	// the original metadata should be preserved under _original.
	fs := &fakeStore{}
	input := `{"tool_name":"foo","metadata":"just a string","extra_key":"extra_val"}`
	_, err := Record(context.Background(), fs, strings.NewReader(input), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs.recorded) != 1 {
		t.Fatalf("expected 1 recorded, got %d", len(fs.recorded))
	}

	var meta map[string]json.RawMessage
	if err := json.Unmarshal(fs.recorded[0].Metadata, &meta); err != nil {
		t.Fatalf("unmarshaling metadata: %v", err)
	}
	// Original non-object metadata should be under _original.
	if _, ok := meta["_original"]; !ok {
		t.Error("metadata should contain _original key for non-object metadata")
	}
	// Extra field should also be present.
	if _, ok := meta["extra_key"]; !ok {
		t.Error("metadata should contain extra_key")
	}
}

func TestRecordReadError(t *testing.T) {
	fs := &fakeStore{}
	_, err := Record(context.Background(), fs, &errReader{}, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "reading input") {
		t.Errorf("error %q should contain 'reading input'", err.Error())
	}
}

// errReader always returns an error on Read.
type errReader struct{}

func (e *errReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("simulated read failure")
}

func TestRecordAutoGeneratesIDAndTimestamp(t *testing.T) {
	fs := &fakeStore{}
	before := time.Now()
	input := `{"tool_name":"auto_gen_test"}`
	if _, err := Record(context.Background(), fs, strings.NewReader(input), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now()

	if len(fs.recorded) != 1 {
		t.Fatalf("expected 1, got %d", len(fs.recorded))
	}
	d := fs.recorded[0]

	// ID should be a non-empty UUID.
	if d.ID == "" {
		t.Error("ID should be auto-generated")
	}
	if len(d.ID) != 36 { // standard UUID format
		t.Errorf("ID %q does not look like a UUID", d.ID)
	}

	// Timestamp should be between before and after.
	if d.Timestamp.Before(before) || d.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", d.Timestamp, before, after)
	}
}

func TestRecordEmptyObject(t *testing.T) {
	fs := &fakeStore{}
	_, err := Record(context.Background(), fs, strings.NewReader(`{}`), "")
	if err == nil {
		t.Fatal("expected error for empty object, got nil")
	}
	if !strings.Contains(err.Error(), "tool_name") {
		t.Errorf("error %q should mention tool_name", err.Error())
	}
}

func TestRecordWithOnlyUnknownFields(t *testing.T) {
	fs := &fakeStore{}
	_, err := Record(context.Background(), fs, strings.NewReader(`{"unknown":"value","another":123}`), "")
	if err == nil {
		t.Fatal("expected error (no tool_name), got nil")
	}
	if !strings.Contains(err.Error(), "tool_name") {
		t.Errorf("error %q should mention tool_name", err.Error())
	}
}
