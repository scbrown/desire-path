package signpost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProcessGatingAndContract(t *testing.T) {
	tests := []struct {
		name, response, condition string
		count                     int
		wantOutput                bool
		wantPredicate             string
	}{
		{"ordinary stays silent", "a\nb", "gated-signpost", 3, false, "none"},
		{"null emits", "", "gated-signpost", 2, true, "null"},
		{"large emits", "a\nb\nc\nd", "gated-signpost", 1, true, "high-cardinality"},
		{"always emits on ordinary result", "a\nb", "always-signpost", 3, true, "none"},
		{"bare baseline silent", "", "bare-literal", 2, false, "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := `{"session_id":"s1","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"rg 'retry backoff' ."},"tool_response":` + string(mustJSON(tt.response)) + `}`
			cfg := Config{Threshold: 2, Timeout: time.Millisecond, Condition: tt.condition}
			out, event, err := Process(context.Background(), []byte(raw), cfg, func(context.Context, string) (int, error) { return tt.count, nil })
			if err != nil {
				t.Fatal(err)
			}
			if (len(out) > 0) != tt.wantOutput {
				t.Fatalf("output=%q", out)
			}
			if event.Predicate != tt.wantPredicate {
				t.Fatalf("predicate=%q", event.Predicate)
			}
			if len(out) > 0 && tt.response != "" && strings.Contains(string(out), tt.response) {
				t.Fatal("signpost materialized literal payload")
			}
		})
	}
}

func TestProcessScopesSignpostToRepo(t *testing.T) {
	raw := []byte(`{"tool_name":"Bash","tool_input":{"command":"rg missing ."},"tool_response":""}`)
	out, _, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "gated-signpost", Repo: "desire-path"}, func(context.Context, string) (int, error) { return 1, nil })
	if err != nil || !strings.Contains(string(out), "bobbin search --repo 'desire-path' 'missing'") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestHTTPSearcherScopesRequestToRepo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("repo") != "desire-path" || r.URL.Query().Get("q") != "semantic query" {
			t.Fatalf("query=%v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"count":2}`))
	}))
	defer server.Close()
	count, err := HTTPSearcher(server.URL, "desire-path", time.Second)(context.Background(), "semantic query")
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestProcessTimeoutDropsAnnotation(t *testing.T) {
	raw := []byte(`{"tool_name":"Bash","tool_input":{"command":"grep retries ."},"tool_response":""}`)
	out, event, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "gated-signpost"}, func(context.Context, string) (int, error) { return 0, errors.New("timeout") })
	if err != nil || len(out) != 0 || event.SignpostShown {
		t.Fatalf("out=%q event=%+v err=%v", out, event, err)
	}
}

func TestWarmPrefetchFeedsPostToolUseWithinBudget(t *testing.T) {
	dir := t.TempDir()
	pre := []byte(`{"tool_use_id":"tool-7","tool_name":"Bash","tool_input":{"command":"rg 'retry backoff' ."}}`)
	id, query, ok := PrefetchRequest(pre)
	if !ok || id != "tool-7" || query != "retry backoff" {
		t.Fatalf("id=%q query=%q ok=%v", id, query, ok)
	}
	if err := WriteWarmResult(dir, CacheKey("", query), 3); err != nil {
		t.Fatal(err)
	}
	post := []byte(`{"tool_use_id":"tool-7","tool_name":"Bash","tool_input":{"command":"rg 'retry backoff' ."},"tool_response":""}`)
	out, event, err := Process(context.Background(), post, Config{Threshold: 2, Condition: "gated-signpost", CacheDir: dir}, nil)
	if err != nil || len(out) == 0 || !event.WarmCacheHit || event.SemanticCandidateCount != 3 {
		t.Fatalf("out=%q event=%+v err=%v", out, event, err)
	}
}

func TestWarmMissDropsAtContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	post := []byte(`{"tool_use_id":"missing","tool_name":"Bash","tool_input":{"command":"rg absent ."},"tool_response":""}`)
	out, event, err := Process(ctx, post, Config{Threshold: 2, Condition: "gated-signpost", CacheDir: t.TempDir()}, func(ctx context.Context, _ string) (int, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	})
	if err != nil || len(out) != 0 || event.SignpostShown || event.WarmCacheHit {
		t.Fatalf("out=%q event=%+v err=%v", out, event, err)
	}
}

func TestNonBashIsOutsideScope(t *testing.T) {
	out, _, err := Process(context.Background(), []byte(`{"tool_name":"Grep","tool_input":{"pattern":"x"}}`), Config{}, nil)
	if err != nil || len(out) != 0 {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func mustJSON(s string) []byte { b, _ := json.Marshal(s); return b }
