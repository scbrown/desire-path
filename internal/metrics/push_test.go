package metrics

import (
	"net/http"
	"net/http/httptest"
	"os"
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

// The inline form is a SUPPORTED configuration, so it is tested with the file
// form explicitly out of the way. Without that this test reads whatever the host
// has configured: on any machine where DESIRE_PATH_METRICS_PASSWORD_FILE points
// at a real secret it fails, and its failure message prints that secret into the
// test log. An environment-dependent test that leaks a credential when it fails
// is worse than no test, so the environment is pinned and the assertion never
// echoes the password it read.
func TestCredentialsInTheURLBecomeBasicAuth(t *testing.T) {
	t.Setenv(EnvPasswordFile, "")
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
		t.Fatalf("auth=%v user=%q password matched the URL: %v", okAuth, user, pass == "p")
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

// ── credential file (aegis-4rgadh) ────────────────────────────────────────────
//
// The point of the file is that the secret never enters a process environment,
// so these assert on what actually reaches the wire: the Authorization header.

func basicAuthOf(t *testing.T, gateway string) (string, string, bool) {
	t.Helper()
	var user, pass string
	var ok bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok = r.BasicAuth()
	}))
	defer srv.Close()
	// Rewrite the caller's userinfo onto the test server's host.
	u := strings.SplitN(strings.TrimPrefix(gateway, "http://"), "@", 2)
	target := srv.URL
	if len(u) == 2 {
		target = "http://" + u[0] + "@" + strings.TrimPrefix(srv.URL, "http://")
	}
	if ok, why := Push("j", "x 1\n", target, nil); !ok {
		t.Fatalf("push failed: %s", why)
	}
	return user, pass, ok
}

func TestPasswordComesFromTheFileWhenSet(t *testing.T) {
	f := t.TempDir() + "/pw"
	if err := os.WriteFile(f, []byte("s3cret\n"), 0o600); err != nil { // note the newline
		t.Fatal(err)
	}
	t.Setenv(EnvPasswordFile, f)

	user, pass, ok := basicAuthOf(t, "http://someuser@placeholder")
	if !ok {
		t.Fatal("no basic auth was sent")
	}
	if user != "someuser" {
		t.Errorf("user = %q, want someuser", user)
	}
	// The trailing newline must be gone: a password with a stray \n fails auth
	// with a 401 that reads exactly like a wrong password.
	if pass != "s3cret" {
		t.Errorf("pass = %q, want %q (newline not trimmed?)", pass, "s3cret")
	}
}

func TestTheFileBeatsAnInlinePassword(t *testing.T) {
	f := t.TempDir() + "/pw"
	if err := os.WriteFile(f, []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvPasswordFile, f)

	if _, pass, _ := basicAuthOf(t, "http://someuser:from-url@placeholder"); pass != "from-file" {
		t.Errorf("pass = %q, want from-file", pass)
	}
}

func TestAnUnsetPasswordFileLeavesTheInlinePasswordAlone(t *testing.T) {
	t.Setenv(EnvPasswordFile, "")
	if _, pass, _ := basicAuthOf(t, "http://someuser:from-url@placeholder"); pass != "from-url" {
		t.Errorf("pass = %q, want from-url", pass)
	}
}

// A configured-but-broken credential must REFUSE and say so, never fall back to
// pushing unauthenticated (which 401s) or to a stale inline value.
func TestAnUnreadablePasswordFileIsRefusedLoudly(t *testing.T) {
	t.Setenv(EnvPasswordFile, t.TempDir()+"/does-not-exist")
	ok, why := Push("j", "x 1\n", "http://someuser:from-url@127.0.0.1:1/", nil)
	if ok || !strings.Contains(why, EnvPasswordFile) {
		t.Fatalf("ok=%v why=%q", ok, why)
	}
}

func TestAnEmptyPasswordFileIsRefusedLoudly(t *testing.T) {
	f := t.TempDir() + "/pw"
	if err := os.WriteFile(f, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvPasswordFile, f)
	ok, why := Push("j", "x 1\n", "http://someuser:from-url@127.0.0.1:1/", nil)
	if ok || !strings.Contains(why, "empty") {
		t.Fatalf("ok=%v why=%q", ok, why)
	}
}
