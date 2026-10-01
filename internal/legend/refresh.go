package legend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DefaultClasses are the governed classes whose labels name things an agent
// would want to know are in the graph. Generated and bulk kinds (Observation,
// Chunk, Commit, CodeSymbol, Section) are excluded, as is Credential.
//
// Formula is excluded too (fix 3 from the replay's noise sample): its labels
// are generic phrases — `code-review` matched "code-review request" twice in 50.
var DefaultClasses = strings.Fields(`Host BareMetalHost ProxmoxNode LXCContainer DockerContainer RemoteHost
Service SystemdService SystemdTimer DatabaseService SearchService ExternalService MCPServer GitRepo
CrewMember CrewRole Rig Skill CLI AgentTool Script AlertRule CronJob PushgatewayJob Metric
MonitoringProbe ReverseProxyRoute NetworkSegment StoragePool NFSExport ConfigFile Directive
FailureMode FailurePattern Incident Feature Decision Component Capability`)

// Stoplist holds labels that are correct entities but mislead as annotations.
//
// Hosts (fix 2): ubiquitous in URLs, so annotating them tells the reader
// nothing.
//
// Phrases (fix 3b): names that are also everyday phrases. Dropping Formula
// (fix 3) did not remove `code-review`, because the same IRI is also a Skill,
// and it matched "code-review request" twice in the replay's 50-hit sample.
// Evidence-driven only: add a phrase when a hand-checked sample shows it
// misleading, never from a guess. A structural rule (every two-part
// dictionary compound) was measured and rejected: it would drop 84 labels,
// including bobbin.service, desire-path and message-router.
var Stoplist = map[string]bool{
	"github.com": true, "gitlab.com": true, "raw.githubusercontent.com": true,
	"api.github.com": true, "docs.rs": true, "crates.io": true, "pkg.go.dev": true,
	"pypi.org": true, "npmjs.com": true, "localhost": true,
	"code-review": true,
	// A generic workflow filename bound to ONE repository's file: wrong
	// referent in 2 of 2 hand-checked hits (both meant another repo's).
	"deploy.yml": true,
}

var stopwords = map[string]bool{"the": true, "and": true, "for": true, "with": true,
	"from": true, "this": true, "that": true, "into": true, "over": true}

var identShape = regexp.MustCompile(`[-_./0-9]|[a-z][A-Z]`)

// Admissible is the replay's precision guard plus the stoplist. dict is
// a lower-cased word list; nil disables the dictionary test.
func Admissible(label string, dict map[string]bool) bool {
	l := strings.TrimSpace(label)
	if len(l) < 4 || Stoplist[strings.ToLower(l)] {
		return false
	}
	toks := strings.Fields(l)
	if len(toks) >= 2 {
		for _, t := range toks {
			t = strings.ToLower(t)
			if !stopwords[t] && !dict[t] {
				return true
			}
		}
		return false
	}
	if dict[strings.ToLower(l)] {
		return false
	}
	return identShape.MatchString(l)
}

// LoadDict reads a newline-separated word list, lower-cased. A missing file
// returns an empty dictionary, which only makes the guard more permissive for
// single-token labels that are not identifier-shaped — and those are refused
// by the shape test regardless.
func LoadDict(path string) map[string]bool {
	d := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return d
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if w := strings.ToLower(strings.TrimSpace(s.Text())); w != "" {
			d[w] = true
		}
	}
	return d
}

// ClassReport is one class's export outcome.
type ClassReport struct {
	Class     string `json:"class"`
	Rows      int    `json:"rows"`
	Admitted  int    `json:"admitted"`
	Truncated bool   `json:"truncated,omitempty"`
	Err       string `json:"error,omitempty"`
}

// Refresh exports the label rows for each class from Quipu's /query endpoint,
// keeps the admissible ones, and returns the gazetteer file. Classes are
// queried one at a time: a grouped all-types count timed out at 10 s.
// A class that fails is reported and skipped; if EVERY class fails, Refresh
// returns an error so a cron never replaces a good file with an empty one.
func Refresh(ctx context.Context, queryURL, namespace string, classes []string, dict map[string]bool) (File, []ClassReport, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	f := File{Generated: time.Now().UTC(), Source: queryURL}
	var reports []ClassReport
	ok := 0
	for _, c := range classes {
		r := ClassReport{Class: c}
		q := fmt.Sprintf("SELECT ?s ?l WHERE { ?s a <%s%s> . ?s <http://www.w3.org/2000/01/rdf-schema#label> ?l }", namespace, c)
		rows, trunc, err := query(ctx, client, queryURL, q)
		if err != nil {
			r.Err = err.Error()
			reports = append(reports, r)
			continue
		}
		ok++
		r.Rows, r.Truncated = len(rows), trunc
		for _, row := range rows {
			if Admissible(row.L, dict) {
				f.Entries = append(f.Entries, Entry{IRI: row.S, Label: strings.TrimSpace(row.L), Type: c})
				r.Admitted++
			}
		}
		reports = append(reports, r)
	}
	if ok == 0 {
		return f, reports, fmt.Errorf("every class query failed; not writing a gazetteer")
	}
	return f, reports, nil
}

type row struct {
	S string `json:"s"`
	L string `json:"l"`
}

func query(ctx context.Context, client *http.Client, url, sparql string) ([]row, bool, error) {
	body, _ := json.Marshal(map[string]string{"query": sparql})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Quipu-Client", "agent-adhoc")
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("quipu status %d", resp.StatusCode)
	}
	var out struct {
		Rows      []row `json:"rows"`
		Truncated bool  `json:"truncated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, false, err
	}
	if out.Rows == nil {
		return nil, false, fmt.Errorf("quipu response carried no rows field")
	}
	return out.Rows, out.Truncated, nil
}

// Write stores the file atomically (temp + rename) so a hook never reads a
// half-written gazetteer.
func Write(path string, f File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".legend-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
