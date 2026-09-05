package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/source"
)

func searchBackend(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("probe went to %q, want /search", r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

// The whole defect this flag exists for is a hook installed against a backend
// that does not answer: the hook is then SILENT, which is its contract, so
// nothing ever reports the misconfiguration. The install must fail closed.
func TestSignpostURLIsVerifiedBeforeItIsWritten(t *testing.T) {
	t.Run("a working backend is accepted and the route derived", func(t *testing.T) {
		server := searchBackend(t, `{"count":1,"results":[]}`, http.StatusOK)
		env, err := signpostEnv(server.URL, "-", false)
		if err != nil {
			t.Fatal(err)
		}
		if got := env["DP_SIGNPOST_BOBBIN_URL"]; got != server.URL+"/search" {
			t.Fatalf("url = %q, want %q", got, server.URL+"/search")
		}
	})

	t.Run("an explicit /search route is not doubled", func(t *testing.T) {
		server := searchBackend(t, `{"count":0}`, http.StatusOK)
		env, err := signpostEnv(server.URL+"/search", "-", false)
		if err != nil {
			t.Fatal(err)
		}
		if got := env["DP_SIGNPOST_BOBBIN_URL"]; got != server.URL+"/search" {
			t.Fatalf("url = %q, want %q", got, server.URL+"/search")
		}
	})

	t.Run("an unreachable backend is refused", func(t *testing.T) {
		// A port nothing is bound to: exactly the shipped default's situation.
		if _, err := signpostEnv("http://127.0.0.1:1/", "-", false); err == nil {
			t.Fatal("an unreachable backend was accepted")
		}
	})

	t.Run("a non-search answer is refused", func(t *testing.T) {
		server := searchBackend(t, `<html>hello</html>`, http.StatusOK)
		if _, err := signpostEnv(server.URL, "-", false); err == nil {
			t.Fatal("a backend that is not a search API was accepted")
		}
	})

	t.Run("an error status is refused", func(t *testing.T) {
		server := searchBackend(t, `{"count":1}`, http.StatusBadGateway)
		if _, err := signpostEnv(server.URL, "-", false); err == nil {
			t.Fatal("a 502 backend was accepted")
		}
	})

	t.Run("the check can be skipped deliberately", func(t *testing.T) {
		env, err := signpostEnv("http://127.0.0.1:1/", "-", true)
		if err != nil {
			t.Fatal(err)
		}
		if env["DP_SIGNPOST_BOBBIN_URL"] != "http://127.0.0.1:1/search" {
			t.Fatalf("env = %v", env)
		}
	})

	t.Run("no url means no env", func(t *testing.T) {
		env, err := signpostEnv("", "-", false)
		if err != nil || env != nil {
			t.Fatalf("env=%v err=%v", env, err)
		}
	})
}

// The settings file already carries environment somebody else put there. An
// installer that clobbers it trades one outage for another.
func TestInstallMergesEnvWithoutClobbering(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settings, []byte(`{"env":{"EXISTING_VAR":"keep me"},"model":"some-model"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	installer := source.Get("claude-code").(source.Installer)
	if err := installer.Install(source.InstallOpts{
		SettingsPath: settings,
		Env:          map[string]string{"DP_SIGNPOST_BOBBIN_URL": "http://127.0.0.1:3030/search"},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Env   map[string]string `json:"env"`
		Model string            `json:"model"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Env["EXISTING_VAR"] != "keep me" {
		t.Errorf("clobbered an existing env var: %v", got.Env)
	}
	if got.Env["DP_SIGNPOST_BOBBIN_URL"] != "http://127.0.0.1:3030/search" {
		t.Errorf("did not install the signpost url: %v", got.Env)
	}
	if got.Model != "some-model" {
		t.Errorf("clobbered unrelated settings: model = %q", got.Model)
	}
	if !strings.Contains(string(raw), "dp signpost") {
		t.Errorf("hooks were not installed alongside the env")
	}
}

// The fleet case IS a correct hook set with no environment. Reporting
// "already configured" and returning would leave it broken forever, which is
// how the hook ran on every pane for weeks reaching nothing.
func TestAlreadyInstalledHooksStillGetTheirEnvironment(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "settings.json")
	installer := source.Get("claude-code").(source.Installer)
	if err := installer.Install(source.InstallOpts{SettingsPath: settings}); err != nil {
		t.Fatal(err)
	}
	installed, err := installer.IsInstalled(dir)
	if err != nil || !installed {
		t.Fatalf("expected the hooks to be installed: %v %v", installed, err)
	}

	if err := runInit("claude-code", false, settings,
		map[string]string{"DP_SIGNPOST_BOBBIN_URL": "http://127.0.0.1:3030/search"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "DP_SIGNPOST_BOBBIN_URL") {
		t.Fatalf("env was skipped because the hooks were already there:\n%s", raw)
	}
}

// Production had no event log at all, which is why "wired on every pane"
// survived for weeks as a claim about delivery when it was only ever a claim
// about installation. The log ships with the URL by default.
func TestSignpostLogShipsWithTheURL(t *testing.T) {
	server := searchBackend(t, `{"count":1}`, http.StatusOK)

	env, err := signpostEnv(server.URL, "", false)
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "log", "dp-signpost-events.jsonl")
	if env["DP_SIGNPOST_LOG"] != want {
		t.Errorf("log = %q, want %q", env["DP_SIGNPOST_LOG"], want)
	}

	env, err = signpostEnv(server.URL, "/tmp/custom.jsonl", false)
	if err != nil || env["DP_SIGNPOST_LOG"] != "/tmp/custom.jsonl" {
		t.Errorf("custom log path not honoured: %v %v", env, err)
	}

	env, err = signpostEnv(server.URL, "-", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := env["DP_SIGNPOST_LOG"]; ok {
		t.Errorf(`"-" must disable the log, got %v`, env)
	}
}
