package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/source"
	"github.com/scbrown/desire-path/internal/store"
)

// fakeSource implements source.Source for testing.
type fakeSource struct {
	name    string
	fields  *source.Fields
	err     error
}

func (f *fakeSource) Name() string                           { return f.name }
func (f *fakeSource) Description() string                     { return "fake source for testing" }
func (f *fakeSource) Extract([]byte) (*source.Fields, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.fields, nil
}

// fakeStore records calls to RecordInvocation and RecordDesire for test inspection.
type fakeStore struct {
	recorded      []model.Invocation
	desires       []model.Desire
	err           error
	desireErr     error
}

func (f *fakeStore) RecordInvocation(_ context.Context, inv model.Invocation) error {
	if f.err != nil {
		return f.err
	}
	f.recorded = append(f.recorded, inv)
	return nil
}

func (f *fakeStore) RecordDesire(_ context.Context, d model.Desire) error {
	if f.desireErr != nil {
		return f.desireErr
	}
	f.desires = append(f.desires, d)
	return nil
}
func (f *fakeStore) ListDesires(context.Context, store.ListOpts) ([]model.Desire, error) {
	return nil, nil
}
func (f *fakeStore) GetPaths(context.Context, store.PathOpts) ([]model.Path, error) {
	return nil, nil
}
func (f *fakeStore) SetAlias(context.Context, model.Alias) error { return nil }
func (f *fakeStore) GetAlias(context.Context, string, string, string, string, string) (*model.Alias, error) {
	return nil, nil
}
func (f *fakeStore) GetAliases(context.Context) ([]model.Alias, error)                       { return nil, nil }
func (f *fakeStore) DeleteAlias(context.Context, string, string, string, string, string) (bool, error) {
	return false, nil
}
func (f *fakeStore) GetRulesForTool(context.Context, string) ([]model.Alias, error) { return nil, nil }
func (f *fakeStore) Stats(context.Context) (store.Stats, error)        { return store.Stats{}, nil }
func (f *fakeStore) InspectPath(context.Context, store.InspectOpts) (*store.InspectResult, error) {
	return &store.InspectResult{}, nil
}
func (f *fakeStore) ListInvocations(context.Context, store.InvocationOpts) ([]model.Invocation, error) {
	return nil, nil
}
func (f *fakeStore) InvocationStats(context.Context) (store.InvocationStatsResult, error) {
	return store.InvocationStatsResult{}, nil
}
func (f *fakeStore) Close() error { return nil }

// registerTestSource registers a fake source and returns a cleanup function
// that removes it from the registry. Since the source registry uses a global
// map with no unregister, we use a unique name per test to avoid conflicts.
func registerTestSource(t *testing.T, name string, fields *source.Fields, err error) {
	t.Helper()
	src := &fakeSource{name: name, fields: fields, err: err}
	source.Register(src)
}

func TestIngestFullClaudeCodePayload(t *testing.T) {
	srcName := "test-claude-code-full"
	registerTestSource(t, srcName, &source.Fields{
		ToolName:   "Bash",
		InstanceID: "session-abc123",
		CWD:        "/home/user/project",
		Error:      "command not found",
		Extra: map[string]json.RawMessage{
			"hook_event_name":  json.RawMessage(`"PostToolUseFailure"`),
			"tool_use_id":      json.RawMessage(`"toolu_01xyz"`),
			"transcript_path":  json.RawMessage(`"/tmp/transcript"`),
			"permission_mode":  json.RawMessage(`"default"`),
		},
	}, nil)

	fs := &fakeStore{}
	before := time.Now()
	inv, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	after := time.Now()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs.recorded) != 1 {
		t.Fatalf("expected 1 recorded invocation, got %d", len(fs.recorded))
	}

	// Verify all Invocation fields.
	if inv.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", inv.ToolName, "Bash")
	}
	if inv.Source != srcName {
		t.Errorf("Source = %q, want %q", inv.Source, srcName)
	}
	if inv.InstanceID != "session-abc123" {
		t.Errorf("InstanceID = %q, want %q", inv.InstanceID, "session-abc123")
	}
	if inv.CWD != "/home/user/project" {
		t.Errorf("CWD = %q, want %q", inv.CWD, "/home/user/project")
	}
	if inv.Error != "command not found" {
		t.Errorf("Error = %q, want %q", inv.Error, "command not found")
	}
	if !inv.IsError {
		t.Error("IsError should be true when Error is non-empty")
	}

	// UUID should be auto-generated.
	if inv.ID == "" {
		t.Error("ID should be auto-generated")
	}
	if len(inv.ID) != 36 {
		t.Errorf("ID %q does not look like a UUID", inv.ID)
	}

	// Timestamp should be auto-generated between before and after.
	if inv.Timestamp.Before(before) || inv.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", inv.Timestamp, before, after)
	}
}

