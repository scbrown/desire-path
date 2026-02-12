package analyze

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

// MinPatternSessions is the minimum number of distinct sessions a turn pattern
// must appear in before it gets surfaced as a desire path.
const MinPatternSessions = 3

// SurfaceTurnPatternDesires detects recurring turn patterns and creates Desire
// records for patterns that appear 3+ times across sessions with turn length
// exceeding the threshold. Returns the newly created desires.
//
// This is idempotent: patterns that already have a corresponding turn-pattern
// desire are skipped. The abstract pattern string is stored in the desire's
// Metadata field for deduplication.
func SurfaceTurnPatternDesires(ctx context.Context, s store.Store, threshold int) ([]model.Desire, error) {
	patterns, err := s.TurnPatternStats(ctx, store.TurnOpts{MinLength: threshold})
	if err != nil {
		return nil, fmt.Errorf("querying turn patterns: %w", err)
	}

	// Load existing turn-pattern desires for dedup.
	existing, err := s.ListDesires(ctx, store.ListOpts{Category: model.CategoryTurnPattern})
	if err != nil {
		return nil, fmt.Errorf("listing existing turn-pattern desires: %w", err)
	}
	seen := make(map[string]bool, len(existing))
	for _, d := range existing {
		p := extractPattern(d.Metadata)
		if p != "" {
			seen[p] = true
		}
	}

	var created []model.Desire
	for _, p := range patterns {
		if p.Sessions < MinPatternSessions {
			continue
		}
		if seen[p.Pattern] {
			continue
		}

		meta, _ := json.Marshal(map[string]any{
			"pattern":    p.Pattern,
			"count":      p.Count,
			"avg_length": p.AvgLength,
			"sessions":   p.Sessions,
		})

		d := model.Desire{
			ID:       uuid.New().String(),
			ToolName: firstTool(p.Pattern),
			Error: fmt.Sprintf("Repeated pattern: %s (avg %.1f calls, seen %d times across %d sessions)",
				p.Pattern, p.AvgLength, p.Count, p.Sessions),
			Category:  model.CategoryTurnPattern,
			Source:    "transcript-analysis",
			Timestamp: time.Now(),
			Metadata:  meta,
		}

		if err := s.RecordDesire(ctx, d); err != nil {
			return nil, fmt.Errorf("recording turn-pattern desire: %w", err)
		}
		created = append(created, d)
		seen[p.Pattern] = true
	}

	return created, nil
}

// firstTool extracts the first tool name from an abstract pattern like
// "Grep → Read{2+} → Edit", returning "Grep".
func firstTool(pattern string) string {
	parts := strings.SplitN(pattern, " → ", 2)
	tool := parts[0]
	// Strip {N+} suffix if the first element has repeats.
	if idx := strings.Index(tool, "{"); idx >= 0 {
		tool = tool[:idx]
	}
	return tool
}

// extractPattern reads the "pattern" field from a desire's metadata JSON.
func extractPattern(metadata json.RawMessage) string {
	if len(metadata) == 0 {
		return ""
	}
	var m struct {
		Pattern string `json:"pattern"`
	}
	if json.Unmarshal(metadata, &m) != nil {
		return ""
	}
	return m.Pattern
}
