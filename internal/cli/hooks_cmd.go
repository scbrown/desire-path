package cli

// `dp hooks bundle|install|uninstall|status` — desire-path owns its hook
// definitions and installs them into Claude Code AND Codex (aegis-5s32or).
//
// `dp init --source <plugin>` predates this and installs one source's ingest
// hooks; it is unchanged. This command is the harness-neutral one, and it
// carries dp's FULL hook set (ingest, signpost, signpost-prefetch,
// pave-correct and the codex notify), which until now existed only as a copy
// inside caboodle.
//
// On a host running shantytown, install REGISTERS the bundle with `st ops hooks
// register`; st renders it into every role's Claude and Codex settings on every
// emit, so a re-emit or relaunch cannot drop it. Without st, install writes the
// harness config directly. Claude's settings.json and Codex's config.toml carry
// the same `hooks.<Event> = [{matcher, hooks: [{type, command}]}]` shape.

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

//go:embed hookbundle/desire-path.bundle.json
var hookBundleJSON string

// HookBundle returns dp's st.hook-bundle/1 with the build's version.
func HookBundle() map[string]any {
	v := Version
	if v == "" {
		v = "dev"
	}
	var b map[string]any
	if err := json.Unmarshal([]byte(strings.ReplaceAll(hookBundleJSON, "{{VERSION}}", v)), &b); err != nil {
		panic(fmt.Sprintf("embedded hook bundle is not valid JSON: %v", err))
	}
	return b
}

type bundleHook struct{ event, matcher, command string }

// bundleHooksFor lists the bundle's hooks for one harness. A hook without a
// harnesses list applies to every harness, as in st's schema.
func bundleHooksFor(b map[string]any, harness string) []bundleHook {
	var out []bundleHook
	hooks, _ := b["hooks"].([]any)
	for _, raw := range hooks {
		h, _ := raw.(map[string]any)
		if hs, ok := h["harnesses"].([]any); ok {
			match := false
			for _, x := range hs {
				if x == harness {
					match = true
				}
			}
			if !match {
				continue
			}
		}
		m, _ := h["matcher"].(string)
		e, _ := h["event"].(string)
		c, _ := h["command"].(string)
		out = append(out, bundleHook{e, m, c})
	}
	return out
}

func groupsFor(cfg map[string]any, event string) []any {
	hooks, _ := cfg["hooks"].(map[string]any)
	g, _ := hooks[event].([]any)
	return g
}

func groupHasCommand(group map[string]any, command string) bool {
	list, _ := group["hooks"].([]any)
	for _, raw := range list {
		if h, _ := raw.(map[string]any); h["command"] == command {
			return true
		}
	}
	return false
}

// mergeHooks adds every bundle hook for harness to cfg. Idempotent; returns
// how many were added. Foreign hooks and other keys are kept.
func mergeHooks(cfg map[string]any, b map[string]any, harness string) int {
	hooks, ok := cfg["hooks"].(map[string]any)
	if !ok {
		hooks = map[string]any{}
		cfg["hooks"] = hooks
	}
	added := 0
	for _, h := range bundleHooksFor(b, harness) {
		groups, _ := hooks[h.event].([]any)
		var group map[string]any
		for _, raw := range groups {
			g, _ := raw.(map[string]any)
			if m, _ := g["matcher"].(string); m == h.matcher {
				group = g
				break
			}
		}
		if group == nil {
			group = map[string]any{"hooks": []any{}}
			if h.matcher != "" {
				group["matcher"] = h.matcher
			}
			groups = append(groups, group)
			hooks[h.event] = groups
		}
		if !groupHasCommand(group, h.command) {
			list, _ := group["hooks"].([]any)
			group["hooks"] = append(list, map[string]any{"type": "command", "command": h.command})
			added++
		}
	}
	return added
}

// removeHooks removes dp's hooks (exact command), dropping emptied groups and
// events. Hooks dp did not declare are untouched.
func removeHooks(cfg map[string]any, b map[string]any, harness string) int {
	ours := map[string]bool{}
	for _, h := range bundleHooksFor(b, harness) {
		ours[h.command] = true
	}
	hooks, ok := cfg["hooks"].(map[string]any)
	if !ok {
		return 0
	}
	removed := 0
	for event, raw := range hooks {
		groups, _ := raw.([]any)
		var keep []any
		for _, rg := range groups {
			g, _ := rg.(map[string]any)
			list, _ := g["hooks"].([]any)
			var kept []any
			for _, rh := range list {
				if h, _ := rh.(map[string]any); ours[fmt.Sprint(h["command"])] {
					removed++
					continue
				}
				kept = append(kept, rh)
			}
			if len(kept) > 0 {
				g["hooks"] = kept
				keep = append(keep, g)
			}
		}
		if len(keep) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = keep
		}
	}
	return removed
}

