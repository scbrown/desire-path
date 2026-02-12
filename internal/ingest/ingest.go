// Package ingest converts raw source payloads into Invocations and persists them.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/scbrown/desire-path/internal/analyze"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/source"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/scbrown/desire-path/internal/transcript"
)

// Ingest parses raw bytes using the named source plugin, converts the
// extracted fields into an Invocation, and persists it via the store.
//
// The source plugin must be registered in the source registry. If the
// source name is unknown, Ingest returns an error.
//
// ToolName is required; if the source plugin returns empty ToolName,
// Ingest returns an error. UUID and timestamp are auto-generated if
// not already set.
func Ingest(ctx context.Context, s store.Store, raw []byte, sourceName string) (model.Invocation, error) {
	src := source.Get(sourceName)
	if src == nil {
		return model.Invocation{}, fmt.Errorf("unknown source: %q", sourceName)
	}

	fields, err := src.Extract(raw)
	if err != nil {
		return model.Invocation{}, fmt.Errorf("extracting fields: %w", err)
	}

	return IngestFields(ctx, s, fields, sourceName)
}

// IngestFields converts pre-extracted Fields into an Invocation and persists
// it via the store. When the invocation is an error, a Desire is also written
// so failures appear in both reporting views (dp list/paths/stats for desires,
// dp export --type invocations for the full picture).
//
// If the source Fields include transcript_path and tool_use_id in Extra,
// the transcript is parsed to enrich the invocation with turn context
// (TurnID, TurnSequence, TurnLength).
func IngestFields(ctx context.Context, s store.Store, fields *source.Fields, sourceName string) (model.Invocation, error) {
	inv, err := toInvocation(fields, sourceName)
	if err != nil {
		return model.Invocation{}, err
	}

	// Enrich with turn context from transcript if available.
	enrichTurnContext(&inv, fields)

	if err := s.RecordInvocation(ctx, inv); err != nil {
		return model.Invocation{}, fmt.Errorf("storing invocation: %w", err)
	}

	if inv.IsError {
		d := toDesire(fields, sourceName, inv.Timestamp, inv.Metadata)
		if err := s.RecordDesire(ctx, d); err != nil {
			return model.Invocation{}, fmt.Errorf("storing desire: %w", err)
		}
	}

	return inv, nil
}

// toDesire converts source.Fields into a model.Desire, reusing the timestamp
// and pre-marshaled metadata from the companion invocation for consistency.
// It also auto-categorizes the desire based on error patterns.
func toDesire(f *source.Fields, sourceName string, ts time.Time, metadata json.RawMessage) model.Desire {
	return model.Desire{
		ID:        uuid.New().String(),
		ToolName:  f.ToolName,
		ToolInput: f.ToolInput,
		Error:     f.Error,
		Category:  analyze.CategorizeDesire(f.ToolName, f.Error, f.ToolInput),
		Source:    sourceName,
		SessionID: f.InstanceID,
		CWD:       f.CWD,
		Timestamp: ts,
		Metadata:  metadata,
	}
}

// toInvocation converts source.Fields into a model.Invocation, generating
// UUID and timestamp if needed, and marshaling Extra into Metadata.
func toInvocation(f *source.Fields, sourceName string) (model.Invocation, error) {
	if f.ToolName == "" {
		return model.Invocation{}, fmt.Errorf("missing required field: tool_name")
	}

	inv := model.Invocation{
		ID:         uuid.New().String(),
		Source:     sourceName,
		InstanceID: f.InstanceID,
		ToolName:   f.ToolName,
		IsError:    f.Error != "",
		Error:      f.Error,
		CWD:        f.CWD,
		Timestamp:  time.Now(),
	}

	if len(f.Extra) > 0 {
		meta, err := json.Marshal(f.Extra)
		if err != nil {
			return model.Invocation{}, fmt.Errorf("marshaling metadata: %w", err)
		}
		inv.Metadata = meta
	}

	return inv, nil
}

// enrichTurnContext parses the transcript file (if available in Fields.Extra)
// to determine the turn context for the current tool invocation. It populates
// TurnID, TurnSequence, and TurnLength on the invocation.
//
// This is best-effort: if the transcript is unavailable or unparseable,
// the invocation proceeds without turn data.
func enrichTurnContext(inv *model.Invocation, fields *source.Fields) {
	if fields.Extra == nil {
		return
	}

	// Extract transcript_path and tool_use_id from source-specific extras.
	var transcriptPath, toolUseID string
	if raw, ok := fields.Extra["transcript_path"]; ok {
		json.Unmarshal(raw, &transcriptPath)
	}
	if raw, ok := fields.Extra["tool_use_id"]; ok {
		json.Unmarshal(raw, &toolUseID)
	}
	if transcriptPath == "" || toolUseID == "" {
		return
	}

	f, err := os.Open(transcriptPath)
	if err != nil {
		return // transcript not accessible, skip silently
	}
	defer f.Close()

	turns, err := transcript.Parse(f)
	if err != nil {
		return // parse error, skip silently
	}

	// Find the turn containing our tool_use_id.
	for _, turn := range turns {
		for _, step := range turn.Steps {
			if step.ToolUseID == toolUseID {
				sessionID := inv.InstanceID
				if turn.SessionID != "" {
					sessionID = turn.SessionID
				}
				inv.TurnID = fmt.Sprintf("%s:%d", sessionID, turn.Index)
				inv.TurnSequence = step.Sequence
				inv.TurnLength = len(turn.Steps)
				return
			}
		}
	}
}
