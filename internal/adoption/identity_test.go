package adoption

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOriginIdentityDropsCredentialsAndTransport(t *testing.T) {
	for _, raw := range []string{"https://user:PRIVATE_MARKER@example.org/team/demo.git?token=PRIVATE_MARKER", "git@example.org:team/demo.git", "ssh://git@example.org/team/demo.git"} {
		got, ok := originIdentity(raw)
		if !ok || got != "example.org/team/demo" {
			t.Fatalf("origin normalization failed")
		}
	}
}

func TestIndependentClonesAndDifferentRepos(t *testing.T) {
	root := t.TempDir()
	makeRepo := func(name, repo string) string {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(p, ".git"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, ".git/config"), []byte("[remote \"origin\"]\nurl = https://example.org/team/"+repo+".git\n"), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a, b, c := makeRepo("clone-a", "same"), makeRepo("clone-b", "same"), makeRepo("clone-c", "different")
	r := Resolver{Roots: map[string]string{"indexed": a}}
	x, ok := r.References([]Reference{{Repo: "indexed", Path: "src/file.go"}}, "")
	if !ok {
		t.Fatal("addressable offered path unknown")
	}
	y, ok := r.File(filepath.Join(b, "src/file.go"), "")
	if !ok || x[0] != y {
		t.Fatal("independent clones do not match")
	}
	z, ok := r.File(filepath.Join(c, "src/file.go"), "")
	if !ok || x[0] == z {
		t.Fatal("same filename in unrelated repo falsely matches")
	}
}

func TestFileOperationsNotMentions(t *testing.T) {
	for _, tc := range []struct {
		tool, input string
		want        int
		known       bool
	}{
		{"Read", `{"file_path":"src/a.go"}`, 1, true},
		{"Edit", `{"file_path":"src/a.go","old_string":"secret"}`, 1, true},
		{"mcp__bobbin__read_chunk", `{"file":"repos/demo/src/a.go"}`, 1, true},
		{"Bash", `{"command":"cat 'path with spaces.go'"}`, 1, true},
		{"Bash", `{"command":"sed -n '1,20p' src/a.go"}`, 1, true},
		{"Bash", `{"command":"echo src/a.go; false"}`, 0, false},
		{"Bash", `{"command":"cat src/a.go && false"}`, 0, false},
		{"Bash", `{"command":"cat src/a.go # ignore other.go"}`, 1, true},
		{"Bash", `{"command":"cat $TARGET"}`, 0, false},
		{"Bash", `{"command":"rg pattern src/a.go"}`, 0, false},
		{"Bash", `{"command":"echo $(cat src/a.go)"}`, 0, false},
		{"mcp__custom__ReadFile", `{"path":"src/a.go"}`, 0, false},
		{"Bash", `{"command":"echo ignored; cat src/a.go"}`, 0, false},
		{"Bash", `{"command":"sed -e '1,20p' src/a.go"}`, 1, true},
		{"Bash", `{"command":"head --lines 20 src/a.go"}`, 1, true},
		{"Read", `{}`, 0, false},
	} {
		paths, known := Files(Hook{Tool: tc.tool, Input: json.RawMessage(tc.input)})
		if len(paths) != tc.want || known != tc.known {
			t.Errorf("%s: got paths=%v known=%v", tc.input, paths, known)
		}
	}
}

func FuzzInvocationRoundTrip(f *testing.F) {
	f.Add("session", "call", "Read")
	f.Add("unicode-λ", "second", "Other")
	f.Fuzz(func(t *testing.T, session, id, tool string) {
		if session == "" || id == "" || tool == "" || len(session) > 512 || len(id) > 512 || len(tool) > 128 {
			return
		}
		h := Hook{Session: session, ID: id, Tool: tool, Input: json.RawMessage(`{}`)}
		raw, err := json.Marshal(h)
		if err != nil {
			t.Fatal(err)
		}
		// Invalid UTF-8 expands to replacement runes during JSON encoding.
		// Apply the parser's byte bounds to the serialized identity, too.
		var normalized Hook
		if err = json.Unmarshal(raw, &normalized); err != nil {
			t.Fatal(err)
		}
		got, err := Parse(raw)
		if len(normalized.Session) > 512 || len(normalized.ID) > 512 || len(normalized.Tool) > 128 {
			if err == nil {
				t.Fatal("expanded identity escaped parser bounds")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if got.Session != normalized.Session || got.ID != normalized.ID || got.Tool != normalized.Tool {
			t.Fatal("invocation identity changed")
		}
	})
}

func TestExplicitExecWorkingDirectoryAndRawPatch(t *testing.T) {
	cwd, ok := WorkingDirectory(Hook{Tool: "exec_command", CWD: "/workspace/first", Input: json.RawMessage(`{"cmd":"cat file.go","workdir":"/workspace/second"}`)})
	if !ok || cwd != "/workspace/second" {
		t.Fatal("exec directory ignored")
	}
	input, _ := json.Marshal("*** Begin Patch\n*** Update File: file.go\n@@\n-old\n+new\n*** End Patch")
	paths, known := Files(Hook{Tool: "functions.apply_patch", Input: input})
	if !known || len(paths) != 1 || paths[0] != "file.go" {
		t.Fatal("raw patch path not observed")
	}
}
