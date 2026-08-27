// Package signpost implements the contract-preserving signposting sibling mode.
package signpost

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	Repo      string
	CacheDir  string
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
	WarmCacheHit           bool      `json:"warm_cache_hit"`
	Adopted                *bool     `json:"adopted"`
	TurnsToLocate          *int      `json:"turns_to_locate"`
	TokensToResolution     *int      `json:"tokens_to_resolution"`
	Correct                *bool     `json:"correct"`
}

type payload struct {
	SessionID    string          `json:"session_id"`
	ToolUseID    string          `json:"tool_use_id"`
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
	CWD          string          `json:"cwd"`
}

type cachedResult struct {
	Count int `json:"count"`
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
	count, hit, err := warmResult(cfg.CacheDir, CacheKey(cfg.Repo, query))
	if !hit {
		count, err = search(ctx, query)
	}
	e.SemanticLatencyMS = time.Since(started).Milliseconds()
	e.WarmCacheHit = hit
	if err != nil || count == 0 {
		return nil, e, nil
	}
	e.SemanticCandidateCount = count
	e.SignpostShown = true
	var out hookOutput
	out.HookSpecificOutput.HookEventName = "PostToolUse"
	command := "bobbin search " + shellQuote(query)
	if cfg.Repo != "" {
		command = "bobbin search --repo " + shellQuote(cfg.Repo) + " " + shellQuote(query)
	}
	out.HookSpecificOutput.AdditionalContext = fmt.Sprintf("Signpost (%s): literal search returned %d line(s). Try semantic search: %s", predicate, cardinality, command)
	b, err := json.Marshal(out)
	return b, e, err
}

// PrefetchRequest extracts the stable join key and semantic query from a
// PreToolUse payload. It performs no search and never changes hook behavior.
func PrefetchRequest(raw []byte) (string, string, bool) {
	var p payload
	if json.Unmarshal(raw, &p) != nil || p.ToolName != "Bash" || p.ToolUseID == "" {
		return "", "", false
	}
	var input struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(p.ToolInput, &input) != nil {
		return "", "", false
	}
	_, query, ok := literalQuery(input.Command)
	return p.ToolUseID, query, ok && query != ""
}

// CacheKey identifies reusable semantic work without coupling it to one hook
// invocation. Repo scoping prevents same-text queries crossing corpora.
func CacheKey(repo, query string) string {
	sum := sha256.Sum256([]byte(repo + "\x00" + query))
	return fmt.Sprintf("%x", sum[:16])
}

// WriteWarmResult atomically publishes a speculative result for PostToolUse.
func WriteWarmResult(dir, id string, count int) error {
	if dir == "" || id == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(cachedResult{Count: count})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".warm-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(b)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, warmPath(dir, id))
}

func warmResult(dir, id string) (int, bool, error) {
	if dir == "" || id == "" {
		return 0, false, nil
	}
	path := warmPath(dir, id)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	_ = os.Remove(path)
	var result cachedResult
	if json.Unmarshal(b, &result) == nil {
		return result.Count, true, nil
	}
	return 0, false, nil
}

func warmPath(dir, id string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, id)
	return filepath.Join(dir, safe+".json")
}

// HTTPSearcher builds a latency-bounded Bobbin search function.
func HTTPSearcher(endpoint, repo string, timeout time.Duration) Searcher {
	client := &http.Client{Timeout: timeout}
	return func(ctx context.Context, query string) (int, error) {
		u, err := url.Parse(endpoint)
		if err != nil {
			return 0, err
		}
		q := u.Query()
		q.Set("q", query)
		q.Set("limit", "3")
		if repo != "" {
			q.Set("repo", repo)
		}
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
	fields := shellFields(command)
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

func shellFields(command string) []string {
	var fields []string
	var b strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if b.Len() > 0 {
			fields = append(fields, b.String())
			b.Reset()
		}
	}
	for _, r := range command {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			flush()
			continue
		}
		b.WriteRune(r)
	}
	flush()
	return fields
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
