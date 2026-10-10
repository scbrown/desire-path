package quipufailure

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/source"
)

func TestResponseEvidence(t *testing.T) {
	cases := []struct{ name, body, outcome, class string }{
		{"MCP refusal", `{"isError":true,"content":[{"type":"text","text":"parse error"}]}`, "failure", "parse"},
		{"nested shape", `{"content":[{"type":"text","text":"{\"conforms\":false}"}]}`, "failure", "shape-refusal"},
		{"HTTP failure", `{"status_code":500,"body":"unavailable"}`, "failure", "5xx"},
		{"valid empty", `{"rows":[],"count":0}`, "success", ""},
		{"error-shaped data", `{"rows":[{"error":"legitimate data","conforms":false}]}`, "success", ""},
		{"transport only", `{"status_code":200}`, "unknown", ""},
		{"malformed response", `"not JSON"`, "unknown", ""},
		{"nested error beats success", `{"content":[{"type":"text","text":"{\"rows\":[]}"},{"type":"text","text":"{\"error\":\"timeout\"}"}]}`, "failure", "timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v any
			if err := json.Unmarshal([]byte(tc.body), &v); err != nil {
				t.Fatal(err)
			}
			got, class := response(v, 0)
			if got != tc.outcome || class != tc.class {
				t.Fatalf("got %s/%s; want %s/%s", got, class, tc.outcome, tc.class)
			}
		})
	}
}

func TestPrepareRedactsAllPersistedFields(t *testing.T) {
	f := &source.Fields{ToolName: "mcp__homelab__quipu_query", InstanceID: "private-session-secret", ToolInput: json.RawMessage(`{"query":"SELECT ?privateVariable WHERE { <https://private.example/node> ?predicate \"secret-literal\" }", "token-secret-key":"token-secret-value"}`), Error: "timeout on secret-host with secret-token", Extra: map[string]json.RawMessage{"opaque": json.RawMessage(`"private response"`)}}
	got, ok := Prepare(f, nil, "gennaro")
	if !ok {
		t.Fatal("recognized MCP call omitted")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-session-secret", "privateVariable", "private.example", "secret-literal", "token-secret-key", "token-secret-value", "secret-host", "secret-token", "private response"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("persisted sensitive marker %q", secret)
		}
	}
	var ev Event
	if err := json.Unmarshal(got.Extra["quipu_review"], &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Outcome != "failure" || ev.ErrorClass != "timeout" || ev.Agent != "gennaro" || ev.PatternID == "" || ev.Session == "unknown" {
		t.Fatalf("lost useful metadata: %+v", ev)
	}
	if f.Error != "timeout on secret-host with secret-token" {
		t.Fatal("mutated caller fields")
	}
}

func TestHTTPHostBoundary(t *testing.T) {
	for _, tc := range []struct {
		command string
		match   bool
	}{
		{"curl https://graph.example/query", true},
		{"curl https://graph.example.evil/query", false},
		{"curl https://other.example/query?q=graph.example", false},
	} {
		raw, _ := json.Marshal(map[string]string{"command": tc.command})
		f := &source.Fields{ToolName: "Bash", ToolInput: raw}
		_, got := Prepare(f, []string{"graph.example"}, "")
		if got != tc.match {
			t.Errorf("%q: got %v", tc.command, got)
		}
	}
}

