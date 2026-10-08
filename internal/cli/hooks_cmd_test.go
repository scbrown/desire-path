package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookBundleIsDPsOwn(t *testing.T) {
	b := HookBundle()
	if b["schema"] != "st.hook-bundle/1" || b["name"] != "desire-path" || b["owner"] != "desire-path" {
		t.Fatalf("bundle identity wrong: %v %v %v", b["schema"], b["name"], b["owner"])
	}
	if len(bundleHooksFor(b, "claude")) != 6 {
		t.Fatalf("claude hooks = %d, want the full set of 6", len(bundleHooksFor(b, "claude")))
	}
	if len(bundleHooksFor(b, "codex")) == 0 || len(notifyArgv(b)) == 0 {
		t.Fatal("codex hooks and notify must be declared")
	}
}

func TestMergeIsIdempotentAndKeepsForeignHooks(t *testing.T) {
	b := HookBundle()
	cfg := map[string]any{"model": "x", "hooks": map[string]any{"PostToolUse": []any{
		map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "other-tool hook"}}},
	}}}
	want := len(bundleHooksFor(b, "claude"))
	if n := mergeHooks(cfg, b, "claude"); n != want {
		t.Fatalf("first merge added %d, want %d", n, want)
	}
	if n := mergeHooks(cfg, b, "claude"); n != 0 {
		t.Fatalf("second merge added %d, want 0", n)
	}
	if have, all := presentHooks(cfg, b, "claude"); have != all {
		t.Fatalf("present %d/%d", have, all)
	}
	g := cfg["hooks"].(map[string]any)["PostToolUse"].([]any)
	found := false
	for _, raw := range g {
		if groupHasCommand(raw.(map[string]any), "other-tool hook") {
			found = true
		}
	}
	if !found || cfg["model"] != "x" {
		t.Fatal("foreign hook or key lost")
	}
}

func TestRemoveTakesOnlyDPsHooks(t *testing.T) {
	b := HookBundle()
	cfg := map[string]any{"hooks": map[string]any{"PostToolUse": []any{
		map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "other-tool hook"}}},
	}}}
	mergeHooks(cfg, b, "claude")
	if n := removeHooks(cfg, b, "claude"); n != len(bundleHooksFor(b, "claude")) {
		t.Fatalf("removed %d", n)
	}
	if have, _ := presentHooks(cfg, b, "claude"); have != 0 {
		t.Fatalf("%d dp hooks left", have)
	}
	if _, ok := cfg["hooks"].(map[string]any)["PreToolUse"]; ok {
		t.Fatal("emptied event kept")
	}
	if len(cfg["hooks"].(map[string]any)["PostToolUse"].([]any)) != 1 {
		t.Fatal("foreign group lost")
	}
}

func TestCodexConfigRoundTripsThroughTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("model = \"gpt\"\n[mcp_servers.x]\ncommand = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := HookBundle()
	cfg, err := readHooksConfig(path, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if mergeHooks(cfg, b, "codex") == 0 {
		t.Fatal("nothing merged")
	}
	if err := writeHooksConfig(path, "codex", cfg); err != nil {
		t.Fatal(err)
	}
	back, err := readHooksConfig(path, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if have, all := presentHooks(back, b, "codex"); have != all {
		t.Fatalf("present %d/%d after round trip", have, all)
	}
	if back["model"] != "gpt" {
		t.Fatal("model key lost")
	}
	if _, err := os.Stat(path + ".bak-dp"); err != nil {
		t.Fatal("no backup kept")
	}
}

func TestNotifyComparesArgvBoundaries(t *testing.T) {
	if notifyEqual([]any{"a b", "c"}, []any{"a", "b c"}) {
		t.Fatal("argv boundaries ignored")
	}
	if !notifyEqual([]any{"a", "b"}, []any{"a", "b"}) {
		t.Fatal("equal argv not equal")
	}
}

func TestNotifyStateNeverTakesOrDropsAForeignSlot(t *testing.T) {
	b := HookBundle()
	ours := notifyArgv(b)
	// A foreign argv that Sprint-formats like ours would have fooled the old check.
	var joined []string
	for _, x := range ours {
		joined = append(joined, x.(string))
	}
	foreign := []any{strings.Join(joined, " ")}
	cases := map[string]map[string]any{
		"absent":  {},
		"ours":    {"notify": ours},
		"foreign": {"notify": foreign},
	}
	for want, cfg := range cases {
		if got := notifyState(cfg, b); got != want {
			t.Fatalf("notifyState = %q, want %q", got, want)
		}
	}
	if notifyState(map[string]any{"notify": []any{"other-tool", "notify"}}, b) != "foreign" {
		t.Fatal("other tool's notify not foreign")
	}
}