func TestIngestExtraFieldsInMetadata(t *testing.T) {
	srcName := "test-extra-metadata"
	registerTestSource(t, srcName, &source.Fields{
		ToolName: "Read",
		Extra: map[string]json.RawMessage{
			"hook_event_name": json.RawMessage(`"PostToolUseFailure"`),
			"tool_use_id":     json.RawMessage(`"toolu_99abc"`),
			"custom_field":    json.RawMessage(`42`),
		},
	}, nil)

	fs := &fakeStore{}
	inv, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if inv.Metadata == nil {
		t.Fatal("Metadata should not be nil when Extra fields exist")
	}

	var meta map[string]json.RawMessage
	if err := json.Unmarshal(inv.Metadata, &meta); err != nil {
		t.Fatalf("unmarshaling metadata: %v", err)
	}

	// Verify each Extra field is in Metadata.
	var hookEvent string
	if err := json.Unmarshal(meta["hook_event_name"], &hookEvent); err != nil {
		t.Fatalf("unmarshaling hook_event_name: %v", err)
	}
	if hookEvent != "PostToolUseFailure" {
		t.Errorf("metadata.hook_event_name = %q, want %q", hookEvent, "PostToolUseFailure")
	}

	var toolUseID string
	if err := json.Unmarshal(meta["tool_use_id"], &toolUseID); err != nil {
		t.Fatalf("unmarshaling tool_use_id: %v", err)
	}
	if toolUseID != "toolu_99abc" {
		t.Errorf("metadata.tool_use_id = %q, want %q", toolUseID, "toolu_99abc")
	}

	var customField int
	if err := json.Unmarshal(meta["custom_field"], &customField); err != nil {
		t.Fatalf("unmarshaling custom_field: %v", err)
	}
	if customField != 42 {
		t.Errorf("metadata.custom_field = %d, want %d", customField, 42)
	}
}

func TestIngestNoExtraFieldsNoMetadata(t *testing.T) {
	srcName := "test-no-extra"
	registerTestSource(t, srcName, &source.Fields{
		ToolName: "Write",
	}, nil)

	fs := &fakeStore{}
	inv, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if inv.Metadata != nil {
		t.Errorf("Metadata should be nil when no Extra fields, got %s", string(inv.Metadata))
	}
}

func TestIngestMissingToolNameReturnsError(t *testing.T) {
	srcName := "test-no-toolname"
	registerTestSource(t, srcName, &source.Fields{
		ToolName: "",
		CWD:      "/tmp",
	}, nil)

	fs := &fakeStore{}
	_, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err == nil {
		t.Fatal("expected error for missing tool_name, got nil")
	}
	if !strings.Contains(err.Error(), "tool_name") {
		t.Errorf("error %q should mention tool_name", err.Error())
	}

	// Store should not have been called.
	if len(fs.recorded) != 0 {
		t.Errorf("expected 0 recorded invocations, got %d", len(fs.recorded))
	}
}

func TestIngestUnknownSourceReturnsError(t *testing.T) {
	fs := &fakeStore{}
	_, err := Ingest(context.Background(), fs, []byte(`{}`), "nonexistent-source")
	if err == nil {
		t.Fatal("expected error for unknown source, got nil")
	}
	if !strings.Contains(err.Error(), "unknown source") {
		t.Errorf("error %q should contain 'unknown source'", err.Error())
	}
	if !strings.Contains(err.Error(), "nonexistent-source") {
		t.Errorf("error %q should contain source name", err.Error())
	}

	// Store should not have been called.
	if len(fs.recorded) != 0 {
		t.Errorf("expected 0 recorded invocations, got %d", len(fs.recorded))
	}
}