// presentHooks counts how many of the bundle's hooks for harness are in cfg.
func presentHooks(cfg map[string]any, b map[string]any, harness string) (int, int) {
	want := bundleHooksFor(b, harness)
	found := 0
	for _, h := range want {
		for _, raw := range groupsFor(cfg, h.event) {
			g, _ := raw.(map[string]any)
			if m, _ := g["matcher"].(string); m == h.matcher && groupHasCommand(g, h.command) {
				found++
				break
			}
		}
	}
	return found, len(want)
}

func notifyArgv(b map[string]any) []any {
	n, _ := b["codex_notify"].([]any)
	return n
}

// notifyEqual compares two notify argvs element by element, so argument
// boundaries count: ["a b", "c"] is not ["a", "b c"] (ian, desire-path#18).
func notifyEqual(a any, b []any) bool {
	as, ok := a.([]any)
	if !ok || len(as) != len(b) || len(b) == 0 {
		return false
	}
	for i := range as {
		x, xok := as[i].(string)
		y, yok := b[i].(string)
		if !xok || !yok || x != y {
			return false
		}
	}
	return true
}

// notifyState is the codex notify slot's state for dp: "absent", "ours", or
// "foreign" (another tool owns the single slot).
func notifyState(cfg map[string]any, b map[string]any) string {
	cur, exists := cfg["notify"]
	switch {
	case !exists:
		return "absent"
	case notifyEqual(cur, notifyArgv(b)):
		return "ours"
	default:
		return "foreign"
	}
}

func hooksConfigPath(harness string, project bool) string {
	home, _ := os.UserHomeDir()
	switch {
	case harness == "claude" && project:
		return filepath.Join(".claude", "settings.json")
	case harness == "claude":
		return filepath.Join(home, ".claude", "settings.json")
	case project:
		return filepath.Join(".codex", "config.toml")
	default:
		if ch := os.Getenv("CODEX_HOME"); ch != "" {
			return filepath.Join(ch, "config.toml")
		}
		return filepath.Join(home, ".codex", "config.toml")
	}
}

