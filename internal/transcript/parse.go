// Package transcript parses Claude Code JSONL transcripts into structured
// Turn/Step data for desire-path turn-length analysis.
package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// Turn represents one human→model→human cycle in a session.
type Turn struct {
	SessionID  string
	Index      int       // 0-based turn number in session
	StartedAt  time.Time
	DurationMs int       // from turn_duration system event, 0 if absent
	Steps      []Step    // tool calls in execution order
}

// Step represents one tool invocation within a turn.
type Step struct {
	ToolName   string
	ToolUseID  string
	Input      json.RawMessage // tool input parameters
	Sequence   int             // 0-based position in turn
	IsParallel bool            // true if fired concurrently with adjacent steps
	IsError    bool
	Error      string
}

// event is the minimal JSONL event shape we need for parsing.
type event struct {
	UUID       string    `json:"uuid"`
	ParentUUID *string   `json:"parentUuid"`
	Type       string    `json:"type"`
	Subtype    string    `json:"subtype,omitempty"`
	SessionID  string    `json:"sessionId,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	DurationMs int       `json:"durationMs,omitempty"`

	// Message holds the API message for user/assistant events.
	Message json.RawMessage `json:"message,omitempty"`

	// sourceToolAssistantUUID links a tool_result to its tool_use.
	SourceToolAssistantUUID string `json:"sourceToolAssistantUUID,omitempty"`
}

// messageEnvelope is the shape of the message field on user/assistant events.
type messageEnvelope struct {
	Role    string            `json:"role"`
	Content json.RawMessage   `json:"content"`
}

// contentBlock represents one block in an assistant message's content array.
type contentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name,omitempty"`
	ID    string          `json:"id,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// toolResultBlock represents one block in a user message's content array
// when it contains a tool_result.
type toolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	IsError   bool   `json:"is_error"`
	Content   string `json:"content,omitempty"`
}

// Parse reads Claude Code transcript JSONL from r and returns structured turns.
// Events are read in their entirety first, then grouped into turns.
func Parse(r io.Reader) ([]Turn, error) {
	events, err := readEvents(r)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}

	// Sort events by timestamp, preserving JSONL line order for ties.
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	// Detect sessionID from first event that has one.
	sessionID := ""
	for _, e := range events {
		if e.SessionID != "" {
			sessionID = e.SessionID
			break
		}
	}

	// Build tool_use results index: sourceToolAssistantUUID → tool result event.
	toolResults := make(map[string]*event)
	for i := range events {
		if events[i].SourceToolAssistantUUID != "" {
			toolResults[events[i].SourceToolAssistantUUID] = &events[i]
		}
	}

	// Walk events and identify turns.
	var turns []Turn
	var currentTurn *turnBuilder
	turnIndex := 0

	for i := range events {
		e := &events[i]

		switch e.Type {
		case "user":
			if isHumanText(e) {
				// Start a new turn.
				if currentTurn != nil {
					turns = append(turns, currentTurn.build(sessionID, turnIndex))
					turnIndex++
				}
				currentTurn = &turnBuilder{
					startedAt: e.Timestamp,
				}
			}

		case "assistant":
			if currentTurn == nil {
				continue
			}
			// Check for tool_use content blocks.
			block, err := extractToolUse(e)
			if err != nil || block == nil {
				continue
			}
			step := pendingStep{
				toolName:  block.Name,
				toolUseID: block.ID,
				input:     block.Input,
				parentUUID: parentOf(e),
				uuid:      e.UUID,
			}
			currentTurn.steps = append(currentTurn.steps, step)

		case "system":
			if e.Subtype == "stop_hook_summary" && currentTurn != nil {
				// Don't finalize yet — wait for turn_duration which follows.
			}
			if e.Subtype == "turn_duration" && currentTurn != nil {
				currentTurn.durationMs = e.DurationMs
				turns = append(turns, currentTurn.build(sessionID, turnIndex))
				turnIndex++
				currentTurn = nil
			}
		}
	}

	// Finalize any in-progress turn at end of transcript.
	if currentTurn != nil {
		turns = append(turns, currentTurn.build(sessionID, turnIndex))
	}

	// Enrich steps with error info from tool results.
	for ti := range turns {
		for si := range turns[ti].Steps {
			step := &turns[ti].Steps[si]
			enrichStepError(step, toolResults)
		}
	}

	return turns, nil
}

// turnBuilder accumulates data for a turn being constructed.
type turnBuilder struct {
	startedAt  time.Time
	durationMs int
	steps      []pendingStep
}

// pendingStep holds step data before finalization.
type pendingStep struct {
	toolName   string
	toolUseID  string
	input      json.RawMessage
	parentUUID string
	uuid       string
}

func (tb *turnBuilder) build(sessionID string, index int) Turn {
	steps := make([]Step, len(tb.steps))
	for i, ps := range tb.steps {
		steps[i] = Step{
			ToolName:  ps.toolName,
			ToolUseID: ps.toolUseID,
			Input:     ps.input,
			Sequence:  i,
		}
	}

	// Detect parallelism: consecutive tool_use events from the same parent
	// (no intervening tool_result) are parallel.
	detectParallelism(tb.steps, steps)

	return Turn{
		SessionID:  sessionID,
		Index:      index,
		StartedAt:  tb.startedAt,
		DurationMs: tb.durationMs,
		Steps:      steps,
	}
}

// detectParallelism marks steps as parallel when consecutive tool_use events
// share the same parentUUID (they were emitted in a single API response).
func detectParallelism(pending []pendingStep, steps []Step) {
	if len(pending) < 2 {
		return
	}
	for i := 1; i < len(pending); i++ {
		if pending[i].parentUUID != "" && pending[i].parentUUID == pending[i-1].parentUUID {
			steps[i].IsParallel = true
			steps[i-1].IsParallel = true
		}
	}
}

// enrichStepError looks up the tool_result for a step and sets IsError/Error.
func enrichStepError(step *Step, toolResults map[string]*event) {
	// The toolResults map is keyed by sourceToolAssistantUUID → result event.
	// Each tool_result user event contains a tool_use_id linking it back to
	// the original tool_use. We scan all results to find the matching one.
	for _, resultEvt := range toolResults {
		blocks, err := parseToolResults(resultEvt)
		if err != nil {
			continue
		}
		for _, block := range blocks {
			if block.ToolUseID == step.ToolUseID {
				if block.IsError {
					step.IsError = true
					step.Error = block.Content
				}
				return
			}
		}
	}
}

// readEvents reads all JSONL lines from r and returns parsed events.
func readEvents(r io.Reader) ([]event, error) {
	scanner := bufio.NewScanner(r)
	// Allow large lines (transcripts can have big tool outputs).
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var events []event
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading events: %w", err)
	}
	return events, nil
}

// isHumanText returns true if the event is a user event with string content
// (a human message, not a tool result).
func isHumanText(e *event) bool {
	if e.Type != "user" || len(e.Message) == 0 {
		return false
	}
	var env messageEnvelope
	if err := json.Unmarshal(e.Message, &env); err != nil {
		return false
	}
	if env.Role != "user" {
		return false
	}
	// String content means human text. Array content means tool results.
	// JSON: strings start with `"`, arrays start with `[`.
	if len(env.Content) == 0 {
		return false
	}
	return env.Content[0] == '"'
}