func TestIngestUUIDAutoGenerated(t *testing.T) {
	srcName := "test-uuid-autogen"
	registerTestSource(t, srcName, &source.Fields{
		ToolName: "Glob",
	}, nil)

	fs := &fakeStore{}
	inv1, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ID should be a valid UUID (36 chars with hyphens).
	if inv1.ID == "" {
		t.Error("ID should be auto-generated, got empty string")
	}
	if len(inv1.ID) != 36 {
		t.Errorf("ID %q does not look like a UUID (expected 36 chars)", inv1.ID)
	}

	// Two ingestions should produce different UUIDs.
	srcName2 := "test-uuid-autogen-2"
	registerTestSource(t, srcName2, &source.Fields{
		ToolName: "Glob",
	}, nil)
	inv2, err := Ingest(context.Background(), fs, []byte(`{}`), srcName2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv1.ID == inv2.ID {
		t.Errorf("two ingestions should produce different UUIDs, both got %q", inv1.ID)
	}
}

func TestIngestTimestampAutoGenerated(t *testing.T) {
	srcName := "test-timestamp-autogen"
	registerTestSource(t, srcName, &source.Fields{
		ToolName: "Edit",
	}, nil)

	fs := &fakeStore{}
	before := time.Now()
	inv, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	after := time.Now()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if inv.Timestamp.IsZero() {
		t.Error("Timestamp should be auto-generated, got zero value")
	}
	if inv.Timestamp.Before(before) || inv.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", inv.Timestamp, before, after)
	}
}

func TestIngestIsErrorSetFromError(t *testing.T) {
	tests := []struct {
		name    string
		srcName string
		error   string
		want    bool
	}{
		{
			name:    "error present sets IsError true",
			srcName: "test-iserror-true",
			error:   "something failed",
			want:    true,
		},
		{
			name:    "no error sets IsError false",
			srcName: "test-iserror-false",
			error:   "",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registerTestSource(t, tt.srcName, &source.Fields{
				ToolName: "Bash",
				Error:    tt.error,
			}, nil)

			fs := &fakeStore{}
			inv, err := Ingest(context.Background(), fs, []byte(`{}`), tt.srcName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if inv.IsError != tt.want {
				t.Errorf("IsError = %v, want %v", inv.IsError, tt.want)
			}
		})
	}
}

func TestIngestStoreError(t *testing.T) {
	srcName := "test-store-error"
	registerTestSource(t, srcName, &source.Fields{
		ToolName: "Bash",
	}, nil)

	fs := &fakeStore{err: fmt.Errorf("db connection failed")}
	_, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "storing invocation") {
		t.Errorf("error %q should contain 'storing invocation'", err.Error())
	}
}

func TestIngestExtractError(t *testing.T) {
	srcName := "test-extract-error"
	registerTestSource(t, srcName, nil, fmt.Errorf("malformed payload"))

	fs := &fakeStore{}
	_, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "extracting fields") {
		t.Errorf("error %q should contain 'extracting fields'", err.Error())
	}

	if len(fs.recorded) != 0 {
		t.Errorf("expected 0 recorded invocations, got %d", len(fs.recorded))
	}
}

func TestIngestReturnedValueMatchesStored(t *testing.T) {
	srcName := "test-return-matches"
	registerTestSource(t, srcName, &source.Fields{
		ToolName:   "Grep",
		InstanceID: "inst-42",
		CWD:        "/var/log",
	}, nil)

	fs := &fakeStore{}
	inv, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs.recorded) != 1 {
		t.Fatalf("expected 1 recorded, got %d", len(fs.recorded))
	}

	stored := fs.recorded[0]
	if inv.ID != stored.ID {
		t.Errorf("returned ID %q != stored ID %q", inv.ID, stored.ID)
	}
	if inv.ToolName != stored.ToolName {
		t.Errorf("returned ToolName %q != stored ToolName %q", inv.ToolName, stored.ToolName)
	}
	if !inv.Timestamp.Equal(stored.Timestamp) {
		t.Errorf("returned Timestamp %v != stored Timestamp %v", inv.Timestamp, stored.Timestamp)
	}
}

