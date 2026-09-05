package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates HOME for every test in this package.
//
// THIS IS NOT TIDINESS. `runInit(source, _, "")` resolves an empty settings path
// through os.UserHomeDir(), so TestInitCmdJSON installed dp's hooks into the
// REAL ~/.claude/settings.json of whoever ran `go test ./...`. It had done so
// silently for as long as the test existed: the file already contained every
// hook the installer wanted, so the merge was a no-op and nothing was ever
// observed. The moment the install set gained an entry (dp pave-correct), a
// routine test run rewrote a live agent's configuration, and the change was
// investigated as an unexplained edit by an unknown writer before it was traced
// back to the test suite.
//
// A test that can reach the real HOME is a test that can reach anything under
// it. Isolating at TestMain covers the whole package rather than the one case
// that was caught, because the next one will be written by someone who does not
// know this happened.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "dp-cli-test-home-")
	if err != nil {
		panic("cannot create an isolated HOME for the test package: " + err.Error())
	}
	defer os.RemoveAll(home)
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME"} {
		if err := os.Setenv(key, filepath.Join(home, key)); err != nil {
			panic("cannot isolate " + key + ": " + err.Error())
		}
	}
	// HOME itself must be the directory, not a subdirectory named HOME.
	if err := os.Setenv("HOME", home); err != nil {
		panic("cannot isolate HOME: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
