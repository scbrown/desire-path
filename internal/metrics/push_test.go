package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExpositionSortsLabelsSoOutputIsDeterministic(t *testing.T) {
	got := Exposition([]Sample{{Name: "x", Labels: map[string]string{"b": "2", "a": "1"}, Value: 5}})
	if got != "x{a=\"1\",b=\"2\"} 5\n" {
		t.Fatalf("got %q", got)
	}
}

func TestExpositionOmitsBracesWhenThereAreNoLabels(t *testing.T) {
	if got := Exposition([]Sample{{Name: "x", Value: 1}}); got != "x 1\n" {
		t.Fatalf("got %q", got)
	}
}

// A newline in a label would let one sample forge another, because the format is
// line-oriented. This is the escaping test that matters.
func TestANewlineInALabelCannotForgeASecondSample(t *testing.T) {
	got := Exposition([]Sample{{Name: "x", Labels: map[string]string{"a": "one\ntwo"}, Value: 1}})
	if n := len(strings.Split(strings.TrimSpace(got), "\n")); n != 1 {
		t.Fatalf("expected 1 line, got %d: %q", n, got)
	}
}

func TestAQuoteInALabelIsEscaped(t *testing.T) {
	got := Exposition([]Sample{{Name: "x", Labels: map[string]string{"a": `he said "hi"`}, Value: 1}})
	if !strings.Contains(got, `a="he said \"hi\""`) {
		t.Fatalf("got %q", got)
	}
}

// Every refusal must be NAMED and none may be fatal: the caller is an ingest that
// has to finish whatever the gateway does.
func TestAnUnsetGatewayIsReportedNotFatal(t *testing.T) {
	ok, why := Push("j", "x 1\n", "", nil)
	if ok || !strings.Contains(why, EnvVar) {
		t.Fatalf("ok=%v why=%q", ok, why)
	}
}

func TestAGatewayWithNoHostIsRefused(t *testing.T) {
	if ok, why := Push("j", "x 1\n", "not-a-url", nil); ok || !strings.Contains(why, "usable URL") {
		t.Fatalf("ok=%v why=%q", ok, why)
	}
}

func TestAnUnreachableGatewayIsReportedNotFatal(t *testing.T) {
	if ok, why := Push("j", "x 1\n", "http://127.0.0.1:1/", nil); ok || !strings.Contains(why, "unreachable") {
		t.Fatalf("ok=%v why=%q", ok, why)
	}
}

// A gateway that ANSWERS and refuses is a different problem from one that cannot
// be reached — auth or a malformed body, versus a network. The status must be in
// the message, or the reader is sent to neither.
func TestARefusalCarriesTheHTTPStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	ok, why := Push("j", "x 1\n", srv.URL, nil)
	if ok || !strings.Contains(why, "401") {
		t.Fatalf("ok=%v why=%q", ok, why)
	}
}

func TestTheJobLandsOnTheGatewayRoute(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
	}))
	defer srv.Close()
	if ok, why := Push("desire_path", "x 1\n", srv.URL, nil); !ok {
		t.Fatalf("push failed: %s", why)
	}
	if path != "/metrics/job/desire_path" {
		t.Fatalf("route was %q", path)
	}
}

func TestCredentialsInTheURLBecomeBasicAuth(t *testing.T) {
	var user, pass string
	var okAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, okAuth = r.BasicAuth()
	}))
	defer srv.Close()
	url := strings.Replace(srv.URL, "http://", "http://u:p@", 1)
	if ok, why := Push("j", "x 1\n", url, nil); !ok {
		t.Fatalf("push failed: %s", why)
	}
	if !okAuth || user != "u" || pass != "p" {
		t.Fatalf("auth=%v user=%q pass=%q", okAuth, user, pass)
	}
}

// THE REGRESSION THIS FILE EXISTS FOR (aegis-lu5502 review).
//
// `source` must reach the gateway as a GROUPING KEY in the URL, never as a body
// label on a shared group. A POST replaces every sample sharing a metric name
// within a group, so two sources writing one metric name into one group destroy
// each other — measured on the live gateway: claude-code then codex left only
// codex. Asserting the URL is what pins it; asserting the body would pass while
// the series silently clobbered each other in production.
func TestSourceIsAGroupingKeyInTheURLNotABodyLabel(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}))
	defer srv.Close()

	ok, why := Push("desire_path_producer", "x 1\n", srv.URL, map[string]string{"source": "claude-code"})
	if !ok {
		t.Fatalf("push failed: %s", why)
	}
	if want := "/metrics/job/desire_path_producer/source/claude-code"; gotPath != want {
		t.Fatalf("grouping key not in the URL:\n got %q\nwant %q", gotPath, want)
	}
}

// The store totals are GLOBAL (store.Stats is an unfiltered COUNT), so they must
// go to the bare job with no grouping key — one group, one truth. A per-source
// group here would publish four copies of the same global number.
func TestGlobalTotalsGetNoGroupingKey(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}))
	defer srv.Close()

	if ok, why := Push("desire_path", "x 1\n", srv.URL, nil); !ok {
		t.Fatalf("push failed: %s", why)
	}
	if want := "/metrics/job/desire_path"; gotPath != want {
		t.Fatalf("got %q want %q", gotPath, want)
	}
}

// Grouping keys are sorted, so the URL a reader sees is the URL every run builds.
func TestGroupingKeysAreSortedForADeterministicURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}))
	defer srv.Close()

	Push("j", "x 1\n", srv.URL, map[string]string{"zeta": "z", "alpha": "a"})
	if want := "/metrics/job/j/alpha/a/zeta/z"; gotPath != want {
		t.Fatalf("got %q want %q", gotPath, want)
	}
}
