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

// countSearcher answers a retrieval intent with n candidates and no locations,
// which is the pointer arm's whole requirement. It resolves NO symbols, so a
// query that merely looks like an identifier is demoted back to a literal
// search — the default a test should be written against, since promotion is a
// property of the index and not of the parse.
func countSearcher(n int) Searcher {
	return func(_ context.Context, in Intent) (Result, error) {
		if in.Family == FamilySymbol {
			return Result{}, nil
		}
		return Result{Count: n}, nil
	}
}

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
		{"unknown condition fails closed", "", "some-new-arm", 2, false, "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := `{"session_id":"s1","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"rg 'retry backoff' ."},"tool_response":` + string(mustJSON(tt.response)) + `}`
			cfg := Config{Threshold: 2, Timeout: time.Millisecond, Condition: tt.condition}
			out, event, err := Process(context.Background(), []byte(raw), cfg, countSearcher(tt.count))
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
	out, _, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "gated-signpost", Repo: "desire-path"}, countSearcher(1))
	if err != nil || !strings.Contains(string(out), "bobbin search --repo 'desire-path' 'missing'") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

// The pointer arm must keep naming a COMMAND and nothing else: the moment it
// carries results it is the payload arm under a pointer arm's label, and the
// comparison the campaign exists to make is gone.
func TestPointerArmCarriesNoResults(t *testing.T) {
	raw := []byte(`{"tool_name":"Bash","tool_input":{"command":"rg widget ."},"tool_response":""}`)
	search := func(_ context.Context, in Intent) (Result, error) {
		if in.Family == FamilySymbol {
			return Result{}, nil
		}
		return Result{Count: 2, Hits: []Hit{{Path: "internal/widget.go", Line: 12, Snippet: "func Widget()"}}}, nil
	}
	out, event, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "always-signpost"}, search)
	if err != nil {
		t.Fatal(err)
	}
	if event.IntentFamily != FamilyLiteral {
		t.Fatalf("family=%q", event.IntentFamily)
	}
	if strings.Contains(string(out), "internal/widget.go") || event.PayloadMode || event.PayloadBytes != 0 {
		t.Fatalf("pointer arm leaked a payload: out=%q event=%+v", out, event)
	}
}

func TestPayloadArmInjectsResultsAndRecordsPaths(t *testing.T) {
	raw := []byte(`{"tool_name":"Bash","tool_input":{"command":"rg widget ."},"tool_response":""}`)
	search := func(_ context.Context, in Intent) (Result, error) {
		if in.Family == FamilySymbol {
			return Result{}, nil
		}
		return Result{Count: 9, Hits: []Hit{
			{Path: "internal/widget.go", Line: 12, Snippet: "func Widget() error {"},
			{Path: "internal/widget_test.go", Line: 4, Snippet: "func TestWidget(t *testing.T) {"},
		}}, nil
	}
	out, event, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "payload-signpost"}, search)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{"internal/widget.go:12", "func Widget() error {", "2 of 9"} {
		if !strings.Contains(text, want) {
			t.Fatalf("payload missing %q: %s", want, text)
		}
	}
	if !event.PayloadMode || event.PayloadBytes == 0 || len(event.PayloadPaths) != 2 {
		t.Fatalf("event=%+v", event)
	}
}

// A cap that is not enforced is a cap that will be discovered in production by
// a context window rather than by a test.
func TestPayloadRespectsSizeCap(t *testing.T) {
	long := strings.Repeat("x", 4000)
	search := func(_ context.Context, in Intent) (Result, error) {
		if in.Family == FamilySymbol {
			return Result{}, nil
		}
		return Result{Count: 3, Hits: []Hit{{Path: "a.go", Line: 1, Snippet: long}, {Path: "b.go", Line: 2, Snippet: long}}}, nil
	}
	raw := []byte(`{"tool_name":"Bash","tool_input":{"command":"rg widget ."},"tool_response":""}`)
	_, event, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "payload-signpost", PayloadMaxBytes: 300}, search)
	if err != nil {
		t.Fatal(err)
	}
	if event.PayloadBytes > 300 {
		t.Fatalf("payload %d bytes exceeds the 300 byte cap", event.PayloadBytes)
	}
}

// A count with no locations is the backend saying "there are answers" without
// saying where. The payload arm must degrade to a pointer rather than emit an
// empty answer, which would read to the model as a failed search.
func TestPayloadWithoutLocationsDegradesToPointer(t *testing.T) {
	raw := []byte(`{"tool_name":"Bash","tool_input":{"command":"rg widget ."},"tool_response":""}`)
	out, event, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "payload-signpost"}, countSearcher(4))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Try semantic search: bobbin search 'widget'") || len(event.PayloadPaths) != 0 {
		t.Fatalf("out=%q event=%+v", out, event)
	}
}

// The symbol family is promoted by RESOLUTION, not by the parse.
func TestSymbolPromotionRequiresResolution(t *testing.T) {
	raw := []byte(`{"tool_name":"Bash","tool_input":{"command":"grep -rn Widget ."},"tool_response":""}`)
	t.Run("resolves", func(t *testing.T) {
		search := func(_ context.Context, in Intent) (Result, error) {
			if in.Family != FamilySymbol {
				t.Fatalf("expected the symbol lookup first, got %q", in.Family)
			}
			return Result{Count: 1, Hits: []Hit{{Path: "internal/widget.go", Line: 12}}}, nil
		}
		out, event, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "always-signpost"}, search)
		if err != nil || event.IntentFamily != FamilySymbol || !strings.Contains(string(out), "yupana callers 'Widget'") {
			t.Fatalf("out=%q event=%+v err=%v", out, event, err)
		}
	})
	t.Run("does not resolve", func(t *testing.T) {
		var families []string
		search := func(_ context.Context, in Intent) (Result, error) {
			families = append(families, in.Family)
			if in.Family == FamilySymbol {
				return Result{}, nil
			}
			return Result{Count: 2}, nil
		}
		out, event, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "always-signpost"}, search)
		if err != nil || event.IntentFamily != FamilyLiteral || !strings.Contains(string(out), "bobbin search 'Widget'") {
			t.Fatalf("out=%q event=%+v err=%v", out, event, err)
		}
		if len(families) != 2 || families[0] != FamilySymbol || families[1] != FamilyLiteral {
			t.Fatalf("resolution order=%v", families)
		}
	})
}

// The fallback second round trip must not be taken on a budget that cannot pay
// for it: the 150 ms contract outranks the family upgrade.
func TestSymbolFallbackSkippedWhenBudgetIsSpent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	calls := 0
	search := func(_ context.Context, in Intent) (Result, error) {
		calls++
		return Result{}, nil
	}
	raw := []byte(`{"tool_name":"Bash","tool_input":{"command":"grep -rn Widget ."},"tool_response":""}`)
	out, _, err := Process(ctx, raw, Config{Threshold: 2, Condition: "always-signpost"}, search)
	if err != nil || len(out) != 0 {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if calls != 1 {
		t.Fatalf("made %d lookups on a spent budget, want 1", calls)
	}
}

func TestHTTPSearcherScopesRequestToRepo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("repo") != "desire-path" || r.URL.Query().Get("q") != "semantic query" {
			t.Fatalf("query=%v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"count":2,"results":[{"file_path":"a.go","start_line":3,"content_preview":"x"}]}`))
	}))
	defer server.Close()
	got, err := HTTPSearcher(server.URL+"/search", "desire-path", "", time.Second)(context.Background(), Intent{Family: FamilyLiteral, Query: "semantic query"})
	if err != nil || got.Count != 2 || len(got.Hits) != 1 || got.Hits[0].Path != "a.go" || got.Hits[0].Line != 3 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