// extractToolUse returns the tool_use content block from an assistant event,
// or nil if the event doesn't contain one.
func extractToolUse(e *event) (*contentBlock, error) {
	if e.Type != "assistant" || len(e.Message) == 0 {
		return nil, nil
	}
	var env messageEnvelope
	if err := json.Unmarshal(e.Message, &env); err != nil {
		return nil, err
	}
	if env.Role != "assistant" {
		return nil, nil
	}

	var blocks []contentBlock
	if err := json.Unmarshal(env.Content, &blocks); err != nil {
		return nil, nil // content might be a string
	}

	for _, b := range blocks {
		if b.Type == "tool_use" {
			return &b, nil
		}
	}
	return nil, nil
}

// parseToolResults extracts tool_result blocks from a user event.
func parseToolResults(e *event) ([]toolResultBlock, error) {
	if e.Type != "user" || len(e.Message) == 0 {
		return nil, nil
	}
	var env messageEnvelope
	if err := json.Unmarshal(e.Message, &env); err != nil {
		return nil, err
	}
	var blocks []toolResultBlock
	if err := json.Unmarshal(env.Content, &blocks); err != nil {
		return nil, nil
	}
	return blocks, nil
}

// parentOf returns the parentUuid of an event, or empty string if nil.
func parentOf(e *event) string {
	if e.ParentUUID == nil {
		return ""
	}
	return *e.ParentUUID
}
