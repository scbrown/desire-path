// Package legend annotates what an agent just READ with the Quipu entities it
// names: a one-line legend such as "Quipu entities here: build-01 (Host), search-api
// (Service)". It is a second stage of the PostToolUse signpost hook, not a hook
// of its own, so it spends whatever is left of the one per-call context budget
// (aegis-9qgbff, under sattler's one-pipeline ruling).
//
// The hook path is local only: it reads a gazetteer file written out of band by
// `dp legend-refresh`, and makes no network call. A missing, stale or unreadable
// gazetteer means silence, and so does any error. The stage never blocks a
// tool call and never adds an error to one.
package legend

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Defaults. MaxEntities and MaxBytes are the caps the replay gated on
// (aegis-9qgbff: noise 8% at 5 entities / 600 B per read).
const (
	DefaultMaxEntities = 5
	DefaultMaxBytes    = 600
	DefaultMaxAge      = 24 * time.Hour
	header             = "Quipu entities here: "
)

// Entry is one gazetteer row: a label and the entity it names.
type Entry struct {
	IRI   string `json:"iri"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

// File is the on-disk gazetteer written by Refresh.
type File struct {
	Generated time.Time `json:"generated"`
	Source    string    `json:"source,omitempty"`
	Entries   []Entry   `json:"entries"`
}

// Config controls one stage invocation.
type Config struct {
	GazetteerPath string
	MaxAge        time.Duration
	MaxEntities   int
	// Budget is the bytes this stage may add. The caller computes it as the
	// legend cap minus what earlier stages already spent; <= 0 means silent.
	Budget  int
	SeenDir string // per-session dedup; "" disables dedup
	Now     func() time.Time
}

// Event is one stage decision, logged to its OWN JSONL so the signpost
// evaluation rows are not diluted by Read events they were never about.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id,omitempty"`
	Tool      string    `json:"tool"`
	Ref       string    `json:"ref,omitempty"`
	TextBytes int       `json:"text_bytes"`
	RawHits   int       `json:"raw_hits"`
	Shown     []string  `json:"shown,omitempty"`
	Bytes     int       `json:"bytes"`
	Budget    int       `json:"budget"`
	LatencyUS int64     `json:"latency_us"` // stage total, including LoadUS
	LoadUS    int64     `json:"load_us"`    // reading + compiling the gazetteer
	Skipped   string    `json:"skipped,omitempty"`
}

type payload struct {
	SessionID    string          `json:"session_id"`
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
}

// readCommand matches the Bash reads the replay measured: br show <bead> and
// cat/head/sed of a file. Anything else is not a read and stays silent.
var readCommand = regexp.MustCompile(`(?:^|[;&|]\s*|\s)(?:br\s+(?:--db\s+\S+\s+)?show\s+(\S+)|(?:cat|head|tail|sed\s+-n\s+\S+)\s+(\S+))`)

// Target reports whether a tool call read something, and what.
func Target(toolName string, toolInput json.RawMessage) (ref string, ok bool) {
	switch toolName {
	case "Read":
		var in struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal(toolInput, &in) != nil || in.FilePath == "" {
			return "", false
		}
		return in.FilePath, true
	case "Bash":
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(toolInput, &in) != nil {
			return "", false
		}
		m := readCommand.FindStringSubmatch(in.Command)
		if m == nil {
			return "", false
		}
		if m[1] != "" {
			return m[1], true
		}
		return m[2], true
	}
	return "", false
}

// IsRead is the cheap pre-check the hook runs on every tool call before it
// touches the gazetteer file.
func IsRead(raw []byte) bool {
	var p payload
	if json.Unmarshal(raw, &p) != nil {
		return false
	}
	_, ok := Target(p.ToolName, p.ToolInput)
	return ok
}

// responseText extracts the text a tool returned. Claude Code's Read response
// nests it under file.content, which the signpost extractor does not look at.
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
	for _, key := range []string{"file", "stdout", "output", "content", "text"} {
		if v := responseText(m[key]); v != "" {
			return v
		}
	}
	return ""
}