// The history family is a chunk-type filter on the same route, and the symbol
// family is a different route derived from it. Both are contract with the
// backend, so both are asserted rather than assumed.
func TestHTTPSearcherRoutesPerFamily(t *testing.T) {
	var gotPath, gotType, gotSymbol string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotType, gotSymbol = r.URL.Path, r.URL.Query().Get("type"), r.URL.Query().Get("symbol")
		if r.URL.Path == "/refs" {
			_, _ = w.Write([]byte(`{"definition":{"file_path":"internal/widget.go","start_line":12,"signature":"func Widget()"},"usage_count":2,"usages":[{"file_path":"a.go","line":1,"context":"Widget()"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"count":1,"results":[{"file_path":"git:abc","start_line":0,"content_preview":"fix"}]}`))
	}))
	defer server.Close()
	search := HTTPSearcher(server.URL+"/search", "beads", "", time.Second)

	if _, err := search(context.Background(), Intent{Family: FamilyHistory, Query: "timeout"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/search" || gotType != "commit" {
		t.Fatalf("history went to %s type=%q", gotPath, gotType)
	}

	got, err := search(context.Background(), Intent{Family: FamilySymbol, Query: "Widget"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/refs" || gotSymbol != "Widget" {
		t.Fatalf("symbol went to %s symbol=%q", gotPath, gotSymbol)
	}
	if got.Count != 3 || len(got.Hits) != 2 || got.Hits[0].Line != 12 {
		t.Fatalf("refs result=%+v", got)
	}
}

// An unresolvable name must come back as zero, not as an error and not as a
// definition-shaped nothing: zero is what demotes the family back to literal.
func TestHTTPSearcherSymbolMissIsZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"symbol":"nope","usage_count":0,"usages":[]}`))
	}))
	defer server.Close()
	got, err := HTTPSearcher(server.URL+"/search", "", "", time.Second)(context.Background(), Intent{Family: FamilySymbol, Query: "nope"})
	if err != nil || got.Count != 0 || len(got.Hits) != 0 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

// An unset mode must leave the parameter off the request entirely rather than
// send an empty one, which a backend may read as an unsupported mode: the
// shipped hook sends no mode at all, so this is the default path.
func TestHTTPSearcherSendsSearchModeOnlyWhenSet(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{{"semantic", "semantic"}, {"hybrid", "hybrid"}, {"", ""}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("mode"); got != tc.want {
				t.Errorf("mode=%q want %q", got, tc.want)
			}
			if tc.mode == "" && r.URL.Query().Has("mode") {
				t.Errorf("empty mode must not be sent: %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"count":1}`))
		}))
		if _, err := HTTPSearcher(server.URL+"/search", "", tc.mode, time.Second)(context.Background(), Intent{Family: FamilyLiteral, Query: "q"}); err != nil {
			t.Fatalf("mode %q: %v", tc.mode, err)
		}
		server.Close()
	}
}

