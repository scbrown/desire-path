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
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Defaults for the payload arm. The cap is a context budget, not a display
// preference: an injected result competes for the same window the agent is
// reasoning in, and a signpost that costs more than the search it replaces is
// not an improvement.
const (
	DefaultPayloadHits      = 3
	DefaultPayloadMaxBytes  = 800
	defaultSnippetChars     = 120
	defaultFallbackDeadline = 40 * time.Millisecond
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

	// Payload forces payload mode on any emitting condition. The payload
	// conditions set it implicitly; the field exists so a fleet deployment can
	// turn one on without renaming its condition.
	Payload         bool
	PayloadHits     int
	PayloadMaxBytes int
}

// Event is the stable JSONL contract consumed by the evaluation harness.
type Event struct {
	EventID           string    `json:"event_id"`
	Timestamp         time.Time `json:"timestamp"`
	SessionID         string    `json:"session_id,omitempty"`
	TaskID            string    `json:"task_id,omitempty"`
	ModelFamily       string    `json:"model_family,omitempty"`
	Condition         string    `json:"condition"`
	Tool              string    `json:"tool"`
	Command           string    `json:"command"`
	CWD               string    `json:"cwd,omitempty"`
	Query             string    `json:"query"`
	IntentFamily      string    `json:"intent_family"`
	StackCommand      string    `json:"stack_command,omitempty"`
	ResultCardinality int       `json:"result_cardinality"`
	Threshold         int       `json:"threshold"`
	Predicate         string    `json:"predicate"`
	// HookEvent is which event the payload came from, and it QUALIFIES
	// SignpostShown rather than decorating it.
	//
	// SignpostShown means "this hook emitted context", which on PostToolUse is
	// the same thing as the model receiving it. On PostToolUseFailure it is
	// NOT: measured on a live crew pane, additionalContext emitted from that
	// event never reaches the model. Two independent hooks confirm it, each
	// proven to have run by its own side effect — dp signpost wrote an event
	// row with signpost_shown true, dp pave-correct recorded a correction
	// desire — and neither one's text appeared in the session.
	//
	// So a row with hook_event=PostToolUseFailure is evidence the hook FIRED,
	// and is not evidence of delivery. Do not count those rows as shown when
	// measuring adoption; that conflation is the exact mistake that let "wired
	// on every pane" stand for weeks as a claim about delivery.
	HookEvent              string   `json:"hook_event"`
	SignpostShown          bool     `json:"signpost_shown"`
	PayloadMode            bool     `json:"payload_mode"`
	PayloadBytes           int      `json:"payload_bytes"`
	PayloadPaths           []string `json:"payload_paths,omitempty"`
	SemanticCandidateCount int      `json:"semantic_candidate_count"`
	SemanticLatencyMS      int64    `json:"semantic_latency_ms"`
	WarmCacheHit           bool     `json:"warm_cache_hit"`
	Adopted                *bool    `json:"adopted"`
	TurnsToLocate          *int     `json:"turns_to_locate"`
	TokensToResolution     *int     `json:"tokens_to_resolution"`
	Correct                *bool    `json:"correct"`
}

type payload struct {
	SessionID    string          `json:"session_id"`
	ToolUseID    string          `json:"tool_use_id"`
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
	Error        string          `json:"error"`
	CWD          string          `json:"cwd"`
}

// hookEventName reports which event this payload came from. A failed tool call
// carries an error and no response.
//
// THIS MATTERS MORE THAN IT LOOKS. A literal search that finds NOTHING exits
// non-zero, so in production it is routed to PostToolUseFailure — and the null
// predicate, the most valuable trigger signposting has ("your search found
// nothing, try a semantic one"), was therefore unreachable on every crew pane.
// Measured: the identical query run twice, once bare (exit 1) and once with
// `|| true` (exit 0), produced ONE event, from the exit-0 run.
func (p payload) hookEventName() string {
	if len(p.ToolResponse) == 0 && p.Error != "" {
		return "PostToolUseFailure"
	}
	return "PostToolUse"
}