func TestExpectedEmptyRequiresExplicitEvidence(t *testing.T) {
	for _, tc := range []struct{ name, input, body, outcome, class string }{
		{"expected empty", `{"expect_nonempty":true}`, `{"rows":[],"count":0}`, "failure", "empty-when-expected"},
		{"ordinary empty", `{}`, `{"rows":[],"count":0}`, "success", ""},
		{"nonempty control", `{"expect_nonempty":true}`, `{"rows":[{"v":"present"}],"count":1}`, "success", ""},
		{"missing evidence", `{"expect_nonempty":true}`, `{"status_code":200}`, "unknown", ""},
		{"nested empty", `{"expect_nonempty":true}`, `{"content":[{"type":"text","text":"{\"results\":[]}"}]}`, "failure", "empty-when-expected"},
		{"mixed content", `{"expect_nonempty":true}`, `{"content":[{"type":"text","text":"{\"rows\":[]}"},{"type":"text","text":"{\"rows\":[{}]}"}]}`, "success", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &source.Fields{ToolName: "quipu_query", ToolInput: json.RawMessage(tc.input), Extra: map[string]json.RawMessage{"tool_response": json.RawMessage(tc.body)}}
			got, ok := Prepare(f, nil, "gennaro")
			if !ok {
				t.Fatal("unrecognized")
			}
			var ev Event
			if err := json.Unmarshal(got.Extra["quipu_review"], &ev); err != nil {
				t.Fatal(err)
			}
			if ev.Outcome != tc.outcome || ev.ErrorClass != tc.class {
				t.Fatalf("got %s/%s, want %s/%s", ev.Outcome, ev.ErrorClass, tc.outcome, tc.class)
			}
		})
	}
}

func TestFailureOverridesOtherSuccessfulEvidence(t *testing.T) {
	f := &source.Fields{ToolName: "quipu_query", Extra: map[string]json.RawMessage{"tool_response": json.RawMessage(`{"rows":[]}`), "result": json.RawMessage(`{"error":"timeout"}`)}}
	got, _ := Prepare(f, nil, "")
	if got.Error == "" {
		t.Fatal("success wrapper hid explicit failure")
	}
}

func TestNormalizationBounded(t *testing.T) {
	m := map[string]any{}
	for i := 0; i < 10000; i++ {
		m[strings.Repeat("x", i%50)+fmt.Sprint(i)] = "secret"
	}
	raw, _ := json.Marshal(m)
	f := &source.Fields{ToolName: "quipu_query", ToolInput: raw}
	got, _ := Prepare(f, nil, "")
	if len(got.Extra["quipu_review"]) > 16384 {
		t.Fatalf("unbounded review: %d bytes", len(got.Extra["quipu_review"]))
	}
}

func TestRetryFingerprintRequiresExactInputAndSession(t *testing.T) {
	event := func(input, session string) Event {
		f := &source.Fields{ToolName: "quipu_query", InstanceID: session, ToolInput: json.RawMessage(input)}
		got, _ := Prepare(f, nil, "")
		var ev Event
		if err := json.Unmarshal(got.Extra["quipu_review"], &ev); err != nil {
			t.Fatal(err)
		}
		return ev
	}
	a := event(`{"query":"SELECT ?s WHERE { ?s ?p \"secret-a\" }"}`, "session-a")
	b := event(`{"query":"SELECT ?s WHERE { ?s ?p \"secret-b\" }"}`, "session-a")
	c := event(`{"query":"SELECT ?s WHERE { ?s ?p \"secret-a\" }"}`, "session-b")
	if a.InputID != b.InputID {
		t.Fatal("same structural pattern must cluster")
	}
	if a.RetryID == "" || a.RetryID == b.RetryID || a.RetryID == c.RetryID {
		t.Fatal("retry identities crossed literal/session boundary")
	}
	if a.RetryID != event(`{"query":"SELECT ?s WHERE { ?s ?p \"secret-a\" }"}`, "session-a").RetryID {
		t.Fatal("exact retry not stable")
	}
	if event(`{"query":"SELECT ?s {}"}`, "").RetryID != "" {
		t.Fatal("unknown session asserted retry identity")
	}
	f := &source.Fields{ToolName: "Bash", InstanceID: "known", ToolInput: json.RawMessage(`{"command":"curl https://graph.example/query"}`)}
	got, _ := Prepare(f, []string{"graph.example"}, "")
	var ev Event
	json.Unmarshal(got.Extra["quipu_review"], &ev)
	if ev.RetryID != "" {
		t.Fatal("opaque HTTP input asserted retry identity")
	}
}

