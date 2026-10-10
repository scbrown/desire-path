// Package quipufailure produces privacy-preserving review records from observed
// Quipu tool completions. It never executes commands or retries calls.
package quipufailure

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/scbrown/desire-path/internal/source"
)

// Event is the versioned review metadata exported with an invocation.
// InputPattern contains structure only, not query identifiers or literal values.
type Event struct {
	Version      int    `json:"version"`
	Operation    string `json:"operation"`
	Agent        string `json:"agent"`
	Session      string `json:"session"`
	Outcome      string `json:"outcome"`
	ErrorClass   string `json:"error_class,omitempty"`
	InputPattern string `json:"input_pattern"`
	PatternID    string `json:"pattern_id,omitempty"`
	InputID      string `json:"input_id"`
	RetryID      string `json:"retry_id,omitempty"`
}

const maxInputBytes = 64 << 10
const maxResponseBytes = 1 << 20
const maxPatternBytes = 8192

var toolRE = regexp.MustCompile(`(?:^|__)quipu_([a-z_]+)$`)
var opRE = regexp.MustCompile(`^[a-z][a-z_]{0,40}$`)
var agentRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,47}$`)
var queryToken = regexp.MustCompile(`(?s)<[^>]*>|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|\?[A-Za-z_][A-Za-z0-9_]*|[A-Za-z_][A-Za-z0-9_:-]*|[0-9]+|[^\s]`)
var keywords = strings.Fields("SELECT ASK CONSTRUCT DESCRIBE WHERE PREFIX BASE FILTER OPTIONAL UNION GRAPH VALUES BIND AS DISTINCT COUNT SUM AVG MIN MAX GROUP BY ORDER LIMIT OFFSET HAVING EXISTS NOT IN INSERT DELETE DATA WITH USING SILENT SERVICE LOAD CLEAR DROP CREATE ADD MOVE COPY TRUE FALSE")

// Prepare returns sanitized fields for recognized calls, or the original fields
// and false for unrelated calls. Hosts are exact URL hostnames, not substrings.
func Prepare(f *source.Fields, hosts []string, agent string) (*source.Fields, bool) {
	op, input, ok := operation(f, hosts)
	if !ok {
		return f, false
	}
	outcome, class := classify(f)
	if shellTool(f.ToolName) {
		outcome, class = classifyHTTP(f)
	}
	if options, ok := input.(map[string]any); ok && options["expect_nonempty"] == true && outcome == "success" {
		empty, nonempty := false, false
		for _, key := range []string{"tool_response", "tool_output", "result", "aggregated_output"} {
			var v any
			if len(f.Extra[key]) <= maxResponseBytes && json.Unmarshal(f.Extra[key], &v) == nil {
				e, n := cardinalityEvidence(v, 0)
				empty, nonempty = empty || e, nonempty || n
			}
		}
		if empty && !nonempty {
			outcome, class = "failure", "empty-when-expected"
		}
	}
	pattern := normalize(input, "", 0)
	if len(pattern) > maxPatternBytes {
		pattern = "<pattern-too-large>"
	}
	session := "unknown"
	if f.InstanceID != "" {
		session = digest(f.InstanceID)
	}
	if !agentRE.MatchString(agent) {
		agent = "unknown"
	}
	event := Event{Version: 1, Operation: op, Agent: agent, Session: session, Outcome: outcome, ErrorClass: class, InputPattern: pattern, InputID: digest(op + "\n" + pattern)}
	// A keyed fingerprint permits exact observed retries without storing inputs.
	// Unknown sessions and opaque HTTP bodies cannot establish an exact retry.
	if f.InstanceID != "" && input != nil && pattern != "<pattern-too-large>" {
		m, isMap := input.(map[string]any)
		_, opaque := m["shell"]
		if !isMap || !opaque {
			canonical, err := json.Marshal(input)
			if err == nil && len(canonical) <= maxInputBytes {
				h := hmac.New(sha256.New, []byte(f.InstanceID))
				h.Write([]byte(op + "\n"))
				h.Write(canonical)
				event.RetryID = hex.EncodeToString(h.Sum(nil))
			}
		}
	}
	if outcome == "failure" {
		event.PatternID = digest(op + "\n" + class + "\n" + pattern)
	}
	encoded, _ := json.Marshal(event)
	safeInput, _ := json.Marshal(map[string]string{"input_pattern": pattern})
	result := &source.Fields{ToolName: "quipu_" + op, InstanceID: session, ToolInput: safeInput, Extra: map[string]json.RawMessage{"quipu_review": encoded}}
	if outcome == "failure" {
		result.Error = "quipu failure: " + class
	}
	return result, true
}

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func operation(f *source.Fields, hosts []string) (string, any, bool) {
	name := f.ToolName
	if name == "mcp_tool_call" {
		var tool string
		_ = json.Unmarshal(f.Extra["tool"], &tool)
		if toolRE.MatchString(tool) {
			name = tool
		} else {
			return "", nil, false
		}
	}
	// Unrelated hooks never pay to decode their potentially large inputs.
	if !toolRE.MatchString(name) && !shellTool(name) {
		return "", nil, false
	}
	var input any
	if len(f.ToolInput) <= maxInputBytes {
		_ = json.Unmarshal(f.ToolInput, &input)
	}
	if f.ToolName == "mcp_tool_call" {
		if raw, ok := f.Extra["arguments"]; ok && len(raw) <= maxInputBytes {
			_ = json.Unmarshal(raw, &input)
		}
	}
	if match := toolRE.FindStringSubmatch(name); len(match) > 0 {
		op := match[1]
		if m, ok := input.(map[string]any); ok && op == "tool" {
			if path, ok := m["endpoint"].(string); ok {
				if candidate := strings.Trim(path, "/"); opRE.MatchString(candidate) {
					op = candidate
				}
			}
		}
		return op, input, true
	}
	if !shellTool(f.ToolName) {
		return "", nil, false
	}
	command, _ := input.(string)
	if m, ok := input.(map[string]any); ok {
		command, _ = m["command"].(string)
		if command == "" {
			command, _ = m["cmd"].(string)
		}
	}
	return curlOperation(command, f.Extra["review_input"], hosts)
}

func normalize(v any, key string, depth int) string {
	if depth > 8 {
		return "<depth>"
	}
	switch x := v.(type) {
	case map[string]any:
		if len(x) > 32 {
			return "<object-too-wide>"
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			// Unknown keys can themselves be credentials or user prose.
			switch k {
			case "query", "input", "method", "endpoint", "name", "params", "nodes", "edges", "entity", "predicate", "value", "turtle", "scope_kind", "scope_value", "shell", "limit", "task", "expect_nonempty":
				parts = append(parts, k+":"+normalize(x[k], k, depth+1))
			default:
				parts = append(parts, "<field>:<redacted>")
			}
		}
		if len(strings.Join(parts, ",")) > maxPatternBytes {
			return "<pattern-too-large>"
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		parts := []string{}
		for i, item := range x {
			if i >= 8 {
				parts = append(parts, "...")
				break
			}
			parts = append(parts, normalize(item, key, depth+1))
			if len(strings.Join(parts, ",")) > maxPatternBytes {
				return "<pattern-too-large>"
			}
		}
		return "[" + strings.Join(parts, ",") + "]"
	case string:
		if key == "method" && knownMethod(x) {
			return x
		}
		if key != "query" {
			return "<string>"
		}
		if len(x) > 16384 {
			return "<query-too-large>"
		}
		tokens := queryToken.FindAllString(x, -1)
		parts := make([]string, 0, len(tokens))
		for _, t := range tokens {
			upper := strings.ToUpper(t)
			keyword := false
			for _, k := range keywords {
				if upper == k {
					keyword = true
					break
				}
			}
			switch {
			case strings.HasPrefix(t, "<") && len(t) > 1:
				parts = append(parts, "<iri>")
			case strings.HasPrefix(t, "\"") || strings.HasPrefix(t, "'"):
				parts = append(parts, "<literal>")
			case strings.HasPrefix(t, "?"):
				parts = append(parts, "?var")
			case keyword:
				parts = append(parts, upper)
			case strings.Contains("{}().,;*=!<>+-/|&", t) && len(t) == 1:
				parts = append(parts, t)
			default:
				parts = append(parts, "_")
			}
		}
		return strings.Join(parts, " ")
	case float64:
		return "<number>"
	case bool:
		return "<bool>"
	default:
		return "<null>"
	}
}

func classify(f *source.Fields) (string, string) {
	observed := false
	if f.Error != "" {
		return "failure", errorClass(f.Error, "tool-error")
	}
	for _, key := range []string{"tool_response", "tool_output", "result", "aggregated_output"} {
		raw, ok := f.Extra[key]
		if !ok || len(raw) > maxResponseBytes {
			continue
		}
		var v any
		if json.Unmarshal(raw, &v) != nil {
			continue
		}
		status, class := response(v, 0)
		if status == "failure" {
			return status, class
		}
		observed = observed || status == "success"
	}
	if raw, ok := f.Extra["exit_code"]; ok {
		var code int
		if json.Unmarshal(raw, &code) == nil && code != 0 {
			return "failure", "command-exit"
		}
	}
	if observed {
		return "success", ""
	}
	return "unknown", ""
}

func response(v any, depth int) (string, string) {
	if depth > 6 {
		return "unknown", ""
	}
	if s, ok := v.(string); ok {
		var decoded any
		if len(s) <= 1<<20 && json.Unmarshal([]byte(s), &decoded) == nil {
			return response(decoded, depth+1)
		}
		return "unknown", ""
	}
	if blocks, ok := v.([]any); ok {
		observed := false
		for _, block := range blocks {
			b, ok := block.(map[string]any)
			if !ok || b["type"] != "text" {
				continue
			}
			s, c := response(b["text"], depth+1)
			if s == "failure" {
				return s, c
			}
			observed = observed || s == "success"
		}
		if observed {
			return "success", ""
		}
		return "unknown", ""
	}
	m, ok := v.(map[string]any)
	if !ok {
		return "unknown", ""
	}
	if c, ok := m["conforms"].(bool); ok && !c {
		return "failure", "shape-refusal"
	}
	if e, ok := m["error"]; ok && meaningfulError(e) {
		return "failure", errorClass(text(e), "tool-error")
	}
	for _, key := range []string{"status_code", "http_status"} {
		if n, ok := m[key].(float64); ok && n >= 400 && n < 600 {
			if n == 408 {
				return "failure", "timeout"
			}
			return "failure", strconv.Itoa(int(n)/100) + "xx"
		}
	}
	for _, key := range []string{"isError", "is_error"} {
		if b, ok := m[key].(bool); ok && b {
			return "failure", errorClass(text(m["content"]), "tool-error")
		}
	}
	if n, ok := m["exit_code"].(float64); ok && n != 0 {
		return "failure", errorClass(text(m["stderr"]), "command-exit")
	}
	// Follow only protocol wrappers; rows/entities may legitimately contain error/conforms fields.
	observed := false
	for _, key := range []string{"content", "body", "stdout", "result"} {
		child, exists := m[key]
		if !exists {
			continue
		}
		if key == "content" {
			if blocks, ok := child.([]any); ok {
				for _, block := range blocks {
					if b, ok := block.(map[string]any); ok && b["type"] == "text" {
						s, c := response(b["text"], depth+1)
						if s == "failure" {
							return s, c
						}
						if s == "success" {
							observed = true
						}
					}
				}
			}
		} else {
			s, c := response(child, depth+1)
			if s == "failure" {
				return s, c
			}
			if s == "success" {
				observed = true
			}
		}
	}
	if observed {
		return "success", ""
	}
	for _, key := range []string{"rows", "results", "triples", "outcome", "conforms"} {
		if _, ok := m[key]; ok {
			return "success", ""
		}
	}
	// A transport success on its own does not prove a semantically successful call.
	return "unknown", ""
}
func text(v any) string { b, _ := json.Marshal(v); return string(b) }
func meaningfulError(v any) bool {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x) != ""
	case map[string]any:
		return len(x) != 0
	case []any:
		return len(x) != 0
	case bool:
		return x
	case float64:
		return x != 0
	}
	return false
}
func errorClass(s, fallback string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "timeout") || strings.Contains(s, "timed out") || strings.Contains(s, "deadline") || (strings.Contains(s, "exceeded") && strings.Contains(s, "budget")):
		return "timeout"
	case strings.Contains(s, "parse") || strings.Contains(s, "prefix not found") || strings.Contains(s, "syntax"):
		return "parse"
	case strings.Contains(s, "shacl") || strings.Contains(s, "shape") || strings.Contains(s, "off_vocabulary"):
		return "shape-refusal"
	}
	return fallback
}

// cardinalityEvidence follows protocol wrappers only. A count of triples written
// by an idempotent episode is not a result-set cardinality, so count alone is
// deliberately insufficient. Any nonempty result overrides a sibling empty set.
func cardinalityEvidence(v any, depth int) (empty, nonempty bool) {
	if depth > 6 {
		return
	}
	if text, ok := v.(string); ok {
		var decoded any
		if len(text) <= 1<<20 && json.Unmarshal([]byte(text), &decoded) == nil {
			return cardinalityEvidence(decoded, depth+1)
		}
		return
	}
	if blocks, ok := v.([]any); ok {
		for _, block := range blocks {
			if b, ok := block.(map[string]any); ok && b["type"] == "text" {
				e, n := cardinalityEvidence(b["text"], depth+1)
				empty, nonempty = empty || e, nonempty || n
			}
		}
		return
	}
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	for _, key := range []string{"rows", "results", "triples"} {
		if rows, ok := m[key].([]any); ok {
			empty = empty || len(rows) == 0
			nonempty = nonempty || len(rows) > 0
		}
	}
	for _, key := range []string{"body", "stdout", "result"} {
		e, n := cardinalityEvidence(m[key], depth+1)
		empty, nonempty = empty || e, nonempty || n
	}
	if blocks, ok := m["content"].([]any); ok {
		for _, block := range blocks {
			if b, ok := block.(map[string]any); ok && b["type"] == "text" {
				e, n := cardinalityEvidence(b["text"], depth+1)
				empty, nonempty = empty || e, nonempty || n
			}
		}
	}
	return
}