// Process runs the stage on one PostToolUse payload and returns the legend
// line ("" for silence) and the decision event. It never returns an error:
// every failure is a reason to stay silent, recorded in Event.Skipped.
func Process(raw []byte, cfg Config, g *Gazetteer) (string, Event) {
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	started := now()
	e := Event{Timestamp: started.UTC(), Budget: cfg.Budget}
	finish := func(skip string) (string, Event) {
		e.Skipped = skip
		e.LatencyUS = now().Sub(started).Microseconds()
		return "", e
	}
	var p payload
	if json.Unmarshal(raw, &p) != nil {
		return finish("bad-payload")
	}
	e.SessionID, e.Tool = p.SessionID, p.ToolName
	ref, ok := Target(p.ToolName, p.ToolInput)
	if !ok {
		return finish("not-a-read")
	}
	e.Ref = ref
	if cfg.Budget <= len(header)+8 {
		return finish("no-budget")
	}
	if g == nil {
		return finish("no-gazetteer")
	}
	text := responseText(p.ToolResponse)
	e.TextBytes = len(text)
	if text == "" {
		return finish("empty-text")
	}

	max := cfg.MaxEntities
	if max <= 0 {
		max = DefaultMaxEntities
	}
	seen := loadSeen(cfg.SeenDir, p.SessionID)
	var parts, shown []string
	full := false
	matches := g.Match(text)
	for _, m := range matches {
		if len(m.IRIs) == 1 && ref != "" && strings.Contains(m.IRIs[0].IRI, ref) {
			continue // the item naming itself
		}
		e.RawHits++ // counts every hit, as the replay does, even past the caps
		if full || seen[m.Key] || len(shown) >= max {
			continue
		}
		var part string
		if len(m.IRIs) > 1 {
			part = fmt.Sprintf("%s (ambiguous %d)", m.Text, len(m.IRIs))
		} else {
			part = fmt.Sprintf("%s (%s)", m.Text, m.IRIs[0].Type)
		}
		sep := 0
		if len(parts) > 0 {
			sep = 2
		}
		if len(header)+joinedLen(parts)+sep+len(part) > cfg.Budget {
			full = true
			continue
		}
		parts = append(parts, part)
		shown = append(shown, m.Key)
	}
	if len(parts) == 0 {
		return finish("no-match")
	}
	saveSeen(cfg.SeenDir, p.SessionID, shown)
	out := header + strings.Join(parts, ", ")
	e.Shown, e.Bytes = shown, len(out)
	e.LatencyUS = now().Sub(started).Microseconds()
	return out, e
}

func joinedLen(parts []string) int {
	n := 0
	for i, p := range parts {
		if i > 0 {
			n += 2
		}
		n += len(p)
	}
	return n
}

// Load reads a gazetteer and compiles it. A file older than maxAge is refused:
// a stale legend names entities that may have been renamed or retired, and a
// confident wrong annotation is worse than none.
func Load(path string, maxAge time.Duration, now time.Time) (*Gazetteer, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if maxAge > 0 && now.Sub(f.Generated) > maxAge {
		return nil, fmt.Errorf("gazetteer %s is stale (generated %s)", path, f.Generated.Format(time.RFC3339))
	}
	return Compile(f.Entries), nil
}

// AppendEvent writes one event as a JSONL line. "" or "-" disables logging.
func AppendEvent(path string, e Event) error {
	if path == "" || path == "-" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func seenPath(dir, session string) string {
	sum := sha256.Sum256([]byte(session))
	return filepath.Join(dir, hex.EncodeToString(sum[:8]))
}

func loadSeen(dir, session string) map[string]bool {
	seen := map[string]bool{}
	if dir == "" || session == "" {
		return seen
	}
	b, err := os.ReadFile(seenPath(dir, session))
	if err != nil {
		return seen
	}
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" {
			seen[l] = true
		}
	}
	return seen
}

func saveSeen(dir, session string, keys []string) {
	if dir == "" || session == "" || len(keys) == 0 {
		return
	}
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	f, err := os.OpenFile(seenPath(dir, session), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	sort.Strings(keys)
	_, _ = f.WriteString(strings.Join(keys, "\n") + "\n")
}
