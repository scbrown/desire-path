package signpost

import "testing"

func TestDiscoverIntentFamilies(t *testing.T) {
	tests := []struct {
		name, command, wantFamily, wantQuery, wantSymbol string
		wantOK                                           bool
	}{
		{"rg pattern", "rg 'retry backoff' .", FamilyLiteral, "retry backoff", "", true},
		{"grep skips flags", "grep -rn --color=never handler ./src", FamilyLiteral, "handler", "handler", true},
		{"regex is not a symbol", `grep -rn 'Allow()\|circuit' .`, FamilyLiteral, `Allow()\|circuit`, "", true},
		{"short pattern is not a symbol", "grep -rn ok .", FamilyLiteral, "ok", "", true},
		{"find -name", "find . -name '*_handler.go'", FamilyFileFind, "_handler.go", "", true},
		{"find -iname strips dirs", "find . -iname 'internal/store*.go'", FamilyFileFind, "store.go", "", true},
		{"find with no name predicate", "find . -type f -mtime -1", "", "", "", false},
		{"find -name too short after cleaning", "find . -name '*.go'", "", "", "", false},
		{"git log pickaxe attached", "git log -SErrCircuitOpen", FamilyHistory, "ErrCircuitOpen", "", true},
		{"git log pickaxe detached", "git log -S 'circuit breaker' --oneline", FamilyHistory, "circuit breaker", "", true},
		{"git log grep equals", "git log --grep=timeout", FamilyHistory, "timeout", "", true},
		{"git log grep detached", "git log --grep 'fix the retry'", FamilyHistory, "fix the retry", "", true},
		{"git grep is a literal search, not history", "git grep -n foo", FamilyLiteral, "foo", "foo", true},
		{"git log with no query flag", "git log --oneline -20", "", "", "", false},
		{"git diff -S is not log", "git diff -Sfoo", "", "", "", false},
		{"unrelated command", "ls -la /tmp", "", "", "", false},
		{"pipeline finds the searcher", "cat x | grep -n Process | head -5", FamilyLiteral, "Process", "Process", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DiscoverIntent(tt.command)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v want %v (intent=%+v)", ok, tt.wantOK, got)
			}
			if !ok {
				return
			}
			if got.Family != tt.wantFamily || got.Query != tt.wantQuery || got.SymbolCandidate != tt.wantSymbol {
				t.Fatalf("got %+v want family=%q query=%q symbol=%q", got, tt.wantFamily, tt.wantQuery, tt.wantSymbol)
			}
		})
	}
}

func TestStackCommandPerFamily(t *testing.T) {
	tests := []struct {
		intent Intent
		repo   string
		want   string
	}{
		{Intent{Family: FamilyLiteral, Query: "retry"}, "beads", "bobbin search --repo 'beads' 'retry'"},
		{Intent{Family: FamilyFileFind, Query: "store.go"}, "beads", "bobbin search --repo 'beads' 'store.go'"},
		{Intent{Family: FamilyHistory, Query: "timeout"}, "beads", "bobbin search --repo 'beads' --type commit 'timeout'"},
		{Intent{Family: FamilySymbol, Query: "Process"}, "beads", "yupana callers 'Process'"},
		{Intent{Family: FamilyLiteral, Query: "retry"}, "", "bobbin search 'retry'"},
	}
	for _, tt := range tests {
		if got := tt.intent.StackCommand(tt.repo); got != tt.want {
			t.Errorf("family %s: got %q want %q", tt.intent.Family, got, tt.want)
		}
	}
}

// A pointer that mislabels the route it leads to spends its one line of
// credibility on being wrong: yupana does not do semantic search.
func TestInviteNamesTheRoute(t *testing.T) {
	tests := []struct{ family, want string }{
		{FamilyLiteral, "Try semantic search"},
		{FamilyFileFind, "Try semantic search"},
		{FamilyHistory, "Try the commit index"},
		{FamilySymbol, "Try the structural index"},
	}
	for _, tt := range tests {
		if got := (Intent{Family: tt.family}).Invite(); got != tt.want {
			t.Errorf("%s: got %q want %q", tt.family, got, tt.want)
		}
	}
}
