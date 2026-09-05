// Package metrics reports what desire-path has actually DONE, to a Pushgateway.
//
// WHY PUSHED AND NOT SCRAPED. `dp` is a CLI. `dp ingest` runs once per tool call
// and exits; there is no process alive for Prometheus to scrape, and the service
// name the server would answer on returns 404 because nothing deploys it today.
// `dp serve` exists in internal/server and promhttp would be the right answer the
// moment it IS deployed — but adding an endpoint to a binary nobody runs would
// produce a metric nobody can read, which is the shape this is meant to end.
//
// WHY THE STORE'S TOTALS AND NOT A PER-RUN COUNT. A Pushgateway REPLACES a job's
// samples on every push. So a short-lived CLI pushing "I did 1 ingest" would sit
// at a flat 1 forever, no matter how much work happened — a metric that looks
// alive and cannot move is worse than none, and it is exactly the up=1-while-dead
// shape aegis-wou8k exists to close. What moves is the STORE: total desires,
// unique paths, the rolling windows. Those are cumulative facts about work done,
// they are the numbers `dp stats` already reports to a human, and the difference
// between two readings is real work.
//
// TWO GROUPS, following creel (creel/docs/metrics.md). A gap in a pushed series
// has two causes that are identical at the gateway — the producer died, or it ran
// and had nothing to say:
//
//	desire_path_producer  every ingest, unconditionally: timestamp, duration,
//	                      exit status, grouped by source. Stale => nothing is
//	                      ingesting (per agent).
//	desire_path           only on success: the store totals, UNLABELLED. Stale
//	                      with a fresh producer => ingests are running and
//	                      storing nothing.
//
// SOURCE IS A GROUPING KEY, NOT A BODY LABEL, AND THAT IS LOAD-BEARING. A POST to
// the gateway replaces every sample sharing a metric NAME within the group, so two
// sources writing the same metric name into one group destroy each other: measured
// on the live gateway, pushing {source="claude-code"} then {source="codex"} left
// ONLY codex. There are four source plugins (claude-code, codex, cursor, kiro), so
// that is the ordinary case. Putting source in the URL grouping key gives each its
// own group and both survive — also measured. It additionally keeps the series
// identity stable per source, which matters because a churning label defeats an
// alert's `for:` clock (aegis-m8omn).
//
// FAILS OPEN, LOUDLY. An unset variable, a missing credential or an unreachable
// gateway must never fail an ingest — the ingest is the point and the metric is
// the observation of it. Every path returns a reason and none is fatal, but none
// is silent either: a metrics pipeline that quietly does nothing is the thing
// being guarded against.
//
// Configure with DESIRE_PATH_METRICS_PUSHGATEWAY=http://[user:pass@]host[:port].
// The credential arrives at run time and is never written to the repo.
package metrics

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// EnvVar names the gateway. Unset means metrics are off, which is a supported
// configuration and not an error.
const EnvVar = "DESIRE_PATH_METRICS_PUSHGATEWAY"

const (
	jobSamples  = "desire_path"
	jobProducer = "desire_path_producer"
	timeout     = 10 * time.Second
)

// Sample is one metric line.
type Sample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

func escape(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ")
	return r.Replace(v)
}

// Exposition renders Prometheus text format. Labels are sorted so the output is
// deterministic, and every label value is escaped — an unescaped newline in a
// label would let one sample forge another, since the format is line-oriented.
func Exposition(samples []Sample) string {
	var b strings.Builder
	for _, s := range samples {
		if len(s.Labels) == 0 {
			fmt.Fprintf(&b, "%s %g\n", s.Name, s.Value)
			continue
		}
		keys := make([]string, 0, len(s.Labels))
		for k := range s.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			// %s and NOT %q: escape() has already done it, and %q would escape
			// the escapes — measured, a quoted label came out as \\\" (caught by
			// TestAQuoteInALabelIsEscaped, which is why it is written against the
			// exact bytes rather than "contains a backslash").
			parts = append(parts, fmt.Sprintf(`%s="%s"`, k, escape(s.Labels[k])))
		}
		fmt.Fprintf(&b, "%s{%s} %g\n", s.Name, strings.Join(parts, ","), s.Value)
	}
	return b.String()
}

// Push sends one job's exposition to the gateway. It never returns a fatal
// error; the string is the reason, for the caller to print.
func Push(job, body, gateway string, grouping map[string]string) (bool, string) {
	gateway = strings.TrimSpace(gateway)
	if gateway == "" {
		return false, EnvVar + " is unset — nothing pushed (set it to enable metrics)"
	}
	u, err := url.Parse(gateway)
	if err != nil || u.Host == "" {
		return false, fmt.Sprintf("%s is not a usable URL (%v) — nothing pushed", EnvVar, err)
	}
	// GROUPING KEY, not a body label — see the package comment. Sorted so the URL
	// is deterministic; PathEscape because a source name reaches this from config.
	path := strings.TrimRight(u.Path, "/") + "/metrics/job/" + url.PathEscape(job)
	gkeys := make([]string, 0, len(grouping))
	for k := range grouping {
		gkeys = append(gkeys, k)
	}
	sort.Strings(gkeys)
	for _, k := range gkeys {
		path += "/" + url.PathEscape(k) + "/" + url.PathEscape(grouping[k])
	}
	target := &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   path,
	}
	if target.Scheme == "" {
		target.Scheme = "http"
	}
	req, err := http.NewRequest(http.MethodPost, target.String(), strings.NewReader(body))
	if err != nil {
		return false, fmt.Sprintf("cannot build request for job=%s: %v", job, err)
	}
	req.Header.Set("Content-Type", "text/plain; version=0.0.4")
	if u.User != nil {
		pw, _ := u.User.Password()
		req.SetBasicAuth(u.User.Username(), pw)
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return false, fmt.Sprintf("gateway unreachable for job=%s: %v", job, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		// The gateway ANSWERED and refused: auth, or a malformed body. Naming the
		// status separates "fix your credential" from "fix your exposition"; a bare
		// "push failed" sends the reader to neither.
		return false, fmt.Sprintf("gateway refused job=%s: HTTP %d", job, resp.StatusCode)
	}
	return true, fmt.Sprintf("pushed job=%s (%d)", job, resp.StatusCode)
}

// Report pushes one ingest's producer liveness and, on success, the store
// totals. Never fatal; every outcome is printed to w.
func Report(w *os.File, source string, totals []Sample, started time.Time, status int) {
	gateway := os.Getenv(EnvVar)
	now := time.Now()

	// Unconditional: its whole job is to separate "did not run" from "ran and
	// stored nothing". `source` is the GROUPING KEY, so each agent keeps its own
	// liveness series instead of the four of them clobbering one another.
	_, why := Push(jobProducer, Exposition([]Sample{
		{Name: "desire_path_producer_run_timestamp_seconds", Value: float64(now.UnixMilli()) / 1000},
		{Name: "desire_path_producer_duration_seconds", Value: now.Sub(started).Seconds()},
		{Name: "desire_path_producer_exit_status", Value: float64(status)},
	}), gateway, map[string]string{"source": source})
	fmt.Fprintf(w, "metrics: %s\n", why)

	if status != 0 {
		fmt.Fprintf(w, "metrics: ingest failed — store totals NOT pushed "+
			"(the producer group carries the failure)\n")
		return
	}
	// The totals are GLOBAL (store.Stats is an unfiltered COUNT over `desires`),
	// so they carry NO source label and NO grouping key: one group, one truth.
	_, why = Push(jobSamples, Exposition(totals), gateway, nil)
	fmt.Fprintf(w, "metrics: %s\n", why)
}