func readHooksConfig(path, harness string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	cfg := map[string]any{}
	if harness == "claude" {
		err = json.Unmarshal(data, &cfg)
	} else {
		err = toml.Unmarshal(data, &cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// writeHooksConfig writes cfg, keeping the previous file as <name>.bak-dp
// (a TOML rewrite does not keep comments).
func writeHooksConfig(path, harness string, cfg map[string]any) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak-dp", old, 0o600); err != nil {
			return err
		}
	}
	var data []byte
	var err error
	if harness == "claude" {
		data, err = json.MarshalIndent(cfg, "", "  ")
		data = append(data, '\n')
	} else {
		data, err = toml.Marshal(cfg)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func stAvailable() bool {
	return exec.Command("st", "ops", "hooks", "list").Run() == nil
}

func stRegister(register bool) (string, error) {
	var cmd *exec.Cmd
	if register {
		f, err := os.CreateTemp("", "dp-bundle-*.json")
		if err != nil {
			return "", err
		}
		defer os.Remove(f.Name())
		data, _ := json.MarshalIndent(HookBundle(), "", "  ")
		if _, err := f.Write(data); err != nil {
			return "", err
		}
		f.Close()
		cmd = exec.Command("st", "ops", "hooks", "register", f.Name())
	} else {
		cmd = exec.Command("st", "ops", "hooks", "unregister", "desire-path")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("st refused: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

var (
	hooksHarness []string
	hooksProject bool
	hooksNoST    bool
)

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Install, remove or check dp's hooks in Claude Code and Codex",
}

func hooksTargets() []string {
	if len(hooksHarness) == 0 {
		return []string{"claude", "codex"}
	}
	return hooksHarness
}

func runHooks(action string) error {
	b := HookBundle()
	if !hooksNoST && stAvailable() {
		switch action {
		case "install", "uninstall":
			out, err := stRegister(action == "install")
			if err != nil {
				return err
			}
			fmt.Println(out)
			fmt.Printf("dp hooks %sed through shantytown: st renders them into every role's claude and codex settings. Check with `st ops hooks check`.\n", map[string]string{"install": "register", "uninstall": "unregister"}[action])
			return nil
		case "status":
			out, err := exec.Command("st", "ops", "hooks", "check", "--json").Output()
			if err != nil && len(out) == 0 {
				return fmt.Errorf("st ops hooks check: %w", err)
			}
			var report struct {
				Items []map[string]any `json:"items"`
			}
			if err := json.Unmarshal(out, &report); err != nil {
				return err
			}
			ok := true
			for _, h := range hooksTargets() {
				n, conf, live, firing, silent := 0, 0, 0, 0, 0
				for _, i := range report.Items {
					if i["bundle"] != "desire-path" || i["harness"] != h {
						continue
					}
					n++
					if i["configured"] == "ok" {
						conf++
					}
					if i["live"] == "ok" {
						live++
					}
					if i["firing"] == "ok" {
						firing++
					}
					if i["firing"] == "silent" {
						silent++
					}
				}
				fmt.Printf("%s: %d hook item(s) via st; configured ok %d, live ok %d, firing ok %d, silent %d\n", h, n, conf, live, firing, silent)
				ok = ok && n > 0 && conf == n
			}
			if !ok {
				os.Exit(1)
			}
			return nil
		}
	}
	ok := true
	for _, h := range hooksTargets() {
		path := hooksConfigPath(h, hooksProject)
		cfg, err := readHooksConfig(path, h)
		if err != nil {
			return err
		}
		switch action {
		case "install":
			n := mergeHooks(cfg, b, h)
			if h == "codex" && len(notifyArgv(b)) > 0 {
				switch notifyState(cfg, b) {
				case "absent":
					cfg["notify"] = notifyArgv(b)
					n++
				case "foreign":
					fmt.Printf("codex: notify is owned by another tool; left unchanged: %q\n", cfg["notify"])
				}
			}
			if n > 0 {
				if err := writeHooksConfig(path, h, cfg); err != nil {
					return err
				}
			}
			fmt.Printf("%s: added %d hook(s) to %s\n", h, n, path)
		case "uninstall":
			n := removeHooks(cfg, b, h)
			if h == "codex" && notifyState(cfg, b) == "ours" {
				delete(cfg, "notify")
				n++
			}
			if n > 0 {
				if err := writeHooksConfig(path, h, cfg); err != nil {
					return err
				}
			}
			fmt.Printf("%s: removed %d hook(s) from %s\n", h, n, path)
		default:
			have, want := presentHooks(cfg, b, h)
			note := ""
			if h == "codex" && len(notifyArgv(b)) > 0 {
				// notify is part of dp's codex install: a foreign-owned slot is
				// NOT installed, and must not read green (ian, desire-path#18).
				want++
				switch notifyState(cfg, b) {
				case "ours":
					have++
				case "foreign":
					note = " (notify is owned by another tool)"
				}
			}
			fmt.Printf("%s: %d/%d dp hook(s) in %s%s\n", h, have, want, path, note)
			ok = ok && have == want
		}
	}
	if !ok {
		os.Exit(1)
	}
	return nil
}

func init() {
	bundleCmd := &cobra.Command{
		Use:   "bundle",
		Short: "Print dp's hook bundle (st.hook-bundle/1), the one source of truth",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := json.MarshalIndent(HookBundle(), "", " ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		},
	}
	hooksCmd.AddCommand(bundleCmd)
	for _, action := range []string{"install", "uninstall", "status"} {
		action := action
		sub := &cobra.Command{
			Use:   action,
			Short: map[string]string{"install": "Install dp's hooks (st-registered when shantytown is present)", "uninstall": "Remove dp's hooks (only dp's)", "status": "Report per harness whether dp's hooks are installed"}[action],
			RunE:  func(cmd *cobra.Command, args []string) error { return runHooks(action) },
		}
		sub.Flags().StringSliceVar(&hooksHarness, "harness", nil, "harness to write directly when st is absent: claude, codex (default both)")
		sub.Flags().BoolVar(&hooksProject, "project", false, "write ./.claude or ./.codex instead of the user's config")
		sub.Flags().BoolVar(&hooksNoST, "no-st", false, "write the harness config directly even when st is present")
		hooksCmd.AddCommand(sub)
	}
	rootCmd.AddCommand(hooksCmd)
}
