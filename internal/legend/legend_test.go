package legend

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testEntries = []Entry{
	{IRI: "ex:build-01", Label: "build-01.example", Type: "Host"},
	{IRI: "ex:bobbin-webhook", Label: "bobbin-webhook", Type: "SystemdService"},
	{IRI: "ex:bobbin-webhook-index", Label: "bobbin-webhook-index", Type: "Script"},
	{IRI: "ex:preflight", Label: "preflight-monitoring-secrets", Type: "Script"},
	{IRI: "ex:a1", Label: "quipu-recycle", Type: "SystemdService"},
	{IRI: "ex:a2", Label: "quipu-recycle", Type: "AlertRule"},
	{IRI: "ex:issue-42", Label: "issue-42", Type: "Incident"},
}

func keys(ms []Match) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Key)
	}
	return out
}

func TestMatchBoundariesAndLongest(t *testing.T) {
	g := Compile(testEntries)
	cases := []struct {
		name, text string
		want       []string
	}{
		{"plain mention", "deployed on build-01.example today", []string{"build-01.example"}},
		{"case-insensitive", "BUILD-01.EXAMPLE rebooted", []string{"build-01.example"}},
		{"word boundary before", "xbuild-01.example", nil},
		{"word boundary after", "build-01.examplex", nil},
		{"hyphen is a word byte", "pre-build-01.example", nil},
		{"longest wins at a position", "restart bobbin-webhook-index now", []string{"bobbin-webhook-index"}},
		{"shorter alone still matches", "restart bobbin-webhook now", []string{"bobbin-webhook"}},
		{"fix 1: filename stem rejected", "run preflight-monitoring-secrets.sh first", nil},
		{"fix 1: sentence period is fine", "ran preflight-monitoring-secrets.", []string{"preflight-monitoring-secrets"}},
		{"first occurrence only, text order", "build-01.example and bobbin-webhook and build-01.example", []string{"build-01.example", "bobbin-webhook"}},
		{"no overlap with an earlier choice", "bobbin-webhook-index", []string{"bobbin-webhook-index"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := keys(g.Match(c.text))
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("Match(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

func TestMatchAmbiguousLabelGroupsIRIs(t *testing.T) {
	ms := Compile(testEntries).Match("the quipu-recycle fired")
	if len(ms) != 1 || len(ms[0].IRIs) != 2 {
		t.Fatalf("want one ambiguous match over 2 IRIs, got %+v", ms)
	}
}

func TestAdmissible(t *testing.T) {
	dict := map[string]bool{"review": true, "code": true, "server": true, "build": true}
	cases := []struct {
		label string
		want  bool
	}{
		{"build-01.example", true},
		{"build", false},      // dictionary word, single token
		{"abc", false},        // too short
		{"github.com", false}, // fix 2: host stoplist
		{"GitHub.com", false},
		{"code-review", false},       // fix 3b: phrase stoplist        // stoplist is case-insensitive
		{"code review", false},       // multi-word, all dictionary words
		{"quipu server", true},       // multi-word with one non-dictionary token
		{"plainword", false},         // single token, not identifier-shaped
		{"camelCase", true},          // CamelCase is identifier-shaped
		{"bobbin-webhook", true},     // contains a hyphen
		{"  bobbin-webhook  ", true}, // trimmed
	}
	for _, c := range cases {
		if got := Admissible(c.label, dict); got != c.want {
			t.Errorf("Admissible(%q) = %v, want %v", c.label, got, c.want)
		}
	}
}

func TestFormulaIsNotAGazetteerClass(t *testing.T) {
	for _, c := range DefaultClasses {
		if c == "Formula" {
			t.Fatal("Formula must be excluded (fix 3: generic phrases such as code-review)")
		}
	}
}

func readPayload(session, path, content string) []byte {
	b, _ := json.Marshal(map[string]any{
		"session_id": session, "tool_name": "Read",
		"tool_input":    map[string]string{"file_path": path},
		"tool_response": map[string]any{"type": "text", "file": map[string]string{"filePath": path, "content": content}},
	})
	return b
}

func bashPayload(session, command, stdout string) []byte {
	b, _ := json.Marshal(map[string]any{
		"session_id": session, "tool_name": "Bash",
		"tool_input":    map[string]string{"command": command},
		"tool_response": map[string]string{"stdout": stdout},
	})
	return b
}

func TestProcessReadAnnotates(t *testing.T) {
	g := Compile(testEntries)
	out, e := Process(readPayload("s1", "/x/notes.md", "build-01.example runs bobbin-webhook"), Config{Budget: 600}, g)
	want := "Quipu entities here: build-01.example (Host), bobbin-webhook (SystemdService)"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	if e.Bytes != len(want) || len(e.Shown) != 2 || e.Skipped != "" || e.Tool != "Read" {
		t.Fatalf("bad event %+v", e)
	}
}

func TestProcessAmbiguousRendering(t *testing.T) {
	out, _ := Process(readPayload("s1", "/x", "quipu-recycle again"), Config{Budget: 600}, Compile(testEntries))
	if out != "Quipu entities here: quipu-recycle (ambiguous 2)" {
		t.Fatalf("got %q", out)
	}
}

func TestProcessBashReads(t *testing.T) {
	g := Compile(testEntries)
	for _, cmd := range []string{
		"br --db /x/beads.db show aegis-xyz",
		"br show aegis-xyz",
		"cat docs/a.md",
		"sed -n 1,40p notes.md",
		"cd /tmp && head -5 a.md",
	} {
		out, e := Process(bashPayload("s", cmd, "on build-01.example"), Config{Budget: 600}, g)
		if out == "" {
			t.Errorf("%q: want a legend, got silence (%s)", cmd, e.Skipped)
		}
	}
	for _, cmd := range []string{"grep -r build .", "ls -la", "go test ./..."} {
		if out, _ := Process(bashPayload("s", cmd, "on build-01.example"), Config{Budget: 600}, g); out != "" {
			t.Errorf("%q is not a read; want silence, got %q", cmd, out)
		}
	}
}

func TestProcessSelfExclusion(t *testing.T) {
	out, _ := Process(bashPayload("s", "br show issue-42", "issue-42: build-01.example"), Config{Budget: 600}, Compile(testEntries))
	if strings.Contains(out, "issue-42") || !strings.Contains(out, "build-01.example") {
		t.Fatalf("the bead must not annotate itself: %q", out)
	}
}

func TestProcessRespectsBudgetAndCap(t *testing.T) {
	g := Compile(testEntries)
	text := "build-01.example bobbin-webhook quipu-recycle preflight-monitoring-secrets"
	// Room for the header and the first entity only.
	budget := len("Quipu entities here: build-01.example (Host)") + 3
	out, e := Process(readPayload("s", "/x", text), Config{Budget: budget}, g)
	if len(out) > budget || out != "Quipu entities here: build-01.example (Host)" {
		t.Fatalf("budget %d: got %q (%d B)", budget, out, len(out))
	}
	if e.RawHits != 4 {
		t.Fatalf("raw hits = %d, want 4", e.RawHits)
	}
	out, _ = Process(readPayload("s", "/x", text), Config{Budget: 600, MaxEntities: 2}, g)
	if strings.Count(out, "(") != 2 {
		t.Fatalf("entity cap 2: got %q", out)
	}
}

func TestProcessSilentPaths(t *testing.T) {
	g := Compile(testEntries)
	cases := []struct {
		name    string
		raw     []byte
		cfg     Config
		g       *Gazetteer
		skipped string
	}{
		{"bad payload", []byte("{"), Config{Budget: 600}, g, "bad-payload"},
		{"not a read", bashPayload("s", "ls", "build-01.example"), Config{Budget: 600}, g, "not-a-read"},
		{"no budget left", readPayload("s", "/x", "build-01.example"), Config{Budget: 10}, g, "no-budget"},
		{"no gazetteer", readPayload("s", "/x", "build-01.example"), Config{Budget: 600}, nil, "no-gazetteer"},
		{"empty text", readPayload("s", "/x", ""), Config{Budget: 600}, g, "empty-text"},
		{"no match", readPayload("s", "/x", "nothing here"), Config{Budget: 600}, g, "no-match"},
	}
	for _, c := range cases {
		out, e := Process(c.raw, c.cfg, c.g)
		if out != "" || e.Skipped != c.skipped {
			t.Errorf("%s: out=%q skipped=%q, want silence/%q", c.name, out, e.Skipped, c.skipped)
		}
	}
}

func TestProcessDedupPerSession(t *testing.T) {
	g := Compile(testEntries)
	cfg := Config{Budget: 600, SeenDir: t.TempDir()}
	first, _ := Process(readPayload("s1", "/a", "build-01.example"), cfg, g)
	again, e := Process(readPayload("s1", "/b", "build-01.example and bobbin-webhook"), cfg, g)
	other, _ := Process(readPayload("s2", "/c", "build-01.example"), cfg, g)
	if first == "" || other == "" {
		t.Fatal("first sighting in each session must annotate")
	}
	if again != "Quipu entities here: bobbin-webhook (SystemdService)" || e.RawHits != 2 {
		t.Fatalf("build-01.example was already shown in s1: got %q (raw %d)", again, e.RawHits)
	}
}

func TestLoadRefusesStaleAndWriteIsReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "g.json")
	now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	if err := Write(path, File{Generated: now.Add(-2 * time.Hour), Entries: testEntries}); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, time.Hour, now); err == nil {
		t.Fatal("a 2h-old gazetteer must be refused under a 1h max age")
	}
	g, err := Load(path, 24*time.Hour, now)
	if err != nil || len(g.Match("build-01.example")) != 1 {
		t.Fatalf("fresh gazetteer should load and match: %v", err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json"), time.Hour, now); err == nil {
		t.Fatal("a missing gazetteer must be an error (the caller stays silent)")
	}
}