func TestHTTPObservedEvidenceAndInputPatterns(t *testing.T) {
	cases := []struct {
		name, command, body, outcome, class string
		match                               bool
	}{
		{"pipeline zero", "curl https://graph.example/query | grep -c absent", `{"stdout":"0","exit_code":1}`, "unknown", "", true},
		{"printed URL", "echo https://graph.example/query; false", `{"exit_code":1}`, "", "", false},
		{"HTTP408", "curl https://graph.example/query", `{"status_code":408}`, "failure", "timeout", true},
		{"curl timeout", "curl https://graph.example/query", `{"stderr":"curl: (28) Operation timed out","exit_code":28}`, "failure", "timeout", true},
		{"semantic positive", "curl https://graph.example/query", `{"stdout":"{\"error\":\"parse error\"}"}`, "failure", "parse", true},
		{"successful empty", "curl https://graph.example/query", `{"stdout":"{\"rows\":[],\"count\":0}"}`, "success", "", true},
		{"header URL", "curl -H 'X-Link: https://graph.example/query' https://other.example/query", `{}`, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := json.Marshal(map[string]string{"command": tc.command})
			f := &source.Fields{ToolName: "Bash", InstanceID: "session", ToolInput: input, Error: "command exited nonzero", Extra: map[string]json.RawMessage{"tool_response": json.RawMessage(tc.body)}}
			got, match := Prepare(f, []string{"graph.example"}, "")
			if match != tc.match {
				t.Fatalf("match=%v", match)
			}
			if !match {
				return
			}
			var ev Event
			json.Unmarshal(got.Extra["quipu_review"], &ev)
			if ev.Outcome != tc.outcome || ev.ErrorClass != tc.class || ev.RetryID != "" {
				t.Fatalf("unexpected evidence %+v", ev)
			}
		})
	}
	patterns := map[string]bool{}
	for _, body := range []string{`{"query":"SELECT ?s WHERE {?s ?p ?o}"}`, `{"query":"ASK {?s ?p ?o}"}`} {
		input, _ := json.Marshal(map[string]string{"command": "curl --json '" + body + "' https://graph.example/query"})
		got, _ := Prepare(&source.Fields{ToolName: "Bash", ToolInput: input}, []string{"graph.example"}, "")
		var ev Event
		json.Unmarshal(got.Extra["quipu_review"], &ev)
		patterns[ev.InputPattern] = true
	}
	if len(patterns) != 2 {
		t.Fatal("distinct REST inputs collapsed")
	}
}

func TestEmptyErrorAndTopLevelContent(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"error":{},"rows":[]}`, "success"},
		{`[{"type":"text","text":"{\"error\":\"query exceeded budget\"}"}]`, "failure"},
		{`[{"type":"text","text":"{\"rows\":[],\"count\":0}"}]`, "success"},
	} {
		var v any
		json.Unmarshal([]byte(tc.body), &v)
		got, _ := response(v, 0)
		if got != tc.want {
			t.Fatalf("%s: %s", tc.body, got)
		}
	}
}

// Fixture emitted by Claude Code 2.1.295 PostToolUse, not a handbuilt envelope.
// Tool name/input and content-block structure are unchanged; identifiers redacted.
func TestOwnedClaudeHookEnvelope(t *testing.T) {
	raw, err := os.ReadFile("testdata/owned-claude-hook.json")
	if err != nil {
		t.Fatal(err)
	}
	plugin := source.Get("claude-code")
	if plugin == nil {
		t.Fatal("claude-code source unavailable")
	}
	fields, err := plugin.Extract(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := Prepare(fields, nil, "")
	if !ok {
		t.Fatal("actual hook unrecognized")
	}
	var ev Event
	json.Unmarshal(got.Extra["quipu_review"], &ev)
	if ev.Operation != "query" || ev.Outcome != "success" || fields.ToolName != "mcp__example__quipu_query" {
		t.Fatalf("actual hook %+v", ev)
	}
}
