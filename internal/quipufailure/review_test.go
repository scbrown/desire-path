package quipufailure

import (
	"encoding/json"
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