// Hit is one stack answer: where it is, and enough of it to judge without
// opening the file.
type Hit struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet,omitempty"`
}

// Result is what the stack returned for one intent.
type Result struct {
	Count int   `json:"count"`
	Hits  []Hit `json:"hits,omitempty"`
}

type cachedResult struct {
	Family string `json:"family,omitempty"`
	Count  int    `json:"count"`
	Hits   []Hit  `json:"hits,omitempty"`
}

type hookOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// Searcher answers one intent from the Quipu stack. It replaced a
// query-string-in, count-out function when payload mode landed: pointing at a
// command needs a count, RUNNING it needs the result.
type Searcher func(context.Context, Intent) (Result, error)

// behavior decides, from the condition name alone, whether this hook emits and
// in which mode. Conditions absent from the table never emit — the baseline
// arms and anything unrecognised, which fails closed.
var behavior = map[string]struct{ gated, payload bool }{
	"always-signpost":        {gated: false, payload: false},
	"gated-signpost":         {gated: true, payload: false},
	"payload-signpost":       {gated: false, payload: true},
	"payload-gated-signpost": {gated: true, payload: true},
}

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
	intent, ok := DiscoverIntent(input.Command)
	if !ok || intent.Query == "" {
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
		TaskID: cfg.TaskID, ModelFamily: cfg.Model, Condition: cfg.Condition, Tool: intent.Tool,
		Command: input.Command, CWD: p.CWD, Query: intent.Query, IntentFamily: intent.Family,
		ResultCardinality: cardinality, Threshold: cfg.Threshold, Predicate: predicate,
		HookEvent: p.hookEventName()}

	mode, emits := behavior[cfg.Condition]
	if !emits || (mode.gated && predicate == "none") {
		return nil, e, nil
	}
	usePayload := mode.payload || cfg.Payload
	e.PayloadMode = usePayload

	started := time.Now()
	cached, hit := warmResult(cfg.CacheDir, CacheKey(cfg.Repo, intent.Query))
	var result Result
	var err error
	if hit {
		if cached.Family != "" {
			intent.Family = cached.Family
		}
		result = Result{Count: cached.Count, Hits: cached.Hits}
	} else {
		intent, result, err = Resolve(ctx, intent, search)
	}
	e.SemanticLatencyMS = time.Since(started).Milliseconds()
	e.WarmCacheHit = hit
	e.IntentFamily = intent.Family
	if err != nil || result.Count == 0 {
		return nil, e, nil
	}
	e.SemanticCandidateCount = result.Count
	e.SignpostShown = true
	command := intent.StackCommand(cfg.Repo)
	e.StackCommand = command

	var injected string
	if usePayload {
		var paths []string
		injected, paths = renderPayload(intent, predicate, cardinality, command, result, cfg)
		e.PayloadPaths = paths
		e.PayloadBytes = len(injected)
	} else {
		injected = fmt.Sprintf("Signpost (%s): %s returned %d line(s). %s: %s",
			predicate, intent.Asks(), cardinality, intent.Invite(), command)
	}

	var out hookOutput
	out.HookSpecificOutput.HookEventName = p.hookEventName()
	out.HookSpecificOutput.AdditionalContext = injected
	b, err := json.Marshal(out)
	return b, e, err
}

// Resolve answers an intent, promoting a symbol candidate to FamilySymbol only
// when the index resolves it. An identifier that resolves nowhere is just a
// string, and must be searched as one — but only if the remaining budget can
// pay for a second round trip, because the 150 ms contract outranks the
// upgrade.
func Resolve(ctx context.Context, in Intent, search Searcher) (Intent, Result, error) {
	if search == nil {
		return in, Result{}, nil
	}
	if in.SymbolCandidate != "" {
		sym := in
		sym.Family = FamilySymbol
		sym.Query = in.SymbolCandidate
		res, err := search(ctx, sym)
		if err == nil && res.Count > 0 {
			return sym, res, nil
		}
		if !budgetRemains(ctx, defaultFallbackDeadline) {
			return sym, Result{}, err
		}
	}
	res, err := search(ctx, in)
	return in, res, err
}

