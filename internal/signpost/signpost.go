// Package signpost implements the contract-preserving signposting sibling mode.
package signpost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config controls the gating predicate and semantic side query.
type Config struct {
	Threshold int
	Timeout   time.Duration
	BobbinURL string
	LogPath   string
	Condition string
	TaskID    string
	Model     string
}

// Event is the stable JSONL contract consumed by the evaluation harness.
type Event struct {
	EventID                string    `json:"event_id"`
	Timestamp              time.Time `json:"timestamp"`
	SessionID              string    `json:"session_id,omitempty"`
	TaskID                 string    `json:"task_id,omitempty"`
	ModelFamily            string    `json:"model_family,omitempty"`
	Condition              string    `json:"condition"`
	Tool                   string    `json:"tool"`
	Command                string    `json:"command"`
	CWD                    string    `json:"cwd,omitempty"`
	Query                  string    `json:"query"`
	ResultCardinality      int       `json:"result_cardinality"`
	Threshold              int       `json:"threshold"`
	Predicate              string    `json:"predicate"`
	SignpostShown          bool      `json:"signpost_shown"`
	SemanticCandidateCount int       `json:"semantic_candidate_count"`
	SemanticLatencyMS      int64     `json:"semantic_latency_ms"`
	Adopted                *bool     `json:"adopted"`
	TurnsToLocate          *int      `json:"turns_to_locate"`
	TokensToResolution     *int      `json:"tokens_to_resolution"`
	Correct                *bool     `json:"correct"`
}

type payload struct {
	SessionID    string          `json:"session_id"`
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
	CWD          string          `json:"cwd"`
}

type hookOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// Searcher returns the number of semantic candidates for a query.
type Searcher func(context.Context, string) (int, error)

// Process evaluates one PostToolUse payload. Empty output means stay silent.
func Process(ctx context.Context, raw []byte, cfg Config, search Searcher) ([]byte, Event, error) {
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, Event{}, nil // hook input is best-effort
	}
	if p.ToolName != "Bash" {
		return nil, Event{}, nil
	}
	var input struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(p.ToolInput, &input) != nil {
		return nil, Event{}, nil
	}
	tool, query, ok := literalQuery(input.Command)
	if !ok || query == "" {
		return nil, Event{}, nil
	}

	cardinality := lineCount(responseText(p.ToolResponse))
	predicate := "none"
	if cardinality == 0 {
		predicate = "null"
	} else if cardinality > cfg.Threshold {
		predicate = "high-cardinality"
	}
	e := Event{EventID: uuid.NewString(), Timestamp: time.Now().UTC(), SessionID: p.SessionID,
		TaskID: cfg.TaskID, ModelFamily: cfg.Model, Condition: cfg.Condition, Tool: tool,
		Command: input.Command, CWD: p.CWD, Query: query, ResultCardinality: cardinality,
		Threshold: cfg.Threshold, Predicate: predicate}
	if (predicate == "none" && cfg.Condition != "always-signpost") || cfg.Condition == "bare-literal" || cfg.Condition == "prompt-semantic" || cfg.Condition == "replacement" {
		return nil, e, nil
	}

	started := time.Now()
	count, err := search(ctx, query)
	e.SemanticLatencyMS = time.Since(started).Milliseconds()
	if err != nil || count == 0 {
		return nil, e, nil
	}
	e.SemanticCandidateCount = count
	e.SignpostShown = true
	var out hookOutput
	out.HookSpecificOutput.HookEventName = "PostToolUse"
	out.HookSpecificOutput.AdditionalContext = fmt.Sprintf("Signpost (%s): literal search returned %d line(s). Try semantic search: bobbin search %s", predicate, cardinality, shellQuote(query))
	b, err := json.Marshal(out)
	return b, e, err
}

// HTTPSearcher builds a latency-bounded Bobbin search function.
func HTTPSearcher(endpoint string, timeout time.Duration) Searcher {
	client := &http.Client{Timeout: timeout}
	return func(ctx context.Context, query string) (int, error) {
		u, err := url.Parse(endpoint)
		if err != nil {
			return 0, err
		}
		q := u.Query()
		q.Set("q", query)
		q.Set("limit", "3")
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return 0, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("bobbin status %d", resp.StatusCode)
		}
		var result struct {
			Count int `json:"count"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return 0, err
		}
		return result.Count, nil
	}
}

// AppendEvent appends one evaluation record without affecting hook output.
func AppendEvent(path string, event Event) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func literalQuery(command string) (string, string, bool) {
	fields := strings.Fields(command)
	for i, field := range fields {
		base := filepath.Base(strings.Trim(field, "'\""))
		if base != "grep" && base != "rg" {
			continue
		}
		for _, arg := range fields[i+1:] {
			arg = strings.Trim(arg, "'\"")
			if arg == "" || strings.HasPrefix(arg, "-") {
				continue
			}
			return base, arg, true
		}
	}
	return "", "", false
}

func responseText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, key := range []string{"stdout", "output", "content", "text"} {
		if value := responseText(m[key]); value != "" {
			return value
		}
	}
	return ""
}

func lineCount(s string) int {
	s = strings.TrimRight(s, "\r\n")
	if s == "" {
		return 0
	}
	return bytes.Count([]byte(s), []byte{'\n'}) + 1
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
