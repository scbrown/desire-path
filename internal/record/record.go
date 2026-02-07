// Package record handles parsing and recording of failed tool call data from
// JSON input into the desire-path store.
package record

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

// knownFields lists the JSON keys that map directly to Desire struct fields.
// Any other keys in the input JSON are collected into metadata.
var knownFields = map[string]bool{
	"id":         true,
	"tool_name":  true,
	"tool_input": true,
	"error":      true,
	"source":     true,
	"session_id": true,
	"cwd":        true,
	"timestamp":  true,
	"metadata":   true,
}

// Record reads JSON from input, extracts known fields into a Desire, collects
// unknown fields into metadata, and persists the result via the store.
//
// The source parameter overrides any "source" field in the input JSON. If source
// is empty and the input JSON contains a "source" field, that value is used.
//
// Only tool_name is required. If missing, Record returns an error. UUID and
// timestamp are generated automatically if not provided in the input.
func Record(ctx context.Context, s store.Store, input io.Reader, source string) error {
	raw, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	d, err := extractDesire(fields, source)
	if err != nil {
		return err
	}

	if err := s.RecordDesire(ctx, d); err != nil {
		return fmt.Errorf("storing desire: %w", err)
	}

	return nil
}

// extractDesire builds a Desire from a parsed JSON map, extracting known fields
// and collecting the rest into metadata.
func extractDesire(fields map[string]json.RawMessage, source string) (model.Desire, error) {
	var d model.Desire

	// tool_name is required.
	tn, ok := fields["tool_name"]
	if !ok {
		return d, fmt.Errorf("missing required field: tool_name")
	}
	if err := json.Unmarshal(tn, &d.ToolName); err != nil {
		return d, fmt.Errorf("parsing tool_name: %w", err)
	}
	if d.ToolName == "" {
		return d, fmt.Errorf("missing required field: tool_name")
	}

	// Optional known fields.
	if v, ok := fields["id"]; ok {
		if err := json.Unmarshal(v, &d.ID); err != nil {
			return d, fmt.Errorf("parsing id: %w", err)
		}
	}
	if d.ID == "" {
		d.ID = uuid.New().String()
	}

	if v, ok := fields["tool_input"]; ok {
		d.ToolInput = v
	}

	if v, ok := fields["error"]; ok {
		if err := json.Unmarshal(v, &d.Error); err != nil {
			return d, fmt.Errorf("parsing error: %w", err)
		}
	}

	if v, ok := fields["session_id"]; ok {
		if err := json.Unmarshal(v, &d.SessionID); err != nil {
			return d, fmt.Errorf("parsing session_id: %w", err)
		}
	}

	if v, ok := fields["cwd"]; ok {
		if err := json.Unmarshal(v, &d.CWD); err != nil {
			return d, fmt.Errorf("parsing cwd: %w", err)
		}
	}

	if v, ok := fields["timestamp"]; ok {
		if err := json.Unmarshal(v, &d.Timestamp); err != nil {
			return d, fmt.Errorf("parsing timestamp: %w", err)
		}
	}
	if d.Timestamp.IsZero() {
		d.Timestamp = time.Now()
	}

	// Source: CLI flag overrides JSON field.
	if source != "" {
		d.Source = source
	} else if v, ok := fields["source"]; ok {
		if err := json.Unmarshal(v, &d.Source); err != nil {
			return d, fmt.Errorf("parsing source: %w", err)
		}
	}

	// Collect unknown fields into metadata. If the input already has a
	// "metadata" field, merge extra fields into it.
	extra := make(map[string]json.RawMessage)
	for k, v := range fields {
		if !knownFields[k] {
			extra[k] = v
		}
	}

	if len(extra) > 0 {
		// Start with existing metadata if present.
		existing := make(map[string]json.RawMessage)
		if v, ok := fields["metadata"]; ok {
			if err := json.Unmarshal(v, &existing); err != nil {
				// If metadata isn't an object, keep it as-is under a special key.
				existing["_original"] = v
			}
		}
		for k, v := range extra {
			existing[k] = v
		}
		merged, err := json.Marshal(existing)
		if err != nil {
			return d, fmt.Errorf("marshaling metadata: %w", err)
		}
		d.Metadata = merged
	} else if v, ok := fields["metadata"]; ok {
		d.Metadata = v
	}

	return d, nil
}