func budgetRemains(ctx context.Context, need time.Duration) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(deadline) >= need
}

// renderPayload injects the ANSWER instead of the command. It returns the
// context text and the paths it named, which is what the harness scores
// adoption against: taking a payload means using a location it handed over,
// which is a different act from running a command and must not be summed with
// pointer adoption.
func renderPayload(in Intent, predicate string, cardinality int, command string, result Result, cfg Config) (string, []string) {
	maxHits := cfg.PayloadHits
	if maxHits <= 0 {
		maxHits = DefaultPayloadHits
	}
	maxBytes := cfg.PayloadMaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultPayloadMaxBytes
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Signpost (%s): %s returned %d line(s). %s answers it:",
		predicate, in.Asks(), cardinality, command)
	var paths []string
	for i, h := range result.Hits {
		if i >= maxHits {
			break
		}
		line := "\n  " + h.Path
		if h.Line > 0 {
			line += fmt.Sprintf(":%d", h.Line)
		}
		if h.Snippet != "" {
			line += "  " + truncate(oneLine(h.Snippet), defaultSnippetChars)
		}
		if b.Len()+len(line) > maxBytes {
			break
		}
		b.WriteString(line)
		paths = append(paths, h.Path)
	}
	if len(paths) == 0 {
		// Nothing fit, or the backend returned a count with no locations. A
		// payload arm with no payload is a pointer, and says so rather than
		// emitting a header with an empty body.
		return fmt.Sprintf("Signpost (%s): %s returned %d line(s). %s: %s",
			predicate, in.Asks(), cardinality, in.Invite(), command), nil
	}
	if result.Count > len(paths) {
		fmt.Fprintf(&b, "\n  (%d of %d; run %s for the rest)", len(paths), result.Count, command)
	}
	return b.String(), paths
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// PrefetchRequest extracts the stable join key and semantic query from a
// PreToolUse payload. It performs no search and never changes hook behavior.
func PrefetchRequest(raw []byte) (string, Intent, bool) {
	var p payload
	if json.Unmarshal(raw, &p) != nil || p.ToolName != "Bash" || p.ToolUseID == "" {
		return "", Intent{}, false
	}
	var input struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(p.ToolInput, &input) != nil {
		return "", Intent{}, false
	}
	intent, ok := DiscoverIntent(input.Command)
	return p.ToolUseID, intent, ok && intent.Query != ""
}

// CacheKey identifies reusable semantic work without coupling it to one hook
// invocation. Repo scoping prevents same-text queries crossing corpora. The
// family is NOT part of the key: the prefetch resolves the family the same way
// Process would and records it in the entry, so PostToolUse can adopt a
// resolution it could not have predicted before looking.
func CacheKey(repo, query string) string {
	sum := sha256.Sum256([]byte(repo + "\x00" + query))
	return fmt.Sprintf("%x", sum[:16])
}

