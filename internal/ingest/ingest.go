// Package ingest converts raw source payloads into Invocations and persists them.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/source"
	"github.com/scbrown/desire-path/internal/store"
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
func IngestFields(ctx context.Context, s store.Store, fields *source.Fields, sourceName string) (model.Invocation, error) {
	inv, err := toInvocation(fields, sourceName)
	if err != nil {
		return model.Invocation{}, err
	}

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
func toDesire(f *source.Fields, sourceName string, ts time.Time, metadata json.RawMessage) model.Desire {
	return model.Desire{
		ID:        uuid.New().String(),
		ToolName:  f.ToolName,
		ToolInput: f.ToolInput,
		Error:     f.Error,
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