func TestIngestDualWriteOnError(t *testing.T) {
	srcName := "test-dualwrite-error"
	registerTestSource(t, srcName, &source.Fields{
		ToolName:   "Bash",
		InstanceID: "sess-abc",
		ToolInput:  json.RawMessage(`{"command":"rm -rf /"}`),
		CWD:        "/home/user",
		Error:      "permission denied",
		Extra: map[string]json.RawMessage{
			"hook_event_name": json.RawMessage(`"PostToolUseFailure"`),
		},
	}, nil)

	fs := &fakeStore{}
	inv, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invocation should be recorded.
	if len(fs.recorded) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(fs.recorded))
	}

	// Desire should also be recorded (dual-write).
	if len(fs.desires) != 1 {
		t.Fatalf("expected 1 desire, got %d", len(fs.desires))
	}

	d := fs.desires[0]

	// Desire should have its own UUID, distinct from the invocation.
	if d.ID == "" {
		t.Error("desire ID should be auto-generated")
	}
	if d.ID == inv.ID {
		t.Error("desire ID should differ from invocation ID")
	}

	// Fields should map correctly.
	if d.ToolName != "Bash" {
		t.Errorf("desire ToolName = %q, want %q", d.ToolName, "Bash")
	}
	if d.Error != "permission denied" {
		t.Errorf("desire Error = %q, want %q", d.Error, "permission denied")
	}
	if d.Source != srcName {
		t.Errorf("desire Source = %q, want %q", d.Source, srcName)
	}
	if d.SessionID != "sess-abc" {
		t.Errorf("desire SessionID = %q, want %q", d.SessionID, "sess-abc")
	}
	if d.CWD != "/home/user" {
		t.Errorf("desire CWD = %q, want %q", d.CWD, "/home/user")
	}
	if string(d.ToolInput) != `{"command":"rm -rf /"}` {
		t.Errorf("desire ToolInput = %s, want %s", d.ToolInput, `{"command":"rm -rf /"}`)
	}

	// Timestamp should match the invocation.
	if !d.Timestamp.Equal(inv.Timestamp) {
		t.Errorf("desire Timestamp %v != invocation Timestamp %v", d.Timestamp, inv.Timestamp)
	}

	// Metadata should be populated from Extra.
	if d.Metadata == nil {
		t.Fatal("desire Metadata should not be nil when Extra fields exist")
	}
}

func TestIngestNoDualWriteOnSuccess(t *testing.T) {
	srcName := "test-dualwrite-success"
	registerTestSource(t, srcName, &source.Fields{
		ToolName: "Read",
		CWD:      "/tmp",
	}, nil)

	fs := &fakeStore{}
	_, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invocation should be recorded.
	if len(fs.recorded) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(fs.recorded))
	}

	// Desire should NOT be recorded for successful invocations.
	if len(fs.desires) != 0 {
		t.Errorf("expected 0 desires for success, got %d", len(fs.desires))
	}
}

func TestIngestCategorizesEnvNeed(t *testing.T) {
	srcName := "test-categorize-envneed"
	registerTestSource(t, srcName, &source.Fields{
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`{"command":"cargo-insta test"}`),
		Error:     "bash: cargo-insta: command not found",
	}, nil)

	fs := &fakeStore{}
	_, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fs.desires) != 1 {
		t.Fatalf("expected 1 desire, got %d", len(fs.desires))
	}

	d := fs.desires[0]
	if d.Category != "env-need" {
		t.Errorf("desire Category = %q, want %q", d.Category, "env-need")
	}
}

func TestIngestNoCategoryForGenericBashError(t *testing.T) {
	srcName := "test-no-category-generic"
	registerTestSource(t, srcName, &source.Fields{
		ToolName: "Bash",
		Error:    "permission denied",
	}, nil)

	fs := &fakeStore{}
	_, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fs.desires) != 1 {
		t.Fatalf("expected 1 desire, got %d", len(fs.desires))
	}

	d := fs.desires[0]
	if d.Category != "" {
		t.Errorf("desire Category = %q, want empty", d.Category)
	}
}

func TestIngestDualWriteDesireStoreError(t *testing.T) {
	srcName := "test-dualwrite-desire-err"
	registerTestSource(t, srcName, &source.Fields{
		ToolName: "Bash",
		Error:    "tool failed",
	}, nil)

	fs := &fakeStore{desireErr: fmt.Errorf("desire table full")}
	_, err := Ingest(context.Background(), fs, []byte(`{}`), srcName)
	if err == nil {
		t.Fatal("expected error when RecordDesire fails, got nil")
	}
	if !strings.Contains(err.Error(), "storing desire") {
		t.Errorf("error %q should contain 'storing desire'", err.Error())
	}
}