// WriteWarmResult atomically publishes a speculative result for PostToolUse.
func WriteWarmResult(dir, id, family string, result Result) error {
	if dir == "" || id == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(cachedResult{Family: family, Count: result.Count, Hits: result.Hits})
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

func warmResult(dir, id string) (cachedResult, bool) {
	if dir == "" || id == "" {
		return cachedResult{}, false
	}
	path := warmPath(dir, id)
	b, err := os.ReadFile(path)
	if err != nil {
		return cachedResult{}, false
	}
	_ = os.Remove(path)
	var result cachedResult
	if json.Unmarshal(b, &result) == nil {
		return result, true
	}
	return cachedResult{}, false
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

// HTTPSearcher builds a latency-bounded stack search function. mode selects
// the backend's search mode; empty leaves the backend default in place, which
// is what the hook ships with.
//
// The parameter exists so an arm can STATE its mode rather than inherit one.
// Measured against a warm resident index, 40 distinct workload queries, the two
// modes interleaved to cancel warm-up: hybrid p50 98 ms, semantic p50 93 ms,
// identical candidate counts. An earlier unpaired reading of 330 ms vs 106 ms
// was an ordering artifact of a cold server and is not a reason to change the
// default.
//
// endpoint names the /search route; sibling routes (/refs) are derived from it,
// so one configured URL still describes the whole backend.
func HTTPSearcher(endpoint, repo, mode string, timeout time.Duration) Searcher {
	client := &http.Client{Timeout: timeout}
	return func(ctx context.Context, in Intent) (Result, error) {
		if in.Family == FamilySymbol {
			return refsQuery(ctx, client, sibling(endpoint, "refs"), repo, in.Query)
		}
		return searchQuery(ctx, client, endpoint, repo, mode, in)
	}
}

// sibling replaces the last path segment of endpoint with name. A backend that
// answers /search answers /refs beside it; deriving the route keeps one
// configured URL rather than one per family, and an endpoint that is not a
// route at all is returned unchanged rather than mangled.
func sibling(endpoint, name string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Path == "" || u.Path == "/" {
		return endpoint
	}
	u.Path = path.Join(path.Dir(u.Path), name)
	return u.String()
}

func searchQuery(ctx context.Context, client *http.Client, endpoint, repo, mode string, in Intent) (Result, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return Result{}, err
	}
	q := u.Query()
	q.Set("q", in.Query)
	q.Set("limit", "3")
	if repo != "" {
		q.Set("repo", repo)
	}
	if mode != "" {
		q.Set("mode", mode)
	}
	if in.Family == FamilyHistory {
		q.Set("type", "commit")
	}
	u.RawQuery = q.Encode()
	body, err := get(ctx, client, u.String())
	if err != nil {
		return Result{}, err
	}
	var resp struct {
		Count   int `json:"count"`
		Results []struct {
			FilePath       string `json:"file_path"`
			Name           string `json:"name"`
			StartLine      int    `json:"start_line"`
			ContentPreview string `json:"content_preview"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return Result{}, err
	}
	out := Result{Count: resp.Count}
	for _, r := range resp.Results {
		snippet := r.ContentPreview
		if snippet == "" {
			snippet = r.Name
		}
		out.Hits = append(out.Hits, Hit{Path: r.FilePath, Line: r.StartLine, Snippet: snippet})
	}
	return out, nil
}

// refsQuery asks the structural route whether a name resolves. Count is the
// definition plus its usages: a name with neither is not a symbol, which is
// exactly the negative this family needs to fail on.
func refsQuery(ctx context.Context, client *http.Client, endpoint, repo, symbol string) (Result, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return Result{}, err
	}
	q := u.Query()
	q.Set("symbol", symbol)
	if repo != "" {
		q.Set("repo", repo)
	}
	u.RawQuery = q.Encode()
	body, err := get(ctx, client, u.String())
	if err != nil {
		return Result{}, err
	}
	var resp struct {
		Definition *struct {
			FilePath  string `json:"file_path"`
			StartLine int    `json:"start_line"`
			Signature string `json:"signature"`
		} `json:"definition"`
		UsageCount int `json:"usage_count"`
		Usages     []struct {
			FilePath string `json:"file_path"`
			Line     int    `json:"line"`
			Context  string `json:"context"`
		} `json:"usages"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return Result{}, err
	}
	var out Result
	if resp.Definition != nil {
		out.Count++
		out.Hits = append(out.Hits, Hit{Path: resp.Definition.FilePath, Line: resp.Definition.StartLine, Snippet: resp.Definition.Signature})
	}
	out.Count += resp.UsageCount
	for _, u := range resp.Usages {
		out.Hits = append(out.Hits, Hit{Path: u.FilePath, Line: u.Line, Snippet: u.Context})
	}
	return out, nil
}

func get(ctx context.Context, client *http.Client, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bobbin status %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
