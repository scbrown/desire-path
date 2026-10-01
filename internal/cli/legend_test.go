package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/legend"
	"github.com/scbrown/desire-path/internal/signpost"
)

func legendEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gz := filepath.Join(dir, "g.json")
	if err := legend.Write(gz, legend.File{Generated: time.Now().UTC(), Entries: []legend.Entry{
		{IRI: "ex:build-01", Label: "build-01.example", Type: "Host"},
		{IRI: "ex:bobbin-webhook", Label: "bobbin-webhook", Type: "SystemdService"},
	}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DP_LEGEND_GAZETTEER", gz)
	t.Setenv("DP_LEGEND_LOG", filepath.Join(dir, "events.jsonl"))
	t.Setenv("DP_SIGNPOST_CACHE_DIR", filepath.Join(dir, "cache"))
	return dir
}

func readRaw(session, content string) []byte {
	b, _ := json.Marshal(map[string]any{
		"session_id": session, "tool_name": "Read",
		"tool_input":    map[string]string{"file_path": "/x/notes.md"},
		"tool_response": map[string]any{"file": map[string]string{"content": content}},
	})
	return b
}

func TestWithLegendOffByDefault(t *testing.T) {
	dir := legendEnv(t)
	t.Setenv("DP_LEGEND", "")
	if out := withLegend(readRaw("s", "build-01.example"), nil); out != nil {
		t.Fatalf("flag off must add nothing, got %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "events.jsonl")); err == nil {
		t.Fatal("flag off must not even log")
	}
}

func TestWithLegendAloneAndLogged(t *testing.T) {
	dir := legendEnv(t)
	t.Setenv("DP_LEGEND", "1")
	out := withLegend(readRaw("s", "build-01.example"), nil)
	ev, text := signpost.ContextOf(out)
	if ev != "PostToolUse" || text != "Quipu entities here: build-01.example (Host)" {
		t.Fatalf("got event=%q text=%q", ev, text)
	}
	log, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil || !strings.Contains(string(log), `"shown":["build-01.example"]`) {
		t.Fatalf("decision must be logged: %v %s", err, log)
	}
}

func TestWithLegendSharesTheBudgetWithSignpost(t *testing.T) {
	legendEnv(t)
	t.Setenv("DP_LEGEND", "1")
	t.Setenv("DP_LEGEND_MAX_BYTES", "100")
	raw := readRaw("s", "build-01.example and bobbin-webhook")

	// A signpost stage that already spent 50 of 100 bytes leaves 50: the
	// header (21) plus "build-01.example (Host)" (23) fits, the second entity does not.
	prior := strings.Repeat("p", 50)
	sp, _ := signpost.WithContext("PostToolUse", prior)
	_, text := signpost.ContextOf(withLegend(raw, sp))
	want := prior + "\nQuipu entities here: build-01.example (Host)"
	if text != want {
		t.Fatalf("got %q, want %q", text, want)
	}

	// A signpost that spent the whole budget leaves the legend nothing, and
	// its own output must come back byte-identical.
	full, _ := signpost.WithContext("PostToolUse", strings.Repeat("p", 100))
	if got := withLegend(readRaw("s2", "build-01.example"), full); string(got) != string(full) {
		t.Fatalf("signpost output must be untouched when no budget remains: %s", got)
	}
}

func TestWithLegendFailsOpenOnMissingGazetteer(t *testing.T) {
	legendEnv(t)
	t.Setenv("DP_LEGEND", "1")
	t.Setenv("DP_LEGEND_GAZETTEER", filepath.Join(t.TempDir(), "missing.json"))
	sp, _ := signpost.WithContext("PostToolUse", "signpost text")
	if got := withLegend(readRaw("s", "build-01.example"), sp); string(got) != string(sp) {
		t.Fatalf("a missing gazetteer must leave the output untouched: %s", got)
	}
}