func TestProcessTimeoutDropsAnnotation(t *testing.T) {
	raw := []byte(`{"tool_name":"Bash","tool_input":{"command":"grep retries ."},"tool_response":""}`)
	out, event, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "gated-signpost"}, func(context.Context, Intent) (Result, error) {
		return Result{}, errors.New("timeout")
	})
	if err != nil || len(out) != 0 || event.SignpostShown {
		t.Fatalf("out=%q event=%+v err=%v", out, event, err)
	}
}

func TestWarmPrefetchFeedsPostToolUseWithinBudget(t *testing.T) {
	dir := t.TempDir()
	pre := []byte(`{"tool_use_id":"tool-7","tool_name":"Bash","tool_input":{"command":"rg 'retry backoff' ."}}`)
	id, intent, ok := PrefetchRequest(pre)
	if !ok || id != "tool-7" || intent.Query != "retry backoff" {
		t.Fatalf("id=%q intent=%+v ok=%v", id, intent, ok)
	}
	if err := WriteWarmResult(dir, CacheKey("", intent.Query), FamilyLiteral, Result{Count: 3}); err != nil {
		t.Fatal(err)
	}
	post := []byte(`{"tool_use_id":"tool-7","tool_name":"Bash","tool_input":{"command":"rg 'retry backoff' ."},"tool_response":""}`)
	out, event, err := Process(context.Background(), post, Config{Threshold: 2, Condition: "gated-signpost", CacheDir: dir}, nil)
	if err != nil || len(out) == 0 || !event.WarmCacheHit || event.SemanticCandidateCount != 3 {
		t.Fatalf("out=%q event=%+v err=%v", out, event, err)
	}
}

// The prefetch resolves the family on the 5 s budget; PostToolUse must adopt
// that resolution rather than re-derive it from the parse, which is the only
// way the symbol family can be delivered inside 150 ms.
func TestWarmResultCarriesTheResolvedFamily(t *testing.T) {
	dir := t.TempDir()
	if err := WriteWarmResult(dir, CacheKey("", "Widget"), FamilySymbol,
		Result{Count: 1, Hits: []Hit{{Path: "internal/widget.go", Line: 12}}}); err != nil {
		t.Fatal(err)
	}
	post := []byte(`{"tool_name":"Bash","tool_input":{"command":"grep -rn Widget ."},"tool_response":""}`)
	out, event, err := Process(context.Background(), post, Config{Threshold: 2, Condition: "gated-signpost", CacheDir: dir}, nil)
	if err != nil || event.IntentFamily != FamilySymbol || !strings.Contains(string(out), "yupana callers 'Widget'") {
		t.Fatalf("out=%q event=%+v err=%v", out, event, err)
	}
}

func TestWarmMissDropsAtContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	post := []byte(`{"tool_use_id":"missing","tool_name":"Bash","tool_input":{"command":"rg absent ."},"tool_response":""}`)
	out, event, err := Process(ctx, post, Config{Threshold: 2, Condition: "gated-signpost", CacheDir: t.TempDir()}, func(ctx context.Context, _ Intent) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
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

// A literal search that finds NOTHING exits non-zero, so production routes it
// to PostToolUseFailure — which means the null predicate, the most valuable
// trigger signposting has, is unreachable unless the hook is installed there
// too. The payload shape differs (an error, no response) and must still gate to
// "null", and the emitted event name must match the event it came from.
func TestFailedSearchIsTheNullPredicate(t *testing.T) {
	raw := []byte(`{"tool_name":"Bash","tool_input":{"command":"grep -rn missing ."},"error":"Exit code 1"}`)
	out, event, err := Process(context.Background(), raw, Config{Threshold: 2, Condition: "gated-signpost"}, countSearcher(3))
	if err != nil {
		t.Fatal(err)
	}
	if event.Predicate != "null" || !event.SignpostShown {
		t.Fatalf("event=%+v", event)
	}
	var got hookOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.HookSpecificOutput.HookEventName != "PostToolUseFailure" {
		t.Fatalf("event name = %q, want PostToolUseFailure", got.HookSpecificOutput.HookEventName)
	}

	// A successful call must still say PostToolUse.
	ok := []byte(`{"tool_name":"Bash","tool_input":{"command":"grep -rn missing ."},"tool_response":"a\nb\nc"}`)
	out, _, err = Process(context.Background(), ok, Config{Threshold: 2, Condition: "always-signpost"}, countSearcher(3))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.HookSpecificOutput.HookEventName != "PostToolUse" {
		t.Fatalf("event name = %q, want PostToolUse", got.HookSpecificOutput.HookEventName)
	}
}
